package tusd_test

import (
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/tus/tusd"
)

type FullDataStore interface {
	tusd.DataStore
	tusd.TerminaterDataStore
	tusd.ConcaterDataStore
	tusd.GetReaderDataStore
	tusd.FinisherDataStore
}

type Locker interface {
	tusd.LockerDataStore
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

type readerMatcher struct {
	expect string
}

func NewReaderMatcher(expect string) gomock.Matcher {
	return readerMatcher{
		expect: expect,
	}
}

func (m readerMatcher) Matches(x interface{}) bool {
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

func (m readerMatcher) String() string {
	return fmt.Sprintf("reads to %s", m.expect)
}
