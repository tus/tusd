package handler

import "net/http"

// HookEvent represents an event from tusd which can be handled by the application.
type HookEvent struct {
	// Upload contains information about the upload that caused this hook
	// to be fired.
	Upload FileInfo
	// HTTPRequest contains details about the HTTP request that reached
	// tusd.
	HTTPRequest HTTPRequest
}

// HookResponse is the response after a hook is executed.
type HookResponse struct {
	// Updated metadata which can be set from the pre create hook and is then used instead of the initial metadata.
	UpdatedMetaData MetaData
}

func newHookEvent(info FileInfo, r *http.Request) HookEvent {
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
