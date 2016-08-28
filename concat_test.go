package tusd_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"

	. "github.com/tus/tusd"
)

type concatPartialStore struct {
	t *assert.Assertions
	zeroStore
}

func (s concatPartialStore) NewUpload(info FileInfo) (string, error) {
	s.t.True(info.IsPartial)
	s.t.False(info.IsFinal)
	s.t.Nil(info.PartialUploads)

	return "foo", nil
}

func (s concatPartialStore) GetInfo(id string) (FileInfo, error) {
	return FileInfo{
		IsPartial: true,
	}, nil
}

func (s concatPartialStore) ConcatUploads(id string, uploads []string) error {
	return nil
}

func TestConcatPartial(t *testing.T) {
	handler, _ := NewHandler(Config{
		MaxSize:  400,
		BasePath: "files",
		DataStore: concatPartialStore{
			t: assert.New(t),
		},
	})

	(&httpTest{
		Name:   "Successful OPTIONS request",
		Method: "OPTIONS",
		URL:    "",
		ResHeader: map[string]string{
			"Tus-Extension": "creation,concatenation",
		},
		Code: http.StatusOK,
	}).Run(handler, t)

	(&httpTest{
		Name:   "Successful POST request",
		Method: "POST",
		ReqHeader: map[string]string{
			"Tus-Resumable": "1.0.0",
			"Upload-Length": "300",
			"Upload-Concat": "partial",
		},
		Code: http.StatusCreated,
	}).Run(handler, t)

	(&httpTest{
		Name:   "Successful HEAD request",
		Method: "HEAD",
		URL:    "foo",
		ReqHeader: map[string]string{
			"Tus-Resumable": "1.0.0",
		},
		Code: http.StatusOK,
		ResHeader: map[string]string{
			"Upload-Concat": "partial",
		},
	}).Run(handler, t)
}

type concatFinalStore struct {
	t *assert.Assertions
	zeroStore
}

func (s concatFinalStore) NewUpload(info FileInfo) (string, error) {
	s.t.False(info.IsPartial)
	s.t.True(info.IsFinal)
	s.t.Equal([]string{"a", "b"}, info.PartialUploads)

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

func (s concatFinalStore) ConcatUploads(id string, uploads []string) error {
	s.t.Equal("foo", id)
	s.t.Equal([]string{"a", "b"}, uploads)

	return nil
}

func TestConcatFinal(t *testing.T) {
	a := assert.New(t)

	handler, _ := NewHandler(Config{
		MaxSize:  400,
		BasePath: "files",
		DataStore: concatFinalStore{
			t: a,
		},
		NotifyCompleteUploads: true,
	})

	c := make(chan FileInfo, 1)
	handler.CompleteUploads = c

	(&httpTest{
		Name:   "Successful POST request",
		Method: "POST",
		ReqHeader: map[string]string{
			"Tus-Resumable": "1.0.0",
			"Upload-Concat": "final; http://tus.io/files/a /files/b/",
		},
		Code: http.StatusCreated,
	}).Run(handler, t)

	info := <-c
	a.Equal("foo", info.ID)
	a.Equal(int64(10), info.Size)
	a.Equal(int64(10), info.Offset)
	a.False(info.IsPartial)
	a.True(info.IsFinal)
	a.Equal([]string{"a", "b"}, info.PartialUploads)

	(&httpTest{
		Name:   "Successful HEAD request",
		Method: "HEAD",
		URL:    "foo",
		ReqHeader: map[string]string{
			"Tus-Resumable": "1.0.0",
		},
		Code: http.StatusOK,
		ResHeader: map[string]string{
			"Upload-Concat": "final; http://tus.io/files/a http://tus.io/files/b",
			"Upload-Length": "10",
			"Upload-Offset": "10",
		},
	}).Run(handler, t)

	(&httpTest{
		Name:   "Concatenating non finished upload (id: c)",
		Method: "POST",
		ReqHeader: map[string]string{
			"Tus-Resumable": "1.0.0",
			"Upload-Concat": "final; http://tus.io/files/c",
		},
		Code: http.StatusBadRequest,
	}).Run(handler, t)

	handler, _ = NewHandler(Config{
		MaxSize:  9,
		BasePath: "files",
		DataStore: concatFinalStore{
			t: assert.New(t),
		},
	})

	(&httpTest{
		Name:   "Exceeding MaxSize",
		Method: "POST",
		ReqHeader: map[string]string{
			"Tus-Resumable": "1.0.0",
			"Upload-Concat": "final; http://tus.io/files/a /files/b/",
		},
		Code: http.StatusRequestEntityTooLarge,
	}).Run(handler, t)
}
