package tusd

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

type headStore struct {
	zeroStore
}

func (s headStore) GetInfo(id string) (FileInfo, error) {
	if id != "yes" {
		return FileInfo{}, os.ErrNotExist
	}

	return FileInfo{
		Offset: 11,
		Size:   44,
	}, nil
}

func TestHead(t *testing.T) {
	handler, _ := NewHandler(Config{
		BasePath:  "https://buy.art/",
		DataStore: headStore{},
	})

	// Test successful request
	req, _ := http.NewRequest("HEAD", "yes", nil)
	req.Header.Set("TUS-Resumable", "1.0.0")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Errorf("Expected %v (got %v)", http.StatusNoContent, w.Code)
	}

	headers := map[string]string{
		"Offset":        "11",
		"Entity-Length": "44",
	}
	for header, value := range headers {
		if v := w.HeaderMap.Get(header); value != v {
			t.Errorf("Unexpected header value '%s': %v", header, v)
		}
	}

	// Test non-existing file
	req, _ = http.NewRequest("HEAD", "no", nil)
	req.Header.Set("TUS-Resumable", "1.0.0")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("Expected %v (got %v)", http.StatusNotFound, w.Code)
	}
}
