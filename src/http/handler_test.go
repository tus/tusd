// handler_test.go focuses on functional tests that verify that the Handler
// implements the tus protocol correctly.

package http

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"testing"
)

const basePath = "/files/"

func Setup() *TestSetup {
	dir, err := ioutil.TempDir("", "tus_handler_test")
	if err != nil {
		panic(err)
	}

	config := HandlerConfig{
		Dir:      dir,
		MaxSize:  1024 * 1024,
		BasePath: basePath,
	}

	handler, err := NewHandler(config)
	if err != nil {
		panic(err)
	}

	server := httptest.NewServer(handler)
	return &TestSetup{
		Handler: handler,
		Server:  server,
	}
}

type TestSetup struct {
	Handler *Handler
	Server  *httptest.Server
}

func (s *TestSetup) Teardown() {
	s.Server.Close()
	if err := os.RemoveAll(s.Handler.config.Dir); err != nil {
		panic(err)
	}
}

var Protocol_FileCreation_Tests = []struct {
	Description string
	*TestRequest
}{
	{
		Description: "Bad method",
		TestRequest: &TestRequest{
			Method:           "PUT",
			ExpectStatusCode: http.StatusMethodNotAllowed,
			ExpectHeaders:    map[string]string{"Allow": "POST"},
		},
	},
	{
		Description: "Missing Final-Length header",
		TestRequest: &TestRequest{
			ExpectStatusCode: http.StatusBadRequest,
		},
	},
	{
		Description: "Invalid Final-Length header",
		TestRequest: &TestRequest{
			Headers:          map[string]string{"Final-Length": "fuck"},
			ExpectStatusCode: http.StatusBadRequest,
		},
	},
	{
		Description: "Negative Final-Length header",
		TestRequest: &TestRequest{
			Headers:          map[string]string{"Final-Length": "-10"},
			ExpectStatusCode: http.StatusBadRequest,
		},
	},
	{
		Description: "Valid Request",
		TestRequest: &TestRequest{
			Headers:          map[string]string{"Final-Length": "1024"},
			ExpectStatusCode: http.StatusCreated,
			MatchHeaders: map[string]*regexp.Regexp{
				"Location": regexp.MustCompile("^http://.+" + regexp.QuoteMeta(basePath) + "[a-z0-9]{32}$"),
			},
		},
	},
}

func TestProtocol_FileCreation(t *testing.T) {
	setup := Setup()
	defer setup.Teardown()

	for _, test := range Protocol_FileCreation_Tests {
		t.Logf("test: %s", test.Description)

		test.Url = setup.Server.URL + setup.Handler.config.BasePath
		if test.Method == "" {
			test.Method = "POST"
		}

		if err := test.Do(); err != nil {
			t.Error(err)
			continue
		}
	}
}

// TestRequest is a test helper that performs and validates requests according
// to the struct fields below.
type TestRequest struct {
	Method           string
	Url              string
	Headers          map[string]string
	ExpectStatusCode int
	ExpectHeaders    map[string]string
	MatchHeaders     map[string]*regexp.Regexp
	Response         *http.Response
}

func (r *TestRequest) Do() error {
	req, err := http.NewRequest(r.Method, r.Url, nil)
	if err != nil {
		return err
	}

	for key, val := range r.Headers {
		req.Header.Set(key, val)
	}

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode != r.ExpectStatusCode {
		return fmt.Errorf("unexpected status code: %d, expected: %d", res.StatusCode, r.ExpectStatusCode)
	}

	for key, val := range r.ExpectHeaders {
		if got := res.Header.Get(key); got != val {
			return fmt.Errorf("expected \"%s: %s\" header, but got: \"%s: %s\"", key, val, key, got)
		}
	}

	for key, matcher := range r.MatchHeaders {
		got := res.Header.Get(key)
		if !matcher.MatchString(got) {
			return fmt.Errorf("expected %s header to match: %s but got: %s", key, matcher.String(), got)
		}
	}

	r.Response = res

	return nil
}

var Protocol_Core_Tests = []struct {
	Description string
	FinalLength int64
	Requests    []TestRequest
}{
	{
		Description: "Bad method",
		FinalLength: 1024,
		Requests: []TestRequest{
			{
				Method:           "PUT",
				ExpectStatusCode: http.StatusMethodNotAllowed,
				ExpectHeaders:    map[string]string{"Allow": "PATCH"},
			},
		},
	},
}

func TestProtocol_Core(t *testing.T) {
	setup := Setup()
	defer setup.Teardown()

Tests:
	for _, test := range Protocol_Core_Tests {
		t.Logf("test: %s", test.Description)

		location := createFile(setup, test.FinalLength)
		for _, request := range test.Requests {
			request.Url = location
			if err := request.Do(); err != nil {
				t.Error(err)
				continue Tests
			}
		}
	}
}

// createFile is a test helper that creates a new file and returns the url.
func createFile(setup *TestSetup, finalLength int64) (url string) {
	req := TestRequest{
		Method:           "POST",
		Url:              setup.Server.URL + setup.Handler.config.BasePath,
		Headers:          map[string]string{"Final-Length": fmt.Sprintf("%d", finalLength)},
		ExpectStatusCode: http.StatusCreated,
	}

	if err := req.Do(); err != nil {
		panic(err)
	}

	location := req.Response.Header.Get("Location")
	if location == "" {
		panic("empty Location header")
	}

	return location
}
