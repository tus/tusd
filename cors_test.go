package tusd

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCORS(t *testing.T) {
	handler, _ := NewHandler(Config{})

	// Test preflight request
	req, _ := http.NewRequest("OPTIONS", "", nil)
	req.Header.Set("Origin", "tus.io")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Errorf("Expected 204 No Content for OPTIONS request (got %v)", w.Code)
	}

	headers := []string{
		"Access-Control-Allow-Headers",
		"Access-Control-Allow-Methods",
		"Access-Control-Max-Age",
	}
	for _, header := range headers {
		if _, ok := w.HeaderMap[header]; !ok {
			t.Errorf("Header '%s' not contained in response", header)
		}
	}

	origin := w.HeaderMap.Get("Access-Control-Allow-Origin")
	if origin != "tus.io" {
		t.Errorf("Allowed origin not 'tus.io' but '%s'", origin)
	}

	// Test actual request
	req, _ = http.NewRequest("GET", "", nil)
	req.Header.Set("Origin", "tus.io")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	origin = w.HeaderMap.Get("Access-Control-Allow-Origin")
	if origin != "tus.io" {
		t.Errorf("Allowed origin not 'tus.io' but '%s'", origin)
	}
	if _, ok := w.HeaderMap["Access-Control-Expose-Headers"]; !ok {
		t.Error("Expose-Headers not contained in response")
	}
}
