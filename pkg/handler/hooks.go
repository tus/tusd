package handler

import "net/http"

// TODO: Move some parts to hooks package

// HookEvent represents an event from tusd which can be handled by the application.
type HookEvent struct {
	// Upload contains information about the upload that caused this hook
	// to be fired.
	Upload FileInfo
	// HTTPRequest contains details about the HTTP request that reached
	// tusd.
	HTTPRequest HTTPRequest
}

func newHookEvent(info FileInfo, r *http.Request) HookEvent {
	// The Host header field is not present in the header map, see https://pkg.go.dev/net/http#Request:
	// > For incoming requests, the Host header is promoted to the
	// > Request.Host field and removed from the Header map.
	// That's why we add it back manually.
	r.Header.Set("Host", r.Host)

	return HookEvent{
		Upload: info,
		HTTPRequest: HTTPRequest{
			Method:     r.Method,
			URI:        r.RequestURI,
			RemoteAddr: r.RemoteAddr,
			Header:     r.Header,
		},
	}
}
