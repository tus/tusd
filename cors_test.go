package tusd_test

import (
	"net/http"
	"testing"

	. "github.com/tus/tusd"
)

func TestCORS(t *testing.T) {
	SubTest(t, "Preflight", func(t *testing.T, store *MockFullDataStore) {
		handler, _ := NewHandler(Config{
			DataStore: store,
		})

		(&httpTest{
			Method: "OPTIONS",
			ReqHeader: map[string]string{
				"Origin": "tus.io",
			},
			Code: http.StatusOK,
			ResHeader: map[string]string{
				"Access-Control-Allow-Headers": "Origin, X-Requested-With, Content-Type, Upload-Length, Upload-Offset, Tus-Resumable, Upload-Metadata",
				"Access-Control-Allow-Methods": "POST, GET, HEAD, PATCH, DELETE, OPTIONS",
				"Access-Control-Max-Age":       "86400",
				"Access-Control-Allow-Origin":  "tus.io",
			},
		}).Run(handler, t)
	})

	SubTest(t, "Request", func(t *testing.T, store *MockFullDataStore) {
		handler, _ := NewHandler(Config{
			DataStore: store,
		})

		(&httpTest{
			Name:   "Actual request",
			Method: "GET",
			ReqHeader: map[string]string{
				"Origin": "tus.io",
			},
			Code: http.StatusMethodNotAllowed,
			ResHeader: map[string]string{
				"Access-Control-Expose-Headers": "Upload-Offset, Location, Upload-Length, Tus-Version, Tus-Resumable, Tus-Max-Size, Tus-Extension, Upload-Metadata",
				"Access-Control-Allow-Origin":   "tus.io",
			},
		}).Run(handler, t)
	})
}
