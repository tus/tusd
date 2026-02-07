package e2e_test

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

var toxiClient *localProxyClient
var TUSD_BINARY string
var TUSD_ENDPOINT_RE = regexp.MustCompile(`You can now upload files to: (https?://([^/]+)/\S*)`)

func TestMain(m *testing.M) {
	// Fetch path to compiled tusd binary
	TUSD_BINARY = os.Getenv("TUSD_BINARY")
	if TUSD_BINARY == "" {
		fmt.Println(`The TUSD_BINARY environment variable is missing. It must to the location of a compiled tusd binary and can be obtained by running:
	export TUSD_BINARY=$PWD/tusd
	go build -o $TUSD_BINARY cmd/tusd/main.go`)
		os.Exit(1)
	}

	toxiClient = newLocalProxyClient()

	// Run actual tests
	exitVal := m.Run()

	os.Exit(exitVal)
}

// TestSuccessfulUpload tests that tusd can perform a single upload
// from actual HTTP requests.
func TestSuccessfulUpload(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	endpoint, _, _ := spawnTusd(ctx, t)

	data := bytes.NewBufferString("hello world")
	length := data.Len()

	req, err := http.NewRequest("POST", endpoint, nil)
	require.NoError(t, err)

	req.Header.Add("Tus-Resumable", "1.0.0")
	req.Header.Add("Upload-Length", strconv.Itoa(length))

	res, err := http.DefaultClient.Do(req)
	require.NoError(t, err)

	if res.StatusCode != http.StatusCreated {
		t.Fatal("invalid response code")
	}

	uploadUrl := res.Header.Get("Location")

	req, err = http.NewRequest("PATCH", uploadUrl, data)
	require.NoError(t, err)

	req.Header.Add("Tus-Resumable", "1.0.0")
	req.Header.Add("Upload-Offset", "0")
	req.Header.Add("Content-Type", "application/offset+octet-stream")

	res, err = http.DefaultClient.Do(req)
	require.NoError(t, err)

	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("invalid response code %d", res.StatusCode)
	}

	offset := res.Header.Get("Upload-Offset")
	if offset != strconv.Itoa(length) {
		t.Fatalf("invalid offset %s", offset)
	}
}

