package tusd_test

import (
	"bytes"
	"net/http"
	"strings"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	. "github.com/tus/tusd"
)

func TestPost(t *testing.T) {
	SubTest(t, "Create", func(t *testing.T, store *MockFullDataStore) {
		handler, _ := NewHandler(Config{
			DataStore:            store,
			BasePath:             "https://buy.art/files/",
			NotifyCreatedUploads: true,
		})

		c := make(chan FileInfo, 1)
		handler.CreatedUploads = c

		test := &httpTest{
			Method: "POST",
			ReqHeader: map[string]string{
				"Tus-Resumable": "1.0.0",
				"Upload-Length": "300",
				// Invalid Base64-encoded values should be ignored
				"Upload-Metadata": "foo aGVsbG8=, bar d29ybGQ=, hah INVALID",
			},
			Code: http.StatusCreated,
			ResHeader: map[string]string{
				"Location": "https://buy.art/files/foo",
			},
		}

		store.EXPECT().NewUpload(FileInfo{
			Size: 300,
			MetaData: map[string]string{
				"foo": "hello",
				"bar": "world",
			},
			OriginalRequest: OriginalRequest{
				Proto:   "HTTP/1.1",
				Host:    "tus.io",
				Headers: toHTTPHeader(test.ReqHeader),
			},
		}).Return("foo", nil)

		test.Run(handler, t)

		info := <-c

		a := assert.New(t)
		a.Equal("foo", info.ID)
		a.Equal(int64(300), info.Size)
		a.Equal(toHTTPHeader(test.ReqHeader), info.OriginalRequest.Headers)
	})

	SubTest(t, "CreateEmptyUpload", func(t *testing.T, store *MockFullDataStore) {
		handler, _ := NewHandler(Config{
			DataStore:             store,
			BasePath:              "https://buy.art/files/",
			NotifyCompleteUploads: true,
		})

		handler.CompleteUploads = make(chan FileInfo, 1)

		test := &httpTest{
			Method: "POST",
			ReqHeader: map[string]string{
				"Tus-Resumable": "1.0.0",
				"Upload-Length": "0",
			},
			Code: http.StatusCreated,
			ResHeader: map[string]string{
				"Location": "https://buy.art/files/foo",
			},
		}

		store.EXPECT().NewUpload(FileInfo{
			Size:     0,
			MetaData: map[string]string{},
			OriginalRequest: OriginalRequest{
				Proto:   "HTTP/1.1",
				Host:    "tus.io",
				Headers: toHTTPHeader(test.ReqHeader),
			},
		}).Return("foo", nil)

		store.EXPECT().FinishUpload("foo").Return(nil)

		test.Run(handler, t)

		info := <-handler.CompleteUploads

		a := assert.New(t)
		a.Equal("foo", info.ID)
		a.Equal(int64(0), info.Size)
		a.Equal(int64(0), info.Offset)
		a.Equal(toHTTPHeader(test.ReqHeader), info.OriginalRequest.Headers)
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

	SubTest(t, "InvalidUploadLengthFail", func(t *testing.T, store *MockFullDataStore) {
		handler, _ := NewHandler(Config{
			DataStore: store,
		})

		(&httpTest{
			Method: "POST",
			URL:    "",
			ReqHeader: map[string]string{
				"Tus-Resumable": "1.0.0",
				"Upload-Length": "-5",
			},
			Code: http.StatusBadRequest,
		}).Run(handler, t)
	})

	SubTest(t, "UploadLengthAndUploadDeferLengthFail", func(t *testing.T, store *MockFullDataStore) {
		handler, _ := NewHandler(Config{
			DataStore: store,
		})

		(&httpTest{
			Method: "POST",
			URL:    "",
			ReqHeader: map[string]string{
				"Tus-Resumable":       "1.0.0",
				"Upload-Length":       "10",
				"Upload-Defer-Length": "1",
			},
			Code: http.StatusBadRequest,
		}).Run(handler, t)
	})

	SubTest(t, "NeitherUploadLengthNorUploadDeferLengthFail", func(t *testing.T, store *MockFullDataStore) {
		handler, _ := NewHandler(Config{
			DataStore: store,
		})

		(&httpTest{
			Method: "POST",
			URL:    "",
			ReqHeader: map[string]string{
				"Tus-Resumable": "1.0.0",
			},
			Code: http.StatusBadRequest,
		}).Run(handler, t)
	})

	SubTest(t, "InvalidUploadDeferLengthFail", func(t *testing.T, store *MockFullDataStore) {
		handler, _ := NewHandler(Config{
			DataStore: store,
		})

		(&httpTest{
			Method: "POST",
			URL:    "",
			ReqHeader: map[string]string{
				"Tus-Resumable":       "1.0.0",
				"Upload-Defer-Length": "bad",
			},
			Code: http.StatusBadRequest,
		}).Run(handler, t)
	})

	SubTest(t, "ForwardHeaders", func(t *testing.T, store *MockFullDataStore) {
		SubTest(t, "IgnoreXForwarded", func(t *testing.T, store *MockFullDataStore) {
			handler, _ := NewHandler(Config{
				DataStore: store,
				BasePath:  "/files/",
			})

			test := &httpTest{
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
			}

			store.EXPECT().NewUpload(FileInfo{
				Size:     300,
				MetaData: map[string]string{},
				OriginalRequest: OriginalRequest{
					Proto:   "HTTP/1.1",
					Host:    "tus.io",
					Headers: toHTTPHeader(test.ReqHeader),
				},
			}).Return("foo", nil)

			test.Run(handler, t)
		})

		SubTest(t, "RespectXForwarded", func(t *testing.T, store *MockFullDataStore) {
			handler, _ := NewHandler(Config{
				DataStore:               store,
				BasePath:                "/files/",
				RespectForwardedHeaders: true,
			})

			test := &httpTest{
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
			}

			store.EXPECT().NewUpload(FileInfo{
				Size:     300,
				MetaData: map[string]string{},
				OriginalRequest: OriginalRequest{
					Proto:   "HTTP/1.1",
					Host:    "tus.io",
					Headers: toHTTPHeader(test.ReqHeader),
				},
			}).Return("foo", nil)

			test.Run(handler, t)
		})

		SubTest(t, "RespectForwarded", func(t *testing.T, store *MockFullDataStore) {
			handler, _ := NewHandler(Config{
				DataStore:               store,
				BasePath:                "/files/",
				RespectForwardedHeaders: true,
			})

			test := &httpTest{
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
			}

			store.EXPECT().NewUpload(FileInfo{
				Size:     300,
				MetaData: map[string]string{},
				OriginalRequest: OriginalRequest{
					Proto:   "HTTP/1.1",
					Host:    "tus.io",
					Headers: toHTTPHeader(test.ReqHeader),
				},
			}).Return("foo", nil)

			test.Run(handler, t)
		})

		SubTest(t, "FilterForwardedProtocol", func(t *testing.T, store *MockFullDataStore) {
			handler, _ := NewHandler(Config{
				DataStore:               store,
				BasePath:                "/files/",
				RespectForwardedHeaders: true,
			})

			test := &httpTest{
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
			}

			store.EXPECT().NewUpload(FileInfo{
				Size:     300,
				MetaData: map[string]string{},
				OriginalRequest: OriginalRequest{
					Proto:   "HTTP/1.1",
					Host:    "tus.io",
					Headers: toHTTPHeader(test.ReqHeader),
				},
			}).Return("foo", nil)

			test.Run(handler, t)
		})
	})

	SubTest(t, "WithUpload", func(t *testing.T, store *MockFullDataStore) {
		SubTest(t, "Create", func(t *testing.T, store *MockFullDataStore) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			locker := NewMockLocker(ctrl)

			composer := NewStoreComposer()
			composer.UseCore(store)
			composer.UseLocker(locker)

			handler, _ := NewHandler(Config{
				StoreComposer: composer,
				BasePath:      "/files/",
			})

			test := &httpTest{
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
			}

			gomock.InOrder(
				store.EXPECT().NewUpload(FileInfo{
					Size: 300,
					MetaData: map[string]string{
						"foo": "hello",
						"bar": "world",
					},
					OriginalRequest: OriginalRequest{
						Proto:   "HTTP/1.1",
						Host:    "tus.io",
						Headers: toHTTPHeader(test.ReqHeader),
					},
				}).Return("foo", nil),
				locker.EXPECT().LockUpload("foo"),
				store.EXPECT().WriteChunk("foo", int64(0), NewReaderMatcher("hello")).Return(int64(5), nil),
				locker.EXPECT().UnlockUpload("foo"),
			)
			test.Run(handler, t)
		})

		SubTest(t, "CreateExceedingUploadSize", func(t *testing.T, store *MockFullDataStore) {
			handler, _ := NewHandler(Config{
				DataStore: store,
				BasePath:  "/files/",
			})

			test := &httpTest{
				Method: "POST",
				ReqHeader: map[string]string{
					"Tus-Resumable": "1.0.0",
					"Upload-Length": "300",
					"Content-Type":  "application/offset+octet-stream",
				},
				ReqBody: bytes.NewReader(make([]byte, 400)),
				Code:    http.StatusRequestEntityTooLarge,
			}

			store.EXPECT().NewUpload(FileInfo{
				Size:     300,
				MetaData: map[string]string{},
				OriginalRequest: OriginalRequest{
					Proto:   "HTTP/1.1",
					Host:    "tus.io",
					Headers: toHTTPHeader(test.ReqHeader),
				},
			}).Return("foo", nil)

			test.Run(handler, t)
		})

		SubTest(t, "IncorrectContentType", func(t *testing.T, store *MockFullDataStore) {
			handler, _ := NewHandler(Config{
				DataStore: store,
				BasePath:  "/files/",
			})

			test := &httpTest{
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
			}

			store.EXPECT().NewUpload(FileInfo{
				Size:     300,
				MetaData: map[string]string{},
				OriginalRequest: OriginalRequest{
					Proto:   "HTTP/1.1",
					Host:    "tus.io",
					Headers: toHTTPHeader(test.ReqHeader),
				},
			}).Return("foo", nil)

			test.Run(handler, t)
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
					"Upload-Concat": "final;http://tus.io/files/a http://tus.io/files/b",
				},
				ReqBody: strings.NewReader("hello"),
				Code:    http.StatusForbidden,
			}).Run(handler, t)
		})
	})
}
