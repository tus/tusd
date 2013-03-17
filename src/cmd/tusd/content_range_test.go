package main

import (
	"regexp"
	"testing"
)

var ContentRangeTests = []struct {
	s    string
	want contentRange
	err  string
}{
	{s: "bytes 0-5/100", want: contentRange{Start: 0, End: 5, Size: 100}},
	{s: "bytes 5-20/30", want: contentRange{Start: 5, End: 20, Size: 30}},
	{s: "bytes */100", want: contentRange{Start: 0, End: -1, Size: 100}},
	{s: "bytes 5-20/*", want: contentRange{Start: 5, End: 20, Size: -1}},
	{s: "bytes */*", want: contentRange{Start: 0, End: -1, Size: -1}},
	{s: "bytes 0-2147483647/2147483648", want: contentRange{Start: 0, End: 2147483647, Size: 2147483648}},
	{s: "bytes 5-20", err: "invalid"},
	{s: "bytes 5-5/100", err: "invalid"},
	{s: "bytes 5-4/100", err: "invalid"},
	{s: "bytes ", err: "invalid"},
	{s: "", err: "invalid"},
}

func TestParseContentRange(t *testing.T) {
	for _, test := range ContentRangeTests {
		t.Logf("testing: %s", test.s)

		r, err := parseContentRange(test.s)
		if test.err != "" {
			if err == nil {
				t.Errorf("got no error, but expected: %s", test.err)
				continue
			}

			errMatch := regexp.MustCompile(test.err)
			if !errMatch.MatchString(err.Error()) {
				t.Errorf("unexpected error: %s, wanted: %s", err, test.err)
				continue
			}

			continue
		} else if err != nil {
			t.Errorf("unexpected error: %s, wanted: %+v", err, test.want)
			continue
		}

		if *r != test.want {
			t.Errorf("got: %+v, wanted: %+v", r, test.want)
			continue
		}
	}
}
