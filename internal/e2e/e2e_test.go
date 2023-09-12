package e2e_test

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	toxiproxy_server "github.com/Shopify/toxiproxy/v2"
	toxiproxy "github.com/Shopify/toxiproxy/v2/client"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog"
	"golang.org/x/exp/constraints"
)

var toxiClient *toxiproxy.Client
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

	// Create a new toxiproxy server instance
	metrics := toxiproxy_server.NewMetricsContainer(prometheus.NewRegistry())
	logger := zerolog.New(os.Stderr).Level(zerolog.ErrorLevel)
	server := toxiproxy_server.NewServer(metrics, logger)

	addr := "localhost:8474"
	go func(server *toxiproxy_server.ApiServer, addr string) {
		if err := server.Listen(addr); err != nil {
			log.Fatalf("failed to start toxiproxy: %s", err)
		}
	}(server, addr)

	// Create a new toxiproxy client instance
	toxiClient = toxiproxy.NewClient(addr)

	// Run actual tests
	exitVal := m.Run()

	server.Shutdown()
	os.Exit(exitVal)
}

// TestSuccessfulUpload tests that tusd can perform a single upload
// from actual HTTP requests.
func TestSuccessfulUpload(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	endpoint, addr, _ := spawnTusd(ctx, t)
	fmt.Println(endpoint, addr)

	data := bytes.NewBufferString("hello world")
	length := data.Len()

	req, err := http.NewRequest("POST", endpoint, nil)
	if err != nil {
		t.Fatal(err)
	}

	req.Header.Add("Tus-Resumable", "1.0.0")
	req.Header.Add("Upload-Length", strconv.Itoa(length))

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}

	if res.StatusCode != http.StatusCreated {
		t.Fatal("invalid response code")
	}

	uploadUrl := res.Header.Get("Location")

	req, err = http.NewRequest("PATCH", uploadUrl, data)
	if err != nil {
		t.Fatal(err)
	}

	req.Header.Add("Tus-Resumable", "1.0.0")
	req.Header.Add("Upload-Offset", "0")
	req.Header.Add("Content-Type", "application/offset+octet-stream")

	res, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}

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
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// We configure tusd with a read timeout of 5s, meaning that if no data
	// is received for 5s, it terminates the connection.
	_, addr, _ := spawnTusd(ctx, t, "-network-timeout=5s")

	proxy, _ := toxiClient.CreateProxy("tusd_"+t.Name(), "", addr)
	defer proxy.Delete()

	// We limit the upstream connection to tusd to 5KB/s. The downstream connection
	// from tusd is not limited.
	proxy.AddToxic("", "bandwidth", "upstream", 1, toxiproxy.Attributes{
		"rate": 5,
	})

	// Endpoint address point to toxiproxy
	endpoint := "http://" + proxy.Listen + "/files/"

	// We tell tusd to create a 50KB upload, but only upload 10KB of data.
	payloadLength := 10 * 1024
	uploadLength := 50 * 1024
	data := make([]byte, payloadLength)
	_, err := rand.Read(data)
	if err != nil {
		t.Fatal(err)
	}

	// Create upload
	req, err := http.NewRequest("POST", endpoint, nil)
	if err != nil {
		t.Fatal(err)
	}

	req.Header.Add("Tus-Resumable", "1.0.0")
	req.Header.Add("Upload-Length", strconv.Itoa(uploadLength))

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}

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
	if err != nil {
		t.Fatal(err)
	}

	req.Header.Add("Tus-Resumable", "1.0.0")
	req.Header.Add("Upload-Offset", "0")
	req.Header.Add("Content-Type", "application/offset+octet-stream")

	start := time.Now()
	res, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	// Assert the response to see if tusd correctly emitted a timeout.
	// In reality, clients may often not receive this message due to network issues.
	if res.StatusCode != http.StatusInternalServerError {
		t.Fatalf("invalid response code %d", res.StatusCode)
	}

	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(string(body), "ERR_READ_TIMEOUT") {
		t.Fatalf("invalid response body %s", string(body))
	}

	// Send HEAD request to fetch offset
	req, err = http.NewRequest("HEAD", uploadUrl, nil)
	if err != nil {
		t.Fatal(err)
	}

	req.Header.Add("Tus-Resumable", "1.0.0")

	res, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	offset, err := strconv.Atoi(res.Header.Get("Upload-Offset"))
	if err != nil {
		t.Fatal(err)
	}

	// Data was allowed to flow for 2s at 5KB/s, so we should have
	// uploaded approximately 10KB.
	if !isApprox(offset, 10_000, 0.1) {
		t.Fatalf("invalid offset %d", offset)
	}

	// Data was allowed to flow for 2s and tusd is configured to time
	// out after 5s, so the entire request should have ran for 7s.
	duration := time.Since(start)
	if !isApprox(duration, 7*time.Second, 0.1) {
		t.Fatalf("invalid request duration %v", duration)
	}
}

