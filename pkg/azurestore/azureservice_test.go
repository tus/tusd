package azurestore_test

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/blockblob"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tus/tusd/v2/pkg/azurestore"
	"github.com/tus/tusd/v2/pkg/handler"
)

// Azure Blob Storage SDK has no interface types, so we can't easily mock the SDK itself.
// We could define a wrapper interface and mock this, but that would be a lot of boilerplate.
// Instead, we inject a custom fakeTransport into the SDK's HTTP client, which allows us to control the responses returned
// This has the added benefit that we get the real Azure SDK error handling logic for free, since the SDK will parse
// the response and return an azcore.ResponseError as appropriate.

// fakeTransport implements policy.Transporter, returning a canned response/error.
type fakeTransport struct {
	resp    *http.Response
	err     error
	lastReq *http.Request
}

func (f *fakeTransport) Do(req *http.Request) (*http.Response, error) {
	f.lastReq = req
	if f.resp != nil {
		f.resp.Request = req // azcore error/response handling expects resp.Request set
	}
	return f.resp, f.err
}

// failingTransport fails the test if it is ever called. Used to prove that
// ServeContent bails out before performing a download.
type failingTransport struct {
	t *testing.T
}

func (f *failingTransport) Do(req *http.Request) (*http.Response, error) {
	f.t.Fatal("transport.Do should not have been called")
	return nil, nil
}

// errorResponseWriter is an http.ResponseWriter whose Write always fails. It is
// used to simulate an io.Copy failure while ServeContent streams the blob body to
type errorResponseWriter struct {
	header http.Header
	status int
}

func (w *errorResponseWriter) Header() http.Header {
	if w.header == nil {
		w.header = http.Header{}
	}
	return w.header
}

func (w *errorResponseWriter) WriteHeader(status int) { w.status = status }

func (w *errorResponseWriter) Write([]byte) (int, error) {
	return 0, errors.New("simulated write failure")
}

// newTestBlockBlob builds a real BlockBlob whose client routes through the given
// transport. Retries are disabled so the transport is invoked exactly once per
// request, keeping tests deterministic.
func newTestBlockBlob(t *testing.T, tr policy.Transporter) *azurestore.BlockBlob {
	t.Helper()
	client, err := blockblob.NewClientWithNoCredential(
		"https://azure.blob.core.windows.net/container/blob",
		&blockblob.ClientOptions{ClientOptions: azcore.ClientOptions{
			Transport: tr,
			Retry:     policy.RetryOptions{MaxRetries: -1}, // disable retries
		}},
	)
	require.NoError(t, err)
	return &azurestore.BlockBlob{BlobClient: client}
}