// TestNetworkReadTimeout tests that tusd correctly stops a request if no
// data has been received for the specified timeout. All data until this timeout
// should be stored, however.
func TestNetworkReadTimeout(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// We configure tusd with a read timeout of 5s, meaning that if no data
	// is received for 5s, it terminates the connection.
	_, addr, _ := spawnTusd(ctx, t, "-network-timeout=5s")

	proxy, _ := toxiClient.CreateProxy("tusd_"+t.Name(), "", addr)
	defer proxy.Delete()

	// We limit the upstream connection to tusd to 5KB/s. The downstream connection
	// from tusd is not limited.
	proxy.AddToxic("", "bandwidth", "upstream", 1, map[string]any{
		"rate": 5,
	})

	// Endpoint address points to the network proxy.
	endpoint := "http://" + proxy.Listen + "/files/"

	// We tell tusd to create a 50KB upload, but only upload 10KB of data.
	payloadLength := 10 * 1024
	uploadLength := 50 * 1024
	data := make([]byte, payloadLength)
	_, err := rand.Read(data)
	require.NoError(t, err)

	// Create upload
	req, err := http.NewRequest("POST", endpoint, nil)
	require.NoError(t, err)

	req.Header.Add("Tus-Resumable", "1.0.0")
	req.Header.Add("Upload-Length", strconv.Itoa(uploadLength))

	res, err := http.DefaultClient.Do(req)
	require.NoError(t, err)

	if res.StatusCode != http.StatusCreated {
		t.Fatal("invalid response code")
	}

	uploadUrl := res.Header.Get("Location")

	// We write data using a pipe instead of bytes.Reader (or similar) to better simulate
	// a network interruption. The writer is never closed and tusd does not
	// know that we won't be sending the entire upload here.
	// The write must happen in another goroutine because it waits for a suitable read.
	reader, writer := io.Pipe()
	go writer.Write(data)

	// Begin uploading data. The 10KB are transmitted completely after 2s, after which no
	// more data is received by tusd. The TCP connection stays open.
	req, err = http.NewRequest("PATCH", uploadUrl, reader)
	require.NoError(t, err)

	req.Header.Add("Tus-Resumable", "1.0.0")
	req.Header.Add("Upload-Offset", "0")
	req.Header.Add("Content-Type", "application/offset+octet-stream")

	start := time.Now()
	res, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()
	duration := time.Since(start)

	// Assert the response to see if tusd correctly emitted a timeout.
	// In reality, clients may often not receive this message due to network issues.
	if res.StatusCode != http.StatusInternalServerError {
		t.Fatalf("invalid response code %d", res.StatusCode)
	}

	body, err := io.ReadAll(res.Body)
	require.NoError(t, err)

	if !strings.Contains(string(body), "ERR_READ_TIMEOUT") {
		t.Fatalf("invalid response body %s", string(body))
	}

	// Send HEAD request to fetch offset
	req, err = http.NewRequest("HEAD", uploadUrl, nil)
	require.NoError(t, err)

	req.Header.Add("Tus-Resumable", "1.0.0")

	res, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()

	offset, err := strconv.Atoi(res.Header.Get("Upload-Offset"))
	require.NoError(t, err)

	// Data was allowed to flow for 2s at 5KB/s, so we should have
	// uploaded approximately 10KB.
	require.InDelta(t, 10_000, offset, 1_000, "invalid offset %d", offset)

	// Data was allowed to flow for 2s and tusd is configured to time
	// out after 5s, so the entire request should have ran for 7s.
	require.InDelta(t, 7*time.Second, duration, float64(7*time.Second)*0.2, "invalid request duration %v", duration)
}

// TestUnexpectedNetworkClose tests that tusd correctly saves the transmitted data
// if the client connection gets interrupted unexpectedly during the upload.
func TestUnexpectedNetworkClose(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	_, addr, _ := spawnTusd(ctx, t)

	proxy, _ := toxiClient.CreateProxy("tusd_"+t.Name(), "", addr)
	defer proxy.Delete()

	// We limit the upstream connection to tusd to 5KB/s. The downstream connection
	// from tusd is not limited. The upstream connection will be closed after sending
	// 10KB.
	proxy.AddToxic("", "bandwidth", "upstream", 1, map[string]any{
		"rate": 5,
	})
	proxy.AddToxic("", "limit_data", "upstream", 1, map[string]any{
		"bytes": 10_000,
	})

	// Endpoint address points to the network proxy.
	endpoint := "http://" + proxy.Listen + "/files/"

	// 50KB of random upload data
	length := 50 * 1024
	data := make([]byte, length)
	_, err := rand.Read(data)
	require.NoError(t, err)

	// Create upload
	req, err := http.NewRequest("POST", endpoint, nil)
	require.NoError(t, err)

	req.Header.Add("Tus-Resumable", "1.0.0")
	req.Header.Add("Upload-Length", strconv.Itoa(length))

	res, err := http.DefaultClient.Do(req)
	require.NoError(t, err)

	if res.StatusCode != http.StatusCreated {
		t.Fatal("invalid response code")
	}

	uploadUrl := res.Header.Get("Location")

	req, err = http.NewRequest("PATCH", uploadUrl, bytes.NewReader(data))
	require.NoError(t, err)

	req.Header.Add("Tus-Resumable", "1.0.0")
	req.Header.Add("Upload-Offset", "0")
	req.Header.Add("Content-Type", "application/offset+octet-stream")

	// Send the PATCH request. The connection will be closed by the proxy,
	// so we get an EOF error here.
	start := time.Now()
	_, err = http.DefaultClient.Do(req)
	require.Error(t, err)
	if !isExpectedNetworkCloseError(err) {
		t.Fatalf("unexpected error %s", err)
	}
	duration := time.Since(start)

	// Send HEAD request to fetch offset
	req, err = http.NewRequest("HEAD", uploadUrl, nil)
	require.NoError(t, err)

	req.Header.Add("Tus-Resumable", "1.0.0")

	res, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()

	offset, err := strconv.Atoi(res.Header.Get("Upload-Offset"))
	require.NoError(t, err)

	// 10KB were allowed before the proxy cut the connection. Accounting
	// the overhead of HTTP request, tusd should have received about 10KB.
	require.InDelta(t, 10_000, offset, 1_000, "invalid offset %d", offset)

	// Data was allowed to flow for 2s.
	require.InDelta(t, 2*time.Second, duration, float64(2*time.Second)*0.2, "invalid request duration %v", duration)
}