// TestUnexpectedNetworkClose tests that tusd correctly saves the transmitted data
// if the client connection gets interrupted unexpectedly during the upload.
func TestUnexpectedNetworkClose(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	_, addr, _ := spawnTusd(ctx, t)

	proxy, _ := toxiClient.CreateProxy("tusd_"+t.Name(), "", addr)
	defer proxy.Delete()

	// We limit the upstream connection to tusd to 5KB/s. The downstream connection
	// from tusd is not limited. The upstream connection will be closed after sending
	// 10KB.
	proxy.AddToxic("", "bandwidth", "upstream", 1, toxiproxy.Attributes{
		"rate": 5,
	})
	proxy.AddToxic("", "limit_data", "upstream", 1, toxiproxy.Attributes{
		"bytes": 10_000,
	})

	// Endpoint address point to toxiproxy
	endpoint := "http://" + proxy.Listen + "/files/"

	// 50KB of random upload data
	length := 50 * 1024
	data := make([]byte, length)
	_, err := rand.Read(data)
	if err != nil {
		t.Fatal(err)
	}

	// Create upload
	req, err := http.NewRequest("POST", endpoint, nil)
	if err != nil {
		t.Fatal(err)
	}

	req.Header.Add("Tus-Resumable", "1.0.0")
	req.Header.Add("Upload-Length", strconv.Itoa(length))

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}

	if res.StatusCode != http.StatusCreated {
		t.Fatal("invalid response code")
	}

	uploadUrl := res.Header.Get("Location")

	req, err = http.NewRequest("PATCH", uploadUrl, bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}

	req.Header.Add("Tus-Resumable", "1.0.0")
	req.Header.Add("Upload-Offset", "0")
	req.Header.Add("Content-Type", "application/offset+octet-stream")

	// Send the PATCH request. The connection will be closed by the toxiproxy,
	// so we get an EOF error here.
	start := time.Now()
	_, err = http.DefaultClient.Do(req)
	if !errors.Is(err, io.EOF) {
		t.Fatalf("unexpected error %s", err)
	}

	// Send HEAD request to fetch offset
	req, err = http.NewRequest("HEAD", uploadUrl, nil)
	if err != nil {
		t.Fatal(err)
	}

	req.Header.Add("Tus-Resumable", "1.0.0")

	res, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	offset, err := strconv.Atoi(res.Header.Get("Upload-Offset"))
	if err != nil {
		t.Fatal(err)
	}

	// 10KB were allowed before toxiproxy cut the connection. Accounting
	// the overhead of HTTP request, tusd should have received about 10KB.
	if !isApprox(offset, 10_000, 0.1) {
		t.Fatalf("invalid offset %d", offset)
	}

	// Data was allowed to flow for 2s.
	duration := time.Since(start)
	if !isApprox(duration, 2*time.Second, 0.1) {
		t.Fatalf("invalid request duration %v", duration)
	}
}

// // TestUnexpectedNetworkReset tests that tusd correctly saves the transmitted data
// // if the client connection gets interrupted unexpectedly by a TCP RST.
// func TestUnexpectedNetworkReset(t *testing.T) {
// 	// Skip the test for now because the reset_peer toxic does not work as hoped.
// 	// It does not let any data through, but we need it to pass data through
// 	// until the connection is reset.
// 	// t.SkipNow()

