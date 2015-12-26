package tusd_test

import (
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	. "github.com/tus/tusd"
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
	return reader, nil
}

type closingStringReader struct {
	*strings.Reader
	closed bool
}

func (reader *closingStringReader) Close() error {
	reader.closed = true
	return nil
}

var reader = &closingStringReader{
	Reader: strings.NewReader("hello"),
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

	if !reader.closed {
		t.Error("expected reader to be closed")
	}
}
