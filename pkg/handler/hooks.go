package handler

import (
	"context"
)

// HookEvent represents an event from tusd which can be handled by the application.
type HookEvent struct {
	Context context.Context `json:"-"`
	// Upload contains information about the upload that caused this hook
	// to be fired.
	Upload FileInfo
	// HTTPRequest contains details about the HTTP request that reached
	// tusd.
	HTTPRequest HTTPRequest
}

func newHookEvent(c *httpContext, info FileInfo) HookEvent {
	// The Host header field is not present in the header map, see https://pkg.go.dev/net/http#Request:
	// > For incoming requests, the Host header is promoted to the
	// > Request.Host field and removed from the Header map.
	// That's why we add it back manually.
	c.req.Header.Set("Host", c.req.Host)

	return HookEvent{
		Context: c,
		Upload:  info,
		HTTPRequest: HTTPRequest{
			Method:     c.req.Method,
			URI:        c.req.RequestURI,
			RemoteAddr: c.req.RemoteAddr,
			Header:     c.req.Header,
		},
	}
}
