package handler_test

import (
	"bytes"
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	. "github.com/tus/tusd/pkg/handler"
)

func TestPost(t *testing.T) {
	SubTest(t, "Create", func(t *testing.T, store *MockFullDataStore, composer *StoreComposer) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		upload := NewMockFullUpload(ctrl)

		gomock.InOrder(
			store.EXPECT().NewUpload(context.Background(), FileInfo{
				Size: 300,
				MetaData: map[string]string{
					"foo": "hello",
					"bar": "world",
				},
			}).Return(upload, nil),
			upload.EXPECT().GetInfo(context.Background()).Return(FileInfo{
				ID:   "foo",
				Size: 300,
				MetaData: map[string]string{
					"foo": "hello",
					"bar": "world",
				},
			}, nil),
		)

		handler, _ := NewHandler(Config{
			StoreComposer:        composer,
			BasePath:             "https://buy.art/files/",
			NotifyCreatedUploads: true,
		})

		c := make(chan HookEvent, 1)
		handler.CreatedUploads = c

		(&httpTest{
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
		}).Run(handler, t)

		event := <-c
		info := event.Upload

		a := assert.New(t)
		a.Equal("foo", info.ID)
		a.Equal(int64(300), info.Size)
	})

	SubTest(t, "CreateEmptyUpload", func(t *testing.T, store *MockFullDataStore, composer *StoreComposer) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		upload := NewMockFullUpload(ctrl)

		gomock.InOrder(
			store.EXPECT().NewUpload(context.Background(), FileInfo{
				Size:     0,
				MetaData: map[string]string{},
			}).Return(upload, nil),
			upload.EXPECT().GetInfo(context.Background()).Return(FileInfo{
				ID:       "foo",
				Size:     0,
				MetaData: map[string]string{},
			}, nil),
			upload.EXPECT().FinishUpload(context.Background()).Return(nil),
		)

		handler, _ := NewHandler(Config{
			StoreComposer:         composer,
			BasePath:              "https://buy.art/files/",
			NotifyCompleteUploads: true,
		})

		handler.CompleteUploads = make(chan HookEvent, 1)

		(&httpTest{
			Method: "POST",
			ReqHeader: map[string]string{
				"Tus-Resumable": "1.0.0",
				"Upload-Length": "0",
			},
			Code: http.StatusCreated,
			ResHeader: map[string]string{
				"Location": "https://buy.art/files/foo",
			},
		}).Run(handler, t)

		event := <-handler.CompleteUploads
		info := event.Upload

		a := assert.New(t)
		a.Equal("foo", info.ID)
		a.Equal(int64(0), info.Size)
		a.Equal(int64(0), info.Offset)

		req := event.HTTPRequest
		a.Equal("POST", req.Method)
		a.Equal("", req.URI)
	})

	SubTest(t, "CreateExceedingMaxSizeFail", func(t *testing.T, store *MockFullDataStore, composer *StoreComposer) {
		handler, _ := NewHandler(Config{
			MaxSize:       400,
			StoreComposer: composer,
			BasePath:      "/files/",
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

	SubTest(t, "InvalidUploadLengthFail", func(t *testing.T, store *MockFullDataStore, composer *StoreComposer) {
		handler, _ := NewHandler(Config{
			StoreComposer: composer,
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

	SubTest(t, "UploadLengthAndUploadDeferLengthFail", func(t *testing.T, store *MockFullDataStore, composer *StoreComposer) {
		handler, _ := NewHandler(Config{
			StoreComposer: composer,
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

	SubTest(t, "NeitherUploadLengthNorUploadDeferLengthFail", func(t *testing.T, store *MockFullDataStore, composer *StoreComposer) {
		handler, _ := NewHandler(Config{
			StoreComposer: composer,
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

	SubTest(t, "InvalidUploadDeferLengthFail", func(t *testing.T, store *MockFullDataStore, composer *StoreComposer) {
		handler, _ := NewHandler(Config{
			StoreComposer: composer,
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

	SubTest(t, "ForwardHeaders", func(t *testing.T, store *MockFullDataStore, composer *StoreComposer) {
		SubTest(t, "IgnoreXForwarded", func(t *testing.T, store *MockFullDataStore, composer *StoreComposer) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			upload := NewMockFullUpload(ctrl)

			gomock.InOrder(
				store.EXPECT().NewUpload(context.Background(), FileInfo{
					Size:     300,
					MetaData: map[string]string{},
				}).Return(upload, nil),
				upload.EXPECT().GetInfo(context.Background()).Return(FileInfo{
					ID:       "foo",
					Size:     300,
					MetaData: map[string]string{},
				}, nil),
			)

			handler, _ := NewHandler(Config{
				StoreComposer: composer,
				BasePath:      "/files/",
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

		SubTest(t, "RespectXForwarded", func(t *testing.T, store *MockFullDataStore, composer *StoreComposer) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			upload := NewMockFullUpload(ctrl)

			gomock.InOrder(
				store.EXPECT().NewUpload(context.Background(), FileInfo{
					Size:     300,
					MetaData: map[string]string{},
				}).Return(upload, nil),
				upload.EXPECT().GetInfo(context.Background()).Return(FileInfo{
					ID:       "foo",
					Size:     300,
					MetaData: map[string]string{},
				}, nil),
			)

			handler, _ := NewHandler(Config{
				StoreComposer:           composer,
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

		SubTest(t, "RespectForwarded", func(t *testing.T, store *MockFullDataStore, composer *StoreComposer) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			upload := NewMockFullUpload(ctrl)

			gomock.InOrder(
				store.EXPECT().NewUpload(context.Background(), FileInfo{
					Size:     300,
					MetaData: map[string]string{},
				}).Return(upload, nil),
				upload.EXPECT().GetInfo(context.Background()).Return(FileInfo{
					ID:       "foo",
					Size:     300,
					MetaData: map[string]string{},
				}, nil),
			)

			handler, _ := NewHandler(Config{
				StoreComposer:           composer,
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

		SubTest(t, "FilterForwardedProtocol", func(t *testing.T, store *MockFullDataStore, composer *StoreComposer) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			upload := NewMockFullUpload(ctrl)

			gomock.InOrder(
				store.EXPECT().NewUpload(context.Background(), FileInfo{
					Size:     300,
					MetaData: map[string]string{},
				}).Return(upload, nil),
				upload.EXPECT().GetInfo(context.Background()).Return(FileInfo{
					ID:       "foo",
					Size:     300,
					MetaData: map[string]string{},
				}, nil),
			)

			handler, _ := NewHandler(Config{
				StoreComposer:           composer,
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

	SubTest(t, "WithUpload", func(t *testing.T, store *MockFullDataStore, composer *StoreComposer) {
		SubTest(t, "Create", func(t *testing.T, store *MockFullDataStore, composer *StoreComposer) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			locker := NewMockFullLocker(ctrl)
			lock := NewMockFullLock(ctrl)
			upload := NewMockFullUpload(ctrl)

			gomock.InOrder(
				store.EXPECT().NewUpload(context.Background(), FileInfo{
					Size: 300,
					MetaData: map[string]string{
						"foo": "hello",
						"bar": "world",
					},
				}).Return(upload, nil),
				upload.EXPECT().GetInfo(context.Background()).Return(FileInfo{
					ID:   "foo",
					Size: 300,
					MetaData: map[string]string{
						"foo": "hello",
						"bar": "world",
					},
				}, nil),
				locker.EXPECT().NewLock("foo").Return(lock, nil),
				lock.EXPECT().Lock().Return(nil),
				upload.EXPECT().WriteChunk(context.Background(), int64(0), NewReaderMatcher("hello")).Return(int64(5), nil),
				lock.EXPECT().Unlock().Return(nil),
			)

			composer = NewStoreComposer()
			composer.UseCore(store)
			composer.UseLocker(locker)

			handler, _ := NewHandler(Config{
				StoreComposer: composer,
				BasePath:      "/files/",
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

		SubTest(t, "CreateExceedingUploadSize", func(t *testing.T, store *MockFullDataStore, composer *StoreComposer) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			upload := NewMockFullUpload(ctrl)

			gomock.InOrder(
				store.EXPECT().NewUpload(context.Background(), FileInfo{
					Size:     300,
					MetaData: map[string]string{},
				}).Return(upload, nil),
				upload.EXPECT().GetInfo(context.Background()).Return(FileInfo{
					ID:       "foo",
					Size:     300,
					MetaData: map[string]string{},
				}, nil),
			)

			handler, _ := NewHandler(Config{
				StoreComposer: composer,
				BasePath:      "/files/",
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

		SubTest(t, "IncorrectContentType", func(t *testing.T, store *MockFullDataStore, composer *StoreComposer) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			upload := NewMockFullUpload(ctrl)

			gomock.InOrder(
				store.EXPECT().NewUpload(context.Background(), FileInfo{
					Size:     300,
					MetaData: map[string]string{},
				}).Return(upload, nil),
				upload.EXPECT().GetInfo(context.Background()).Return(FileInfo{
					ID:       "foo",
					Size:     300,
					MetaData: map[string]string{},
				}, nil),
			)

			handler, _ := NewHandler(Config{
				StoreComposer: composer,
				BasePath:      "/files/",
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

		SubTest(t, "UploadToFinalUpload", func(t *testing.T, store *MockFullDataStore, composer *StoreComposer) {
			handler, _ := NewHandler(Config{
				StoreComposer: composer,
				BasePath:      "/files/",
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
