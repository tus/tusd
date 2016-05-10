package tusd_test

import (
	"net/http"
	"testing"

	. "github.com/tus/tusd"

	"github.com/stretchr/testify/assert"
)

type terminateStore struct {
	t *testing.T
	zeroStore
}

func (s terminateStore) GetInfo(id string) (FileInfo, error) {
	return FileInfo{
		ID:   id,
		Size: 10,
	}, nil
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
		NotifyTerminatedUploads: true,
	})

	c := make(chan FileInfo, 1)
	handler.TerminatedUploads = c

	(&httpTest{
		Name:   "Successful OPTIONS request",
		Method: "OPTIONS",
		URL:    "",
		ResHeader: map[string]string{
			"Tus-Extension": "creation,termination",
		},
		Code: http.StatusOK,
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

	info := <-c

	a := assert.New(t)
	a.Equal("foo", info.ID)
	a.Equal(int64(10), info.Size)
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
