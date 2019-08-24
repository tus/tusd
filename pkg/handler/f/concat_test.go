package handler_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	. "github.com/tus/tusd/pkg/handler"
)

func TestConcat(t *testing.T) {
	SubTest(t, "ExtensionDiscovery", func(t *testing.T, store *MockFullDataStore, composer *StoreComposer) {
		composer = NewStoreComposer()
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
			store.EXPECT().NewUpload(FileInfo{
				Size:           300,
				IsPartial:      true,
				IsFinal:        false,
				PartialUploads: nil,
				MetaData:       make(map[string]string),
			}).Return("foo", nil)

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
			store.EXPECT().GetInfo("foo").Return(FileInfo{
				IsPartial: true,
			}, nil)

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

			gomock.InOrder(
				store.EXPECT().GetInfo("a").Return(FileInfo{
					IsPartial: true,
					Size:      5,
					Offset:    5,
				}, nil),
				store.EXPECT().GetInfo("b").Return(FileInfo{
					IsPartial: true,
					Size:      5,
					Offset:    5,
				}, nil),
				store.EXPECT().NewUpload(FileInfo{
					Size:           10,
					IsPartial:      false,
					IsFinal:        true,
					PartialUploads: []string{"a", "b"},
					MetaData:       make(map[string]string),
				}).Return("foo", nil),
				store.EXPECT().ConcatUploads("foo", []string{"a", "b"}).Return(nil),
			)

			handler, _ := NewHandler(Config{
				BasePath:              "files",
				StoreComposer:         composer,
				NotifyCompleteUploads: true,
			})

			c := make(chan FileInfo, 1)
			handler.CompleteUploads = c

			(&httpTest{
				Method: "POST",
				ReqHeader: map[string]string{
					"Tus-Resumable": "1.0.0",
					// A space between `final;` and the first URL should be allowed due to
					// compatibility reasons, even if the specification does not define
					// it. Therefore this character is included in this test case.
					"Upload-Concat": "final; http://tus.io/files/a /files/b/",
				},
				Code: http.StatusCreated,
			}).Run(handler, t)

			info := <-c
			a.Equal("foo", info.ID)
			a.EqualValues(10, info.Size)
			a.EqualValues(10, info.Offset)
			a.False(info.IsPartial)
			a.True(info.IsFinal)
			a.Equal([]string{"a", "b"}, info.PartialUploads)
		})

		SubTest(t, "Status", func(t *testing.T, store *MockFullDataStore, composer *StoreComposer) {
			store.EXPECT().GetInfo("foo").Return(FileInfo{
				IsFinal:        true,
				PartialUploads: []string{"a", "b"},
				Size:           10,
				Offset:         10,
			}, nil)

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
			// This upload is still unfinished (mismatching offset and size) and
			// will therefore cause the POST request to fail.
			store.EXPECT().GetInfo("c").Return(FileInfo{
				IsPartial: true,
				Size:      5,
				Offset:    3,
			}, nil)

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
			store.EXPECT().GetInfo("huge").Return(FileInfo{
				Size:   1000,
				Offset: 1000,
			}, nil)

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
			store.EXPECT().GetInfo("foo").Return(FileInfo{
				Size:    10,
				Offset:  0,
				IsFinal: true,
			}, nil)

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
	})
}
