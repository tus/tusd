package handler

import (
	"context"
)

// HookEvent represents an event from tusd which can be handled by the application.
type HookEvent struct {
	// Context provides access to the context from the HTTP request. This context is
	// not the exact value as the request context from http.Request.Context() but
	// a similar context that retains the same values as the request context. In
	// addition, Context will be cancelled after a short delay when the request context
	// is done. This delay is controlled by Config.GracefulRequestCompletionTimeout.
	//
	// The reason is that we want stores to be able to continue processing a request after
	// its context has been cancelled. For example, assume a PATCH request is incoming. If
	// the end-user pauses the upload, the connection is closed causing the request context
	// to be cancelled immediately. However, we want the store to be able to save the last
	// few bytes that were transmitted before the request was aborted. To allow this, we
	// copy the request context but cancel it with a brief delay to give the data store
	// time to finish its operations.
	Context context.Context `json:"-"`
	// Upload contains information about the upload that caused this hook
	// to be fired.
	Upload FileInfo
	// HTTPRequest contains details about the HTTP request that reached
	// tusd.
	HTTPRequest HTTPRequest
	// Only use by pre-access and pre-create (when Upload-Concat) hook,
	// for uploads protection for instance
	Access AccessInfo
}

type AccessMode = string

const (
	AccessModeRead  AccessMode = "read"
	AccessModeWrite AccessMode = "write"
)

type AccessInfo struct {
	// read (Head/Get/Upload-Concat) or write (Patch/Delete)
	Mode AccessMode

	// All files info that will be access by http request
	// Use an array because of Upload-Concat that may target several files
	Uploads []FileInfo
}

func newHookEvent(c *httpContext, info *FileInfo) HookEvent {
	// The Host header field is not present in the header map, see https://pkg.go.dev/net/http#Request:
	// > For incoming requests, the Host header is promoted to the
	// > Request.Host field and removed from the Header map.
	// That's why we add it back manually.
	c.req.Header.Set("Host", c.req.Host)

	var upload FileInfo
	if info != nil {
		upload = *info
	}
	return HookEvent{
		Context: c,
		Upload:  upload,
		HTTPRequest: HTTPRequest{
			Method:     c.req.Method,
			URI:        c.req.RequestURI,
			RemoteAddr: c.req.RemoteAddr,
			Header:     c.req.Header,
		},
		Access: AccessInfo{},
	}
}

func newHookAccessEvent(c *httpContext, mode AccessMode, uploads []FileInfo) HookEvent {
	event := newHookEvent(c, nil)
	event.Access = AccessInfo{
		Mode:    mode,
		Uploads: uploads,
	}
	return event
}
