package handler_test

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	. "github.com/tus/tusd/v2/pkg/handler"
)

func TestCORS(t *testing.T) {
	SubTest(t, "DefaultConfiguration", func(t *testing.T, store *MockFullDataStore, composer *StoreComposer) {
		handler, _ := NewHandler(Config{
			StoreComposer: composer,
		})

		// Preflight request
		(&httpTest{
			Method: "OPTIONS",
			ReqHeader: map[string]string{
				"Origin": "https://tus.io",
			},
			Code: http.StatusOK,
			ResHeader: map[string]string{
				"Access-Control-Allow-Headers":     "Authorization, Origin, X-Requested-With, X-Request-ID, X-HTTP-Method-Override, Content-Type, Upload-Length, Upload-Offset, Tus-Resumable, Upload-Metadata, Upload-Defer-Length, Upload-Concat, Upload-Complete, Upload-Draft-Interop-Version",
				"Access-Control-Allow-Methods":     "POST, HEAD, PATCH, OPTIONS, GET, DELETE",
				"Access-Control-Max-Age":           "86400",
				"Access-Control-Allow-Origin":      "https://tus.io",
				"Vary":                             "Origin",
				"Access-Control-Allow-Credentials": "",
			},
		}).Run(handler, t)

		// Actual request
		(&httpTest{
			Method: "POST",
			ReqHeader: map[string]string{
				"Origin": "https://tus.io",
			},
			ResHeader: map[string]string{
				"Access-Control-Allow-Origin":      "https://tus.io",
				"Access-Control-Expose-Headers":    "Upload-Offset, Location, Upload-Length, Tus-Version, Tus-Resumable, Tus-Max-Size, Tus-Extension, Upload-Metadata, Upload-Defer-Length, Upload-Concat, Upload-Complete, Upload-Draft-Interop-Version",
				"Vary":                             "Origin",
				"Access-Control-Allow-Methods":     "",
				"Access-Control-Allow-Headers":     "",
				"Access-Control-Max-Age":           "",
				"Access-Control-Allow-Credentials": "",
			},
			// Error response is expected
			Code: http.StatusPreconditionFailed,
		}).Run(handler, t)
	})

	SubTest(t, "CustomAllowedOrigin", func(t *testing.T, store *MockFullDataStore, composer *StoreComposer) {
		handler, _ := NewHandler(Config{
			StoreComposer: composer,
			Cors: &CorsConfig{
				AllowOrigin:   regexp.MustCompile(`^https?://tus\.io$`),
				AllowMethods:  DefaultCorsConfig.AllowMethods,
				AllowHeaders:  DefaultCorsConfig.AllowHeaders,
				ExposeHeaders: DefaultCorsConfig.ExposeHeaders,
				MaxAge:        DefaultCorsConfig.MaxAge,
			},
		})

		// Preflight request
		(&httpTest{
			Method: "OPTIONS",
			ReqHeader: map[string]string{
				"Origin": "http://tus.io",
			},
			Code: http.StatusOK,
			ResHeader: map[string]string{
				"Access-Control-Allow-Headers":     "Authorization, Origin, X-Requested-With, X-Request-ID, X-HTTP-Method-Override, Content-Type, Upload-Length, Upload-Offset, Tus-Resumable, Upload-Metadata, Upload-Defer-Length, Upload-Concat, Upload-Complete, Upload-Draft-Interop-Version",
				"Access-Control-Allow-Methods":     "POST, HEAD, PATCH, OPTIONS, GET, DELETE",
				"Access-Control-Max-Age":           "86400",
				"Access-Control-Allow-Origin":      "http://tus.io",
				"Vary":                             "Origin",
				"Access-Control-Allow-Credentials": "",
			},
		}).Run(handler, t)

		// Actual request
		(&httpTest{
			Method: "POST",
			ReqHeader: map[string]string{
				"Origin": "http://tus.io",
			},
			ResHeader: map[string]string{
				"Access-Control-Allow-Origin":      "http://tus.io",
				"Access-Control-Expose-Headers":    "Upload-Offset, Location, Upload-Length, Tus-Version, Tus-Resumable, Tus-Max-Size, Tus-Extension, Upload-Metadata, Upload-Defer-Length, Upload-Concat, Upload-Complete, Upload-Draft-Interop-Version",
				"Vary":                             "Origin",
				"Access-Control-Allow-Methods":     "",
				"Access-Control-Allow-Headers":     "",
				"Access-Control-Allow-Credentials": "",
				"Access-Control-Max-Age":           "",
			},
			// Error response is expected
			Code: http.StatusPreconditionFailed,
		}).Run(handler, t)
	})

	SubTest(t, "CustomForbiddenOrigin", func(t *testing.T, store *MockFullDataStore, composer *StoreComposer) {
		handler, _ := NewHandler(Config{
			StoreComposer: composer,
			Cors: &CorsConfig{
				AllowOrigin: regexp.MustCompile(`^https?://tus\.io$`),
			},
		})

		// Preflight request
		(&httpTest{
			Method: "OPTIONS",
			ReqHeader: map[string]string{
				"Origin": "http://example.com",
			},
			Code: http.StatusForbidden,
			ResHeader: map[string]string{
				"Access-Control-Allow-Origin":      "",
				"Access-Control-Expose-Headers":    "",
				"Access-Control-Allow-Methods":     "",
				"Access-Control-Allow-Headers":     "",
				"Access-Control-Allow-Credentials": "",
				"Access-Control-Max-Age":           "",
			},
			ResBody: "ERR_ORIGIN_NOT_ALLOWED: request origin is not allowed\n",
		}).Run(handler, t)

		// Actual request
		(&httpTest{
			Method: "POST",
			ReqHeader: map[string]string{
				"Origin": "http://example.com",
			},
			Code: http.StatusForbidden,
			ResHeader: map[string]string{
				"Access-Control-Allow-Origin":      "",
				"Access-Control-Expose-Headers":    "",
				"Access-Control-Allow-Methods":     "",
				"Access-Control-Allow-Headers":     "",
				"Access-Control-Allow-Credentials": "",
				"Access-Control-Max-Age":           "",
			},
			ResBody: "ERR_ORIGIN_NOT_ALLOWED: request origin is not allowed\n",
		}).Run(handler, t)
	})

	SubTest(t, "CustomConfig", func(t *testing.T, store *MockFullDataStore, composer *StoreComposer) {
		handler, _ := NewHandler(Config{
			StoreComposer: composer,
			Cors: &CorsConfig{
				AllowOrigin:      regexp.MustCompile(`^https?://tus\.io$`),
				AllowMethods:     "POST, PATCH",
				AllowHeaders:     "A, B, C",
				ExposeHeaders:    "D, E, F",
				MaxAge:           "500",
				AllowCredentials: true,
			},
		})

		// Preflight request
		(&httpTest{
			Method: "OPTIONS",
			ReqHeader: map[string]string{
				"Origin": "http://tus.io",
			},
			Code: http.StatusOK,
			ResHeader: map[string]string{
				"Access-Control-Allow-Headers":     "A, B, C",
				"Access-Control-Allow-Methods":     "POST, PATCH",
				"Access-Control-Max-Age":           "500",
				"Access-Control-Allow-Origin":      "http://tus.io",
				"Access-Control-Allow-Credentials": "true",
				"Vary":                             "Origin",
			},
		}).Run(handler, t)

		// Actual request
		(&httpTest{
			Method: "POST",
			ReqHeader: map[string]string{
				"Origin": "http://tus.io",
			},
			ResHeader: map[string]string{
				"Access-Control-Allow-Origin":      "http://tus.io",
				"Access-Control-Expose-Headers":    "D, E, F",
				"Access-Control-Allow-Credentials": "true",
				"Vary":                             "Origin",
				"Access-Control-Allow-Methods":     "",
				"Access-Control-Allow-Headers":     "",
				"Access-Control-Max-Age":           "",
			},
			// Error response is expected
			Code: http.StatusPreconditionFailed,
		}).Run(handler, t)
	})

	SubTest(t, "DisabledConfig", func(t *testing.T, store *MockFullDataStore, composer *StoreComposer) {
		handler, _ := NewHandler(Config{
			StoreComposer: composer,
			Cors: &CorsConfig{
				Disable: true,
			},
		})

		// Preflight request
		(&httpTest{
			Method: "OPTIONS",
			ReqHeader: map[string]string{
				"Origin": "http://example.com",
			},
			Code: http.StatusOK,
			ResHeader: map[string]string{
				"Access-Control-Allow-Origin":      "",
				"Access-Control-Expose-Headers":    "",
				"Access-Control-Allow-Methods":     "",
				"Access-Control-Allow-Headers":     "",
				"Access-Control-Allow-Credentials": "",
				"Access-Control-Max-Age":           "",
			},
		}).Run(handler, t)

		// Actual request
		(&httpTest{
			Method: "POST",
			ReqHeader: map[string]string{
				"Origin": "http://example.com",
			},
			Code: http.StatusPreconditionFailed,
			ResHeader: map[string]string{
				"Access-Control-Allow-Origin":      "",
				"Access-Control-Expose-Headers":    "",
				"Access-Control-Allow-Methods":     "",
				"Access-Control-Allow-Headers":     "",
				"Access-Control-Allow-Credentials": "",
				"Access-Control-Max-Age":           "",
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
