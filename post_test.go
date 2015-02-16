package tusd

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

type postStore struct {
	t *testing.T
	zeroStore
}

func (s postStore) NewUpload(info FileInfo) (string, error) {
	if info.Size != 300 {
		s.t.Errorf("Expected size to be 300 (got %v)", info.Size)
	}

	metaData := info.MetaData
	if len(metaData) != 2 {
		s.t.Errorf("Expected two elements in metadata")
	}

	if v := metaData["foo"]; v != "hello" {
		s.t.Errorf("Expected foo element to be 'hello' but got %s", v)
	}

	if v := metaData["bar"]; v != "world" {
		s.t.Errorf("Expected bar element to be 'world' but got %s", v)
	}

	return "foo", nil
}

func TestPost(t *testing.T) {
	handler, _ := NewHandler(Config{
		MaxSize:  400,
		BasePath: "files",
		DataStore: postStore{
			t: t,
		},
	})

	// Test successful request
	req, _ := http.NewRequest("POST", "", nil)
	req.Header.Set("TUS-Resumable", "1.0.0")
	req.Header.Set("Entity-Length", "300")
	req.Header.Set("Metadata", "foo aGVsbG8=, bar d29ybGQ=")
	req.Host = "tus.io"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Errorf("Expected 201 Created for OPTIONS request (got %v)", w.Code)
	}

	if location := w.HeaderMap.Get("Location"); location != "http://tus.io/files/foo" {
		t.Errorf("Unexpected location header (got '%v')", location)
	}

	// Test exceeding MaxSize
	req, _ = http.NewRequest("POST", "", nil)
	req.Header.Set("TUS-Resumable", "1.0.0")
	req.Header.Set("Entity-Length", "500")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("Expected %v for OPTIONS request (got %v)", http.StatusRequestEntityTooLarge, w.Code)
	}
}
