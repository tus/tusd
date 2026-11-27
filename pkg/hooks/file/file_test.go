package file

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/tus/tusd/v2/pkg/handler"
	"github.com/tus/tusd/v2/pkg/hooks"
)

func newRequest(hookType hooks.HookType) hooks.HookRequest {
	return hooks.HookRequest{
		Type: hookType,
		Event: handler.HookEvent{
			Upload: handler.FileInfo{
				ID:   "id",
				Size: 100,
			},
		},
	}
}

func skipOnWindows(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("file hook tests rely on Unix scripts")
	}
}

func TestFileHookMissingScript(t *testing.T) {
	a := assert.New(t)
	hook := FileHook{Directory: t.TempDir()}

	res, err := hook.InvokeHook(newRequest(hooks.HookPreCreate))
	a.NoError(err)
	a.Equal(hooks.HookResponse{}, res)
}

func TestFileHookWorkingScript(t *testing.T) {
	skipOnWindows(t)
	a := assert.New(t)
	dir := t.TempDir()
	script := "#!/bin/sh\necho '{\"RejectUpload\":true}'\n"
	a.NoError(os.WriteFile(filepath.Join(dir, string(hooks.HookPreCreate)), []byte(script), 0o755))

	hook := FileHook{Directory: dir}
	res, err := hook.InvokeHook(newRequest(hooks.HookPreCreate))
	a.NoError(err)
	a.True(res.RejectUpload)
}

func TestFileHookMissingInterpreter(t *testing.T) {
	skipOnWindows(t)
	a := assert.New(t)
	dir := t.TempDir()
	script := "#!/nonexistent/sh\nexit 0\n"
	a.NoError(os.WriteFile(filepath.Join(dir, string(hooks.HookPreCreate)), []byte(script), 0o755))

	hook := FileHook{Directory: dir}
	_, err := hook.InvokeHook(newRequest(hooks.HookPreCreate))
	a.Error(err)
}

func TestFileHookDanglingSymlink(t *testing.T) {
	skipOnWindows(t)
	a := assert.New(t)
	dir := t.TempDir()
	a.NoError(os.Symlink(filepath.Join(dir, "does-not-exist"), filepath.Join(dir, string(hooks.HookPreTerminate))))

	hook := FileHook{Directory: dir}
	_, err := hook.InvokeHook(newRequest(hooks.HookPreTerminate))
	a.Error(err)
}
