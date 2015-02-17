package tusd

import (
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
)

type getStore struct {
	zeroStore
}

func (s getStore) GetInfo(id string) (FileInfo, error) {
	if id != "yes" {
		return FileInfo{}, os.ErrNotExist
	}

	return FileInfo{
		Offset: 5,
		Size:   20,
	}, nil
}

func (s getStore) GetReader(id string) (io.Reader, error) {
	return strings.NewReader("hello"), nil
}

func TestGet(t *testing.T) {
	handler, _ := NewHandler(Config{
		DataStore: getStore{},
	})

	(&httpTest{
		Name:    "Successful download",
		Method:  "GET",
		URL:     "yes",
		Code:    http.StatusOK,
		ResBody: "hello",
		ResHeader: map[string]string{
			"Content-Length": "5",
		},
	}).Run(handler, t)
}
