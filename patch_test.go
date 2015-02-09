package tusd

import (
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

type patchStore struct {
	zeroStore
	t      *testing.T
	called bool
}

func (s patchStore) GetInfo(id string) (FileInfo, error) {
	if id != "yes" {
		return FileInfo{}, os.ErrNotExist
	}

	return FileInfo{
		Offset: 5,
		Size:   20,
	}, nil
}

func (s patchStore) WriteChunk(id string, offset int64, src io.Reader) error {
	if s.called {
		s.t.Errorf("WriteChunk must be called only once")
	}
	s.called = true

	if offset != 5 {
		s.t.Errorf("Expected offset to be 5 (got %v)", offset)
	}

	data, err := ioutil.ReadAll(src)
	if err != nil {
		s.t.Error(err)
	}

	if string(data) != "hello" {
		s.t.Errorf("Expected source to be 'hello'")
	}

	return nil
}

func TestPatch(t *testing.T) {
	handler, _ := NewHandler(Config{
		MaxSize: 100,
		DataStore: patchStore{
			t: t,
		},
	})

	// Test successful request
	req, _ := http.NewRequest("PATCH", "yes", strings.NewReader("hello"))
	req.Header.Set("TUS-Resumable", "1.0.0")
	req.Header.Set("Offset", "5")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Errorf("Expected %v (got %v)", http.StatusNoContent, w.Code)
	}

	// Test non-existing file
	req, _ = http.NewRequest("PATCH", "no", nil)
	req.Header.Set("TUS-Resumable", "1.0.0")
	req.Header.Set("Offset", "0")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("Expected %v (got %v)", http.StatusNotFound, w.Code)
	}

	// Test wrong offset
	req, _ = http.NewRequest("PATCH", "yes", nil)
	req.Header.Set("TUS-Resumable", "1.0.0")
	req.Header.Set("Offset", "4")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusConflict {
		t.Errorf("Expected %v (got %v)", http.StatusConflict, w.Code)
	}

	// Test exceeding file size
	req, _ = http.NewRequest("PATCH", "yes", strings.NewReader("hellothisismorethan15bytes"))
	req.Header.Set("TUS-Resumable", "1.0.0")
	req.Header.Set("Offset", "5")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("Expected %v (got %v)", http.StatusRequestEntityTooLarge, w.Code)
	}
}

type overflowPatchStore struct {
	zeroStore
	t      *testing.T
	called bool
}

func (s overflowPatchStore) GetInfo(id string) (FileInfo, error) {
	if id != "yes" {
		return FileInfo{}, os.ErrNotExist
	}

	return FileInfo{
		Offset: 5,
		Size:   20,
	}, nil
}

func (s overflowPatchStore) WriteChunk(id string, offset int64, src io.Reader) error {
	if s.called {
		s.t.Errorf("WriteChunk must be called only once")
	}
	s.called = true

	if offset != 5 {
		s.t.Errorf("Expected offset to be 5 (got %v)", offset)
	}

	data, err := ioutil.ReadAll(src)
	if err != nil {
		s.t.Error(err)
	}

	if len(data) != 15 {
		s.t.Errorf("Expected 15 bytes got %v", len(data))
	}

	return nil
}

// noEOFReader implements io.Reader, io.Writer, io.Closer but does not return
// an io.EOF when the internal buffer is empty. This way we can simulate slow
// networks.
type noEOFReader struct {
	closed bool
	buffer []byte
}

func (r *noEOFReader) Read(dst []byte) (int, error) {
	if r.closed && len(r.buffer) == 0 {
		return 0, io.EOF
	}

	n := copy(dst, r.buffer)
	r.buffer = r.buffer[n:]
	return n, nil
}

func (r *noEOFReader) Close() error {
	r.closed = true
	return nil
}

func (r *noEOFReader) Write(src []byte) (int, error) {
	r.buffer = append(r.buffer, src...)
	return len(src), nil
}

func TestPatchOverflow(t *testing.T) {
	handler, _ := NewHandler(Config{
		MaxSize: 100,
		DataStore: overflowPatchStore{
			t: t,
		},
	})

	body := &noEOFReader{}

	go func() {
		body.Write([]byte("hellothisismorethan15bytes"))
		body.Close()
	}()

	// Test too big body exceeding file size
	req, _ := http.NewRequest("PATCH", "yes", body)
	req.Header.Set("TUS-Resumable", "1.0.0")
	req.Header.Set("Offset", "5")
	req.Header.Set("Content-Length", "3")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Errorf("Expected %v (got %v)", http.StatusNoContent, w.Code)
	}
}
