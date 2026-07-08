package handler_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	. "github.com/tus/tusd/v2/pkg/handler"
	"github.com/tus/tusd/v2/pkg/memoryidempotencystore"
)

func TestBackgroundConcatenation(t *testing.T) {
	// A completed background concatenation responds 201 immediately and emits
	// the finish event asynchronously once the concat finishes.
	SubTest(t, "Complete", func(t *testing.T, store *MockFullDataStore, composer *StoreComposer) {
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
			BasePath:                      "files",
			StoreComposer:                 composer,
			NotifyCompleteUploads:         true,
			EnableBackgroundConcatenation: true,
		})

		c := make(chan HookEvent, 1)
		handler.CompleteUploads = c

		(&httpTest{
			Method: "POST",
			ReqHeader: map[string]string{
				"Tus-Resumable": "1.0.0",
				"Upload-Concat": "final;http://tus.io/files/a /files/b",
			},
			Code: http.StatusCreated,
			ResHeader: map[string]string{
				"Location": "http://tus.io/files/foo",
			},
		}).Run(handler, t)

		// The finish event is emitted by the background goroutine once concat completes.
		select {
		case event := <-c:
			a.Equal("foo", event.Upload.ID)
			a.EqualValues(10, event.Upload.Offset)
		case <-time.After(5 * time.Second):
			t.Fatal("timed out waiting for background concatenation to finish")
		}
	})

	// A retry that arrives while a background concatenation is still running must
	// return 201 without starting a second concat or reporting a corrupted state,
	// even though the upload's offset is between 0 and its size.
	SubTest(t, "RetryWhileInProgress", func(t *testing.T, store *MockFullDataStore, composer *StoreComposer) {
		a := assert.New(t)

		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		uploadA := NewMockFullUpload(ctrl)
		uploadB := NewMockFullUpload(ctrl)
		uploadC := NewMockFullUpload(ctrl)
		uploadCRetry := NewMockFullUpload(ctrl)

		idempotencyStore := memoryidempotencystore.New()
		idempotencyStore.UseIn(composer)

		// sizeOfUploads runs on every request, so the partials are fetched twice.
		store.EXPECT().GetUpload(gomock.Any(), "a").Return(uploadA, nil).Times(2)
		uploadA.EXPECT().GetInfo(gomock.Any()).Return(FileInfo{
			IsPartial: true,
			Size:      5,
			Offset:    5,
		}, nil).Times(2)
		store.EXPECT().GetUpload(gomock.Any(), "b").Return(uploadB, nil).Times(2)
		uploadB.EXPECT().GetInfo(gomock.Any()).Return(FileInfo{
			IsPartial: true,
			Size:      5,
			Offset:    5,
		}, nil).Times(2)

		// First request creates the upload.
		store.EXPECT().NewUpload(gomock.Any(), FileInfo{
			Size:           10,
			IsPartial:      false,
			IsFinal:        true,
			PartialUploads: []string{"a", "b"},
			MetaData:       make(map[string]string),
		}).Return(uploadC, nil)
		uploadC.EXPECT().GetInfo(gomock.Any()).Return(FileInfo{
			ID:             "foo",
			Size:           10,
			Offset:         0,
			IsPartial:      false,
			IsFinal:        true,
			PartialUploads: []string{"a", "b"},
			MetaData:       make(map[string]string),
		}, nil)

		// The background concat blocks until the test releases it, simulating a
		// slow, in-progress concatenation.
		concatStarted := make(chan struct{})
		concatRelease := make(chan struct{})
		store.EXPECT().AsConcatableUpload(uploadC).Return(uploadC)
		uploadC.EXPECT().ConcatUploads(gomock.Any(), []Upload{uploadA, uploadB}).DoAndReturn(
			func(ctx context.Context, partials []Upload) error {
				close(concatStarted)
				<-concatRelease
				return nil
			})

		// The retry's idempotency lookup finds the existing upload mid-concat with a
		// partial offset. The in-progress tracker must short-circuit this.
		store.EXPECT().GetUpload(gomock.Any(), "foo").Return(uploadCRetry, nil)
		uploadCRetry.EXPECT().GetInfo(gomock.Any()).Return(FileInfo{
			ID:             "foo",
			Size:           10,
			Offset:         3,
			IsFinal:        true,
			PartialUploads: []string{"a", "b"},
		}, nil)

		handler, _ := NewHandler(Config{
			BasePath:                      "files",
			StoreComposer:                 composer,
			NotifyCompleteUploads:         true,
			EnableBackgroundConcatenation: true,
		})

		c := make(chan HookEvent, 1)
		handler.CompleteUploads = c

		// First request: starts the background concat (which blocks).
		(&httpTest{
			Method: "POST",
			ReqHeader: map[string]string{
				"Tus-Resumable":   "1.0.0",
				"Upload-Concat":   "final;http://tus.io/files/a /files/b",
				"Idempotency-Key": "bg-key",
			},
			Code: http.StatusCreated,
			ResHeader: map[string]string{
				"Location": "http://tus.io/files/foo",
			},
		}).Run(handler, t)

		// Ensure the background concat has actually begun before retrying.
		select {
		case <-concatStarted:
		case <-time.After(5 * time.Second):
			t.Fatal("timed out waiting for background concatenation to start")
		}

		// Retry while concat is in progress: must return 201 and not call ConcatUploads again.
		(&httpTest{
			Method: "POST",
			ReqHeader: map[string]string{
				"Tus-Resumable":   "1.0.0",
				"Upload-Concat":   "final;http://tus.io/files/a /files/b",
				"Idempotency-Key": "bg-key",
			},
			Code: http.StatusCreated,
			ResHeader: map[string]string{
				"Location": "http://tus.io/files/foo",
			},
		}).Run(handler, t)

		// Let the original concat finish and confirm exactly one finish event fires.
		close(concatRelease)

		select {
		case event := <-c:
			a.Equal("foo", event.Upload.ID)
		case <-time.After(5 * time.Second):
			t.Fatal("timed out waiting for background concatenation to finish")
		}
	})
}
