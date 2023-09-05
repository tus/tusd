package handler_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	. "github.com/tus/tusd/pkg/handler"
)

func TestCORS(t *testing.T) {
	SubTest(t, "PreFlight - Conditional allow methods", func(t *testing.T, store *MockFullDataStore, composer *StoreComposer) {
		handler, _ := NewHandler(Config{
			StoreComposer:      composer,
			CorsOrigin:         "https://tus.io",
			DisableTermination: true,
			DisableDownload:    true,
		})

		(&httpTest{
			Method: "OPTIONS",
			ReqHeader: map[string]string{
				"Origin": "https://tus.io",
			},
			Code: http.StatusOK,
			ResHeader: map[string]string{
				"Access-Control-Allow-Headers": "Authorization, Origin, X-Requested-With, X-Request-ID, X-HTTP-Method-Override, Content-Type, Upload-Length, Upload-Offset, Tus-Resumable, Upload-Metadata, Upload-Defer-Length, Upload-Concat, Upload-Incomplete, Upload-Draft-Interop-Version",
				"Access-Control-Allow-Methods": "POST, HEAD, PATCH, OPTIONS",
				"Access-Control-Max-Age":       "86400",
				"Access-Control-Allow-Origin":  "https://tus.io",
			},
		}).Run(handler, t)
	})
	SubTest(t, "PreFlight - No Origin configured", func(t *testing.T, store *MockFullDataStore, composer *StoreComposer) {
		composer = NewStoreComposer()
		composer.UseCore(store)

		handler, _ := NewHandler(Config{
			StoreComposer: composer,
			CorsOrigin:    "",
		})

		(&httpTest{
			Method: "OPTIONS",
			DisallowedResHeader: []string{
				"Access-Control-Allow-Origin",
				"Access-Control-Allow-Methods",
				"Access-Control-Allow-Headers",
				"Access-Control-Max-Age",
			},
			Code: http.StatusOK,
			ReqHeader: map[string]string{
				"Origin": "https://tus.io",
			},
		}).Run(handler, t)
	})
	SubTest(t, "PreFlight - Disabled CORS", func(t *testing.T, store *MockFullDataStore, composer *StoreComposer) {
		composer = NewStoreComposer()
		composer.UseCore(store)

		handler, _ := NewHandler(Config{
			StoreComposer: composer,
			CorsOrigin:    "",
			DisableCors:   true,
		})

		(&httpTest{
			Method: "OPTIONS",
			DisallowedResHeader: []string{
				"Access-Control-Allow-Origin",
				"Access-Control-Allow-Methods",
				"Access-Control-Allow-Headers",
				"Access-Control-Max-Age",
			},
			Code: http.StatusOK,
			ReqHeader: map[string]string{
				"Origin": "https://tus.io",
			},
		}).Run(handler, t)
	})
	SubTest(t, "PreFlight - Wildcard Origin", func(t *testing.T, store *MockFullDataStore, composer *StoreComposer) {
		composer = NewStoreComposer()
		composer.UseCore(store)

		handler, _ := NewHandler(Config{
			StoreComposer: composer,
			CorsOrigin:    "*",
		})

		(&httpTest{
			Method: "OPTIONS",
			ResHeader: map[string]string{
				"Access-Control-Allow-Origin":  "*",
				"Access-Control-Allow-Methods": "POST, HEAD, PATCH, OPTIONS, GET, DELETE",
				"Access-Control-Allow-Headers": "Authorization, Origin, X-Requested-With, X-Request-ID, X-HTTP-Method-Override, Content-Type, Upload-Length, Upload-Offset, Tus-Resumable, Upload-Metadata, Upload-Defer-Length, Upload-Concat, Upload-Incomplete, Upload-Draft-Interop-Version",
				"Access-Control-Max-Age":       "86400",
			},
			Code: http.StatusOK,
			ReqHeader: map[string]string{
				"Origin": "https://tus.io",
			},
		}).Run(handler, t)
	})
	SubTest(t, "PreFlight - Matching Origin", func(t *testing.T, store *MockFullDataStore, composer *StoreComposer) {
		composer = NewStoreComposer()
		composer.UseCore(store)

		handler, _ := NewHandler(Config{
			StoreComposer: composer,
			CorsOrigin:    "https://tus.io",
		})

		(&httpTest{
			Method: "OPTIONS",
			ResHeader: map[string]string{
				"Access-Control-Allow-Origin":  "https://tus.io",
				"Access-Control-Allow-Methods": "POST, HEAD, PATCH, OPTIONS, GET, DELETE",
				"Access-Control-Allow-Headers": "Authorization, Origin, X-Requested-With, X-Request-ID, X-HTTP-Method-Override, Content-Type, Upload-Length, Upload-Offset, Tus-Resumable, Upload-Metadata, Upload-Defer-Length, Upload-Concat, Upload-Incomplete, Upload-Draft-Interop-Version",
				"Access-Control-Max-Age":       "86400",
			},
			Code: http.StatusOK,
			ReqHeader: map[string]string{
				"Origin": "https://tus.io",
			},
		}).Run(handler, t)
	})
	SubTest(t, "PreFlight - Not Matching Origin", func(t *testing.T, store *MockFullDataStore, composer *StoreComposer) {
		composer = NewStoreComposer()
		composer.UseCore(store)

		handler, _ := NewHandler(Config{
			StoreComposer: composer,
			CorsOrigin:    "https://tus.net",
		})

		(&httpTest{
			Method: "OPTIONS",
			DisallowedResHeader: []string{
				"Access-Control-Allow-Origin",
				"Access-Control-Allow-Methods",
				"Access-Control-Allow-Headers",
				"Access-Control-Max-Age",
			},
			Code: http.StatusOK,
			ReqHeader: map[string]string{
				"Origin": "https://tus.io",
			},
		}).Run(handler, t)
	})
	SubTest(t, "Actual Request - Wildcard Origin", func(t *testing.T, store *MockFullDataStore, composer *StoreComposer) {
		composer = NewStoreComposer()
		composer.UseCore(store)

		handler, _ := NewHandler(Config{
			StoreComposer: composer,
			CorsOrigin:    "*",
		})

		(&httpTest{
			Method: "POST",
			ResHeader: map[string]string{
				"Access-Control-Allow-Origin":   "*",
				"Access-Control-Expose-Headers": "Upload-Offset, Location, Upload-Length, Tus-Version, Tus-Resumable, Tus-Max-Size, Tus-Extension, Upload-Metadata, Upload-Defer-Length, Upload-Concat",
			},
			DisallowedResHeader: []string{
				"Access-Control-Allow-Methods",
				"Access-Control-Allow-Headers",
				"Access-Control-Max-Age",
			},
			Code: http.StatusPreconditionFailed,
			ReqHeader: map[string]string{
				"Origin": "https://tus.io",
			},
		}).Run(handler, t)
	})
	SubTest(t, "Actual Request - Matching Origin", func(t *testing.T, store *MockFullDataStore, composer *StoreComposer) {
		composer = NewStoreComposer()
		composer.UseCore(store)

		handler, _ := NewHandler(Config{
			StoreComposer: composer,
			CorsOrigin:    "https://tus.io",
		})

		(&httpTest{
			Method: "POST",
			ResHeader: map[string]string{
				"Access-Control-Allow-Origin":   "https://tus.io",
				"Access-Control-Expose-Headers": "Upload-Offset, Location, Upload-Length, Tus-Version, Tus-Resumable, Tus-Max-Size, Tus-Extension, Upload-Metadata, Upload-Defer-Length, Upload-Concat",
			},
			DisallowedResHeader: []string{
				"Access-Control-Allow-Methods",
				"Access-Control-Allow-Headers",
				"Access-Control-Max-Age",
			},
			Code: http.StatusPreconditionFailed,
			ReqHeader: map[string]string{
				"Origin": "https://tus.io",
			},
		}).Run(handler, t)
	})
	SubTest(t, "AppendHeaders", func(t *testing.T, store *MockFullDataStore, composer *StoreComposer) {
		handler, _ := NewHandler(Config{
			StoreComposer: composer,
		})

		req, _ := http.NewRequest("OPTIONS", "", nil)
		req.Header.Set("Tus-Resumable", "1.0.0")
		req.Header.Set("Origin", "https://tus.io")
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
