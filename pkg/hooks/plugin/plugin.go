// Package plugin provides a hook system based on Hashicorp's plugin system. You can
// write a plugin in many languages. The plugin is then executed as a separate process
// and communicates with tusd over RPC. More details can be found at https://github.com/hashicorp/go-plugin.
// An example for a Go-based plugin implementation is at github.com/tus/tusd/examples/hooks/plugin.
package plugin

import (
	"fmt"
	"log"
	"net/rpc"
	"os"
	"os/exec"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/go-plugin"
	"github.com/tus/tusd/v2/pkg/hooks"
)

type PluginHook struct {
	Path string
	// JSONLogFormat specifies whether the logs from the plugin hook should be formatted as JSON.
	JSONLogFormat bool
	// Timeout is the maximum duration for a single hook invocation. If the plugin
	// does not respond in time, the invocation returns an error. A value of zero
	// disables the timeout.
	Timeout time.Duration

	handlerImpl hooks.HookHandler
}

// Setup initiates the connection to the plugin. Note: When the main process ends,
// you must call CleanupClients() to ensure that the subprocess is properly cleaned up.
func (h *PluginHook) Setup() error {
	// We're a host! Start by launching the plugin process.
	client := plugin.NewClient(&plugin.ClientConfig{
		HandshakeConfig: handshakeConfig,
		Plugins:         pluginMap,
		Cmd:             exec.Command(h.Path),
		SyncStdout:      os.Stdout,
		SyncStderr:      os.Stderr,
		// We use a managed client, so we can use plugin.CleanupClients() to shut it down.
		Managed: true,
		Logger: hclog.New(&hclog.LoggerOptions{
			Name:       "plugin",
			Level:      hclog.Debug,
			Output:     os.Stdout,
			JSONFormat: h.JSONLogFormat,
			TimeFormat: "2006/01/02 03:04:05.000000",
		}),
	})
	//defer client.Kill()

	// Connect via RPC
	rpcClient, err := client.Client()
	if err != nil {
		log.Fatal(err)
	}

	// Request the plugin
	raw, err := rpcClient.Dispense("hookHandler")
	if err != nil {
		log.Fatal(err)
	}

	// We should have a HookHandler now! This feels like a normal interface
	// implementation but is in fact over an RPC connection.
	h.handlerImpl = raw.(hooks.HookHandler)
	if rpcImpl, ok := h.handlerImpl.(*HookHandlerRPC); ok {
		rpcImpl.timeout = h.Timeout
	}

	return h.handlerImpl.Setup()
}

func (h *PluginHook) InvokeHook(req hooks.HookRequest) (hooks.HookResponse, error) {
	return h.handlerImpl.InvokeHook(req)
}

// CleanupPlugins closes the connections to all plugins and ensures that their processes are
// properly stopped. You must call this function when the main process exits.
func CleanupPlugins() {
	plugin.CleanupClients()
}

// handshakeConfigs are used to just do a basic handshake between
// a plugin and host. If the handshake fails, a user friendly error is shown.
// This prevents users from executing bad plugins or executing a plugin
// directory. It is a UX feature, not a security feature.
var handshakeConfig = plugin.HandshakeConfig{
	ProtocolVersion:  1,
	MagicCookieKey:   "TUSD_PLUGIN",
	MagicCookieValue: "yes",
}

// pluginMap is the map of plugins we can dispense.
var pluginMap = map[string]plugin.Plugin{
	"hookHandler": &HookHandlerPlugin{},
}

// Here is an implementation that talks over RPC
type HookHandlerRPC struct {
	client *rpc.Client
	// timeout limits the duration of InvokeHook calls. Zero disables the timeout.
	timeout time.Duration
}

func (g *HookHandlerRPC) Setup() error {
	var res any
	err := g.client.Call("Plugin.Setup", new(any), &res)
	return err
}

func (g *HookHandlerRPC) InvokeHook(req hooks.HookRequest) (res hooks.HookResponse, err error) {
	// Empty the context field because it is no usable in the plugin and
	// we would get runtime errors like:
	//  gob: type not registered for interface: context.cancelCtx
	req.Event.Context = nil

	if g.timeout <= 0 {
		err = g.client.Call("Plugin.InvokeHook", req, &res)
		return res, err
	}

	// net/rpc does not support contexts or deadlines, so we start the call
	// asynchronously and stop waiting for it once the timeout is reached.
	// The late response, if any, is discarded by net/rpc. We decode into a
	// separate value, so a late response cannot race with our return value.
	var callRes hooks.HookResponse
	call := g.client.Go("Plugin.InvokeHook", req, &callRes, make(chan *rpc.Call, 1))

	timer := time.NewTimer(g.timeout)
	defer timer.Stop()

	select {
	case <-call.Done:
		return callRes, call.Error
	case <-timer.C:
		return res, fmt.Errorf("plugin hook %s timed out after %s", req.Type, g.timeout)
	}
}

// Here is the RPC server that HookHandlerRPC talks to, conforming to
// the requirements of net/rpc
type HookHandlerRPCServer struct {
	// This is the real implementation
	Impl hooks.HookHandler
}

func (s *HookHandlerRPCServer) Setup(args any, resp *any) error {
	return s.Impl.Setup()
}

func (s *HookHandlerRPCServer) InvokeHook(args hooks.HookRequest, resp *hooks.HookResponse) (err error) {
	*resp, err = s.Impl.InvokeHook(args)
	return err
}

// This is the implementation of plugin.Plugin so we can serve/consume this
//
// This has two methods: Server must return an RPC server for this plugin
// type. We construct a HookHandlerRPCServer for this.
//
// Client must return an implementation of our interface that communicates
// over an RPC client. We return HookHandlerRPC for this.
//
// Ignore MuxBroker. That is used to create more multiplexed streams on our
// plugin connection and is a more advanced use case.
type HookHandlerPlugin struct {
	// Impl Injection
	Impl hooks.HookHandler
}

func (p *HookHandlerPlugin) Server(*plugin.MuxBroker) (any, error) {
	return &HookHandlerRPCServer{Impl: p.Impl}, nil
}

func (HookHandlerPlugin) Client(b *plugin.MuxBroker, c *rpc.Client) (any, error) {
	return &HookHandlerRPC{client: c}, nil
}
