// Package file provides a file-based hook implementation. A directory is specified, whose
// files will be executed for specific hook events. When the pre-create events is emitted,
// the file called pre-create will be executed, similar to Git hooks. If such a file does not
// exist, the event will be ignored.
// Information about the current upload and HTTP request is provided on stdin and in the
// environment variables. By writing to stdout, the response from tusd can be influenced.
package file

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"

	"github.com/Nealsoni00/tusd/v2/pkg/hooks"
)

type FileHook struct {
	Directory string
}

func (FileHook) Setup() error {
	return nil
}

func (h FileHook) InvokeHook(req hooks.HookRequest) (res hooks.HookResponse, err error) {
	hookPath := h.Directory + string(os.PathSeparator) + string(req.Type)
	cmd := exec.Command(hookPath)
	env := os.Environ()
	env = append(env, "TUS_ID="+req.Event.Upload.ID)
	env = append(env, "TUS_SIZE="+strconv.FormatInt(req.Event.Upload.Size, 10))
	env = append(env, "TUS_OFFSET="+strconv.FormatInt(req.Event.Upload.Offset, 10))

	jsonReq, err := json.Marshal(req)
	if err != nil {
		return res, err
	}

	reader := bytes.NewReader(jsonReq)
	cmd.Stdin = reader

	cmd.Env = env
	cmd.Dir = h.Directory
	cmd.Stderr = os.Stderr

	output, err := cmd.Output()

	// Ignore the error if the hook's file could not be found. This usually
	// means that the user is only using a subset of the available hooks.
	if os.IsNotExist(err) {
		return res, nil
	}

	// Report error if the exit code was non-zero
	if err, ok := err.(*exec.ExitError); ok {
		return res, fmt.Errorf("unexpected return code %d from hook endpoint: %s", err.ProcessState.ExitCode(), string(output))
	}

	if err != nil {
		return res, err
	}

	// Do not parse the output as JSON, if we received no output to reduce possible
	// errors.
	if len(output) > 0 {
		if err = json.Unmarshal(output, &res); err != nil {
			return res, fmt.Errorf("failed to parse hook response: %w, response was: %s", err, string(output))
		}
	}

	return res, nil
}
