package tusd_test

import (
	"bytes"
	"io"
	"io/ioutil"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	. "github.com/tus/tusd"
)

type postStore struct {
	t *assert.Assertions
	zeroStore
}

func (s postStore) NewUpload(info FileInfo) (string, error) {
	s.t.Equal(int64(300), info.Size)

	metaData := info.MetaData
	s.t.Equal(2, len(metaData))
	s.t.Equal("hello", metaData["foo"])
	s.t.Equal("world", metaData["bar"])

	return "foo", nil
}

func (s postStore) WriteChunk(id string, offset int64, src io.Reader) (int64, error) {
	s.t.Equal(int64(0), offset)

	data, err := ioutil.ReadAll(src)
	s.t.Nil(err)
	s.t.Equal("hello", string(data))

	return 5, nil
}

func (s postStore) ConcatUploads(id string, uploads []string) error {
	s.t.True(false, "concatenation should not be attempted")
	return nil
}

func TestPost(t *testing.T) {
	a := assert.New(t)

	handler, _ := NewHandler(Config{
		MaxSize:  400,
		BasePath: "files",
		DataStore: postStore{
			t: a,
		},
	})

	(&httpTest{
		Name:   "Successful request",
		Method: "POST",
		ReqHeader: map[string]string{
			"Tus-Resumable":   "1.0.0",
			"Upload-Length":   "300",
			"Upload-Metadata": "foo aGVsbG8=, bar d29ybGQ=",
		},
		Code: http.StatusCreated,
		ResHeader: map[string]string{
			"Location": "http://tus.io/files/foo",
		},
	}).Run(handler, t)

	(&httpTest{
		Name:   "Exceeding MaxSize",
		Method: "POST",
		ReqHeader: map[string]string{
			"Tus-Resumable":   "1.0.0",
			"Upload-Length":   "500",
			"Upload-Metadata": "foo aGVsbG8=, bar d29ybGQ=",
		},
		Code: http.StatusRequestEntityTooLarge,
	}).Run(handler, t)

	(&httpTest{
		Name:   "Ignore Forwarded headers",
		Method: "POST",
		ReqHeader: map[string]string{
			"Tus-Resumable":     "1.0.0",
			"Upload-Length":     "300",
			"Upload-Metadata":   "foo aGVsbG8=, bar d29ybGQ=",
			"X-Forwarded-Host":  "foo.com",
			"X-Forwarded-Proto": "https",
		},
		Code: http.StatusCreated,
		ResHeader: map[string]string{
			"Location": "http://tus.io/files/foo",
		},
	}).Run(handler, t)

	handler, _ = NewHandler(Config{
		MaxSize:  400,
		BasePath: "files",
		DataStore: postStore{
			t: a,
		},
		RespectForwardedHeaders: true,
	})

	(&httpTest{
		Name:   "Respect X-Forwarded-* headers",
		Method: "POST",
		ReqHeader: map[string]string{
			"Tus-Resumable":     "1.0.0",
			"Upload-Length":     "300",
			"Upload-Metadata":   "foo aGVsbG8=, bar d29ybGQ=",
			"X-Forwarded-Host":  "foo.com",
			"X-Forwarded-Proto": "https",
		},
		Code: http.StatusCreated,
		ResHeader: map[string]string{
			"Location": "https://foo.com/files/foo",
		},
	}).Run(handler, t)

	(&httpTest{
		Name:   "Respect Forwarded headers",
		Method: "POST",
		ReqHeader: map[string]string{
			"Tus-Resumable":     "1.0.0",
			"Upload-Length":     "300",
			"Upload-Metadata":   "foo aGVsbG8=, bar d29ybGQ=",
			"X-Forwarded-Host":  "bar.com",
			"X-Forwarded-Proto": "http",
			"Forwarded":         "proto=https,host=foo.com",
		},
		Code: http.StatusCreated,
		ResHeader: map[string]string{
			"Location": "https://foo.com/files/foo",
		},
	}).Run(handler, t)

	(&httpTest{
		Name:   "Filter forwarded protocol",
		Method: "POST",
		ReqHeader: map[string]string{
			"Tus-Resumable":     "1.0.0",
			"Upload-Length":     "300",
			"Upload-Metadata":   "foo aGVsbG8=, bar d29ybGQ=",
			"X-Forwarded-Proto": "aaa",
			"Forwarded":         "proto=bbb",
		},
		Code: http.StatusCreated,
		ResHeader: map[string]string{
			"Location": "http://tus.io/files/foo",
		},
	}).Run(handler, t)
}

func TestPostWithUpload(t *testing.T) {
	a := assert.New(t)

	handler, _ := NewHandler(Config{
		MaxSize:  400,
		BasePath: "files",
		DataStore: postStore{
			t: a,
		},
	})

	(&httpTest{
		Name:   "Successful request",
		Method: "POST",
		ReqHeader: map[string]string{
			"Tus-Resumable":   "1.0.0",
			"Upload-Length":   "300",
			"Content-Type":    "application/offset+octet-stream",
			"Upload-Metadata": "foo aGVsbG8=, bar d29ybGQ=",
		},
		ReqBody: strings.NewReader("hello"),
		Code:    http.StatusCreated,
		ResHeader: map[string]string{
			"Location":      "http://tus.io/files/foo",
			"Upload-Offset": "5",
		},
	}).Run(handler, t)

	(&httpTest{
		Name:   "Exceeding upload size",
		Method: "POST",
		ReqHeader: map[string]string{
			"Tus-Resumable":   "1.0.0",
			"Upload-Length":   "300",
			"Content-Type":    "application/offset+octet-stream",
			"Upload-Metadata": "foo aGVsbG8=, bar d29ybGQ=",
		},
		ReqBody: bytes.NewReader(make([]byte, 400)),
		Code:    http.StatusRequestEntityTooLarge,
	}).Run(handler, t)

	(&httpTest{
		Name:   "Incorrect content type",
		Method: "POST",
		ReqHeader: map[string]string{
			"Tus-Resumable":   "1.0.0",
			"Upload-Length":   "300",
			"Upload-Metadata": "foo aGVsbG8=, bar d29ybGQ=",
			"Content-Type":    "application/false",
		},
		ReqBody: strings.NewReader("hello"),
		Code:    http.StatusCreated,
		ResHeader: map[string]string{
			"Location":      "http://tus.io/files/foo",
			"Upload-Offset": "",
		},
	}).Run(handler, t)

	(&httpTest{
		Name:   "Upload and final concatenation",
		Method: "POST",
		ReqHeader: map[string]string{
			"Tus-Resumable":   "1.0.0",
			"Upload-Length":   "300",
			"Content-Type":    "application/offset+octet-stream",
			"Upload-Metadata": "foo aGVsbG8=, bar d29ybGQ=",
			"Upload-Concat":   "final; http://tus.io/files/a http://tus.io/files/b",
		},
		ReqBody: strings.NewReader("hello"),
		Code:    http.StatusForbidden,
	}).Run(handler, t)
}
