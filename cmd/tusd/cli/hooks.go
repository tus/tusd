package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/sethgrid/pester"
	"github.com/tus/tusd"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"time"
)

type HookType string

const (
	HookPostFinish    HookType = "post-finish"
	HookPostTerminate HookType = "post-terminate"
	HookPostReceive   HookType = "post-receive"
	HookPreCreate     HookType = "pre-create"
)

type hookDataStore struct {
	tusd.DataStore
}

func (store hookDataStore) NewUpload(info tusd.FileInfo) (id string, err error) {
	if output, err := invokeHookSync(HookPreCreate, info, true); err != nil {
		return "", fmt.Errorf("pre-create hook failed:  %s\n%s", err, string(output))
	}
	return store.DataStore.NewUpload(info)
}

func SetupPreHooks(composer *tusd.StoreComposer) {
	composer.UseCore(hookDataStore{
		DataStore: composer.Core,
	})
}

func SetupPostHooks(handler *tusd.Handler) {
	go func() {
		for {
			select {
			case info := <-handler.CompleteUploads:
				invokeHook(HookPostFinish, info)
			case info := <-handler.TerminatedUploads:
				invokeHook(HookPostTerminate, info)
			case info := <-handler.UploadProgress:
				invokeHook(HookPostReceive, info)
			}
		}
	}()
}

func invokeHook(typ HookType, info tusd.FileInfo) {
	go func() {
		// Error handling is token care of by the function.
		_, _ = invokeHookSync(typ, info, false)
	}()
}

func invokeHookSync(typ HookType, info tusd.FileInfo, captureOutput bool) ([]byte, error) {
	switch typ {
	case HookPostFinish:
		logEv("UploadFinished", "id", info.ID, "size", strconv.FormatInt(info.Size, 10))
	case HookPostTerminate:
		logEv("UploadTerminated", "id", info.ID)
	}

	if !Flags.FileHooksInstalled && !Flags.HttpHooksInstalled {
		return nil, nil
	}
	name := string(typ)
	logEv("HookInvocationStart", "type", name, "id", info.ID)

	output := []byte{}
	err := error(nil)

	if Flags.FileHooksInstalled {
		output, err = invokeFileHook(name, typ, info, captureOutput)
	}

	if Flags.HttpHooksInstalled {
		output, err = invokeHttpHook(name, typ, info, captureOutput)
	}

	return output, err
}

func invokeHttpHook(name string, typ HookType, info tusd.FileInfo, captureOutput bool) ([]byte, error) {
	url := Flags.HttpHooksEndpoint
	jsonInfo, err := json.Marshal(info)

	req, err := http.NewRequest("POST", url, bytes.NewReader(jsonInfo))
	req.Header.Set("Upload-State", name)
	req.Header.Set("Content-Type", "application/json")
	if err != nil {
		return nil, err
	}

	//retry 3 times at most.
	client := pester.New()
	client.KeepLog = true
	client.Backoff = func(_ int) time.Duration {
		return time.Duration(Flags.HttpHooksBackoff) * time.Second
	}
	client.MaxRetries = Flags.HttpHooksRetry

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	response := []byte(resp.Status)
	if resp.StatusCode >= 400 {
		err := errors.New("Invalid Response Code")
		return response, err
	}

	//if capture output is true, this function will return both the response and rthe err. Else it will return only the nil and err
	if captureOutput {
		return response, err
	}

	return nil, err
}

func invokeFileHook(name string, typ HookType, info tusd.FileInfo, captureOutput bool) ([]byte, error) {
	cmd := exec.Command(Flags.FileHooksDir + "/" + name)
	env := os.Environ()
	env = append(env, "TUS_ID="+info.ID)
	env = append(env, "TUS_SIZE="+strconv.FormatInt(info.Size, 10))
	env = append(env, "TUS_OFFSET="+strconv.FormatInt(info.Offset, 10))

	jsonInfo, err := json.Marshal(info)
	if err != nil {
		return nil, err
	}

	reader := bytes.NewReader(jsonInfo)
	cmd.Stdin = reader

	cmd.Env = env
	cmd.Dir = Flags.FileHooksDir
	cmd.Stderr = os.Stderr

	// If `captureOutput` is true, this function will return the output (both,
	// stderr and stdout), else it will use this process' stdout
	var output []byte
	if !captureOutput {
		cmd.Stdout = os.Stdout
		err = cmd.Run()
	} else {
		output, err = cmd.Output()
	}

	if err != nil {
		logEv("HookInvocationError", "type", string(typ), "id", info.ID, "error", err.Error())
	} else {
		logEv("HookInvocationFinish", "type", string(typ), "id", info.ID)
	}

	// Ignore the error, only, if the hook's file could not be found. This usually
	// means that the user is only using a subset of the available hooks.
	if os.IsNotExist(err) {
		err = nil
	}

	return output, err
}
