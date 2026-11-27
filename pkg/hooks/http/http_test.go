package http

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tus/tusd/v2/pkg/hooks"
)

func newTestHook(t *testing.T, delay time.Duration, timeout time.Duration) *HttpHook {
	t.Helper()

	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(delay):
		case <-release:
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"RejectUpload":true}`))
	}))
	t.Cleanup(server.Close)
	// Cleanups run in reverse order, so hanging handlers are released before
	// the server is closed.
	t.Cleanup(func() { close(release) })

	hook := &HttpHook{
		Endpoint:  server.URL,
		Timeout:   timeout,
		SizeLimit: 1024,
	}
	require.NoError(t, hook.Setup())
	return hook
}

// TestTimeout ensures that a hanging hook endpoint causes InvokeHook to return an
// error once the timeout is reached.
func TestTimeout(t *testing.T) {
	hook := newTestHook(t, time.Minute, 100*time.Millisecond)

	start := time.Now()
	_, err := hook.InvokeHook(hooks.HookRequest{Type: hooks.HookPreCreate})
	require.Error(t, err)
	assert.Less(t, time.Since(start), 5*time.Second)
}

// TestTimeoutDisabled ensures that a timeout of zero disables the timeout instead
// of failing every request immediately.
func TestTimeoutDisabled(t *testing.T) {
	hook := newTestHook(t, 100*time.Millisecond, 0)

	res, err := hook.InvokeHook(hooks.HookRequest{Type: hooks.HookPreCreate})
	require.NoError(t, err)
	assert.True(t, res.RejectUpload)
}
