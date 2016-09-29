package tusd_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	. "github.com/tus/tusd"
)

type zeroStore struct{}

func (store zeroStore) NewUpload(info FileInfo) (string, error) {
	return "", nil
}
func (store zeroStore) WriteChunk(id string, offset int64, src io.Reader) (int64, error) {
	return 0, nil
}

func (store zeroStore) GetInfo(id string) (FileInfo, error) {
	return FileInfo{}, nil
}

type httpTest struct {
	Name string

	Method string
	URL    string

	ReqBody   io.Reader
	ReqHeader map[string]string

	Code      int
	ResBody   string
	ResHeader map[string]string
}

func (test *httpTest) Run(handler http.Handler, t *testing.T) *httptest.ResponseRecorder {
	t.Logf("'%s' in %s", test.Name, assert.CallerInfo()[1])

	req, _ := http.NewRequest(test.Method, test.URL, test.ReqBody)

	// Add headers
	for key, value := range test.ReqHeader {
		req.Header.Set(key, value)
	}

	req.Host = "tus.io"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != test.Code {
		t.Errorf("Expected %v %s as status code (got %v %s)", test.Code, http.StatusText(test.Code), w.Code, http.StatusText(w.Code))
	}

	for key, value := range test.ResHeader {
		header := w.HeaderMap.Get(key)

		if value != header {
			t.Errorf("Expected '%s' as '%s' (got '%s')", value, key, header)
		}
	}

	if test.ResBody != "" && string(w.Body.Bytes()) != test.ResBody {
		t.Errorf("Expected '%s' as body (got '%s'", test.ResBody, string(w.Body.Bytes()))
	}

	return w
}

type methodOverrideStore struct {
	zeroStore
	t      *testing.T
	called bool
}

func (s methodOverrideStore) GetInfo(id string) (FileInfo, error) {
	if id != "yes" {
		return FileInfo{}, os.ErrNotExist
	}

	return FileInfo{
		Offset: 5,
		Size:   20,
	}, nil
}

func (s *methodOverrideStore) WriteChunk(id string, offset int64, src io.Reader) (int64, error) {
	s.called = true

	return 5, nil
}

func TestMethodOverride(t *testing.T) {
	store := &methodOverrideStore{
		t: t,
	}
	handler, _ := NewHandler(Config{
		DataStore: store,
	})

	(&httpTest{
		Name:   "Successful request",
		Method: "POST",
		URL:    "yes",
		ReqHeader: map[string]string{
			"Tus-Resumable":          "1.0.0",
			"Upload-Offset":          "5",
			"Content-Type":           "application/offset+octet-stream",
			"X-HTTP-Method-Override": "PATCH",
		},
		ReqBody: strings.NewReader("hello"),
		Code:    http.StatusNoContent,
		ResHeader: map[string]string{
			"Upload-Offset": "10",
		},
	}).Run(handler, t)

	if !store.called {
		t.Fatal("WriteChunk implementation not called")
	}
}
