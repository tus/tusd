package tusd

import (
	"net/http"
	"os"
	"testing"
)

type headStore struct {
	zeroStore
}

func (s headStore) GetInfo(id string) (FileInfo, error) {
	if id != "yes" {
		return FileInfo{}, os.ErrNotExist
	}

	return FileInfo{
		Offset: 11,
		Size:   44,
		MetaData: map[string]string{
			"name": "lunrjs.png",
			"type": "image/png",
		},
	}, nil
}

func TestHead(t *testing.T) {
	handler, _ := NewRoutedHandler(Config{
		BasePath:  "https://buy.art/",
		DataStore: headStore{},
	})

	(&httpTest{
		Name:   "Successful request",
		Method: "HEAD",
		URL:    "yes",
		ReqHeader: map[string]string{
			"Tus-Resumable": "1.0.0",
		},
		Code: http.StatusNoContent,
		ResHeader: map[string]string{
			"Upload-Offset":   "11",
			"Upload-Length":   "44",
			"Upload-Metadata": "name bHVucmpzLnBuZw==,type aW1hZ2UvcG5n",
			"Cache-Control":   "no-store",
		},
	}).Run(handler, t)

	(&httpTest{
		Name:   "Non-existing file",
		Method: "HEAD",
		URL:    "no",
		ReqHeader: map[string]string{
			"Tus-Resumable": "1.0.0",
		},
		Code: http.StatusNotFound,
	}).Run(handler, t)
}
