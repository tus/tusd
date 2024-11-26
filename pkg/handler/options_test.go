package handler_test

import (
	"net/http"
	"testing"

	. "github.com/tus/tusd/v2/pkg/handler"
)

func TestOptions(t *testing.T) {
	SubTest(t, "Discovery", func(t *testing.T, store *MockFullDataStore, _ *StoreComposer) {
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

	SubTest(t, "InvalidVersion", func(t *testing.T, store *MockFullDataStore, composer *StoreComposer) {
		handler, _ := NewHandler(Config{
			StoreComposer: composer,
		})

		(&httpTest{
			Method: "POST",
			ReqHeader: map[string]string{
				"Tus-Resumable": "foo",
			},
			Code: http.StatusPreconditionFailed,
		}).Run(handler, t)
	})

	SubTest(t, "ExperimentalProtocol", func(t *testing.T, store *MockFullDataStore, _ *StoreComposer) {
		composer := NewStoreComposer()
		composer.UseCore(store)

		handler, _ := NewHandler(Config{
			StoreComposer:              composer,
			EnableExperimentalProtocol: true,
			MaxSize:                    400,
		})

		(&httpTest{
			Method: "OPTIONS",
			ReqHeader: map[string]string{
				"Upload-Draft-Interop-Version": "6",
			},
			ResHeader: map[string]string{
				"Upload-Limit": "min-size=0,max-size=400",
			},
			Code: http.StatusOK,
		}).Run(handler, t)
	})
}
