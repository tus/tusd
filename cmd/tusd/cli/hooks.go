package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"strconv"

	"github.com/tus/tusd"
)

type HookType string

const (
	HookPostFinish    HookType = "post-finish"
	HookPostTerminate HookType = "post-terminate"
)

func SetupHooks(handler *tusd.Handler) {
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
	switch typ {
	case HookPostFinish:
		stdout.Printf("Upload %s (%d bytes) finished\n", info.ID, info.Size)
	case HookPostTerminate:
		stdout.Printf("Upload %s terminated\n", info.ID)
	}

	if !Flags.HooksInstalled {
		return
	}

	name := string(typ)
	stdout.Printf("Invoking %s hook…\n", name)

	cmd := exec.Command(Flags.HooksDir + "/" + name)
	env := os.Environ()
	env = append(env, "TUS_ID="+info.ID)
	env = append(env, "TUS_SIZE="+strconv.FormatInt(info.Size, 10))

	jsonInfo, err := json.Marshal(info)
	if err != nil {
		stderr.Printf("Error encoding JSON for hook: %s", err)
	}

	reader := bytes.NewReader(jsonInfo)
	cmd.Stdin = reader

	cmd.Env = env
	cmd.Dir = Flags.HooksDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	go func() {
		err := cmd.Run()
		if err != nil {
			stderr.Printf("Error running %s hook for %s: %s", name, info.ID, err)
		}
	}()
}
