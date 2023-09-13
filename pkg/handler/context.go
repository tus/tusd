package handler

import (
	"context"
	"errors"
	"net/http"
	"time"
)

// httpContext is wrapper around context.Context that also carries the
// corresponding HTTP request and response writer, as well as an
// optional body reader
type httpContext struct {
	context.Context

	res  http.ResponseWriter
	resC *http.ResponseController
	req  *http.Request
	body *bodyReader

	// TODO: Add structured logger
}

// TODO: Ensure that newContext is only called once.
func (h UnroutedHandler) newContext(w http.ResponseWriter, r *http.Request) *httpContext {
	// requestCtx is the context from the native request instance. It gets cancelled
	// if the connection closes, the request is cancelled (HTTP/2), ServeHTTP returns
	// or the server's base context is cancelled.
	requestCtx := r.Context()
	// On top of requestCtx, we construct a context that we can cancel, for example when
	// the post-receive hook stops an upload or if another uploads requests a lock to be released.
	cancellableCtx, _ := context.WithCancelCause(requestCtx)
	// On top of cancellableCtx, we construct a new context which gets cancelled with a delay.
	// See HookEvent.Context for more details, but the gist is that we want to give data stores
	// some more time to finish their buisness.
	delayedCtx := newDelayedContext(cancellableCtx, h.config.GracefulRequestCompletionTimeout)

	ctx := &httpContext{
		Context: delayedCtx,
		res:     w,
		resC:    http.NewResponseController(w),
		req:     r,
		body:    nil, // body can be filled later for PATCH requests
	}

	go func() {
		<-cancellableCtx.Done()

		// If the cause is one of our own errors, close a potential body and relay the error.
		cause := context.Cause(cancellableCtx)
		if errors.Is(cause, ErrServerShutdown) && ctx.body != nil {
			ctx.body.closeWithError(cause)
		}
	}()

	return ctx
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
