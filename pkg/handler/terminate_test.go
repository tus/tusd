package handler_test

import (
	"net/http"
	"testing"

	. "github.com/fetlife/tusd/v2/pkg/handler"
	"github.com/golang/mock/gomock"

	"github.com/stretchr/testify/assert"
)

func TestTerminate(t *testing.T) {
	SubTest(t, "ExtensionDiscovery", func(t *testing.T, store *MockFullDataStore, _ *StoreComposer) {
		composer := NewStoreComposer()
		composer.UseCore(store)
		composer.UseTerminater(store)

		handler, _ := NewHandler(Config{
			StoreComposer: composer,
		})

		(&httpTest{
			Method: "OPTIONS",
			Code:   http.StatusOK,
			ResHeader: map[string]string{
				"Tus-Extension": "creation,creation-with-upload,termination",
			},
		}).Run(handler, t)
	})

	SubTest(t, "Termination", func(t *testing.T, store *MockFullDataStore, _ *StoreComposer) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		locker := NewMockFullLocker(ctrl)
		lock := NewMockFullLock(ctrl)
		upload := NewMockFullUpload(ctrl)

		gomock.InOrder(
			locker.EXPECT().NewLock("foo").Return(lock, nil),
			lock.EXPECT().Lock(gomock.Any(), gomock.Any()).Return(nil),
			store.EXPECT().GetUpload(gomock.Any(), "foo").Return(upload, nil),
			upload.EXPECT().GetInfo(gomock.Any()).Return(FileInfo{
				ID:   "foo",
				Size: 10,
			}, nil),
			store.EXPECT().AsTerminatableUpload(upload).Return(upload),
			upload.EXPECT().Terminate(gomock.Any()).Return(nil),
			lock.EXPECT().Unlock().Return(nil),
		)

		composer := NewStoreComposer()
		composer.UseCore(store)
		composer.UseTerminater(store)
		composer.UseLocker(locker)

		preTerminateCalled := false
		handler, _ := NewHandler(Config{
			StoreComposer:           composer,
			NotifyTerminatedUploads: true,
			PreUploadTerminateCallback: func(hook HookEvent) (HTTPResponse, error) {
				preTerminateCalled = true
				return HTTPResponse{}, nil
			},
		})

		c := make(chan HookEvent, 1)
		handler.TerminatedUploads = c

		(&httpTest{
			Method: "DELETE",
			URL:    "foo",
			ReqHeader: map[string]string{
				"Tus-Resumable": "1.0.0",
			},
			Code: http.StatusNoContent,
		}).Run(handler, t)

		event := <-c
		info := event.Upload

		a := assert.New(t)
		a.Equal("foo", info.ID)
		a.Equal(int64(10), info.Size)

		req := event.HTTPRequest
		a.Equal("DELETE", req.Method)
		a.Equal("foo", req.URI)

		a.True(preTerminateCalled)
	})

	SubTest(t, "RejectTermination", func(t *testing.T, store *MockFullDataStore, _ *StoreComposer) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		locker := NewMockFullLocker(ctrl)
		lock := NewMockFullLock(ctrl)
		upload := NewMockFullUpload(ctrl)

		gomock.InOrder(
			locker.EXPECT().NewLock("foo").Return(lock, nil),
			lock.EXPECT().Lock(gomock.Any(), gomock.Any()).Return(nil),
			store.EXPECT().GetUpload(gomock.Any(), "foo").Return(upload, nil),
			upload.EXPECT().GetInfo(gomock.Any()).Return(FileInfo{
				ID:   "foo",
				Size: 10,
			}, nil),
			lock.EXPECT().Unlock().Return(nil),
		)

		composer := NewStoreComposer()
		composer.UseCore(store)
		composer.UseTerminater(store)
		composer.UseLocker(locker)

		a := assert.New(t)

		handler, _ := NewHandler(Config{
			StoreComposer:           composer,
			NotifyTerminatedUploads: true,
			PreUploadTerminateCallback: func(hook HookEvent) (HTTPResponse, error) {
				a.Equal("foo", hook.Upload.ID)
				a.Equal(int64(10), hook.Upload.Size)

				req := hook.HTTPRequest
				a.Equal("DELETE", req.Method)
				a.Equal("foo", req.URI)

				return HTTPResponse{}, ErrUploadTerminationRejected
			},
		})

		c := make(chan HookEvent, 1)
		handler.TerminatedUploads = c

		(&httpTest{
			Method: "DELETE",
			URL:    "foo",
			ReqHeader: map[string]string{
				"Tus-Resumable": "1.0.0",
			},
			Code:    http.StatusBadRequest,
			ResBody: "ERR_UPLOAD_TERMINATION_REJECTED: upload termination has been rejected by server\n",
		}).Run(handler, t)

		select {
		case <-c:
			a.Fail("Expected termination to be rejected")
		default:
			// Expected no event
		}
	})

	SubTest(t, "NotProvided", func(t *testing.T, store *MockFullDataStore, _ *StoreComposer) {
		composer := NewStoreComposer()
		composer.UseCore(store)

		handler, _ := NewUnroutedHandler(Config{
			StoreComposer: composer,
		})

		(&httpTest{
			Method: "DELETE",
			URL:    "foo",
			ReqHeader: map[string]string{
				"Tus-Resumable": "1.0.0",
			},
			Code: http.StatusNotImplemented,
		}).Run(http.HandlerFunc(handler.DelFile), t)
	})
}
