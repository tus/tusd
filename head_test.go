package tusd_test

import (
	"net/http"
	"os"
	"testing"

	"github.com/golang/mock/gomock"
	. "github.com/tus/tusd"
)

func TestHead(t *testing.T) {
	SubTest(t, "Status", func(t *testing.T, store *MockFullDataStore) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		locker := NewMockLocker(ctrl)

		gomock.InOrder(
			locker.EXPECT().LockUpload("yes"),
			store.EXPECT().GetInfo("yes").Return(FileInfo{
				Offset: 11,
				Size:   44,
				MetaData: map[string]string{
					"name": "lunrjs.png",
					"type": "image/png",
				},
			}, nil),
			locker.EXPECT().UnlockUpload("yes"),
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
				"Upload-Offset": "11",
				"Upload-Length": "44",
				"Cache-Control": "no-store",
			},
		}).Run(handler, t)

		// Since the order of a map is not guaranteed in Go, we need to be prepared
		// for the case, that the order of the metadata may have been changed
		if v := res.Header().Get("Upload-Metadata"); v != "name bHVucmpzLnBuZw==,type aW1hZ2UvcG5n" &&
			v != "type aW1hZ2UvcG5n,name bHVucmpzLnBuZw==" {
			t.Errorf("Expected valid metadata (got '%s')", v)
		}
	})

	SubTest(t, "UploadNotFoundFail", func(t *testing.T, store *MockFullDataStore) {
		store.EXPECT().GetInfo("no").Return(FileInfo{}, os.ErrNotExist)

		handler, _ := NewHandler(Config{
			DataStore: store,
		})

		res := (&httpTest{
			Method: "HEAD",
			URL:    "no",
			ReqHeader: map[string]string{
				"Tus-Resumable": "1.0.0",
			},
			Code: http.StatusNotFound,
			ResHeader: map[string]string{
				"Content-Length": "0",
			},
		}).Run(handler, t)

		if string(res.Body.Bytes()) != "" {
			t.Errorf("Expected empty body for failed HEAD request")
		}
	})
}
