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
		Name:   "Successful request",
		Method: "DELETE",
		URL:    "foo",
		ReqHeader: map[string]string{
			"Tus-Resumable": "1.0.0",
		},
		Code: http.StatusNoContent,
	}).Run(handler, t)
}