// makeResponse builds an *http.Response with the given status, headers, and body.
func makeResponse(status int, headers map[string]string, body string) *http.Response {
	h := http.Header{}
	for k, v := range headers {
		h.Set(k, v)
	}
	return &http.Response{
		StatusCode: status,
		Status:     fmt.Sprintf("%d %s", status, http.StatusText(status)),
		Header:     h,
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func TestServeContentFullDownload(t *testing.T) {
	assert := assert.New(t)

	body := "test content"
	md5b64 := base64.StdEncoding.EncodeToString([]byte(body))
	lastModified := time.Date(2026, time.June, 22, 10, 0, 0, 0, time.UTC)

	tr := &fakeTransport{resp: makeResponse(http.StatusOK, map[string]string{
		"Content-Type":   "text/plain+test",
		"Content-Length": "12",
		"Accept-Ranges":  "bytes",
		"ETag":           `"abc"`,
		// Additional pass-through headers
		"Cache-Control":       "max-age=3600",
		"Content-Disposition": "attachment; filename=\"f.txt\"",
		"Content-Encoding":    "gzip",
		"Content-Language":    "en-US",
		"Content-MD5":         md5b64,
		"Last-Modified":       lastModified.Format(http.TimeFormat),
	}, body)}

	blob := newTestBlockBlob(t, tr)
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()

	err := blob.ServeContent(context.Background(), rec, req)

	assert.Nil(err)
	assert.Equal(http.StatusOK, rec.Code)
	assert.Equal(body, rec.Body.String())
	assert.Equal("text/plain+test", rec.Header().Get("Content-Type"))
	assert.Equal("12", rec.Header().Get("Content-Length"))
	assert.Equal("bytes", rec.Header().Get("Accept-Ranges"))
	assert.Equal(`"abc"`, rec.Header().Get("ETag"))
	assert.Equal("max-age=3600", rec.Header().Get("Cache-Control"))

	assert.Equal("attachment; filename=\"f.txt\"", rec.Header().Get("Content-Disposition"))
	assert.Equal("gzip", rec.Header().Get("Content-Encoding"))
	assert.Equal("en-US", rec.Header().Get("Content-Language"))
	assert.Equal(md5b64, rec.Header().Get("Content-MD5"))
	assert.Equal(lastModified.Format(http.TimeFormat), rec.Header().Get("Last-Modified"))
}

func TestServeContentRangeRequest(t *testing.T) {
	assert := assert.New(t)

	body := "test"
	tr := &fakeTransport{resp: makeResponse(http.StatusPartialContent, map[string]string{
		"Content-Type":   "text/plain",
		"Content-Length": "4",
		"Content-Range":  "bytes 0-3/12",
		"Accept-Ranges":  "bytes",
	}, body)}

	blob := newTestBlockBlob(t, tr)
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Range", "bytes=0-3")
	rec := httptest.NewRecorder()

	err := blob.ServeContent(context.Background(), rec, req)

	assert.Nil(err)
	assert.Equal(http.StatusPartialContent, rec.Code)
	assert.Equal(body, rec.Body.String())
	assert.Equal("bytes 0-3/12", rec.Header().Get("Content-Range"))
	// Confirm ParseDownloadOptions plumbed the range into the request to Azure.
	// The SDK writes the range using the non-canonical "x-ms-range" map key, so a
	// canonicalizing Header.Get would miss it; scan the raw map instead.
	require.NotNil(t, tr.lastReq)
	var rangeHeader string
	for k, v := range tr.lastReq.Header {
		if strings.EqualFold(k, "x-ms-range") {
			rangeHeader = strings.Join(v, ",")
		}
	}
	assert.Contains(rangeHeader, "bytes=0-3")
}

func TestServeContentRangeNotSatisfiable(t *testing.T) {
	assert := assert.New(t)

	date := time.Now().UTC().Format(http.TimeFormat)
	tr := &fakeTransport{resp: makeResponse(http.StatusRequestedRangeNotSatisfiable, map[string]string{
		"Content-Range":   "bytes */12",
		"x-ms-error-code": "InvalidRange",
		"x-ms-request-id": "req-1",
		"Date":            date,
	}, "")}

	blob := newTestBlockBlob(t, tr)
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Range", "bytes=100-200")
	rec := httptest.NewRecorder()

	err := blob.ServeContent(context.Background(), rec, req)

	assert.Nil(err)
	assert.Equal(http.StatusRequestedRangeNotSatisfiable, rec.Code)
	assert.Empty(rec.Body.String())
	assert.Equal("bytes */12", rec.Header().Get("Content-Range"))
	assert.Equal("InvalidRange", rec.Header().Get("X-Ms-Error-Code"))
	assert.Equal("req-1", rec.Header().Get("X-Ms-Request-Id"))
	assert.Equal(date, rec.Header().Get("Date"))
}

func TestServeContentPreconditionFailed(t *testing.T) {
	assert := assert.New(t)

	tr := &fakeTransport{resp: makeResponse(http.StatusPreconditionFailed, map[string]string{
		"x-ms-error-code": "ConditionNotMet",
		"x-ms-request-id": "req-2",
	}, "")}

	blob := newTestBlockBlob(t, tr)
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("If-Match", `"wrong-etag"`)
	rec := httptest.NewRecorder()

	err := blob.ServeContent(context.Background(), rec, req)

	assert.Nil(err)
	assert.Equal(http.StatusPreconditionFailed, rec.Code)
	assert.Empty(rec.Body.String())
	// Assert that headers helpful for debugging are passed through. If these aren't present, this wouldn't be a problem.
	assert.Equal("ConditionNotMet", rec.Header().Get("X-Ms-Error-Code"))
	assert.Equal("req-2", rec.Header().Get("X-Ms-Request-Id"))
}

func TestServeContentNotFound(t *testing.T) {
	assert := assert.New(t)

	tr := &fakeTransport{resp: makeResponse(http.StatusNotFound, map[string]string{
		"x-ms-error-code": "BlobNotFound",
		"x-ms-request-id": "req-3",
	}, "")}

	blob := newTestBlockBlob(t, tr)
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	rec.Code = 0

	err := blob.ServeContent(context.Background(), rec, req)

	// A 404 is translated into handler.ErrNotFound.
	assert.Equal(handler.ErrNotFound, err)
	// ServeContent does not call WriteHeader when returning ErrNotFound
	assert.Equal(0, rec.Code)
}

func TestServeContentNotModified(t *testing.T) {
	assert := assert.New(t)

	// The Azure SDK only surfaces ErrorCode on a *success* status (200/206); any
	// non-success status becomes a *azcore.ResponseError instead. A conditional
	// GET that is not modified therefore arrives as a 200 carrying the
	// ConditionNotMet error code, which ServeContent maps to 304.
	tr := &fakeTransport{resp: makeResponse(http.StatusOK, map[string]string{
		"x-ms-error-code": "ConditionNotMet",
		"Content-Length":  "12",
	}, "")}

	blob := newTestBlockBlob(t, tr)
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("If-None-Match", `"abc"`)
	rec := httptest.NewRecorder()

	err := blob.ServeContent(context.Background(), rec, req)

	assert.Nil(err)
	assert.Equal(http.StatusNotModified, rec.Code)
	assert.Empty(rec.Body.String())
}

func TestServeContentErrorCodeInternalServerError(t *testing.T) {
	assert := assert.New(t)

	// A success response carrying an error code other than ConditionNotMet maps to
	// a 500 (see the default branch in ServeContent's ErrorCode handling).
	tr := &fakeTransport{resp: makeResponse(http.StatusOK, map[string]string{
		"x-ms-error-code": "InternalError",
		"Content-Length":  "12",
	}, "")}

	blob := newTestBlockBlob(t, tr)
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()

	err := blob.ServeContent(context.Background(), rec, req)

	assert.Nil(err)
	assert.Equal(http.StatusInternalServerError, rec.Code)
	assert.Empty(rec.Body.String())
}

func TestServeEmptyBlob(t *testing.T) {
	assert := assert.New(t)

	tr := &fakeTransport{resp: makeResponse(http.StatusOK, map[string]string{
		"Content-Length": "0",
	}, "")}

	blob := newTestBlockBlob(t, tr)
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()

	err := blob.ServeContent(context.Background(), rec, req)

	assert.Nil(err)
	assert.Equal(http.StatusOK, rec.Code)
	assert.Equal("0", rec.Header().Get("Content-Length"))
	assert.Empty(rec.Body.String())
}

// Content-Length should never be unset. This test is just for "coverage" and to prove that ServeContent
// doesn't panic if the header is missing.
func TestServeContentMissingContentLength(t *testing.T) {
	assert := assert.New(t)

	tr := &fakeTransport{resp: makeResponse(http.StatusOK, map[string]string{}, "")}

	blob := newTestBlockBlob(t, tr)
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()

	err := blob.ServeContent(context.Background(), rec, req)

	assert.Nil(err)
	assert.Equal(http.StatusOK, rec.Code)
	assert.Empty(rec.Body.String())
}

func TestServeContentUnhandledError(t *testing.T) {
	assert := assert.New(t)

	tr := &fakeTransport{resp: makeResponse(http.StatusInternalServerError, map[string]string{
		"x-ms-error-code": "InternalError",
	}, "")}

	blob := newTestBlockBlob(t, tr)
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	rec.Code = 0 // WriteHeader never called

	err := blob.ServeContent(context.Background(), rec, req)

	assert.NotNil(err)
	var respErr *azcore.ResponseError
	assert.True(errors.As(err, &respErr))
	assert.Equal(0, rec.Code)
}

func TestServeContentParseOptionsError(t *testing.T) {
	assert := assert.New(t)

	blob := newTestBlockBlob(t, &failingTransport{t: t})
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Range", "bytes=zZ-") // malformed range
	rec := httptest.NewRecorder()
	rec.Code = 0 // WriteHeader never called

	err := blob.ServeContent(context.Background(), rec, req)

	assert.NotNil(err)
	assert.Equal(0, rec.Code)
}

// Ensure io.Copy errors are returned to the caller
func TestServeContentCopyError(t *testing.T) {
	assert := assert.New(t)

	body := "test content" // 12 bytes
	tr := &fakeTransport{resp: makeResponse(http.StatusOK, map[string]string{
		"Content-Type":   "text/plain",
		"Content-Length": "12",
	}, body)}

	blob := newTestBlockBlob(t, tr)
	req := httptest.NewRequest("GET", "/", nil)
	w := &errorResponseWriter{}

	err := blob.ServeContent(context.Background(), w, req)

	assert.Error(err)
	assert.Contains(err.Error(), "simulated write failure")
}
