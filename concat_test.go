package tusd

import (
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

type concatPartialStore struct {
	t *testing.T
	zeroStore
}

func (s concatPartialStore) NewUpload(info FileInfo) (string, error) {
	if !info.IsPartial {
		s.t.Error("expected upload to be partial")
	}

	if info.IsFinal {
		s.t.Error("expected upload to not be final")
	}

	if len(info.PartialUploads) != 0 {
		s.t.Error("expected no partial uploads")
	}

	return "foo", nil
}

func (s concatPartialStore) GetInfo(id string) (FileInfo, error) {
	return FileInfo{
		IsPartial: true,
	}, nil
}

func TestConcatPartial(t *testing.T) {
	handler, _ := NewHandler(Config{
		MaxSize:  400,
		BasePath: "files",
		DataStore: concatPartialStore{
			t: t,
		},
	})

	// Test successful POST request
	req, _ := http.NewRequest("POST", "", nil)
	req.Header.Set("TUS-Resumable", "1.0.0")
	req.Header.Set("Entity-Length", "300")
	req.Header.Set("Concat", "partial")
	req.Host = "tus.io"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Errorf("Expected 201 Created (got %v)", w.Code)
	}

	// Test successful HEAD request
	req, _ = http.NewRequest("HEAD", "foo", nil)
	req.Header.Set("TUS-Resumable", "1.0.0")
	req.Host = "tus.io"
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Errorf("Expected 204 No Content (got %v)", w.Code)
	}

	if w.HeaderMap.Get("Concat") != "partial" {
		t.Errorf("Expect Concat header to be set")
	}
}

type concatFinalStore struct {
	t *testing.T
	zeroStore
}

func (s concatFinalStore) NewUpload(info FileInfo) (string, error) {
	if info.IsPartial {
		s.t.Error("expected upload to not be partial")
	}

	if !info.IsFinal {
		s.t.Error("expected upload to be final")
	}

	if !reflect.DeepEqual(info.PartialUploads, []string{"a", "b"}) {
		s.t.Error("unexpected partial uploads")
	}

	return "foo", nil
}

func (s concatFinalStore) GetInfo(id string) (FileInfo, error) {
	if id == "a" || id == "b" {
		return FileInfo{
			IsPartial: true,
			Size:      5,
			Offset:    5,
		}, nil
	}

	if id == "c" {
		return FileInfo{
			IsPartial: true,
			Size:      5,
			Offset:    3,
		}, nil
	}

	if id == "foo" {
		return FileInfo{
			IsFinal:        true,
			PartialUploads: []string{"a", "b"},
			Size:           10,
			Offset:         10,
		}, nil
	}

	return FileInfo{}, ErrNotFound
}

func (s concatFinalStore) GetReader(id string) (io.Reader, error) {
	if id == "a" {
		return strings.NewReader("hello"), nil
	}

	if id == "b" {
		return strings.NewReader("world"), nil
	}

	return nil, ErrNotFound
}

func (s concatFinalStore) WriteChunk(id string, offset int64, src io.Reader) error {
	if id != "foo" {
		s.t.Error("unexpected file id")
	}

	if offset != 0 {
		s.t.Error("expected offset to be 0")
	}

	b, _ := ioutil.ReadAll(src)
	if string(b) != "helloworld" {
		s.t.Error("unexpected content")
	}

	return nil
}

func TestConcatFinal(t *testing.T) {
	handler, _ := NewHandler(Config{
		MaxSize:  400,
		BasePath: "files",
		DataStore: concatFinalStore{
			t: t,
		},
	})

	// Test successful POST request
	req, _ := http.NewRequest("POST", "", nil)
	req.Header.Set("TUS-Resumable", "1.0.0")
	req.Header.Set("Concat", "final; http://tus.io/files/a /files/b/")
	req.Host = "tus.io"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Errorf("Expected 201 Created (got %v)", w.Code)
	}

	// Test successful HEAD request
	req, _ = http.NewRequest("HEAD", "foo", nil)
	req.Header.Set("TUS-Resumable", "1.0.0")
	req.Host = "tus.io"
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Errorf("Expected 204 No Content (got %v)", w.Code)
	}

	if w.HeaderMap.Get("Concat") != "final; http://tus.io/files/a http://tus.io/files/b" {
		t.Errorf("Expect Concat header to be set")
	}

	if w.HeaderMap.Get("Entity-Length") != "10" {
		t.Errorf("Expect Entity-Length header to be 10")
	}

	// Test concatenating non finished upload (id: c)
	req, _ = http.NewRequest("POST", "", nil)
	req.Header.Set("TUS-Resumable", "1.0.0")
	req.Header.Set("Concat", "final; http://tus.io/files/c")
	req.Host = "tus.io"
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected 201 Created (got %v)", w.Code)
	}

	// Test exceeding max. size
	handler, _ = NewHandler(Config{
		MaxSize:  9,
		BasePath: "files",
		DataStore: concatFinalStore{
			t: t,
		},
	})

	req, _ = http.NewRequest("POST", "", nil)
	req.Header.Set("TUS-Resumable", "1.0.0")
	req.Header.Set("Concat", "final; http://tus.io/files/a /files/b/")
	req.Host = "tus.io"
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("Expected 201 Created (got %v)", w.Code)
	}

}
