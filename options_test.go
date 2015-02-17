package tusd

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOptions(t *testing.T) {
	handler, _ := NewHandler(Config{
		MaxSize: 400,
	})

	// Test successful OPTIONS request
	req, _ := http.NewRequest("OPTIONS", "", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Errorf("Expected 204 No Content for OPTIONS request (got %v)", w.Code)
	}

	headers := map[string]string{
		"TUS-Extension": "file-creation,metadata,concatenation",
		"TUS-Version":   "1.0.0",
		"TUS-Resumable": "1.0.0",
		"TUS-Max-Size":  "400",
	}
	for header, value := range headers {
		if v := w.HeaderMap.Get(header); value != v {
			t.Errorf("Header '%s' not contained in response", header)
		}
	}

	// Invalid or unsupported version
	req, _ = http.NewRequest("POST", "", nil)
	req.Header.Set("TUS-Resumable", "foo")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusPreconditionFailed {
		t.Errorf("Expected 412 Precondition Failed (got %v)", w.Code)
	}
}