// 	ctx, cancel := context.WithCancel(context.Background())
// 	defer cancel()

// 	endpoint, _, _, _ := spawnTusd(ctx, t)

// 	dialer := &net.Dialer{
// 		Timeout:   30 * time.Second,
// 		KeepAlive: 30 * time.Second,
// 	}
// 	httpTransport := &http.Transport{
// 		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
// 			c, err := dialer.DialContext(ctx, network, addr)
// 			if err != nil {
// 				return nil, err
// 			}

// 			tcpConn, ok := c.(*net.TCPConn)
// 			if !ok {
// 				panic("unable to cast into TCP connection")
// 			}

// 			tcpConn.SetLinger(0)

// 			go func() {
// 				<-time.After(2 * time.Second)
// 				tcpConn.Close()
// 			}()

// 			return c, nil
// 		},
// 		ForceAttemptHTTP2:     true,
// 		MaxIdleConns:          100,
// 		IdleConnTimeout:       90 * time.Second,
// 		TLSHandshakeTimeout:   10 * time.Second,
// 		ExpectContinueTimeout: 1 * time.Second,
// 	}
// 	httpClient := &http.Client{
// 		Transport: httpTransport,
// 	}

// 	// 10KB of random upload data
// 	length := 10 * 1024
// 	data := make([]byte, length/2)
// 	_, err := rand.Read(data)
// 	if err != nil {
// 		t.Fatal(err)
// 	}

// 	// Create upload
// 	req, err := http.NewRequest("POST", endpoint, nil)
// 	if err != nil {
// 		t.Fatal(err)
// 	}

// 	req.Header.Add("Tus-Resumable", "1.0.0")
// 	req.Header.Add("Upload-Length", strconv.Itoa(length))

// 	res, err := httpClient.Do(req)
// 	if err != nil {
// 		t.Fatal(err)
// 	}

// 	if res.StatusCode != http.StatusCreated {
// 		t.Fatal("invalid response code")
// 	}

// 	uploadUrl := res.Header.Get("Location")

// 	reader, writer := io.Pipe()
// 	go func() {
// 		writer.Write(data)
// 		<-time.After(3 * time.Second)
// 		writer.Close()
// 	}()
// 	req, err = http.NewRequest("PATCH", uploadUrl, reader)
// 	if err != nil {
// 		t.Fatal(err)
// 	}

// 	req.Header.Add("Tus-Resumable", "1.0.0")
// 	req.Header.Add("Upload-Offset", "0")
// 	req.Header.Add("Content-Type", "application/offset+octet-stream")

// 	// Send the PATCH request. The connection will be closed by the toxiproxy,
// 	// so we can an EOF error here.
// 	start := time.Now()
// 	_, err = httpClient.Do(req)
// 	fmt.Println("After patch")
// 	if err == nil || !strings.Contains(err.Error(), "connection reset by peer") {
// 		t.Fatalf("unexpected error %s", err)
// 	}

// 	// Send HEAD request to fetch offset
// 	req, err = http.NewRequest("HEAD", uploadUrl, nil)
// 	if err != nil {
// 		t.Fatal(err)
// 	}

// 	req.Header.Add("Tus-Resumable", "1.0.0")

// 	res, err = httpClient.Do(req)
// 	if err != nil {
// 		t.Fatal(err)
// 	}
// 	defer res.Body.Close()

// 	offset, err := strconv.Atoi(res.Header.Get("Upload-Offset"))
// 	if err != nil {
// 		t.Fatal(err)
// 	}

// 	// 10KB were allowed before toxiproxy cut the connection. Accounting
// 	// the overhead of HTTP request, tusd should have received about 10KB.
// 	if !isApprox(offset, 10_000, 0.1) {
// 		t.Fatalf("invalid offset %d", offset)
// 	}

// 	// Data was allowed to flow for 2s.
// 	duration := time.Since(start)
// 	if !isApprox(duration, 2*time.Second, 0.1) {
// 		t.Fatalf("invalid request duration %v", duration)
// 	}
// }

