package handler

import (
	"context"
	"net/http"
	"time"
)

// httpContext is wrapper around context.Context that also carries the
// corresponding HTTP request and response writer, as well as an
// optional body reader
type httpContext struct {
	context.Context

	res  http.ResponseWriter
	req  *http.Request
	body *bodyReader
}

func (h UnroutedHandler) newContext(w http.ResponseWriter, r *http.Request) *httpContext {
	return &httpContext{
		// We construct a new context which gets cancelled with a delay.
		// See HookEvent.Context for more details.
		Context: newDelayedContext(r.Context(), h.config.GracefulRequestCompletionDuration),
		res:     w,
		req:     r,
		body:    nil, // body can be filled later for PATCH requests
	}
}

func (c httpContext) Value(key any) any {
	// We overwrite the Value function to ensure that the values from the request
	// context are returned because c.Context does not contain any values.
	return c.req.Context().Value(key)
}

// newDelayedContext returns a context that is cancelled with a delay. If the parent context
// is done, the new context will also be cancelled but only after waiting the specified delay.
// Note: The parent context MUST be cancelled or otherwise this will leak resources. In the
// case of http.Request.Context, the net/http package ensures that the context is always cancelled.
func newDelayedContext(parent context.Context, delay time.Duration) context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-parent.Done()
		<-time.After(delay)
		cancel()
	}()

	return ctx
}