// TestUnexpectedNetworkReset tests that tusd correctly saves the transmitted data
// if the client connection gets interrupted unexpectedly by a TCP RST.
func TestUnexpectedNetworkReset(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	endpoint, addr, _ := spawnTusd(ctx, t)

	// We don't use the proxy here because we have to control the TCP RST
	// flag directly.

	// We create an upload of 10KB, but only provide 5KB before cutting the connection.
	uploadLength := 10 * 1024
	payloadLength := 5 * 1024
	data := make([]byte, payloadLength)
	_, err := rand.Read(data)
	require.NoError(t, err)

	// Create upload
	req, err := http.NewRequest("POST", endpoint, nil)
	require.NoError(t, err)

	req.Header.Add("Tus-Resumable", "1.0.0")
	req.Header.Add("Upload-Length", strconv.Itoa(uploadLength))

	res, err := http.DefaultClient.Do(req)
	require.NoError(t, err)

	if res.StatusCode != http.StatusCreated {
		t.Fatal("invalid response code")
	}

	uploadUrlStr := res.Header.Get("Location")
	uploadUrl, err := url.Parse(uploadUrlStr)
	require.NoError(t, err)

	// Send upload data via a PATCH request. We directly open a TCP socket and write the HTTP
	// request manually because it allows us to use SetLinger directly and we can send a smaller
	// body than advertised in the Content-Length header.
	conn, err := net.Dial("tcp", addr)
	require.NoError(t, err)

	// SetLinger(0) causes a RST to be sent instead of a normal FIN handshake.
	tcpConn := conn.(*net.TCPConn)
	if err := tcpConn.SetLinger(0); err != nil {
		t.Fatal(err)
	}

	if _, err := fmt.Fprintf(conn, `PATCH %s HTTP/1.1
Host: %s
Content-Length: %d
Content-Type: application/offset+octet-stream
Tus-Resumable: 1.0.0
Upload-Offset: 0

`, uploadUrl.Path, uploadUrl.Host, uploadLength); err != nil {
		t.Fatal(err)
	}

	// Only write the 5KB of data
	if _, err := conn.Write(data); err != nil {
		t.Fatal(err)
	}

	<-time.After(100 * time.Millisecond)

	// Close the connection using RST, thanks to SetLinger.
	if err := tcpConn.Close(); err != nil {
		t.Fatal(err)
	}

	<-time.After(100 * time.Millisecond)

	// Send HEAD request to fetch offset
	req, err = http.NewRequest("HEAD", uploadUrlStr, nil)
	require.NoError(t, err)

	req.Header.Add("Tus-Resumable", "1.0.0")

	res, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()

	offset, err := strconv.Atoi(res.Header.Get("Upload-Offset"))
	require.NoError(t, err)

	// 5KB were transmitted, all of which should be safed.
	require.InDelta(t, payloadLength, offset, float64(payloadLength)*0.1, "invalid offset %d", offset)
}

