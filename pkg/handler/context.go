package handler

import (
	"context"
	"net/http"
)

// httpContext is wrapper around context.Context that also carries the
// corresponding HTTP request and response writer, as well as an
// optional body reader
// TODO: Consider including HTTPResponse as well
type httpContext struct {
	context.Context

	res  http.ResponseWriter
	req  *http.Request
	body *bodyReader
}

func newContext(w http.ResponseWriter, r *http.Request) *httpContext {
	return &httpContext{
		Context: r.Context(),
		res:     w,
		req:     r,
		body:    nil, // body can be filled later for PATCH requests
	}
}
