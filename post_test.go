package tusd_test

import (
	"bytes"
	"net/http"
	"strings"
	"testing"

	"github.com/golang/mock/gomock"

	. "github.com/tus/tusd"
)

func TestPost(t *testing.T) {
	SubTest(t, "Create", func(t *testing.T, store *MockFullDataStore) {
		store.EXPECT().NewUpload(FileInfo{
			Size: 300,
			MetaData: map[string]string{
				"foo": "hello",
				"bar": "world",
			},
		}).Return("foo", nil)

		handler, _ := NewHandler(Config{
			DataStore: store,
			BasePath:  "/files/",
		})

		(&httpTest{
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
	})

	SubTest(t, "CreateExceedingMaxSizeFail", func(t *testing.T, store *MockFullDataStore) {
		handler, _ := NewHandler(Config{
			MaxSize:   400,
			DataStore: store,
			BasePath:  "/files/",
		})

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
	})

	SubTest(t, "ForwardHeaders", func(t *testing.T, store *MockFullDataStore) {
		SubTest(t, "IgnoreXForwarded", func(t *testing.T, store *MockFullDataStore) {
			store.EXPECT().NewUpload(FileInfo{
				Size:     300,
				MetaData: map[string]string{},
			}).Return("foo", nil)

			handler, _ := NewHandler(Config{
				DataStore: store,
				BasePath:  "/files/",
			})

			(&httpTest{
				Method: "POST",
				ReqHeader: map[string]string{
					"Tus-Resumable":     "1.0.0",
					"Upload-Length":     "300",
					"X-Forwarded-Host":  "foo.com",
					"X-Forwarded-Proto": "https",
				},
				Code: http.StatusCreated,
				ResHeader: map[string]string{
					"Location": "http://tus.io/files/foo",
				},
			}).Run(handler, t)
		})

		SubTest(t, "RespectXForwarded", func(t *testing.T, store *MockFullDataStore) {
			store.EXPECT().NewUpload(FileInfo{
				Size:     300,
				MetaData: map[string]string{},
			}).Return("foo", nil)

			handler, _ := NewHandler(Config{
				DataStore:               store,
				BasePath:                "/files/",
				RespectForwardedHeaders: true,
			})

			(&httpTest{
				Method: "POST",
				ReqHeader: map[string]string{
					"Tus-Resumable":     "1.0.0",
					"Upload-Length":     "300",
					"X-Forwarded-Host":  "foo.com",
					"X-Forwarded-Proto": "https",
				},
				Code: http.StatusCreated,
				ResHeader: map[string]string{
					"Location": "https://foo.com/files/foo",
				},
			}).Run(handler, t)
		})

		SubTest(t, "RespectForwarded", func(t *testing.T, store *MockFullDataStore) {
			store.EXPECT().NewUpload(FileInfo{
				Size:     300,
				MetaData: map[string]string{},
			}).Return("foo", nil)

			handler, _ := NewHandler(Config{
				DataStore:               store,
				BasePath:                "/files/",
				RespectForwardedHeaders: true,
			})

			(&httpTest{
				Method: "POST",
				ReqHeader: map[string]string{
					"Tus-Resumable":     "1.0.0",
					"Upload-Length":     "300",
					"X-Forwarded-Host":  "bar.com",
					"X-Forwarded-Proto": "http",
					"Forwarded":         "proto=https,host=foo.com",
				},
				Code: http.StatusCreated,
				ResHeader: map[string]string{
					"Location": "https://foo.com/files/foo",
				},
			}).Run(handler, t)
		})

		SubTest(t, "FilterForwardedProtocol", func(t *testing.T, store *MockFullDataStore) {
			store.EXPECT().NewUpload(FileInfo{
				Size:     300,
				MetaData: map[string]string{},
			}).Return("foo", nil)

			handler, _ := NewHandler(Config{
				DataStore:               store,
				BasePath:                "/files/",
				RespectForwardedHeaders: true,
			})

			(&httpTest{
				Method: "POST",
				ReqHeader: map[string]string{
					"Tus-Resumable":     "1.0.0",
					"Upload-Length":     "300",
					"X-Forwarded-Proto": "aaa",
					"Forwarded":         "proto=bbb",
				},
				Code: http.StatusCreated,
				ResHeader: map[string]string{
					"Location": "http://tus.io/files/foo",
				},
			}).Run(handler, t)
		})
	})

	SubTest(t, "WithUpload", func(t *testing.T, store *MockFullDataStore) {
		SubTest(t, "Create", func(t *testing.T, store *MockFullDataStore) {
			gomock.InOrder(
				store.EXPECT().NewUpload(FileInfo{
					Size: 300,
					MetaData: map[string]string{
						"foo": "hello",
						"bar": "world",
					},
				}).Return("foo", nil),
				store.EXPECT().WriteChunk("foo", int64(0), NewReaderMatcher("hello")).Return(int64(5), nil),
			)

			handler, _ := NewHandler(Config{
				DataStore: store,
				BasePath:  "/files/",
			})

			(&httpTest{
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
		})

		SubTest(t, "CreateExceedingUploadSize", func(t *testing.T, store *MockFullDataStore) {
			store.EXPECT().NewUpload(FileInfo{
				Size:     300,
				MetaData: map[string]string{},
			}).Return("foo", nil)

			handler, _ := NewHandler(Config{
				DataStore: store,
				BasePath:  "/files/",
			})

			(&httpTest{
				Method: "POST",
				ReqHeader: map[string]string{
					"Tus-Resumable": "1.0.0",
					"Upload-Length": "300",
					"Content-Type":  "application/offset+octet-stream",
				},
				ReqBody: bytes.NewReader(make([]byte, 400)),
				Code:    http.StatusRequestEntityTooLarge,
			}).Run(handler, t)
		})

		SubTest(t, "IncorrectContentType", func(t *testing.T, store *MockFullDataStore) {
			store.EXPECT().NewUpload(FileInfo{
				Size:     300,
				MetaData: map[string]string{},
			}).Return("foo", nil)

			handler, _ := NewHandler(Config{
				DataStore: store,
				BasePath:  "/files/",
			})

			(&httpTest{
				Name:   "Incorrect content type",
				Method: "POST",
				ReqHeader: map[string]string{
					"Tus-Resumable": "1.0.0",
					"Upload-Length": "300",
					"Content-Type":  "application/false",
				},
				ReqBody: strings.NewReader("hello"),
				Code:    http.StatusCreated,
				ResHeader: map[string]string{
					"Location":      "http://tus.io/files/foo",
					"Upload-Offset": "",
				},
			}).Run(handler, t)
		})

		SubTest(t, "UploadToFinalUpload", func(t *testing.T, store *MockFullDataStore) {
			handler, _ := NewHandler(Config{
				DataStore: store,
				BasePath:  "/files/",
			})

			(&httpTest{
				Method: "POST",
				ReqHeader: map[string]string{
					"Tus-Resumable": "1.0.0",
					"Upload-Length": "300",
					"Content-Type":  "application/offset+octet-stream",
					"Upload-Concat": "final; http://tus.io/files/a http://tus.io/files/b",
				},
				ReqBody: strings.NewReader("hello"),
				Code:    http.StatusForbidden,
			}).Run(handler, t)
		})
	})
}
