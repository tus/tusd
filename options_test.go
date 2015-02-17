package tusd

import (
	"net/http"
	"testing"
)

func TestOptions(t *testing.T) {
	handler, _ := NewHandler(Config{
		MaxSize: 400,
	})

	(&httpTest{
		Name:   "Successful request",
		Method: "OPTIONS",
		Code:   http.StatusNoContent,
		ResHeader: map[string]string{
			"TUS-Extension": "file-creation,metadata,concatenation",
			"TUS-Version":   "1.0.0",
			"TUS-Resumable": "1.0.0",
			"TUS-Max-Size":  "400",
		},
	}).Run(handler, t)

	(&httpTest{
		Name:   "Invalid or unsupported version",
		Method: "POST",
		ReqHeader: map[string]string{
			"TUS-Resumable": "foo",
		},
		Code: http.StatusPreconditionFailed,
	}).Run(handler, t)
}
