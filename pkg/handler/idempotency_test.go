package handler_test

import (
	"net/http"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	. "github.com/tus/tusd/v2/pkg/handler"
	"github.com/tus/tusd/v2/pkg/memoryidempotencystore"
)

func TestIdempotency(t *testing.T) {
	SubTest(t, "ConcatRetryComplete", func(t *testing.T, store *MockFullDataStore, composer *StoreComposer) {
		a := assert.New(t)

		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		uploadA := NewMockFullUpload(ctrl)
		uploadB := NewMockFullUpload(ctrl)
		uploadC := NewMockFullUpload(ctrl)

		idempotencyStore := memoryidempotencystore.New()
		idempotencyStore.UseIn(composer)

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
				ID:             "concat-upload-1",
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
		})

		c := make(chan HookEvent, 1)
		handler.CompleteUploads = c

		// First request: creates the concat upload and stores the idempotency key.
		(&httpTest{
			Method: "POST",
			ReqHeader: map[string]string{
				"Tus-Resumable":  "1.0.0",
				"Upload-Concat":  "final;http://tus.io/files/a /files/b",
				"Idempotency-Key": "concat-key-1",
			},
			Code: http.StatusCreated,
		}).Run(handler, t)

		event := <-c
		a.Equal("concat-upload-1", event.Upload.ID)

		// Second request: same idempotency key, upload already completed.
		// Should return existing upload without calling NewUpload or ConcatUploads again.
		uploadCRetry := NewMockFullUpload(ctrl)

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
			store.EXPECT().GetUpload(gomock.Any(), "concat-upload-1").Return(uploadCRetry, nil),
			uploadCRetry.EXPECT().GetInfo(gomock.Any()).Return(FileInfo{
				ID:             "concat-upload-1",
				Size:           10,
				Offset:         10,
				IsFinal:        true,
				PartialUploads: []string{"a", "b"},
			}, nil),
		)

		(&httpTest{
			Method: "POST",
			ReqHeader: map[string]string{
				"Tus-Resumable":  "1.0.0",
				"Upload-Concat":  "final;http://tus.io/files/a /files/b",
				"Idempotency-Key": "concat-key-1",
			},
			Code: http.StatusCreated,
			ResHeader: map[string]string{
				"Location": "http://tus.io/files/concat-upload-1",
			},
		}).Run(handler, t)
	})

	SubTest(t, "ConcatRetryIncomplete", func(t *testing.T, store *MockFullDataStore, composer *StoreComposer) {
		a := assert.New(t)

		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		uploadA := NewMockFullUpload(ctrl)
		uploadB := NewMockFullUpload(ctrl)
		uploadC := NewMockFullUpload(ctrl)

		idempotencyStore := memoryidempotencystore.New()
		idempotencyStore.UseIn(composer)

		// Pre-seed the idempotency store with a mapping to simulate a previous
		// attempt that created the upload but didn't complete concat.
		idempotencyStore.StoreUploadID(nil, "concat-key-retry", "concat-upload-2")

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
			// Idempotency lookup finds the existing upload with offset 0
			store.EXPECT().GetUpload(gomock.Any(), "concat-upload-2").Return(uploadC, nil),
			uploadC.EXPECT().GetInfo(gomock.Any()).Return(FileInfo{
				ID:             "concat-upload-2",
				Size:           10,
				Offset:         0,
				IsFinal:        true,
				PartialUploads: []string{"a", "b"},
			}, nil),
			// Should retry the concatenation on the existing upload
			store.EXPECT().AsConcatableUpload(uploadC).Return(uploadC),
			uploadC.EXPECT().ConcatUploads(gomock.Any(), []Upload{uploadA, uploadB}).Return(nil),
		)

		handler, _ := NewHandler(Config{
			BasePath:              "files",
			StoreComposer:         composer,
			NotifyCompleteUploads: true,
		})

		c := make(chan HookEvent, 1)
		handler.CompleteUploads = c

		(&httpTest{
			Method: "POST",
			ReqHeader: map[string]string{
				"Tus-Resumable":  "1.0.0",
				"Upload-Concat":  "final;http://tus.io/files/a /files/b",
				"Idempotency-Key": "concat-key-retry",
			},
			Code: http.StatusCreated,
			ResHeader: map[string]string{
				"Location": "http://tus.io/files/concat-upload-2",
			},
		}).Run(handler, t)

		event := <-c
		a.Equal("concat-upload-2", event.Upload.ID)
	})

	SubTest(t, "ConcatRetryCorrupted", func(t *testing.T, store *MockFullDataStore, composer *StoreComposer) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		uploadA := NewMockFullUpload(ctrl)
		uploadB := NewMockFullUpload(ctrl)
		uploadC := NewMockFullUpload(ctrl)

		idempotencyStore := memoryidempotencystore.New()
		idempotencyStore.UseIn(composer)

		idempotencyStore.StoreUploadID(nil, "concat-key-corrupted", "concat-upload-3")

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
			store.EXPECT().GetUpload(gomock.Any(), "concat-upload-3").Return(uploadC, nil),
			uploadC.EXPECT().GetInfo(gomock.Any()).Return(FileInfo{
				ID:             "concat-upload-3",
				Size:           10,
				Offset:         3,
				IsFinal:        true,
				PartialUploads: []string{"a", "b"},
			}, nil),
		)

		handler, _ := NewHandler(Config{
			BasePath:      "files",
			StoreComposer: composer,
		})

		(&httpTest{
			Method: "POST",
			ReqHeader: map[string]string{
				"Tus-Resumable":  "1.0.0",
				"Upload-Concat":  "final;http://tus.io/files/a /files/b",
				"Idempotency-Key": "concat-key-corrupted",
			},
			Code:    http.StatusInternalServerError,
			ResBody: "ERR_CONCAT_CORRUPTED: previous concatenation attempt was partially completed and left the upload in an inconsistent state\n",
		}).Run(handler, t)
	})

	SubTest(t, "RegularUploadRetry", func(t *testing.T, store *MockFullDataStore, composer *StoreComposer) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		upload := NewMockFullUpload(ctrl)
		uploadRetry := NewMockFullUpload(ctrl)

		idempotencyStore := memoryidempotencystore.New()
		idempotencyStore.UseIn(composer)

		// First request: creates new upload
		gomock.InOrder(
			store.EXPECT().NewUpload(gomock.Any(), FileInfo{
				Size:     100,
				MetaData: make(map[string]string),
			}).Return(upload, nil),
			upload.EXPECT().GetInfo(gomock.Any()).Return(FileInfo{
				ID:       "upload-abc",
				Size:     100,
				MetaData: make(map[string]string),
			}, nil),
		)

		handler, _ := NewHandler(Config{
			BasePath:      "files",
			StoreComposer: composer,
		})

		(&httpTest{
			Method: "POST",
			ReqHeader: map[string]string{
				"Tus-Resumable":  "1.0.0",
				"Upload-Length":  "100",
				"Idempotency-Key": "upload-key-1",
			},
			Code: http.StatusCreated,
			ResHeader: map[string]string{
				"Location": "http://tus.io/files/upload-abc",
			},
		}).Run(handler, t)

		// Second request: same key, returns existing upload
		gomock.InOrder(
			store.EXPECT().GetUpload(gomock.Any(), "upload-abc").Return(uploadRetry, nil),
			uploadRetry.EXPECT().GetInfo(gomock.Any()).Return(FileInfo{
				ID:     "upload-abc",
				Size:   100,
				Offset: 50,
			}, nil),
		)

		(&httpTest{
			Method: "POST",
			ReqHeader: map[string]string{
				"Tus-Resumable":  "1.0.0",
				"Upload-Length":  "100",
				"Idempotency-Key": "upload-key-1",
			},
			Code: http.StatusCreated,
			ResHeader: map[string]string{
				"Location":      "http://tus.io/files/upload-abc",
				"Upload-Offset": "50",
			},
		}).Run(handler, t)
	})

	SubTest(t, "NoStoreConfigured", func(t *testing.T, store *MockFullDataStore, composer *StoreComposer) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		upload := NewMockFullUpload(ctrl)

		// No idempotency store on composer -- header should be ignored.
		gomock.InOrder(
			store.EXPECT().NewUpload(gomock.Any(), FileInfo{
				Size:     100,
				MetaData: make(map[string]string),
			}).Return(upload, nil),
			upload.EXPECT().GetInfo(gomock.Any()).Return(FileInfo{
				ID:       "upload-xyz",
				Size:     100,
				MetaData: make(map[string]string),
			}, nil),
		)

		handler, _ := NewHandler(Config{
			BasePath:      "files",
			StoreComposer: composer,
		})

		(&httpTest{
			Method: "POST",
			ReqHeader: map[string]string{
				"Tus-Resumable":  "1.0.0",
				"Upload-Length":  "100",
				"Idempotency-Key": "some-key",
			},
			Code: http.StatusCreated,
			ResHeader: map[string]string{
				"Location": "http://tus.io/files/upload-xyz",
			},
		}).Run(handler, t)
	})

	SubTest(t, "NoHeaderSent", func(t *testing.T, store *MockFullDataStore, composer *StoreComposer) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		upload := NewMockFullUpload(ctrl)

		idempotencyStore := memoryidempotencystore.New()
		idempotencyStore.UseIn(composer)

		// Store is configured but no Idempotency-Key header -- normal flow.
		gomock.InOrder(
			store.EXPECT().NewUpload(gomock.Any(), FileInfo{
				Size:     100,
				MetaData: make(map[string]string),
			}).Return(upload, nil),
			upload.EXPECT().GetInfo(gomock.Any()).Return(FileInfo{
				ID:       "upload-no-key",
				Size:     100,
				MetaData: make(map[string]string),
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
				"Upload-Length": "100",
			},
			Code: http.StatusCreated,
			ResHeader: map[string]string{
				"Location": "http://tus.io/files/upload-no-key",
			},
		}).Run(handler, t)
	})

	SubTest(t, "DeletedUploadFallsThrough", func(t *testing.T, store *MockFullDataStore, composer *StoreComposer) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		upload := NewMockFullUpload(ctrl)

		idempotencyStore := memoryidempotencystore.New()
		idempotencyStore.UseIn(composer)

		// Pre-seed with a mapping to a since-deleted upload
		idempotencyStore.StoreUploadID(nil, "stale-key", "deleted-upload")

		gomock.InOrder(
			// Idempotency lookup finds the key but the upload is gone
			store.EXPECT().GetUpload(gomock.Any(), "deleted-upload").Return(nil, ErrNotFound),
			// Falls through to create a new upload
			store.EXPECT().NewUpload(gomock.Any(), FileInfo{
				Size:     100,
				MetaData: make(map[string]string),
			}).Return(upload, nil),
			upload.EXPECT().GetInfo(gomock.Any()).Return(FileInfo{
				ID:       "new-upload-after-delete",
				Size:     100,
				MetaData: make(map[string]string),
			}, nil),
		)

		handler, _ := NewHandler(Config{
			BasePath:      "files",
			StoreComposer: composer,
		})

		(&httpTest{
			Method: "POST",
			ReqHeader: map[string]string{
				"Tus-Resumable":  "1.0.0",
				"Upload-Length":  "100",
				"Idempotency-Key": "stale-key",
			},
			Code: http.StatusCreated,
			ResHeader: map[string]string{
				"Location": "http://tus.io/files/new-upload-after-delete",
			},
		}).Run(handler, t)
	})
}
