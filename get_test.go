package tusd

import (
	"io"
	"net/http"
	"net/http/httptest"
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

	// Test successfull download
	req, _ := http.NewRequest("GET", "yes", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("Expected %v (got %v)", http.StatusOK, w.Code)
	}

	if string(w.Body.Bytes()) != "hello" {
		t.Errorf("Expected response body to be 'hello'")
	}

	if w.HeaderMap.Get("Content-Length") != "5" {
		t.Errorf("Expected Content-Length to be 5")
	}
}
