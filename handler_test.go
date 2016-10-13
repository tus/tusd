package tusd_test

import (
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/golang/mock/gomock"

	"github.com/tus/tusd"
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

type FullDataStore interface {
	tusd.DataStore
	tusd.TerminaterDataStore
	tusd.ConcaterDataStore
	tusd.GetReaderDataStore
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

func SubTest(t *testing.T, name string, runTest func(*testing.T, *MockFullDataStore)) {
	t.Run(name, func(subT *testing.T) {
		//subT.Parallel()

		ctrl := gomock.NewController(subT)
		defer ctrl.Finish()

		store := NewMockFullDataStore(ctrl)

		runTest(subT, store)
	})
}

type ReaderMatcher struct {
	expect string
}

func NewReaderMatcher(expect string) gomock.Matcher {
	return ReaderMatcher{
		expect: expect,
	}
}

func (m ReaderMatcher) Matches(x interface{}) bool {
	input, ok := x.(io.Reader)
	if !ok {
		return false
	}

	bytes, err := ioutil.ReadAll(input)
	if err != nil {
		panic(err)
	}

	readStr := string(bytes)
	return reflect.DeepEqual(m.expect, readStr)
}

func (m ReaderMatcher) String() string {
	return fmt.Sprintf("reads to %s", m.expect)
}

func TestMethodOverride(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	store := NewMockFullDataStore(mockCtrl)

	store.EXPECT().GetInfo("yes").Return(FileInfo{
		Offset: 5,
		Size:   20,
	}, nil)

	store.EXPECT().WriteChunk("yes", int64(5), NewReaderMatcher("hello")).Return(int64(5), nil)

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
}
