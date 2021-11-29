package main

import (
	"fmt"
	"log"

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/go-plugin"
	"github.com/tus/tusd/cmd/tusd/cli/hooks"
)

// Here is a real implementation of Greeter
type MyHookHandler struct {
	logger hclog.Logger
}

func (g *MyHookHandler) Setup() error {
	log.Println("MyHookHandler.Setup is invoked")
	return nil
}

func (g *MyHookHandler) InvokeHook(req hooks.HookRequest) (res hooks.HookResponse, err error) {
	log.Println("MyHookHandler.InvokeHook is invoked")

	res.HTTPResponse.Headers = make(map[string]string)

	if req.Type == hooks.HookPreCreate {
		res.HTTPResponse.Headers["X-From-Pre-Create"] = "hello"
	}

	if req.Type == hooks.HookPreFinish {
		res.HTTPResponse.Headers["X-From-Pre-Finish"] = "hello again"
		res.HTTPResponse.Body = "some information"
	}

	return res, nil
}

// handshakeConfigs are used to just do a basic handshake between
// a plugin and host. If the handshake fails, a user friendly error is shown.
// This prevents users from executing bad plugins or executing a plugin
// directory. It is a UX feature, not a security feature.
var handshakeConfig = plugin.HandshakeConfig{
	ProtocolVersion:  1,
	MagicCookieKey:   "BASIC_PLUGIN",
	MagicCookieValue: "hello",
}

func main() {
	myHandler := &MyHookHandler{}
	// pluginMap is the map of plugins we can dispense.
	var pluginMap = map[string]plugin.Plugin{
		"hookHandler": &hooks.HookHandlerPlugin{Impl: myHandler},
	}

	plugin.Serve(&plugin.ServeConfig{
		HandshakeConfig: handshakeConfig,
		Plugins:         pluginMap,
	})

	fmt.Println("DOONE")
}
