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
	handler, _ := NewHandler(Config{
		BasePath:  "https://buy.art/",
		DataStore: headStore{},
	})

	res := (&httpTest{
		Name:   "Successful request",
		Method: "HEAD",
		URL:    "yes",
		ReqHeader: map[string]string{
			"Tus-Resumable": "1.0.0",
		},
		Code: http.StatusNoContent,
		ResHeader: map[string]string{
			"Upload-Offset": "11",
			"Upload-Length": "44",
			"Cache-Control": "no-store",
		},
	}).Run(handler, t)

	// Since the order of a map is not guaranteed in Go, we need to be prepared
	// for the case, that the order of the metadata may have been changed
	if v := res.Header().Get("Upload-Metadata"); v != "name bHVucmpzLnBuZw==,type aW1hZ2UvcG5n" &&
		v != "type aW1hZ2UvcG5n,name bHVucmpzLnBuZw==" {
		t.Errorf("Expected valid metadata (got '%s')", v)
	}

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
