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

type zeroStore struct{}

func (store zeroStore) NewUpload(size int64, metaData MetaData) (string, error) {
	return "", nil
}
func (store zeroStore) WriteChunk(id string, offset int64, src io.Reader) error {
	return nil
}

func (store zeroStore) GetInfo(id string) (FileInfo, error) {
	return FileInfo{}, nil
}

func (store zeroStore) GetReader(id string) (io.Reader, error) {
	return nil, ErrNotImplemented
}

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

func TestProtocolDiscovery(t *testing.T) {
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
		"TUS-Extension": "file-creation,metadata",
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

type postStore struct {
	t *testing.T
	zeroStore
}

func (s postStore) NewUpload(size int64, metaData MetaData) (string, error) {
	if size != 300 {
		s.t.Errorf("Expected size to be 300 (got %v)", size)
	}

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

func TestFileCreation(t *testing.T) {
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

func TestGetInfo(t *testing.T) {
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

type getStore struct {
	zeroStore
}

func (s getStore) GetInfo(id string) (FileInfo, error) {
	if id != "yes" {
		return FileInfo{}, os.ErrNotExist
	}

	return FileInfo{
		Offset: 5,
		Size:   20,
	}, nil
}

func (s getStore) GetReader(id string) (io.Reader, error) {
	return strings.NewReader("hello"), nil
}

func TestGetFile(t *testing.T) {
	handler, _ := NewHandler(Config{
		DataStore: getStore{},
	})

	// Test successfull download
	req, _ := http.NewRequest("GET", "yes", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("Expected %v (got %v)", http.StatusOK, w.Code)
	}

	if string(w.Body.Bytes()) != "hello" {
		t.Errorf("Expected response body to be 'hello'")
	}

	if w.HeaderMap.Get("Content-Length") != "5" {
		t.Errorf("Expected Content-Length to be 5")
	}
}
