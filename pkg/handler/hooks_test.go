package handler

import (
	"encoding/json"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestHookEventHeaderRace demonstrates a race condition that occurs when
// the http.Header map is shared between the original request and HookEvent.
// This test will fail with `go test -race` if Header is not cloned.
//
// See: https://github.com/tus/tusd/issues/1320
func TestHookEventHeaderRace(t *testing.T) {
	req := httptest.NewRequest("POST", "/files", nil)
	req.Header.Set("Upload-Length", "1000")
	req.Header.Set("Tus-Resumable", "1.0.0")
	req.Host = "example.com"

	c := &httpContext{
		req: req,
	}

	info := FileInfo{
		ID:       "test-id",
		Size:     1000,
		MetaData: map[string]string{"filename": "test.txt"},
	}

	event := newHookEvent(c, info)

	var wg sync.WaitGroup
	const iterations = 100

	// Goroutine 1: Simulate async hook processing (JSON encoding)
	// This is what happens in invokeHookAsync -> json.Marshal
	wg.Go(func() {
		for range iterations {
			// This iterates over the Header map
			_, _ = json.Marshal(event.HTTPRequest)
		}
	})

	// Goroutine 2: Simulate concurrent request header modification
	// This could happen if another hook event is created for the same request
	// or if middleware modifies headers
	wg.Go(func() {
		for range iterations {
			// This writes to the Header map
			c.req.Header.Set("X-Request-ID", "some-value")
			c.req.Header.Del("X-Request-ID")
		}
	})

	wg.Wait()
}

// TestHookEventHeaderIsolation verifies that modifying the original request
// headers after creating a HookEvent does not affect the event headers.
func TestHookEventHeaderIsolation(t *testing.T) {
	req := httptest.NewRequest("POST", "/files", nil)
	req.Header.Set("X-Original", "original-value")
	req.Host = "example.com"

	c := &httpContext{
		req: req,
	}

	info := FileInfo{
		ID:   "test-id",
		Size: 1000,
	}

	event := newHookEvent(c, info)

	// Verify original header is present
	assert.Equal(t, "original-value", event.HTTPRequest.Header.Get("X-Original"))

	// Modify the original request header
	c.req.Header.Set("X-Original", "modified-value")
	c.req.Header.Set("X-New", "new-value")

	// Verify event headers are NOT affected (isolation)
	assert.Equal(t, "original-value", event.HTTPRequest.Header.Get("X-Original"),
		"event header should remain unchanged after modifying original request")
	assert.Empty(t, event.HTTPRequest.Header.Get("X-New"),
		"new header added to original request should not appear in event")
}
