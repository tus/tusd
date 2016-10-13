package tusd_test

import (
	"net/http"
	"testing"

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
				"Tus-Extension": "creation,creation-with-upload,termination",
			},
		}).Run(handler, t)
	})

	SubTest(t, "Termination", func(t *testing.T, store *MockFullDataStore) {
		store.EXPECT().GetInfo("foo").Return(FileInfo{
			ID:   "foo",
			Size: 10,
		}, nil)
		store.EXPECT().Terminate("foo").Return(nil)

		handler, _ := NewHandler(Config{
			DataStore:               store,
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

		handler, _ := NewHandler(Config{
			StoreComposer: composer,
		})

		(&httpTest{
			Method: "DELETE",
			URL:    "foo",
			ReqHeader: map[string]string{
				"Tus-Resumable": "1.0.0",
			},
			Code: http.StatusMethodNotAllowed,
		}).Run(handler, t)
	})
}
