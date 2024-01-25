package handler_test

import (
	"net/http"
	"testing"

	"github.com/golang/mock/gomock"
	. "github.com/tus/tusd/v2/pkg/handler"
)

func TestHead(t *testing.T) {
	SubTest(t, "Status", func(t *testing.T, store *MockFullDataStore, _ *StoreComposer) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		locker := NewMockFullLocker(ctrl)
		lock := NewMockFullLock(ctrl)
		upload := NewMockFullUpload(ctrl)

		gomock.InOrder(
			locker.EXPECT().NewLock("yes").Return(lock, nil),
			lock.EXPECT().Lock(gomock.Any(), gomock.Any()).Return(nil),
			store.EXPECT().GetUpload(gomock.Any(), "yes").Return(upload, nil),
			upload.EXPECT().GetInfo(gomock.Any()).Return(FileInfo{
				Offset: 11,
				Size:   44,
				MetaData: map[string]string{
					"name":  "lunrjs.png",
					"empty": "",
				},
			}, nil),
			lock.EXPECT().Unlock().Return(nil),
		)

		composer := NewStoreComposer()
		composer.UseCore(store)
		composer.UseLocker(locker)

		handler, _ := NewHandler(Config{
			StoreComposer: composer,
		})

		res := (&httpTest{
			Method: "HEAD",
			URL:    "yes",
			ReqHeader: map[string]string{
				"Tus-Resumable": "1.0.0",
			},
			Code: http.StatusOK,
			ResHeader: map[string]string{
				"Upload-Offset":  "11",
				"Upload-Length":  "44",
				"Content-Length": "44",
				"Cache-Control":  "no-store",
			},
		}).Run(handler, t)

		// Since the order of a map is not guaranteed in Go, we need to be prepared
		// for the case, that the order of the metadata may have been changed
		if v := res.Header().Get("Upload-Metadata"); v != "name bHVucmpzLnBuZw==,empty " &&
			v != "empty ,name bHVucmpzLnBuZw==" {
			t.Errorf("Expected valid metadata (got '%s')", v)
		}
	})

	SubTest(t, "UploadNotFoundFail", func(t *testing.T, store *MockFullDataStore, composer *StoreComposer) {
		store.EXPECT().GetUpload(gomock.Any(), "no").Return(nil, ErrNotFound)

		handler, _ := NewHandler(Config{
			StoreComposer: composer,
		})

		res := (&httpTest{
			Method: "HEAD",
			URL:    "no",
			ReqHeader: map[string]string{
				"Tus-Resumable": "1.0.0",
			},
			Code:      http.StatusNotFound,
			ResHeader: map[string]string{},
		}).Run(handler, t)

		if res.Body.String() != "" {
			t.Errorf("Expected empty body for failed HEAD request")
		}
	})

	SubTest(t, "DeferLengthHeader", func(t *testing.T, store *MockFullDataStore, composer *StoreComposer) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		upload := NewMockFullUpload(ctrl)

		gomock.InOrder(
			store.EXPECT().GetUpload(gomock.Any(), "yes").Return(upload, nil),
			upload.EXPECT().GetInfo(gomock.Any()).Return(FileInfo{
				SizeIsDeferred: true,
				Size:           0,
			}, nil),
		)

		handler, _ := NewHandler(Config{
			StoreComposer: composer,
		})

		(&httpTest{
			Method: "HEAD",
			URL:    "yes",
			ReqHeader: map[string]string{
				"Tus-Resumable": "1.0.0",
			},
			Code: http.StatusOK,
			ResHeader: map[string]string{
				"Upload-Defer-Length": "1",
			},
		}).Run(handler, t)
	})

	SubTest(t, "NoDeferLengthHeader", func(t *testing.T, store *MockFullDataStore, composer *StoreComposer) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		upload := NewMockFullUpload(ctrl)

		gomock.InOrder(
			store.EXPECT().GetUpload(gomock.Any(), "yes").Return(upload, nil),
			upload.EXPECT().GetInfo(gomock.Any()).Return(FileInfo{
				SizeIsDeferred: false,
				Size:           10,
			}, nil),
		)

		handler, _ := NewHandler(Config{
			StoreComposer: composer,
		})

		(&httpTest{
			Method: "HEAD",
			URL:    "yes",
			ReqHeader: map[string]string{
				"Tus-Resumable": "1.0.0",
			},
			Code: http.StatusOK,
			ResHeader: map[string]string{
				"Upload-Defer-Length": "",
			},
		}).Run(handler, t)
	})

	experimentalUploadTests := []struct {
		name     string
		fileInfo FileInfo
		httpTest httpTest
	}{
		{
			name: "ExperimentalProtocol-Draft-02 IncompleteUpload",
			fileInfo: FileInfo{
				SizeIsDeferred: false,
				Size:           10,
				Offset:         5,
			},
			httpTest: httpTest{
				Method: "HEAD",
				URL:    "yes",
				ReqHeader: map[string]string{
					"Upload-Draft-Interop-Version": "4",
				},
				Code: http.StatusNoContent,
				ResHeader: map[string]string{
					"Upload-Draft-Interop-Version": "4",
					"Upload-Complete":              "?0",
					"Upload-Offset":                "5",
				},
			},
		},
		{
			name: "ExperimentalProtocol-Draft-02 CompleteUpload",
			fileInfo: FileInfo{
				SizeIsDeferred: false,
				Size:           10,
				Offset:         10,
			},
			httpTest: httpTest{
				Method: "HEAD",
				URL:    "yes",
				ReqHeader: map[string]string{
					"Upload-Draft-Interop-Version": "4",
				},
				Code: http.StatusNoContent,
				ResHeader: map[string]string{
					"Upload-Draft-Interop-Version": "4",
					"Upload-Complete":              "?1",
					"Upload-Offset":                "10",
				},
			},
		},
		{
			name: "ExperimentalProtocol-Draft-02 DeferredLength",
			fileInfo: FileInfo{
				SizeIsDeferred: true,
				Offset:         5,
			},
			httpTest: httpTest{
				Method: "HEAD",
				URL:    "yes",
				ReqHeader: map[string]string{
					"Upload-Draft-Interop-Version": "4",
				},
				Code: http.StatusNoContent,
				ResHeader: map[string]string{
					"Upload-Draft-Interop-Version": "4",
					"Upload-Complete":              "?0",
					"Upload-Offset":                "5",
				},
			},
		},
		{
			name: "ExperimentalProtocol-Draft-01 IncompleteUpload",
			fileInfo: FileInfo{
				SizeIsDeferred: false,
				Size:           10,
				Offset:         5,
			},
			httpTest: httpTest{
				Method: "HEAD",
				URL:    "yes",
				ReqHeader: map[string]string{
					"Upload-Draft-Interop-Version": "3",
				},
				Code: http.StatusNoContent,
				ResHeader: map[string]string{
					"Upload-Draft-Interop-Version": "3",
					"Upload-Incomplete":            "?1",
					"Upload-Offset":                "5",
				},
			},
		},
		{
			name: "ExperimentalProtocol-Draft-01 CompleteUpload",
			fileInfo: FileInfo{
				SizeIsDeferred: false,
				Size:           10,
				Offset:         10,
			},
			httpTest: httpTest{
				Method: "HEAD",
				URL:    "yes",
				ReqHeader: map[string]string{
					"Upload-Draft-Interop-Version": "3",
				},
				Code: http.StatusNoContent,
				ResHeader: map[string]string{
					"Upload-Draft-Interop-Version": "3",
					"Upload-Incomplete":            "?0",
					"Upload-Offset":                "10",
				},
			},
		},
		{
			name: "ExperimentalProtocol-Draft-01 DeferredLength",
			fileInfo: FileInfo{
				SizeIsDeferred: true,
				Offset:         5,
			},
			httpTest: httpTest{
				Method: "HEAD",
				URL:    "yes",
				ReqHeader: map[string]string{
					"Upload-Draft-Interop-Version": "3",
				},
				Code: http.StatusNoContent,
				ResHeader: map[string]string{
					"Upload-Draft-Interop-Version": "3",
					"Upload-Incomplete":            "?1",
					"Upload-Offset":                "5",
				},
			},
		},
	}

	for _, test := range experimentalUploadTests {
		SubTest(t, test.name, func(t *testing.T, store *MockFullDataStore, composer *StoreComposer) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			upload := NewMockFullUpload(ctrl)

			gomock.InOrder(
				store.EXPECT().GetUpload(gomock.Any(), "yes").Return(upload, nil),
				upload.EXPECT().GetInfo(gomock.Any()).Return(test.fileInfo, nil),
			)

			handler, _ := NewHandler(Config{
				StoreComposer:              composer,
				EnableExperimentalProtocol: true,
			})

			(&test.httpTest).Run(handler, t)
		})
	}
}
