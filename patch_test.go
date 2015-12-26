package tusd_test

import (
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
	"testing"

	. "github.com/tus/tusd"
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

func (s patchStore) WriteChunk(id string, offset int64, src io.Reader) (int64, error) {
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

	return 5, nil
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
			"Content-Type":  "application/offset+octet-stream",
			"Upload-Offset": "5",
		},
		ReqBody: strings.NewReader("hello"),
		Code:    http.StatusNoContent,
		ResHeader: map[string]string{
			"Upload-Offset": "10",
		},
	}).Run(handler, t)

	(&httpTest{
		Name:   "Non-existing file",
		Method: "PATCH",
		URL:    "no",
		ReqHeader: map[string]string{
			"Tus-Resumable": "1.0.0",
			"Content-Type":  "application/offset+octet-stream",
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
			"Content-Type":  "application/offset+octet-stream",
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
			"Content-Type":  "application/offset+octet-stream",
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

func (s overflowPatchStore) WriteChunk(id string, offset int64, src io.Reader) (int64, error) {
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

	return 15, nil
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

	body.Write([]byte("hellothisismorethan15bytes"))
	body.Close()

	(&httpTest{
		Name:   "Too big body exceeding file size",
		Method: "PATCH",
		URL:    "yes",
		ReqHeader: map[string]string{
			"Tus-Resumable":  "1.0.0",
			"Content-Type":   "application/offset+octet-stream",
			"Upload-Offset":  "5",
			"Content-Length": "3",
		},
		ReqBody: body,
		Code:    http.StatusNoContent,
	}).Run(handler, t)
}

const (
	LOCK = iota
	INFO
	WRITE
	UNLOCK
	END
)

type lockingPatchStore struct {
	zeroStore
	callOrder chan int
}

func (s lockingPatchStore) GetInfo(id string) (FileInfo, error) {
	s.callOrder <- INFO

	return FileInfo{
		Offset: 0,
		Size:   20,
	}, nil
}

func (s lockingPatchStore) WriteChunk(id string, offset int64, src io.Reader) (int64, error) {
	s.callOrder <- WRITE

	return 5, nil
}

func (s lockingPatchStore) LockUpload(id string) error {
	s.callOrder <- LOCK
	return nil
}

func (s lockingPatchStore) UnlockUpload(id string) error {
	s.callOrder <- UNLOCK
	return nil
}

func TestLockingPatch(t *testing.T) {
	callOrder := make(chan int, 10)

	handler, _ := NewHandler(Config{
		DataStore: lockingPatchStore{
			callOrder: callOrder,
		},
	})

	(&httpTest{
		Name:   "Uploading to locking store",
		Method: "PATCH",
		URL:    "yes",
		ReqHeader: map[string]string{
			"Tus-Resumable": "1.0.0",
			"Content-Type":  "application/offset+octet-stream",
			"Upload-Offset": "0",
		},
		ReqBody: strings.NewReader("hello"),
		Code:    http.StatusNoContent,
	}).Run(handler, t)

	callOrder <- END
	close(callOrder)

	if <-callOrder != LOCK {
		t.Error("expected call to LockUpload")
	}

	if <-callOrder != INFO {
		t.Error("expected call to GetInfo")
	}

	if <-callOrder != WRITE {
		t.Error("expected call to WriteChunk")
	}

	if <-callOrder != UNLOCK {
		t.Error("expected call to UnlockUpload")
	}

	if <-callOrder != END {
		t.Error("expected no more calls to happen")
	}
}
