package handler_test

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	. "github.com/tus/tusd/v2/pkg/handler"
)

func TestExpiration(t *testing.T) {
	SubTest(t, "ExtensionDiscovery", func(t *testing.T, store *MockFullDataStore, _ *StoreComposer) {
		composer := NewStoreComposer()
		composer.UseCore(store)

		// The extension is available even if uploads do not expire by default, since the
		// expiration time can always be set by the pre-create hook.
		handler, _ := NewHandler(Config{
			StoreComposer: composer,
		})

		(&httpTest{
			Method: "OPTIONS",
			Code:   http.StatusOK,
			ResHeader: map[string]string{
				"Tus-Extension": "creation,creation-with-upload,expiration",
			},
		}).Run(handler, t)
	})

	SubTest(t, "NoExtensionDiscoveryWhenDisabled", func(t *testing.T, store *MockFullDataStore, _ *StoreComposer) {
		composer := NewStoreComposer()
		composer.UseCore(store)

		handler, _ := NewHandler(Config{
			StoreComposer:     composer,
			DisableExpiration: true,
		})

		(&httpTest{
			Method: "OPTIONS",
			Code:   http.StatusOK,
			ResHeader: map[string]string{
				"Tus-Extension": "creation,creation-with-upload",
			},
		}).Run(handler, t)
	})

	SubTest(t, "Create", func(t *testing.T, store *MockFullDataStore, composer *StoreComposer) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		upload := NewMockFullUpload(ctrl)

		// The expiration time is derived from the current time, so we cannot assert an
		// exact value for the FileInfo that is passed to the data store.
		var createdInfo FileInfo
		gomock.InOrder(
			store.EXPECT().NewUpload(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, info FileInfo) (Upload, error) {
				createdInfo = info
				return upload, nil
			}),
			upload.EXPECT().GetInfo(gomock.Any()).DoAndReturn(func(_ context.Context) (FileInfo, error) {
				info := createdInfo
				info.ID = "foo"
				return info, nil
			}),
		)

		handler, _ := NewHandler(Config{
			StoreComposer:     composer,
			BasePath:          "files",
			DefaultExpiration: time.Hour,
		})

		res := (&httpTest{
			Method: "POST",
			ReqHeader: map[string]string{
				"Tus-Resumable": "1.0.0",
				"Upload-Length": "300",
			},
			Code: http.StatusCreated,
		}).Run(handler, t)

		a := assert.New(t)
		a.WithinDuration(time.Now().Add(time.Hour), createdInfo.ExpiresAt, 5*time.Second)

		// The Upload-Expires header must be an RFC 7231 datetime, which http.ParseTime accepts.
		expires, err := http.ParseTime(res.Header().Get("Upload-Expires"))
		a.NoError(err)
		a.Equal(createdInfo.ExpiresAt.Truncate(time.Second).UTC(), expires.UTC())
	})

	SubTest(t, "CreateEmptyUploadWithoutExpiration", func(t *testing.T, store *MockFullDataStore, composer *StoreComposer) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		upload := NewMockFullUpload(ctrl)

		gomock.InOrder(
			store.EXPECT().NewUpload(gomock.Any(), gomock.Any()).Return(upload, nil),
			upload.EXPECT().GetInfo(gomock.Any()).Return(FileInfo{
				ID:        "foo",
				Size:      0,
				Offset:    0,
				ExpiresAt: time.Now().Add(time.Hour),
			}, nil),
			upload.EXPECT().FinishUpload(gomock.Any()).Return(nil),
		)

		handler, _ := NewHandler(Config{
			StoreComposer:     composer,
			BasePath:          "files",
			DefaultExpiration: time.Hour,
		})

		res := (&httpTest{
			Method: "POST",
			ReqHeader: map[string]string{
				"Tus-Resumable": "1.0.0",
				"Upload-Length": "0",
			},
			Code: http.StatusCreated,
		}).Run(handler, t)

		// An empty upload is already finished when it is created, so it never expires.
		assert.Empty(t, res.Header().Get("Upload-Expires"))
	})

	SubTest(t, "CreateWithHookExpiration", func(t *testing.T, store *MockFullDataStore, composer *StoreComposer) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		upload := NewMockFullUpload(ctrl)

		// Truncated to seconds because the Upload-Expires header has a resolution of one second.
		expiresAt := time.Now().Add(48 * time.Hour).UTC().Truncate(time.Second)

		gomock.InOrder(
			store.EXPECT().NewUpload(gomock.Any(), FileInfo{
				Size:      300,
				MetaData:  map[string]string{},
				ExpiresAt: expiresAt,
			}).Return(upload, nil),
			upload.EXPECT().GetInfo(gomock.Any()).Return(FileInfo{
				ID:        "foo",
				Size:      300,
				ExpiresAt: expiresAt,
			}, nil),
		)

		handler, _ := NewHandler(Config{
			StoreComposer: composer,
			BasePath:      "files",
			// No default expiration, so the hook is the only source of the expiration time.
			PreUploadCreateCallback: func(hook HookEvent) (HTTPResponse, FileInfoChanges, error) {
				assert.True(t, hook.Upload.ExpiresAt.IsZero())

				return HTTPResponse{}, FileInfoChanges{
					ExpiresAt: &expiresAt,
				}, nil
			},
		})

		(&httpTest{
			Method: "POST",
			ReqHeader: map[string]string{
				"Tus-Resumable": "1.0.0",
				"Upload-Length": "300",
			},
			Code: http.StatusCreated,
			ResHeader: map[string]string{
				"Upload-Expires": expiresAt.Format(http.TimeFormat),
			},
		}).Run(handler, t)
	})

	SubTest(t, "CreateWithHookOverridingDuration", func(t *testing.T, store *MockFullDataStore, composer *StoreComposer) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		upload := NewMockFullUpload(ctrl)

		// The zero value of time.Time prevents the upload from expiring, even though a
		// server-wide expiration duration is configured.
		neverExpires := time.Time{}

		gomock.InOrder(
			store.EXPECT().NewUpload(gomock.Any(), FileInfo{
				Size:     300,
				MetaData: map[string]string{},
			}).Return(upload, nil),
			upload.EXPECT().GetInfo(gomock.Any()).Return(FileInfo{
				ID:   "foo",
				Size: 300,
			}, nil),
		)

		handler, _ := NewHandler(Config{
			StoreComposer:     composer,
			BasePath:          "files",
			DefaultExpiration: time.Hour,
			PreUploadCreateCallback: func(hook HookEvent) (HTTPResponse, FileInfoChanges, error) {
				assert.False(t, hook.Upload.ExpiresAt.IsZero())

				return HTTPResponse{}, FileInfoChanges{
					ExpiresAt: &neverExpires,
				}, nil
			},
		})

		res := (&httpTest{
			Method: "POST",
			ReqHeader: map[string]string{
				"Tus-Resumable": "1.0.0",
				"Upload-Length": "300",
			},
			Code: http.StatusCreated,
		}).Run(handler, t)

		assert.Empty(t, res.Header().Get("Upload-Expires"))
	})

	SubTest(t, "CreateWithExpirationInPastFail", func(t *testing.T, store *MockFullDataStore, composer *StoreComposer) {
		expiresAt := time.Now().Add(-time.Hour)

		handler, _ := NewHandler(Config{
			StoreComposer: composer,
			BasePath:      "files",
			PreUploadCreateCallback: func(hook HookEvent) (HTTPResponse, FileInfoChanges, error) {
				return HTTPResponse{}, FileInfoChanges{
					ExpiresAt: &expiresAt,
				}, nil
			},
		})

		// The upload must not be created at all, so the data store is never called.
		(&httpTest{
			Method: "POST",
			ReqHeader: map[string]string{
				"Tus-Resumable": "1.0.0",
				"Upload-Length": "300",
			},
			Code:    http.StatusInternalServerError,
			ResBody: "ERR_INVALID_EXPIRATION: expiration time must not be in the past\n",
		}).Run(handler, t)
	})

	SubTest(t, "Status", func(t *testing.T, store *MockFullDataStore, composer *StoreComposer) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		upload := NewMockFullUpload(ctrl)

		expiresAt := time.Now().Add(time.Hour).UTC().Truncate(time.Second)

		gomock.InOrder(
			store.EXPECT().GetUpload(gomock.Any(), "foo").Return(upload, nil),
			upload.EXPECT().GetInfo(gomock.Any()).Return(FileInfo{
				ID:        "foo",
				Offset:    5,
				Size:      10,
				ExpiresAt: expiresAt,
			}, nil),
		)

		handler, _ := NewHandler(Config{
			StoreComposer:     composer,
			DefaultExpiration: time.Hour,
		})

		(&httpTest{
			Method: "HEAD",
			URL:    "foo",
			ReqHeader: map[string]string{
				"Tus-Resumable": "1.0.0",
			},
			Code: http.StatusOK,
			ResHeader: map[string]string{
				"Upload-Offset":  "5",
				"Upload-Length":  "10",
				"Upload-Expires": expiresAt.Format(http.TimeFormat),
			},
		}).Run(handler, t)
	})

	SubTest(t, "Expired", func(t *testing.T, store *MockFullDataStore, _ *StoreComposer) {
		SubTest(t, "StatusFail", func(t *testing.T, store *MockFullDataStore, composer *StoreComposer) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			upload := NewMockFullUpload(ctrl)

			gomock.InOrder(
				store.EXPECT().GetUpload(gomock.Any(), "foo").Return(upload, nil),
				upload.EXPECT().GetInfo(gomock.Any()).Return(FileInfo{
					ID:        "foo",
					Offset:    5,
					Size:      10,
					ExpiresAt: time.Now().Add(-time.Hour),
				}, nil),
			)

			handler, _ := NewHandler(Config{
				StoreComposer:     composer,
				DefaultExpiration: time.Hour,
			})

			(&httpTest{
				Method: "HEAD",
				URL:    "foo",
				ReqHeader: map[string]string{
					"Tus-Resumable": "1.0.0",
				},
				Code: http.StatusGone,
			}).Run(handler, t)
		})

		SubTest(t, "PatchFail", func(t *testing.T, store *MockFullDataStore, composer *StoreComposer) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			upload := NewMockFullUpload(ctrl)

			// The upload data must not be touched, so WriteChunk is never called.
			gomock.InOrder(
				store.EXPECT().GetUpload(gomock.Any(), "foo").Return(upload, nil),
				upload.EXPECT().GetInfo(gomock.Any()).Return(FileInfo{
					ID:        "foo",
					Offset:    5,
					Size:      10,
					ExpiresAt: time.Now().Add(-time.Hour),
				}, nil),
			)

			handler, _ := NewHandler(Config{
				StoreComposer:     composer,
				DefaultExpiration: time.Hour,
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
				Code:    http.StatusGone,
				ResBody: "ERR_UPLOAD_EXPIRED: upload has expired\n",
			}).Run(handler, t)
		})

		SubTest(t, "DownloadFail", func(t *testing.T, store *MockFullDataStore, composer *StoreComposer) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			upload := NewMockFullUpload(ctrl)

			gomock.InOrder(
				store.EXPECT().GetUpload(gomock.Any(), "foo").Return(upload, nil),
				upload.EXPECT().GetInfo(gomock.Any()).Return(FileInfo{
					ID:        "foo",
					Offset:    5,
					Size:      10,
					ExpiresAt: time.Now().Add(-time.Hour),
				}, nil),
			)

			handler, _ := NewHandler(Config{
				StoreComposer:     composer,
				DefaultExpiration: time.Hour,
			})

			(&httpTest{
				Method: "GET",
				URL:    "foo",
				Code:   http.StatusGone,
			}).Run(handler, t)
		})

		SubTest(t, "TerminationIsStillPossible", func(t *testing.T, store *MockFullDataStore, composer *StoreComposer) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			upload := NewMockFullUpload(ctrl)

			// Terminating an expired upload is the only way to free its storage, so it
			// must not be rejected.
			gomock.InOrder(
				store.EXPECT().GetUpload(gomock.Any(), "foo").Return(upload, nil),
				store.EXPECT().AsTerminatableUpload(upload).Return(upload),
				upload.EXPECT().Terminate(gomock.Any()).Return(nil),
			)

			handler, _ := NewHandler(Config{
				StoreComposer:     composer,
				DefaultExpiration: time.Hour,
			})

			(&httpTest{
				Method: "DELETE",
				URL:    "foo",
				ReqHeader: map[string]string{
					"Tus-Resumable": "1.0.0",
				},
				Code: http.StatusNoContent,
			}).Run(handler, t)
		})

		SubTest(t, "FinishedUploadIsStillServed", func(t *testing.T, store *MockFullDataStore, composer *StoreComposer) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			upload := NewMockFullUpload(ctrl)

			// Expiration is only enforced as long as an upload is not finished yet.
			gomock.InOrder(
				store.EXPECT().GetUpload(gomock.Any(), "foo").Return(upload, nil),
				upload.EXPECT().GetInfo(gomock.Any()).Return(FileInfo{
					ID:        "foo",
					Offset:    10,
					Size:      10,
					ExpiresAt: time.Now().Add(-time.Hour),
				}, nil),
			)

			handler, _ := NewHandler(Config{
				StoreComposer:     composer,
				DefaultExpiration: time.Hour,
			})

			res := (&httpTest{
				Method: "HEAD",
				URL:    "foo",
				ReqHeader: map[string]string{
					"Tus-Resumable": "1.0.0",
				},
				Code: http.StatusOK,
				ResHeader: map[string]string{
					"Upload-Offset": "10",
					"Upload-Length": "10",
				},
			}).Run(handler, t)

			assert.Empty(t, res.Header().Get("Upload-Expires"))
		})

		SubTest(t, "FinishedPartialUploadFail", func(t *testing.T, store *MockFullDataStore, composer *StoreComposer) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			upload := NewMockFullUpload(ctrl)

			// A partial upload is not usable on its own, so it keeps expiring even after
			// all of its data has been transmitted.
			gomock.InOrder(
				store.EXPECT().GetUpload(gomock.Any(), "foo").Return(upload, nil),
				upload.EXPECT().GetInfo(gomock.Any()).Return(FileInfo{
					ID:        "foo",
					IsPartial: true,
					Offset:    10,
					Size:      10,
					ExpiresAt: time.Now().Add(-time.Hour),
				}, nil),
			)

			handler, _ := NewHandler(Config{
				StoreComposer:     composer,
				DefaultExpiration: time.Hour,
			})

			(&httpTest{
				Method: "HEAD",
				URL:    "foo",
				ReqHeader: map[string]string{
					"Tus-Resumable": "1.0.0",
				},
				Code: http.StatusGone,
			}).Run(handler, t)
		})

		SubTest(t, "ConcatenationFail", func(t *testing.T, store *MockFullDataStore, composer *StoreComposer) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			upload := NewMockFullUpload(ctrl)

			// An expired partial upload must not end up in a final upload.
			gomock.InOrder(
				store.EXPECT().GetUpload(gomock.Any(), "a").Return(upload, nil),
				upload.EXPECT().GetInfo(gomock.Any()).Return(FileInfo{
					ID:        "a",
					IsPartial: true,
					Offset:    5,
					Size:      5,
					ExpiresAt: time.Now().Add(-time.Hour),
				}, nil),
			)

			handler, _ := NewHandler(Config{
				StoreComposer:     composer,
				BasePath:          "files",
				DefaultExpiration: time.Hour,
			})

			(&httpTest{
				Method: "POST",
				ReqHeader: map[string]string{
					"Tus-Resumable": "1.0.0",
					"Upload-Concat": "final;http://tus.io/files/a",
				},
				Code: http.StatusGone,
			}).Run(handler, t)
		})

		SubTest(t, "IgnoredWhenDisabled", func(t *testing.T, store *MockFullDataStore, composer *StoreComposer) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			upload := NewMockFullUpload(ctrl)

			// The upload carries an expiration time in the past, but the extension is
			// disabled, so it is still served.
			gomock.InOrder(
				store.EXPECT().GetUpload(gomock.Any(), "foo").Return(upload, nil),
				upload.EXPECT().GetInfo(gomock.Any()).Return(FileInfo{
					ID:        "foo",
					Offset:    5,
					Size:      10,
					ExpiresAt: time.Now().Add(-time.Hour),
				}, nil),
			)

			handler, _ := NewHandler(Config{
				StoreComposer:     composer,
				DefaultExpiration: time.Hour,
				DisableExpiration: true,
			})

			res := (&httpTest{
				Method: "HEAD",
				URL:    "foo",
				ReqHeader: map[string]string{
					"Tus-Resumable": "1.0.0",
				},
				Code: http.StatusOK,
				ResHeader: map[string]string{
					"Upload-Offset": "5",
				},
			}).Run(handler, t)

			assert.Empty(t, res.Header().Get("Upload-Expires"))
		})

		SubTest(t, "Notification", func(t *testing.T, store *MockFullDataStore, composer *StoreComposer) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			upload := NewMockFullUpload(ctrl)

			expiresAt := time.Now().Add(-time.Hour)

			gomock.InOrder(
				store.EXPECT().GetUpload(gomock.Any(), "foo").Return(upload, nil),
				upload.EXPECT().GetInfo(gomock.Any()).Return(FileInfo{
					ID:        "foo",
					Offset:    5,
					Size:      10,
					ExpiresAt: expiresAt,
				}, nil),
			)

			handler, _ := NewHandler(Config{
				StoreComposer:        composer,
				DefaultExpiration:    time.Hour,
				NotifyExpiredUploads: true,
			})

			c := make(chan HookEvent, 1)
			handler.ExpiredUploads = c

			(&httpTest{
				Method: "HEAD",
				URL:    "foo",
				ReqHeader: map[string]string{
					"Tus-Resumable": "1.0.0",
				},
				Code: http.StatusGone,
			}).Run(handler, t)

			event := <-c
			info := event.Upload

			a := assert.New(t)
			a.Equal("foo", info.ID)
			a.Equal(expiresAt, info.ExpiresAt)
			a.Equal("HEAD", event.HTTPRequest.Method)
		})
	})
}