// TestLockRelease asserts that an incoming request will cause any ongoing request
// for the same upload resource to be closed quickly and cleanly.
func TestLockRelease(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// We configure tusd with low poll intervals for the filelocker to get
	// a quick test run and more predictable results
	_, addr, _ := spawnTusd(ctx, t, "-filelock-holder-poll-interval=1s", "-filelock-acquirer-poll-interval=1s")

	proxy, _ := toxiClient.CreateProxy("tusd_"+t.Name(), "", addr)
	defer proxy.Delete()

	// We limit the upstream connection to tusd to 5KB/s. The downstream connection
	// from tusd is not limited.
	proxy.AddToxic("", "bandwidth", "upstream", 1, toxiproxy.Attributes{
		"rate": 5,
	})

	// Endpoint address point to toxiproxy
	endpoint := "http://" + proxy.Listen + "/files/"

	// 50KB of random upload data
	length := 50 * 1024
	data := make([]byte, length)
	_, err := rand.Read(data)
	if err != nil {
		t.Fatal(err)
	}

	// Create upload
	postReq, err := http.NewRequest("POST", endpoint, nil)
	if err != nil {
		t.Fatal(err)
	}

	postReq.Header.Add("Tus-Resumable", "1.0.0")
	postReq.Header.Add("Upload-Length", strconv.Itoa(length))

	postRes, err := http.DefaultClient.Do(postReq)
	if err != nil {
		t.Fatal(err)
	}

	if postRes.StatusCode != http.StatusCreated {
		t.Fatal("invalid response code")
	}

	uploadUrl := postRes.Header.Get("Location")

	// Begin the upload
	patchReq, err := http.NewRequest("PATCH", uploadUrl, bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}

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
	if err != nil {
		t.Fatal(err)
	}
	defer patchRes.Body.Close()

	// Assert the response to see if tusd correctly emitted an interruption message.
	if patchRes.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid response code %d", patchRes.StatusCode)
	}

	body, err := io.ReadAll(patchRes.Body)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(string(body), "ERR_UPLOAD_INTERRUPTED") {
		t.Fatalf("invalid response body %s", string(body))
	}

	// Wait for the HEAD response and assert its response
	headRes := <-headResChan
	err = <-headErrChan
	if err != nil {
		t.Fatal(err)
	}

	if headRes.StatusCode != http.StatusOK {
		t.Fatalf("invalid response code %d", headRes.StatusCode)
	}

	offset, err := strconv.Atoi(headRes.Header.Get("Upload-Offset"))
	if err != nil {
		t.Fatal(err)
	}

	// Data was allowed to flow for 2s at 5KB/s, so we should have
	// uploaded approximately 10KB.
	if !isApprox(offset, 10_000, 0.1) {
		t.Fatalf("invalid offset %d", offset)
	}

	// The interrupting request is sent after 2s, but with the poll intervals it might
	// take some more time for the requests to be finished, so the duration should be
	// 3s +/- 1s.
	duration := time.Since(start)
	if !isApprox(duration, 3*time.Second, 0.3) {
		t.Fatalf("invalid request duration %v", duration)
	}
}

// TestShutdown asserts that tusd closes all ongoing upload requests and shuts down
// cleanly on its own when receiving a signal to stop.
func TestShutdown(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	_, addr, cmd := spawnTusd(ctx, t)

	proxy, _ := toxiClient.CreateProxy("tusd_"+t.Name(), "", addr)
	defer proxy.Delete()

	// We limit the upstream connection to tusd to 5KB/s. The downstream connection
	// from tusd is not limited.
	proxy.AddToxic("", "bandwidth", "upstream", 1, toxiproxy.Attributes{
		"rate": 5,
	})

	// Endpoint address point to toxiproxy
	endpoint := "http://" + proxy.Listen + "/files/"

	// 50KB of random upload data
	length := 50 * 1024
	data := make([]byte, length)
	_, err := rand.Read(data)
	if err != nil {
		t.Fatal(err)
	}

	// Create upload and send data in one request. We do not need the upload URL.
	req, err := http.NewRequest("POST", endpoint, bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}

	req.Header.Add("Tus-Resumable", "1.0.0")
	req.Header.Add("Upload-Length", strconv.Itoa(length))
	req.Header.Add("Content-Type", "application/offset+octet-stream")

	go func() {
		// After 2s, tell tusd to shut down.
		<-time.After(2 * time.Second)
		cmd.Process.Signal(os.Interrupt)
	}()

	start := time.Now()
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	// Assert the response to see if tusd correctly emitted the shutdown response.
	if res.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("invalid response code %d", res.StatusCode)
	}

	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(string(body), "ERR_SERVER_SHUTDOWN") {
		t.Fatalf("invalid response body %s", string(body))
	}

	// Wait until tusd exits on its own. It should exit as soon as the request is finished.
	if err := cmd.Wait(); err != nil {
		t.Fatal(err)
	}

	// tusd should close the request and exit immediately after the signal.
	duration := time.Since(start)
	if !isApprox(duration, 2*time.Second, 0.1) {
		t.Fatalf("invalid request duration %v", duration)
	}

}

