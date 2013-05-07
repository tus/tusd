package http

import (
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"testing"
)

const basePath = "/files/"

var Protocol_FileCreation_Tests = []struct {
	Description      string
	Method           string
	Headers          map[string]string
	ExpectStatusCode int
	ExpectHeaders    map[string]string
	MatchLocation    *regexp.Regexp
}{
	{
		Description:      "Bad method",
		Method:           "PUT",
		ExpectStatusCode: http.StatusMethodNotAllowed,
		ExpectHeaders:    map[string]string{"Allow": "POST"},
	},
	{
		Description:      "Missing Final-Length header",
		ExpectStatusCode: http.StatusBadRequest,
	},
	{
		Description:      "Invalid Final-Length header",
		Headers:          map[string]string{"Final-Length": "fuck"},
		ExpectStatusCode: http.StatusBadRequest,
	},
	{
		Description:      "Negative Final-Length header",
		Headers:          map[string]string{"Final-Length": "-10"},
		ExpectStatusCode: http.StatusBadRequest,
	},
	{
		Description:      "Valid Request",
		Headers:          map[string]string{"Final-Length": "1024"},
		ExpectStatusCode: http.StatusCreated,
		MatchLocation:    regexp.MustCompile("^http://.+" + regexp.QuoteMeta(basePath) + "[a-z0-9]{32}$"),
	},
}

func TestProtocol_FileCreation(t *testing.T) {
	dir, err := ioutil.TempDir("", "tus_handler_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(dir)

	config := HandlerConfig{
		Dir:      dir,
		MaxSize:  1024 * 1024,
		BasePath: basePath,
	}

	handler, err := NewHandler(config)
	if err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(handler)
	defer server.Close()

Tests:
	for _, test := range Protocol_FileCreation_Tests {
		t.Logf("test: %s", test.Description)

		method := test.Method
		if method == "" {
			method = "POST"
		}

		req, err := http.NewRequest(method, server.URL+config.BasePath, nil)
		if err != nil {
			t.Fatal(err)
		}

		for key, val := range test.Headers {
			req.Header.Set(key, val)
		}

		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()

		body, err := ioutil.ReadAll(res.Body)
		if err != nil {
			t.Fatal(err)
		}

		if test.ExpectStatusCode != 0 && test.ExpectStatusCode != res.StatusCode {
			t.Errorf("bad status: %d, expected: %d - %s", res.StatusCode, test.ExpectStatusCode, body)
			continue Tests
		}

		for key, val := range test.ExpectHeaders {
			if got := res.Header.Get(key); got != val {
				t.Errorf("expected \"%s: %s\" header, but got: \"%s: %s\"", key, val, key, got)
				continue Tests
			}
		}

		if test.MatchLocation != nil {
			location := res.Header.Get("Location")
			if !test.MatchLocation.MatchString(location) {
				t.Errorf("location \"%s\" did not match: \"%s\"", location, test.MatchLocation.String())
				continue Tests
			}
		}
	}
}
