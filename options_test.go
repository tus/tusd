package tusd_test

import (
	"net/http"
	"testing"

	. "github.com/tus/tusd"
)

func TestOptions(t *testing.T) {
	SubTest(t, "Discovery", func(t *testing.T, store *MockFullDataStore) {
		composer := NewStoreComposer()
		composer.UseCore(store)

		handler, _ := NewHandler(Config{
			StoreComposer: composer,
			MaxSize:       400,
		})

		(&httpTest{
			Method: "OPTIONS",
			ResHeader: map[string]string{
				"Tus-Extension": "creation,creation-with-upload",
				"Tus-Version":   "1.0.0",
				"Tus-Resumable": "1.0.0",
				"Tus-Max-Size":  "400",
			},
			Code: http.StatusOK,
		}).Run(handler, t)
	})

	SubTest(t, "InvalidVersion", func(t *testing.T, store *MockFullDataStore) {
		handler, _ := NewHandler(Config{
			DataStore: store,
		})

		(&httpTest{
			Method: "POST",
			ReqHeader: map[string]string{
				"Tus-Resumable": "foo",
			},
			Code: http.StatusPreconditionFailed,
		}).Run(handler, t)
	})
}