// TestUploadLengthExceeded asserts that uploading appending requests are limited to
// the length specified in the upload. If more data is transmitted, tusd just ignores
// the remaining data.
func TestUploadLengthExceeded(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	_, addr, _ := spawnTusd(ctx, t)

	proxy, _ := toxiClient.CreateProxy("tusd_"+t.Name(), "", addr)
	defer proxy.Delete()

	// We limit the upstream connection to tusd to 5KB/s. The downstream connection
	// from tusd is not limited.
	proxy.AddToxic("", "bandwidth", "upstream", 1, toxiproxy.Attributes{
		"rate": 5,
	})

	// Endpoint address point to toxiproxy
	endpoint := "http://" + proxy.Listen + "/files/"

	// We specify an upload length of 10KB, but supply 50KB of random upload data.
	uploadLength := 10 * 1024
	payloadLength := 50 * 1024
	data := make([]byte, payloadLength)
	_, err := rand.Read(data)
	if err != nil {
		t.Fatal(err)
	}

	// Create upload
	req, err := http.NewRequest("POST", endpoint, bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}

	req.Header.Add("Tus-Resumable", "1.0.0")
	req.Header.Add("Upload-Length", strconv.Itoa(uploadLength))
	req.Header.Add("Content-Type", "application/offset+octet-stream")

	// Note: This is important! By default, http.NewRequest will inspect the body and fill
	// ContentLength automatically. This causes the Content-Length header to be set. However,
	// in this case, we want to test how tusd behaves without a pre-known request body size.
	// But setting it to -1, we do not use Content-Length but Transfer-Encoding: chunked, so
	// tusd does not know the request size upfront.
	req.ContentLength = -1

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusCreated {
		t.Fatalf("invalid response code %d", res.StatusCode)
	}

	// tusd must only read the amount specified in Upload-Length.
	if res.Header.Get("Upload-Offset") != strconv.Itoa(uploadLength) {
		t.Fatalf("invalid response code %d", res.StatusCode)
	}

	// TODO: Assert that the request is stopped after 2s already instead of
	// waiting for all 50KB to be transmitted. Right now, this is not the case.
}

func spawnTusd(ctx context.Context, t *testing.T, args ...string) (endpoint string, address string, cmd *exec.Cmd) {
	args = append([]string{"-port=0"}, args...)
	cmd = exec.CommandContext(ctx, TUSD_BINARY, args...)
	cmd.Stderr = os.Stderr

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("failed to get stdout pipe: %s", err)
	}

	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start tusd: %s", err)
	}

	scanner := bufio.NewScanner(stdout)

	for scanner.Scan() {
		// fmt.Println(scanner.Text()) // Println will add back the final '\n'
		match := TUSD_ENDPOINT_RE.FindStringSubmatch(scanner.Text())
		if match != nil {
			endpoint = match[1]
			address = match[2]
			return
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("failed to scan output: %s", err)
	}

	panic("unreachable")
}

func isApprox[N constraints.Integer](got N, expected N, tolerance float64) bool {
	min := float64(expected) * (1 - tolerance)
	max := float64(expected) * (1 + tolerance)

	return min <= float64(got) && float64(got) <= max
}
