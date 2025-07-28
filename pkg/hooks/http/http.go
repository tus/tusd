// Package http implements a HTTP-based hook system. For each hook event, it will send a
// POST request to the specified endpoint. The body is a JSON-formatted object including
// the hook type, upload and request information.
// By responding with a JSON object, the response from tusd can be controlled.
package http

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"time"

	"github.com/fetlife/tusd/v2/pkg/hooks"
	"github.com/sethgrid/pester"
)

type HttpHook struct {
	Endpoint       string
	MaxRetries     int
	Backoff        time.Duration
	ForwardHeaders []string
	Timeout        time.Duration
	SizeLimit      int64

	client *pester.Client
}

func (h *HttpHook) Setup() error {
	// Use linear backoff strategy with the user defined values.
	client := pester.New()
	client.KeepLog = true
	client.MaxRetries = h.MaxRetries
	client.Backoff = func(_ int) time.Duration {
		return h.Backoff
	}

	h.client = client

	return nil
}

func (h HttpHook) InvokeHook(hookReq hooks.HookRequest) (hookRes hooks.HookResponse, err error) {
	jsonInfo, err := json.Marshal(hookReq)
	if err != nil {
		return hookRes, err
	}

	ctx := hookReq.Event.Context
	if ctx == nil {
		ctx = context.Background()
	}

	ctx, cancel := context.WithTimeout(ctx, h.Timeout)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(ctx, "POST", h.Endpoint, bytes.NewBuffer(jsonInfo))
	if err != nil {
		return hookRes, err
	}

	for _, k := range h.ForwardHeaders {
		// Lookup the Canonicalised version of the specified header
		if vals, ok := hookReq.Event.HTTPRequest.Header[http.CanonicalHeaderKey(k)]; ok {
			// but set the case specified by the user
			httpReq.Header[k] = vals
		}
	}

	httpReq.Header.Set("Content-Type", "application/json")

	httpRes, err := h.client.Do(httpReq)
	if err != nil {
		return hookRes, err
	}
	defer httpRes.Body.Close()

	httpBody, err := io.ReadAll(io.LimitReader(httpRes.Body, h.SizeLimit+1))
	if err != nil {
		return hookRes, err
	}

	// Report an error, if the response has a non-2XX status code
	if httpRes.StatusCode < http.StatusOK || httpRes.StatusCode >= http.StatusMultipleChoices {
		return hookRes, fmt.Errorf("unexpected response code from hook endpoint (%d): %s", httpRes.StatusCode, string(httpBody))
	}

	if int64(len(httpBody)) > h.SizeLimit {
		return hookRes, fmt.Errorf("hook response exceeded maximum size of %d bytes", h.SizeLimit)
	}

	contentType := httpRes.Header.Get("Content-Type")
	if contentType == "" {
		return hookRes, fmt.Errorf("hook response does not contain the 'Content-Type: application/json' header")
	}

	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return hookRes, fmt.Errorf("failed to parse Content-Type header: %w", err)
	}
	if mediaType != "application/json" {
		return hookRes, fmt.Errorf("expected hook response Content-Type to be application/json, but got '%s'", contentType)
	}

	if err = json.Unmarshal(httpBody, &hookRes); err != nil {
		return hookRes, fmt.Errorf("failed to parse hook response: %w", err)
	}

	return hookRes, nil
}
