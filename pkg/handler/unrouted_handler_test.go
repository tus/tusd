package handler_test

import (
	"net/http"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"

	. "github.com/tus/tusd/pkg/handler"
)

func TestParseMetadataHeader(t *testing.T) {
	a := assert.New(t)

	md := ParseMetadataHeader("")
	a.Equal(md, map[string]string{})

	// Invalidly encoded values are ignored
	md = ParseMetadataHeader("k1 INVALID")
	a.Equal(md, map[string]string{})

	// If the same key occurs multiple times, the last one wins
	md = ParseMetadataHeader("k1 aGVsbG8=,k1 d29ybGQ=")
	a.Equal(md, map[string]string{
		"k1": "world",
	})

	// Empty values are mapped to an empty string
	md = ParseMetadataHeader("k1 aGVsbG8=, k2, k3 , k4 d29ybGQ=")
	a.Equal(md, map[string]string{
		"k1": "hello",
		"k2": "",
		"k3": "",
		"k4": "world",
	})
}

// no harm to copy some lines :D
var (
	reOriginalForwardedHost = regexp.MustCompile(`host=([^,]+)`) // unrouted_handler.go:24
	reModifiedForwardedHost = regexp.MustCompile(`host=([^;]+)`)
	reForwardedProto        = regexp.MustCompile(`proto=(https?)`)
)

const (
	hostToCheck  = "upload.example.tld"
	protoToCheck = "https"
)

func TestGetHostAndProto(t *testing.T) {

	a := assert.New(t)
	r, err := newRequest()
	if err != nil {
		t.Errorf("Error constructing test request : %v", err)
	}

	host, proto := getHostAndProtocol(r, true, false)
	a.Equal(hostToCheck, host)
	a.Equal(protoToCheck, proto)

}

func TestOriginalHostAndProto(t *testing.T) {
	a := assert.New(t)
	r, err := newRequest()
	if err != nil {
		t.Errorf("Error constructing test request : %v", err)
	}

	host, proto := getHostAndProtocol(r, true, true)
	a.Equal(hostToCheck, host)
	a.Equal(protoToCheck, proto)
}

// newRequest creates new test/dummy request
func newRequest() (*http.Request, error) {
	request, err := http.NewRequest(http.MethodGet, "https://upload.example.tld/files/", nil)
	if err != nil {
		return nil, err
	}

	request.Header.Set("X-Forwarded-Host", hostToCheck)
	request.Header.Set("X-Forwarded-Proto", protoToCheck)
	request.Header.Set("Forwarded", "for=192.168.10.112;host=upload.example.tld;proto=https;proto-version=")
	return request, nil
}

func getHostAndProtocol(r *http.Request, allowForwarded bool, shouldFail bool) (host, proto string) {
	if r.TLS != nil {
		proto = "https"
	} else {
		proto = "http"
	}

	host = r.Host

	if !allowForwarded {
		return
	}

	if h := r.Header.Get("X-Forwarded-Host"); h != "" {
		host = h
	}

	if h := r.Header.Get("X-Forwarded-Proto"); h == "http" || h == "https" {
		proto = h
	}

	if h := r.Header.Get("Forwarded"); h != "" {
		var hosts []string
		switch shouldFail {
		case true:
			hosts = reOriginalForwardedHost.FindStringSubmatch(h)
		default:
			hosts = reModifiedForwardedHost.FindStringSubmatch(h)
		}

		if len(hosts) == 2 {
			host = hosts[1]
		}

		if r := reForwardedProto.FindStringSubmatch(h); len(r) == 2 {
			proto = r[1]
		}
	}

	return
}
