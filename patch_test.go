package tusd

import (
	"io"
	"io/ioutil"
	"net/http"
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

	(&httpTest{
		Name:   "Successful request",
		Method: "PATCH",
		URL:    "yes",
		ReqHeader: map[string]string{
			"Tus-Resumable": "1.0.0",
			"Upload-Offset": "5",
		},
		ReqBody: strings.NewReader("hello"),
		Code:    http.StatusNoContent,
	}).Run(handler, t)

	(&httpTest{
		Name:   "Non-existing file",
		Method: "PATCH",
		URL:    "no",
		ReqHeader: map[string]string{
			"Tus-Resumable": "1.0.0",
			"Upload-Offset": "5",
		},
		Code: http.StatusNotFound,
	}).Run(handler, t)

	(&httpTest{
		Name:   "Wrong offset",
		Method: "PATCH",
		URL:    "yes",
		ReqHeader: map[string]string{
			"Tus-Resumable": "1.0.0",
			"Upload-Offset": "4",
		},
		Code: http.StatusConflict,
	}).Run(handler, t)

	(&httpTest{
		Name:   "Exceeding file size",
		Method: "PATCH",
		URL:    "yes",
		ReqHeader: map[string]string{
			"Tus-Resumable": "1.0.0",
			"Upload-Offset": "5",
		},
		ReqBody: strings.NewReader("hellothisismorethan15bytes"),
		Code:    http.StatusRequestEntityTooLarge,
	}).Run(handler, t)
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

	(&httpTest{
		Name:   "Too big body exceeding file size",
		Method: "PATCH",
		URL:    "yes",
		ReqHeader: map[string]string{
			"Tus-Resumable":  "1.0.0",
			"Upload-Offset":  "5",
			"Content-Length": "3",
		},
		ReqBody: body,
		Code:    http.StatusNoContent,
	}).Run(handler, t)
}
