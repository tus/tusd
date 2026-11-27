package plugin

import (
	"net"
	"net/rpc"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tus/tusd/v2/pkg/hooks"
)

// testHandler is a hook handler whose behavior is controlled by a function.
type testHandler struct {
	invoke func(hooks.HookRequest) (hooks.HookResponse, error)
}

func (testHandler) Setup() error { return nil }

func (h testHandler) InvokeHook(req hooks.HookRequest) (hooks.HookResponse, error) {
	return h.invoke(req)
}

// newTestRPCClient serves the given handler over net/rpc, the same way go-plugin
// does, and returns a HookHandlerRPC connected to it.
func newTestRPCClient(t *testing.T, impl hooks.HookHandler, timeout time.Duration) *HookHandlerRPC {
	t.Helper()

	server := rpc.NewServer()
	require.NoError(t, server.RegisterName("Plugin", &HookHandlerRPCServer{Impl: impl}))

	serverConn, clientConn := net.Pipe()
	go server.ServeConn(serverConn)

	client := rpc.NewClient(clientConn)
	t.Cleanup(func() { client.Close() })

	return &HookHandlerRPC{client: client, timeout: timeout}
}

func TestInvokeHook(t *testing.T) {
	impl := testHandler{invoke: func(req hooks.HookRequest) (hooks.HookResponse, error) {
		return hooks.HookResponse{RejectUpload: req.Type == hooks.HookPreCreate}, nil
	}}
	client := newTestRPCClient(t, impl, 10*time.Second)

	res, err := client.InvokeHook(hooks.HookRequest{Type: hooks.HookPreCreate})
	require.NoError(t, err)
	assert.True(t, res.RejectUpload)
}

// TestTimeout ensures that a hanging plugin causes InvokeHook to return an error
// once the timeout is reached, instead of blocking forever.
func TestTimeout(t *testing.T) {
	release := make(chan struct{})
	defer close(release)

	impl := testHandler{invoke: func(hooks.HookRequest) (hooks.HookResponse, error) {
		<-release
		return hooks.HookResponse{RejectUpload: true}, nil
	}}
	client := newTestRPCClient(t, impl, 100*time.Millisecond)

	start := time.Now()
	res, err := client.InvokeHook(hooks.HookRequest{Type: hooks.HookPreCreate})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "timed out")
	assert.False(t, res.RejectUpload)
	assert.Less(t, time.Since(start), 5*time.Second)
}
