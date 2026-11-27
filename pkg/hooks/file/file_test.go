package file

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tus/tusd/v2/pkg/hooks"
)

func writeHook(t *testing.T, dir string, typ hooks.HookType, script string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, string(typ)), []byte(script), 0o755))
}

func TestInvokeHook(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell scripts are not supported on Windows")
	}

	dir := t.TempDir()
	writeHook(t, dir, hooks.HookPreCreate, "#!/bin/sh\necho '{\"RejectUpload\":true}'\n")

	hook := FileHook{Directory: dir, Timeout: 10 * time.Second}
	res, err := hook.InvokeHook(hooks.HookRequest{Type: hooks.HookPreCreate})
	require.NoError(t, err)
	assert.True(t, res.RejectUpload)
}

// TestTimeout ensures that a hanging hook script is killed and returns an error
// once the timeout is reached. The script starts a child process which keeps
// stdout open, so this also checks that we do not wait for such children.
func TestTimeout(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell scripts are not supported on Windows")
	}

	dir := t.TempDir()
	writeHook(t, dir, hooks.HookPreCreate, "#!/bin/sh\nexec 2>/dev/null\nsleep 30\n")

	hook := FileHook{Directory: dir, Timeout: 200 * time.Millisecond}

	start := time.Now()
	_, err := hook.InvokeHook(hooks.HookRequest{Type: hooks.HookPreCreate})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "timed out")
	assert.Less(t, time.Since(start), 10*time.Second)
}

// TestBackgroundChild ensures that a hook, which exits successfully but leaves a
// child process with stdout open behind, does not cause an error.
func TestBackgroundChild(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell scripts are not supported on Windows")
	}

	dir := t.TempDir()
	writeHook(t, dir, hooks.HookPostFinish, "#!/bin/sh\nexec 2>/dev/null\necho '{}'\nsleep 30 &\n")

	hook := FileHook{Directory: dir, Timeout: 10 * time.Second}

	start := time.Now()
	_, err := hook.InvokeHook(hooks.HookRequest{Type: hooks.HookPostFinish})
	require.NoError(t, err)
	assert.Less(t, time.Since(start), 5*time.Second)
}