// TestLockRelease asserts that an incoming request will cause any ongoing request
// for the same upload resource to be closed quickly and cleanly.
func TestLockRelease(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// We configure tusd with low poll intervals for the filelocker to get
	// a quick test run and more predictable results
	_, addr, _ := spawnTusd(ctx, t, "-filelock-holder-poll-interval=1s", "-filelock-acquirer-poll-interval=1s")

	proxy, _ := toxiClient.CreateProxy("tusd_"+t.Name(), "", addr)
	defer proxy.Delete()

	// We limit the upstream connection to tusd to 5KB/s. The downstream connection
	// from tusd is not limited.
	proxy.AddToxic("", "bandwidth", "upstream", 1, map[string]any{
		"rate": 5,
	})

	// Endpoint address points to the network proxy.
	endpoint := "http://" + proxy.Listen + "/files/"

	// 50KB of random upload data
	length := 50 * 1024
	data := make([]byte, length)
	_, err := rand.Read(data)
	require.NoError(t, err)

	// Create upload
	postReq, err := http.NewRequest("POST", endpoint, nil)
	require.NoError(t, err)

	postReq.Header.Add("Tus-Resumable", "1.0.0")
	postReq.Header.Add("Upload-Length", strconv.Itoa(length))

	postRes, err := http.DefaultClient.Do(postReq)
	require.NoError(t, err)

	if postRes.StatusCode != http.StatusCreated {
		t.Fatal("invalid response code")
	}

	uploadUrl := postRes.Header.Get("Location")

	// Begin the upload
	patchReq, err := http.NewRequest("PATCH", uploadUrl, bytes.NewReader(data))
	require.NoError(t, err)

	patchReq.Header.Add("Tus-Resumable", "1.0.0")
	patchReq.Header.Add("Upload-Offset", "0")
	patchReq.Header.Add("Content-Type", "application/offset+octet-stream")

	headResChan := make(chan *http.Response, 1)
	headErrChan := make(chan error, 1)

	go func() {
		// After 2s, we send a HEAD request to simulate that another client
		// is trying to resume the upload
		<-time.After(2 * time.Second)

		headReq, err := http.NewRequest("HEAD", uploadUrl, nil)
		if err != nil {
			close(headResChan)
			headErrChan <- err
			return
		}

		headReq.Header.Add("Tus-Resumable", "1.0.0")

		headRes, err := http.DefaultClient.Do(headReq)
		if err != nil {
			close(headResChan)
			headErrChan <- err
			return
		}
		defer headRes.Body.Close()

		headResChan <- headRes
		close(headErrChan)
	}()

	start := time.Now()
	patchRes, err := http.DefaultClient.Do(patchReq)
	require.NoError(t, err)
	defer patchRes.Body.Close()

	// Assert the response to see if tusd correctly emitted an interruption message.
	if patchRes.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid response code %d", patchRes.StatusCode)
	}

	body, err := io.ReadAll(patchRes.Body)
	require.NoError(t, err)

	if !strings.Contains(string(body), "ERR_UPLOAD_INTERRUPTED") {
		t.Fatalf("invalid response body %s", string(body))
	}

	// Wait for the HEAD response and assert its response
	headRes := <-headResChan
	err = <-headErrChan
	require.NoError(t, err)

	if headRes.StatusCode != http.StatusOK {
		t.Fatalf("invalid response code %d", headRes.StatusCode)
	}

	offset, err := strconv.Atoi(headRes.Header.Get("Upload-Offset"))
	require.NoError(t, err)

	// Data was allowed to flow for 2s at 5KB/s, so we should have
	// uploaded approximately 10KB.
	require.InDelta(t, 10_000, offset, 1_000, "invalid offset %d", offset)

	// The interrupting request is sent after 2s, but with the poll intervals it might
	// take some more time for the requests to be finished, so the duration should be
	// 3s +/- 1s.
	duration := time.Since(start)
	require.InDelta(t, 3*time.Second, duration, float64(3*time.Second)*0.3, "invalid request duration %v", duration)
}

