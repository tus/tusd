package main

import (
	"log"

	"github.com/fetlife/tusd/v2/pkg/hooks"
	tusdplugin "github.com/fetlife/tusd/v2/pkg/hooks/plugin"
	"github.com/hashicorp/go-plugin"
)

// Here is the implementation of our hook handler
type MyHookHandler struct {
}

// Setup is called once the plugin has been loaded by tusd.
func (g *MyHookHandler) Setup() error {
	// Use the log package or the g.logger field to write debug messages.
	// Do not write to stdout directly, as this is used for communication between
	// tusd and the plugin.
	log.Println("MyHookHandler.Setup is invoked")
	return nil
}

// InvokeHook is called for every hook that tusd fires.
func (g *MyHookHandler) InvokeHook(req hooks.HookRequest) (res hooks.HookResponse, err error) {
	log.Println("MyHookHandler.InvokeHook is invoked")

	// Prepare hook response structure
	res.HTTPResponse.Header = make(map[string]string)

	// Example: Use the pre-create hook to check if a filename has been supplied
	// using metadata. If not, the upload is rejected with a custom HTTP response.

	if req.Type == hooks.HookPreCreate {
		if _, ok := req.Event.Upload.MetaData["filename"]; !ok {
			res.RejectUpload = true
			res.HTTPResponse.StatusCode = 400
			res.HTTPResponse.Body = "no filename provided"
			res.HTTPResponse.Header["X-Some-Header"] = "yes"
		}
	}

	// Example: Use the post-finish hook to print information about a completed upload,
	// including its storage location.
	if req.Type == hooks.HookPostFinish {
		id := req.Event.Upload.ID
		size := req.Event.Upload.Size
		storage := req.Event.Upload.Storage

		log.Printf("Upload %s (%d bytes) is finished. Find the file at:\n", id, size)
		log.Println(storage)

	}

	// Return the hook response to tusd.
	return res, nil
}

// handshakeConfigs are used to just do a basic handshake between
// a plugin and tusd. If the handshake fails, a user friendly error is shown.
// This prevents users from executing bad plugins or executing a plugin
// directory. It is a UX feature, not a security feature.
var handshakeConfig = plugin.HandshakeConfig{
	ProtocolVersion:  1,
	MagicCookieKey:   "TUSD_PLUGIN",
	MagicCookieValue: "yes",
}

func main() {
	// 1. Initialize our handler.
	myHandler := &MyHookHandler{}

	// 2. Construct the plugin map. The key must be "hookHandler".
	var pluginMap = map[string]plugin.Plugin{
		"hookHandler": &tusdplugin.HookHandlerPlugin{Impl: myHandler},
	}

	// 3. Expose the plugin to tusd.
	plugin.Serve(&plugin.ServeConfig{
		HandshakeConfig: handshakeConfig,
		Plugins:         pluginMap,
	})
}
