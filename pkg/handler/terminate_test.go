package handler_test

import (
	"net/http"
	"testing"

	"github.com/golang/mock/gomock"
	. "github.com/tus/tusd/v2/pkg/handler"

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

	SubTest(t, "RejectTerminationWithNon2xxStatus", func(t *testing.T, _ *MockFullDataStore, _ *StoreComposer) {
		tests := []struct {
			name        string
			hookResp    HTTPResponse
			expCode     int
			expBody     string
			expResHeads map[string]string
		}{
			{
				name: "ForbiddenWithBodyAndHeader",
				hookResp: HTTPResponse{
					StatusCode: http.StatusForbidden,
					Body:       "not allowed",
					Header:     HTTPHeader{"X-Reason": "locked"},
				},
				expCode:     http.StatusForbidden,
				expBody:     "not allowed",
				expResHeads: map[string]string{"X-Reason": "locked"},
			},
			{
				name:     "ForbiddenWithoutBody",
				hookResp: HTTPResponse{StatusCode: http.StatusForbidden},
				expCode:  http.StatusForbidden,
				expBody:  "ERR_UPLOAD_TERMINATION_REJECTED: upload termination has been rejected by server\n",
			},
			{
				name: "Redirect",
				hookResp: HTTPResponse{
					StatusCode: http.StatusFound,
					Header:     HTTPHeader{"Location": "https://tus.io/"},
				},
				expCode:     http.StatusFound,
				expResHeads: map[string]string{"Location": "https://tus.io/"},
			},
			{
				name:     "Informational",
				hookResp: HTTPResponse{StatusCode: http.StatusContinue},
				expCode:  http.StatusBadRequest,
				expBody:  "ERR_UPLOAD_TERMINATION_REJECTED: upload termination has been rejected by server\n",
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				ctrl := gomock.NewController(t)
				defer ctrl.Finish()
				store := NewMockFullDataStore(ctrl)
				locker := NewMockFullLocker(ctrl)
				lock := NewMockFullLock(ctrl)
				upload := NewMockFullUpload(ctrl)

				// Terminate must not be called, which gomock enforces since
				// no expectation for AsTerminatableUpload/Terminate is set.
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

				handler, _ := NewHandler(Config{
					StoreComposer:           composer,
					NotifyTerminatedUploads: true,
					PreUploadTerminateCallback: func(hook HookEvent) (HTTPResponse, error) {
						return test.hookResp, nil
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
					Code:      test.expCode,
					ResBody:   test.expBody,
					ResHeader: test.expResHeads,
				}).Run(handler, t)

				select {
				case <-c:
					t.Fatal("Expected termination to be rejected")
				default:
				}
			})
		}
	})

	SubTest(t, "AllowTerminationWith2xxOrUnsetStatus", func(t *testing.T, _ *MockFullDataStore, _ *StoreComposer) {
		tests := []struct {
			name     string
			hookResp HTTPResponse
			expCode  int
			expBody  string
		}{
			{
				name:     "Unset",
				hookResp: HTTPResponse{},
				expCode:  http.StatusNoContent,
			},
			{
				name:     "OK",
				hookResp: HTTPResponse{StatusCode: http.StatusOK, Body: "deleted"},
				expCode:  http.StatusOK,
				expBody:  "deleted",
			},
			{
				name:     "Accepted",
				hookResp: HTTPResponse{StatusCode: http.StatusAccepted},
				expCode:  http.StatusAccepted,
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				ctrl := gomock.NewController(t)
				defer ctrl.Finish()
				store := NewMockFullDataStore(ctrl)
				upload := NewMockFullUpload(ctrl)

				gomock.InOrder(
					store.EXPECT().GetUpload(gomock.Any(), "foo").Return(upload, nil),
					upload.EXPECT().GetInfo(gomock.Any()).Return(FileInfo{
						ID:   "foo",
						Size: 10,
					}, nil),
					store.EXPECT().AsTerminatableUpload(upload).Return(upload),
					upload.EXPECT().Terminate(gomock.Any()).Return(nil),
				)

				composer := NewStoreComposer()
				composer.UseCore(store)
				composer.UseTerminater(store)

				handler, _ := NewHandler(Config{
					StoreComposer: composer,
					PreUploadTerminateCallback: func(hook HookEvent) (HTTPResponse, error) {
						return test.hookResp, nil
					},
				})

				(&httpTest{
					Method: "DELETE",
					URL:    "foo",
					ReqHeader: map[string]string{
						"Tus-Resumable": "1.0.0",
					},
					Code:    test.expCode,
					ResBody: test.expBody,
				}).Run(handler, t)
			})
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
