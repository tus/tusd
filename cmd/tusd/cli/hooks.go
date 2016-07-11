package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"

	"github.com/tus/tusd"
)

type HookType string

const (
	HookPostFinish    HookType = "post-finish"
	HookPostTerminate HookType = "post-terminate"
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
			}
		}
	}()
}

func invokeHook(typ HookType, info tusd.FileInfo) {
	go func() {
		_, err := invokeHookSync(typ, info, false)
		if err != nil {
			stderr.Printf("Error running %s hook for %s: %s", string(typ), info.ID, err)
		}
	}()
}

func invokeHookSync(typ HookType, info tusd.FileInfo, captureOutput bool) ([]byte, error) {
	switch typ {
	case HookPostFinish:
		stdout.Printf("Upload %s (%d bytes) finished\n", info.ID, info.Size)
	case HookPostTerminate:
		stdout.Printf("Upload %s terminated\n", info.ID)
	}

	if !Flags.HooksInstalled {
		return nil, nil
	}

	name := string(typ)
	stdout.Printf("Invoking %s hook…\n", name)

	cmd := exec.Command(Flags.HooksDir + "/" + name)
	env := os.Environ()
	env = append(env, "TUS_ID="+info.ID)
	env = append(env, "TUS_SIZE="+strconv.FormatInt(info.Size, 10))

	jsonInfo, err := json.Marshal(info)
	if err != nil {
		return nil, err
	}

	reader := bytes.NewReader(jsonInfo)
	cmd.Stdin = reader

	cmd.Env = env
	cmd.Dir = Flags.HooksDir
	cmd.Stderr = os.Stderr

	if !captureOutput {
		cmd.Stdout = os.Stdout
		return nil, cmd.Run()
	} else {
		return cmd.Output()
	}
}
