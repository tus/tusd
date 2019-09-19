package hooks

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"strconv"

	"github.com/tus/tusd/pkg/handler"
)

type FileHook struct {
	Directory string
}

func (_ FileHook) Setup() error {
	return nil
}

func (h FileHook) InvokeHook(typ HookType, info handler.HookEvent, captureOutput bool) ([]byte, int, error) {
	hookPath := h.Directory + string(os.PathSeparator) + string(typ)
	cmd := exec.Command(hookPath)
	env := os.Environ()
	env = append(env, "TUS_ID="+info.Upload.ID)
	env = append(env, "TUS_SIZE="+strconv.FormatInt(info.Upload.Size, 10))
	env = append(env, "TUS_OFFSET="+strconv.FormatInt(info.Upload.Offset, 10))

	jsonInfo, err := json.Marshal(info)
	if err != nil {
		return nil, 0, err
	}

	reader := bytes.NewReader(jsonInfo)
	cmd.Stdin = reader

	cmd.Env = env
	cmd.Dir = h.Directory
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

	// Ignore the error, only, if the hook's file could not be found. This usually
	// means that the user is only using a subset of the available hooks.
	if os.IsNotExist(err) {
		err = nil
	}

	returnCode := cmd.ProcessState.ExitCode()

	return output, returnCode, err
}
