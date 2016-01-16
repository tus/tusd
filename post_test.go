package tusd_test

import (
	"net/http"
	"testing"

	. "github.com/tus/tusd"
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
			t: t,
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
