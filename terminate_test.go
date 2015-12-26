package tusd_test

import (
	"net/http"
	"testing"

	. "github.com/tus/tusd"
)

type terminateStore struct {
	t *testing.T
	zeroStore
}

func (s terminateStore) Terminate(id string) error {
	if id != "foo" {
		s.t.Fatal("unexpected id")
	}
	return nil
}

func TestTerminate(t *testing.T) {
	handler, _ := NewHandler(Config{
		DataStore: terminateStore{
			t: t,
		},
	})

	(&httpTest{
		Name:   "Successful OPTIONS request",
		Method: "OPTIONS",
		URL:    "",
		ResHeader: map[string]string{
			"Tus-Extension": "creation,concatenation,termination",
		},
		Code: http.StatusNoContent,
	}).Run(handler, t)

	(&httpTest{
		Name:   "Successful request",
		Method: "DELETE",
		URL:    "foo",
		ReqHeader: map[string]string{
			"Tus-Resumable": "1.0.0",
		},
		Code: http.StatusNoContent,
	}).Run(handler, t)
}

func TestTerminateNotImplemented(t *testing.T) {
	handler, _ := NewHandler(Config{
		DataStore: zeroStore{},
	})

	(&httpTest{
		Name:   "TerminaterDataStore not implemented",
		Method: "DELETE",
		URL:    "foo",
		ReqHeader: map[string]string{
			"Tus-Resumable": "1.0.0",
		},
		Code: http.StatusMethodNotAllowed,
	}).Run(handler, t)
}
