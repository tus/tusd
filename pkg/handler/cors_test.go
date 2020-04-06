package handler_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	. "github.com/tus/tusd/pkg/handler"
)

func TestCORS(t *testing.T) {
	SubTest(t, "Preflight", func(t *testing.T, store *MockFullDataStore, composer *StoreComposer) {
		handler, _ := NewHandler(Config{
			StoreComposer: composer,
		})

		(&httpTest{
			Method: "OPTIONS",
			ReqHeader: map[string]string{
				"Origin": "tus.io",
			},
			Code: http.StatusOK,
			ResHeader: map[string]string{
				"Access-Control-Allow-Headers": "Authorization, Origin, X-Requested-With, X-Request-ID, X-HTTP-Method-Override, Content-Type, Upload-Length, Upload-Offset, Tus-Resumable, Upload-Metadata, Upload-Defer-Length, Upload-Concat",
				"Access-Control-Allow-Methods": "POST, GET, HEAD, PATCH, DELETE, OPTIONS",
				"Access-Control-Max-Age":       "86400",
				"Access-Control-Allow-Origin":  "tus.io",
			},
		}).Run(handler, t)
	})

	SubTest(t, "Request", func(t *testing.T, store *MockFullDataStore, composer *StoreComposer) {
		handler, _ := NewHandler(Config{
			StoreComposer: composer,
		})

		(&httpTest{
			Name:   "Actual request",
			Method: "GET",
			ReqHeader: map[string]string{
				"Origin": "tus.io",
			},
			Code: http.StatusMethodNotAllowed,
			ResHeader: map[string]string{
				"Access-Control-Expose-Headers": "Upload-Offset, Location, Upload-Length, Tus-Version, Tus-Resumable, Tus-Max-Size, Tus-Extension, Upload-Metadata, Upload-Defer-Length, Upload-Concat",
				"Access-Control-Allow-Origin":   "tus.io",
			},
		}).Run(handler, t)
	})

	SubTest(t, "AppendHeaders", func(t *testing.T, store *MockFullDataStore, composer *StoreComposer) {
		handler, _ := NewHandler(Config{
			StoreComposer: composer,
		})

		req, _ := http.NewRequest("OPTIONS", "", nil)
		req.Header.Set("Tus-Resumable", "1.0.0")
		req.Header.Set("Origin", "tus.io")
		req.Host = "tus.io"

		res := httptest.NewRecorder()
		res.Header().Set("Access-Control-Allow-Headers", "HEADER")
		res.Header().Set("Access-Control-Allow-Methods", "METHOD")
		handler.ServeHTTP(res, req)

		headers := res.Header()["Access-Control-Allow-Headers"]
		methods := res.Header()["Access-Control-Allow-Methods"]

		if headers[0] != "HEADER" {
			t.Errorf("expected header to contain HEADER but got: %#v", headers)
		}

		if methods[0] != "METHOD" {
			t.Errorf("expected header to contain METHOD but got: %#v", methods)
		}
	})
}
