package file

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func writeHook(t *testing.T, dir string, typ hooks.HookType, script string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, string(typ)), []byte(script), 0o755))
}

// TestFileHookTimeout ensures that a hanging hook script is killed and returns an
// error once the timeout is reached. The script starts a child process which keeps
// stdout open, so this also checks that we do not wait for such children.
func TestFileHookTimeout(t *testing.T) {
	skipOnWindows(t)
	dir := t.TempDir()
	writeHook(t, dir, hooks.HookPreCreate, "#!/bin/sh\nexec 2>/dev/null\nsleep 30\n")

	hook := FileHook{Directory: dir, Timeout: 200 * time.Millisecond}

	start := time.Now()
	_, err := hook.InvokeHook(newRequest(hooks.HookPreCreate))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "timed out")
	assert.Less(t, time.Since(start), 10*time.Second)
}

// TestFileHookBackgroundChild ensures that a hook, which exits successfully but
// leaves a child process with stdout open behind, does not cause an error.
func TestFileHookBackgroundChild(t *testing.T) {
	skipOnWindows(t)
	dir := t.TempDir()
	writeHook(t, dir, hooks.HookPostFinish, "#!/bin/sh\nexec 2>/dev/null\necho '{}'\nsleep 30 &\n")

	hook := FileHook{Directory: dir, Timeout: 10 * time.Second}

	start := time.Now()
	_, err := hook.InvokeHook(newRequest(hooks.HookPostFinish))
	require.NoError(t, err)
	assert.Less(t, time.Since(start), 5*time.Second)
}
