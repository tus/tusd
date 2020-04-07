package hooks

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/tus/tusd/pkg/handler"

	"github.com/sethgrid/pester"
)

type HttpHook struct {
	Endpoint       string
	MaxRetries     int
	Backoff        int
	ForwardHeaders []string
}

func (_ HttpHook) Setup() error {
	return nil
}

func (h HttpHook) InvokeHook(typ HookType, info handler.HookEvent, captureOutput bool) ([]byte, int, error) {
	jsonInfo, err := json.Marshal(info)
	if err != nil {
		return nil, 0, err
	}

	req, err := http.NewRequest("POST", h.Endpoint, bytes.NewBuffer(jsonInfo))
	if err != nil {
		return nil, 0, err
	}

	for _, k := range h.ForwardHeaders {
		// Lookup the Canonicalised version of the specified header
		if vals, ok := info.HTTPRequest.Header[http.CanonicalHeaderKey(k)]; ok {
			// but set the case specified by the user
			req.Header[k] = vals
		}
	}

	req.Header.Set("Hook-Name", string(typ))
	req.Header.Set("Content-Type", "application/json")

	// TODO: Can we initialize this in Setup()?
	// Use linear backoff strategy with the user defined values.
	client := pester.New()
	client.KeepLog = true
	client.MaxRetries = h.MaxRetries
	client.Backoff = func(_ int) time.Duration {
		return time.Duration(h.Backoff) * time.Second
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, 0, err
	}

	if resp.StatusCode >= http.StatusBadRequest {
		return body, resp.StatusCode, NewHookError(fmt.Errorf("endpoint returned: %s", resp.Status), resp.StatusCode, body)
	}

	if captureOutput {
		return body, resp.StatusCode, err
	}

	return nil, resp.StatusCode, err
}
