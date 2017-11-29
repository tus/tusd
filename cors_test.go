package tusd_test

import (
	"net/http"
	"net/http/httptest"
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

	SubTest(t, "AppendHeaders", func(t *testing.T, store *MockFullDataStore) {
		handler, _ := NewHandler(Config{
			DataStore: store,
		})

		req, _ := http.NewRequest("OPTIONS", "", nil)
		req.Header.Set("Tus-Resumable", "1.0.0")
		req.Header.Set("Origin", "tus.io")
		req.Host = "tus.io"

		res := httptest.NewRecorder()
		res.HeaderMap.Set("Access-Control-Allow-Headers", "HEADER")
		res.HeaderMap.Set("Access-Control-Allow-Methods", "METHOD")
		handler.ServeHTTP(res, req)

		headers := res.HeaderMap["Access-Control-Allow-Headers"]
		methods := res.HeaderMap["Access-Control-Allow-Methods"]

		if headers[0] != "HEADER" {
			t.Errorf("expected header to contain HEADER but got: %#v", headers)
		}

		if methods[0] != "METHOD" {
			t.Errorf("expected header to contain HEADER but got: %#v", methods)
		}
	})
}