// TestUploadLengthExceeded asserts that uploading appending requests are limited to
// the length specified in the upload. If more data is transmitted, tusd just ignores
// the remaining data.
func TestUploadLengthExceeded(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	_, addr, _ := spawnTusd(ctx, t)

	proxy, _ := toxiClient.CreateProxy("tusd_"+t.Name(), "", addr)
	defer proxy.Delete()

	// We limit the upstream connection to tusd to 5KB/s. The downstream connection
	// from tusd is not limited.
	proxy.AddToxic("", "bandwidth", "upstream", 1, map[string]any{
		"rate": 5,
	})

	// Endpoint address points to the network proxy.
	endpoint := "http://" + proxy.Listen + "/files/"

	// We specify an upload length of 10KB, but supply 50KB of random upload data.
	uploadLength := 10 * 1024
	payloadLength := 50 * 1024
	data := make([]byte, payloadLength)
	_, err := rand.Read(data)
	require.NoError(t, err)

	// Create upload
	req, err := http.NewRequest("POST", endpoint, nil)
	require.NoError(t, err)

	req.Header.Add("Tus-Resumable", "1.0.0")
	req.Header.Add("Upload-Length", strconv.Itoa(uploadLength))

	res, err := http.DefaultClient.Do(req)
	require.NoError(t, err)

	if res.StatusCode != http.StatusCreated {
		t.Fatal("invalid response code")
	}

	uploadUrl := res.Header.Get("Location")

	req, err = http.NewRequest("PATCH", uploadUrl, bytes.NewReader(data))
	require.NoError(t, err)

	req.Header.Add("Tus-Resumable", "1.0.0")
	req.Header.Add("Upload-Offset", "0")
	req.Header.Add("Content-Type", "application/offset+octet-stream")

	// Note: This is important! By default, http.NewRequest will inspect the body and fill
	// ContentLength automatically. This causes the Content-Length header to be set. However,
	// in this case, we want to test how tusd behaves without a pre-known request body size.
	// But setting it to -1, we do not use Content-Length but Transfer-Encoding: chunked, so
	// tusd does not know the request size upfront.
	req.ContentLength = -1

	start := time.Now()
	res, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()
	duration := time.Since(start)

	if res.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("invalid response code %d", res.StatusCode)
	}

	body, err := io.ReadAll(res.Body)
	require.NoError(t, err)

	if !strings.Contains(string(body), "ERR_UPLOAD_SIZE_EXCEEDED") {
		t.Fatalf("invalid response body %s", string(body))
	}

	// Send HEAD request to fetch offset
	req, err = http.NewRequest("HEAD", uploadUrl, nil)
	require.NoError(t, err)

	req.Header.Add("Tus-Resumable", "1.0.0")

	res, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()

	offset, err := strconv.Atoi(res.Header.Get("Upload-Offset"))
	require.NoError(t, err)

	// tusd must only read the amount specified in Upload-Length.
	if offset != uploadLength {
		t.Fatalf("invalid offset %d", offset)
	}

	// The request should be stopped immediately after 10KB have been transmitted instead of waiting for
	// the entire request body. With 5KB/s, that is 2s.
	require.InDelta(t, 2*time.Second, duration, float64(2*time.Second)*0.2, "invalid request duration %v", duration)
}

