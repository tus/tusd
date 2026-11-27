// Package file provides a file-based hook implementation. A directory is specified, whose
// files will be executed for specific hook events. When the pre-create events is emitted,
// the file called pre-create will be executed, similar to Git hooks. If such a file does not
// exist, the event will be ignored.
// Information about the current upload and HTTP request is provided on stdin and in the
// environment variables. By writing to stdout, the response from tusd can be influenced.
package file

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"time"

	"github.com/tus/tusd/v2/pkg/hooks"
)

type FileHook struct {
	Directory string
	// Timeout is the maximum duration a hook process may run. If it is exceeded,
	// the process is killed and the invocation returns an error. A value of zero
	// disables the timeout.
	Timeout time.Duration
}

// waitDelay is how long we wait for the hook's stdout to be closed after the hook
// process has exited or has been killed. Without it, a child process spawned by the
// hook that inherited stdout could keep us blocked beyond the timeout.
const waitDelay = 1 * time.Second

func (FileHook) Setup() error {
	return nil
}

func (h FileHook) InvokeHook(req hooks.HookRequest) (res hooks.HookResponse, err error) {
	hookPath := h.Directory + string(os.PathSeparator) + string(req.Type)

	ctx := context.Background()
	if h.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, h.Timeout)
		defer cancel()
	}

	// exec.CommandContext kills the process once the context is done.
	cmd := exec.CommandContext(ctx, hookPath)
	if h.Timeout > 0 {
		cmd.WaitDelay = waitDelay
	}
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
	// On Windows, a missing file is reported as exec.ErrNotFound instead.
	// However, exec also reports ENOENT for hooks that exist but cannot be run
	// (e.g. a missing shebang interpreter or a dangling symlink). Those must be
	// reported as errors, so we check whether the hook file itself is missing.
	// Lstat is used, so that a dangling symlink counts as an existing hook.
	if os.IsNotExist(err) || errors.Is(err, exec.ErrNotFound) {
		if _, statErr := os.Lstat(hookPath); os.IsNotExist(statErr) {
			return res, nil
		}
		return res, fmt.Errorf("failed to execute hook %s: %w", hookPath, err)
	}

	// Report error if the exit code was non-zero
	if err, ok := err.(*exec.ExitError); ok {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return res, fmt.Errorf("hook %s timed out after %s and was killed", req.Type, h.Timeout)
		}
		return res, fmt.Errorf("unexpected return code %d from hook endpoint: %s", err.ProcessState.ExitCode(), string(output))
	}

	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return res, fmt.Errorf("hook %s timed out after %s: %w", req.Type, h.Timeout, err)
		}

		// ErrWaitDelay means that the hook process exited successfully, but a child
		// process kept stdout open. The hook itself is done, so we use its output.
		if !errors.Is(err, exec.ErrWaitDelay) {
			return res, err
		}
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
