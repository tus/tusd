package tusd

import (
	"net/http"
	"testing"
)

func TestCORS(t *testing.T) {
	handler, _ := NewRoutedHandler(Config{})

	(&httpTest{
		Name:   "Preflight request",
		Method: "OPTIONS",
		ReqHeader: map[string]string{
			"Origin": "tus.io",
		},
		Code: http.StatusNoContent,
		ResHeader: map[string]string{
			"Access-Control-Allow-Headers": "",
			"Access-Control-Allow-Methods": "",
			"Access-Control-Max-Age":       "",
			"Access-Control-Allow-Origin":  "tus.io",
		},
	}).Run(handler, t)

	(&httpTest{
		Name:   "Actual request",
		Method: "GET",
		ReqHeader: map[string]string{
			"Origin": "tus.io",
		},
		Code: http.StatusMethodNotAllowed,
		ResHeader: map[string]string{
			"Access-Control-Expose-Headers": "",
			"Access-Control-Allow-Origin":   "tus.io",
		},
	}).Run(handler, t)
}