// TestSuccessfulUpload asserts that ongoing upload requests get properly
// closed when the hooks instruct tusd to stop the upload.
func TestStopUpload(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start a hook server that always instructs tusd to stop the upload.
	hookServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"StopUpload":true}`))
	}))
	defer hookServer.Close()

	_, addr, _ := spawnTusd(ctx, t, "-hooks-http", hookServer.URL, "-hooks-enabled-events=post-receive", "-progress-hooks-interval=3s")

	proxy, _ := toxiClient.CreateProxy("tusd_"+t.Name(), "", addr)
	defer proxy.Delete()

	// We limit the upstream connection to tusd to 5KB/s. The downstream connection
	// from tusd is not limited.
	proxy.AddToxic("", "bandwidth", "upstream", 1, map[string]any{
		"rate": 5,
	})

	// Endpoint address points to the network proxy.
	endpoint := "http://" + proxy.Listen + "/files/"

	// We specify an upload length of 50KB.
	uploadLength := 50 * 1024
	data := make([]byte, uploadLength)
	_, err := rand.Read(data)
	require.NoError(t, err)

	// Create upload
	req, err := http.NewRequest("POST", endpoint, nil)
	require.NoError(t, err)

	req.Header.Add("Tus-Resumable", "1.0.0")
	req.Header.Add("Upload-Length", strconv.Itoa(uploadLength))

	res, err := http.DefaultClient.Do(req)
	require.NoError(t, err)

	if res.StatusCode != http.StatusCreated {
		t.Fatal("invalid response code")
	}

	uploadUrl := res.Header.Get("Location")

	req, err = http.NewRequest("PATCH", uploadUrl, bytes.NewReader(data))
	require.NoError(t, err)

	req.Header.Add("Tus-Resumable", "1.0.0")
	req.Header.Add("Upload-Offset", "0")
	req.Header.Add("Content-Type", "application/offset+octet-stream")

	start := time.Now()
	res, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()
	duration := time.Since(start)

	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid response code %d", res.StatusCode)
	}

	body, err := io.ReadAll(res.Body)
	require.NoError(t, err)

	if !strings.Contains(string(body), "ERR_UPLOAD_STOPPED") {
		t.Fatalf("invalid response body %s", string(body))
	}

	// Send HEAD request to check if upload was terminated
	req, err = http.NewRequest("HEAD", uploadUrl, nil)
	require.NoError(t, err)

	req.Header.Add("Tus-Resumable", "1.0.0")

	res, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()

	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("invalid response code %d", res.StatusCode)
	}

	// The first post-receive hook is sent after 3s (due to the progress-hooks-interval flag).
	// The upload should then be quickly stoppped and terminated.
	require.InDelta(t, 3*time.Second, duration, float64(3*time.Second)*0.3, "invalid request duration %v", duration)
}

func spawnTusd(ctx context.Context, t *testing.T, args ...string) (endpoint string, address string, cmd *exec.Cmd) {
	t.Helper()

	args = append([]string{"-port=0"}, args...)
	cmd = exec.CommandContext(ctx, TUSD_BINARY, args...)
	// Note: Leave stderr alone. It is not a good idea to connect the
	// child's output to the test's output because this can lead to deadlocks.
	// In Go <1.21, tests can just hang forever. In Go >=1.21, it will fail
	// with an error. See https://github.com/golang/go/issues/24050
	// cmd.Stderr = os.Stderr

	stdout, err := cmd.StdoutPipe()
	require.NoError(t, err, "failed to get stdout pipe")

	// Ensure that stdout is closed, when child process is stopped.
	cmd.Cancel = func() error {
		stdout.Close()
		return cmd.Process.Kill()
	}

	require.NoError(t, cmd.Start(), "failed to start tusd")

	scanner := bufio.NewScanner(stdout)

	for scanner.Scan() {
		match := TUSD_ENDPOINT_RE.FindStringSubmatch(scanner.Text())
		if match != nil {
			endpoint = match[1]
			address = match[2]

			go func() {
				for scanner.Scan() {
					fmt.Println(scanner.Text()) // Println will add back the final '\n'
				}
			}()
			return
		}
	}
	require.NoError(t, scanner.Err(), "failed to scan output")

	require.FailNow(t, "failed to parse tusd endpoint from tusd output")
	return
}

func isExpectedNetworkCloseError(err error) bool {
	if err == nil {
		return false
	}

	if errors.Is(err, io.EOF) {
		return true
	}

	errMsg := strings.ToLower(err.Error())

	// Different OS/network stacks surface different text for abrupt socket closes.
	return strings.Contains(errMsg, "connection reset") ||
		strings.Contains(errMsg, "forcibly closed by the remote host")
}
