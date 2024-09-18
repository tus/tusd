package handler_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	. "github.com/tus/tusd/v2/pkg/handler"
)

func TestConcat(t *testing.T) {
	SubTest(t, "ExtensionDiscovery", func(t *testing.T, store *MockFullDataStore, _ *StoreComposer) {
		composer := NewStoreComposer()
		composer.UseCore(store)
		composer.UseConcater(store)

		handler, _ := NewHandler(Config{
			StoreComposer: composer,
		})

		(&httpTest{
			Method: "OPTIONS",
			Code:   http.StatusOK,
			ResHeader: map[string]string{
				"Tus-Extension": "creation,creation-with-upload,concatenation",
			},
		}).Run(handler, t)
	})

	SubTest(t, "Partial", func(t *testing.T, store *MockFullDataStore, composer *StoreComposer) {
		SubTest(t, "Create", func(t *testing.T, store *MockFullDataStore, composer *StoreComposer) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			upload := NewMockFullUpload(ctrl)

			gomock.InOrder(
				store.EXPECT().NewUpload(gomock.Any(), FileInfo{
					Size:           300,
					IsPartial:      true,
					IsFinal:        false,
					PartialUploads: nil,
					MetaData:       make(map[string]string),
				}).Return(upload, nil),
				upload.EXPECT().GetInfo(gomock.Any()).Return(FileInfo{
					ID:             "foo",
					Size:           300,
					IsPartial:      true,
					IsFinal:        false,
					PartialUploads: nil,
					MetaData:       make(map[string]string),
				}, nil),
			)

			handler, _ := NewHandler(Config{
				BasePath:      "files",
				StoreComposer: composer,
			})

			(&httpTest{
				Method: "POST",
				ReqHeader: map[string]string{
					"Tus-Resumable": "1.0.0",
					"Upload-Length": "300",
					"Upload-Concat": "partial",
				},
				Code: http.StatusCreated,
			}).Run(handler, t)
		})

		SubTest(t, "Status", func(t *testing.T, store *MockFullDataStore, composer *StoreComposer) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			upload := NewMockFullUpload(ctrl)

			gomock.InOrder(
				store.EXPECT().GetUpload(gomock.Any(), "foo").Return(upload, nil),
				upload.EXPECT().GetInfo(gomock.Any()).Return(FileInfo{
					ID:        "foo",
					IsPartial: true,
				}, nil),
			)

			handler, _ := NewHandler(Config{
				BasePath:      "files",
				StoreComposer: composer,
			})

			(&httpTest{
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
		})
	})

	SubTest(t, "Final", func(t *testing.T, store *MockFullDataStore, composer *StoreComposer) {
		SubTest(t, "Create", func(t *testing.T, store *MockFullDataStore, composer *StoreComposer) {
			a := assert.New(t)

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			uploadA := NewMockFullUpload(ctrl)
			uploadB := NewMockFullUpload(ctrl)
			uploadC := NewMockFullUpload(ctrl)

			gomock.InOrder(
				store.EXPECT().GetUpload(gomock.Any(), "a").Return(uploadA, nil),
				uploadA.EXPECT().GetInfo(gomock.Any()).Return(FileInfo{
					IsPartial: true,
					Size:      5,
					Offset:    5,
				}, nil),
				store.EXPECT().GetUpload(gomock.Any(), "b").Return(uploadB, nil),
				uploadB.EXPECT().GetInfo(gomock.Any()).Return(FileInfo{
					IsPartial: true,
					Size:      5,
					Offset:    5,
				}, nil),
				store.EXPECT().NewUpload(gomock.Any(), FileInfo{
					Size:           10,
					IsPartial:      false,
					IsFinal:        true,
					PartialUploads: []string{"a", "b"},
					MetaData:       make(map[string]string),
				}).Return(uploadC, nil),
				uploadC.EXPECT().GetInfo(gomock.Any()).Return(FileInfo{
					ID:             "foo",
					Size:           10,
					IsPartial:      false,
					IsFinal:        true,
					PartialUploads: []string{"a", "b"},
					MetaData:       make(map[string]string),
				}, nil),
				store.EXPECT().AsConcatableUpload(uploadC).Return(uploadC),
				uploadC.EXPECT().ConcatUploads(gomock.Any(), []Upload{uploadA, uploadB}).Return(nil),
			)

			handler, _ := NewHandler(Config{
				BasePath:              "files",
				StoreComposer:         composer,
				NotifyCompleteUploads: true,
				PreFinishResponseCallback: func(hook HookEvent) (HTTPResponse, error) {
					a.Equal("foo", hook.Upload.ID)
					return HTTPResponse{
						Header: HTTPHeader{
							"X-Custom-Resp-Header": "hello",
						},
					}, nil
				},
			})

			c := make(chan HookEvent, 1)
			handler.CompleteUploads = c

			(&httpTest{
				Method: "POST",
				ReqHeader: map[string]string{
					"Tus-Resumable": "1.0.0",
					// A space between `final;` and the first URL should be allowed due to
					// compatibility reasons, even if the specification does not define
					// it. Therefore this character is included in this test case.
					"Upload-Concat":       "final; http://tus.io/files/a /files/b/",
					"X-Custom-Req-Header": "tada",
				},
				Code: http.StatusCreated,
				ResHeader: map[string]string{
					"X-Custom-Resp-Header": "hello",
				},
			}).Run(handler, t)

			event := <-c
			info := event.Upload
			a.Equal("foo", info.ID)
			a.EqualValues(10, info.Size)
			a.EqualValues(10, info.Offset)
			a.False(info.IsPartial)
			a.True(info.IsFinal)
			a.Equal([]string{"a", "b"}, info.PartialUploads)

			req := event.HTTPRequest
			a.Equal("POST", req.Method)
			a.Equal("", req.URI)
			a.Equal("tada", req.Header.Get("X-Custom-Req-Header"))
		})

		SubTest(t, "Status", func(t *testing.T, store *MockFullDataStore, composer *StoreComposer) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			upload := NewMockFullUpload(ctrl)

			gomock.InOrder(
				store.EXPECT().GetUpload(gomock.Any(), "foo").Return(upload, nil),
				upload.EXPECT().GetInfo(gomock.Any()).Return(FileInfo{
					ID:             "foo",
					IsFinal:        true,
					PartialUploads: []string{"a", "b"},
					Size:           10,
					Offset:         10,
				}, nil),
			)

			handler, _ := NewHandler(Config{
				BasePath:      "files",
				StoreComposer: composer,
			})

			(&httpTest{
				Method: "HEAD",
				URL:    "foo",
				ReqHeader: map[string]string{
					"Tus-Resumable": "1.0.0",
				},
				Code: http.StatusOK,
				ResHeader: map[string]string{
					"Upload-Concat": "final;http://tus.io/files/a http://tus.io/files/b",
					"Upload-Length": "10",
					"Upload-Offset": "10",
				},
			}).Run(handler, t)
		})

		SubTest(t, "CreateWithUnfinishedFail", func(t *testing.T, store *MockFullDataStore, composer *StoreComposer) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			upload := NewMockFullUpload(ctrl)

			// This upload is still unfinished (mismatching offset and size) and
			// will therefore cause the POST request to fail.
			gomock.InOrder(
				store.EXPECT().GetUpload(gomock.Any(), "c").Return(upload, nil),
				upload.EXPECT().GetInfo(gomock.Any()).Return(FileInfo{
					ID:        "c",
					IsPartial: true,
					Size:      5,
					Offset:    3,
				}, nil),
			)

			handler, _ := NewHandler(Config{
				BasePath:      "files",
				StoreComposer: composer,
			})

			(&httpTest{
				Method: "POST",
				ReqHeader: map[string]string{
					"Tus-Resumable": "1.0.0",
					"Upload-Concat": "final;http://tus.io/files/c",
				},
				Code: http.StatusBadRequest,
			}).Run(handler, t)
		})

		SubTest(t, "CreateExceedingMaxSizeFail", func(t *testing.T, store *MockFullDataStore, composer *StoreComposer) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			upload := NewMockFullUpload(ctrl)

			gomock.InOrder(
				store.EXPECT().GetUpload(gomock.Any(), "huge").Return(upload, nil),
				upload.EXPECT().GetInfo(gomock.Any()).Return(FileInfo{
					ID:     "huge",
					Size:   1000,
					Offset: 1000,
				}, nil),
			)

			handler, _ := NewHandler(Config{
				MaxSize:       100,
				BasePath:      "files",
				StoreComposer: composer,
			})

			(&httpTest{
				Method: "POST",
				ReqHeader: map[string]string{
					"Tus-Resumable": "1.0.0",
					"Upload-Concat": "final;/files/huge",
				},
				Code: http.StatusRequestEntityTooLarge,
			}).Run(handler, t)
		})

		SubTest(t, "UploadToFinalFail", func(t *testing.T, store *MockFullDataStore, composer *StoreComposer) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			upload := NewMockFullUpload(ctrl)

			gomock.InOrder(
				store.EXPECT().GetUpload(gomock.Any(), "foo").Return(upload, nil),
				upload.EXPECT().GetInfo(gomock.Any()).Return(FileInfo{
					ID:      "foo",
					Size:    10,
					Offset:  0,
					IsFinal: true,
				}, nil),
			)

			handler, _ := NewHandler(Config{
				StoreComposer: composer,
			})

			(&httpTest{
				Method: "PATCH",
				URL:    "foo",
				ReqHeader: map[string]string{
					"Tus-Resumable": "1.0.0",
					"Content-Type":  "application/offset+octet-stream",
					"Upload-Offset": "5",
				},
				ReqBody: strings.NewReader("hello"),
				Code:    http.StatusForbidden,
			}).Run(handler, t)
		})

		SubTest(t, "InvalidConcatHeaderFail", func(t *testing.T, store *MockFullDataStore, composer *StoreComposer) {
			handler, _ := NewHandler(Config{
				StoreComposer: composer,
			})

			(&httpTest{
				Method: "POST",
				URL:    "",
				ReqHeader: map[string]string{
					"Tus-Resumable": "1.0.0",
					"Upload-Concat": "final;",
				},
				Code: http.StatusBadRequest,
			}).Run(handler, t)
		})

		// Test that we can concatenate uploads, whose IDs contain slashes.
		SubTest(t, "UploadIDsWithSlashes", func(t *testing.T, store *MockFullDataStore, composer *StoreComposer) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			uploadA := NewMockFullUpload(ctrl)
			uploadB := NewMockFullUpload(ctrl)
			uploadC := NewMockFullUpload(ctrl)

			gomock.InOrder(
				store.EXPECT().GetUpload(gomock.Any(), "aaa/123").Return(uploadA, nil),
				uploadA.EXPECT().GetInfo(gomock.Any()).Return(FileInfo{
					IsPartial: true,
					Size:      5,
					Offset:    5,
				}, nil),
				store.EXPECT().GetUpload(gomock.Any(), "bbb/123").Return(uploadB, nil),
				uploadB.EXPECT().GetInfo(gomock.Any()).Return(FileInfo{
					IsPartial: true,
					Size:      5,
					Offset:    5,
				}, nil),
				store.EXPECT().NewUpload(gomock.Any(), FileInfo{
					Size:           10,
					IsPartial:      false,
					IsFinal:        true,
					PartialUploads: []string{"aaa/123", "bbb/123"},
					MetaData:       make(map[string]string),
				}).Return(uploadC, nil),
				uploadC.EXPECT().GetInfo(gomock.Any()).Return(FileInfo{
					ID:             "foo",
					Size:           10,
					IsPartial:      false,
					IsFinal:        true,
					PartialUploads: []string{"aaa/123", "bbb/123"},
					MetaData:       make(map[string]string),
				}, nil),
				store.EXPECT().AsConcatableUpload(uploadC).Return(uploadC),
				uploadC.EXPECT().ConcatUploads(gomock.Any(), []Upload{uploadA, uploadB}).Return(nil),
			)

			handler, _ := NewHandler(Config{
				BasePath:      "files",
				StoreComposer: composer,
			})

			(&httpTest{
				Method: "POST",
				ReqHeader: map[string]string{
					"Tus-Resumable": "1.0.0",
					"Upload-Concat": "final; http://tus.io/files/aaa/123 /files/bbb/123",
				},
				Code: http.StatusCreated,
			}).Run(handler, t)
		})
	})
}
