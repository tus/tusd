package tusd_test

import (
	"net/http"
	"testing"

	"github.com/golang/mock/gomock"
	. "github.com/tus/tusd"

	"github.com/stretchr/testify/assert"
)

func TestTerminate(t *testing.T) {
	SubTest(t, "ExtensionDiscovery", func(t *testing.T, store *MockFullDataStore) {
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
				"Tus-Extension": "creation,creation-with-upload,creation-defer-length,termination",
			},
		}).Run(handler, t)
	})

	SubTest(t, "Termination", func(t *testing.T, store *MockFullDataStore) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		locker := NewMockLocker(ctrl)

		gomock.InOrder(
			locker.EXPECT().LockUpload("foo"),
			store.EXPECT().GetInfo("foo").Return(FileInfo{
				ID:   "foo",
				Size: 10,
			}, nil),
			store.EXPECT().Terminate("foo").Return(nil),
			locker.EXPECT().UnlockUpload("foo"),
		)

		composer := NewStoreComposer()
		composer.UseCore(store)
		composer.UseTerminater(store)
		composer.UseLocker(locker)

		handler, _ := NewHandler(Config{
			StoreComposer:           composer,
			NotifyTerminatedUploads: true,
		})

		c := make(chan FileInfo, 1)
		handler.TerminatedUploads = c

		(&httpTest{
			Method: "DELETE",
			URL:    "foo",
			ReqHeader: map[string]string{
				"Tus-Resumable": "1.0.0",
			},
			Code: http.StatusNoContent,
		}).Run(handler, t)

		info := <-c

		a := assert.New(t)
		a.Equal("foo", info.ID)
		a.Equal(int64(10), info.Size)
	})

	SubTest(t, "NotProvided", func(t *testing.T, store *MockFullDataStore) {
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
