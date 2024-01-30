// Package http implements a HTTP-based hook system. For each hook event, it will send a
// POST request to the specified endpoint. The body is a JSON-formatted object including
// the hook type, upload and request information.
// By responding with a JSON object, the response from tusd can be controlled.
package http

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/Nealsoni00/tusd/v2/pkg/hooks"
	"github.com/sethgrid/pester"
)

type HttpHook struct {
	Endpoint       string
	MaxRetries     int
	Backoff        time.Duration
	ForwardHeaders []string

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

	httpReq, err := http.NewRequest("POST", h.Endpoint, bytes.NewBuffer(jsonInfo))
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

	httpBody, err := io.ReadAll(httpRes.Body)
	if err != nil {
		return hookRes, err
	}

	// Report an error, if the response has a non-2XX status code
	if httpRes.StatusCode < http.StatusOK || httpRes.StatusCode >= http.StatusMultipleChoices {
		return hookRes, fmt.Errorf("unexpected response code from hook endpoint (%d): %s", httpRes.StatusCode, string(httpBody))
	}

	if err = json.Unmarshal(httpBody, &hookRes); err != nil {
		return hookRes, fmt.Errorf("failed to parse hook response: %w", err)
	}

	return hookRes, nil
}
