package tusd

import (
	"net/http"
	"testing"
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
	handler, _ := NewRoutedHandler(Config{
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
