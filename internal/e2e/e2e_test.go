package e2e_test

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	toxiproxy "github.com/Shopify/toxiproxy/v2/client"
	"golang.org/x/exp/constraints"
)

var toxiClient *toxiproxy.Client

func init() {
	toxiClient = toxiproxy.NewClient("localhost:8474")
}

// TestSuccessfulUpload tests that tusd can perform a single upload
// from actual HTTP requests.
func TestSuccessfulUpload(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	endpoint, addr := spawnTusd(ctx, t)
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
	_, addr := spawnTusd(ctx, t, "-network-timeout=5s")

	proxy, _ := toxiClient.CreateProxy("tusd_"+t.Name(), "", addr)
	defer proxy.Delete()

	// We limit the upstream connection to tusd to 5KB/s. The downstream connection
	// from tusd is not limited. We also add a noop operation. When a new toxic is added
	// (like we do later in the test), toxiproxy interrupts the last toxic in the chain
	// to install the new toxic (see https://github.com/Shopify/toxiproxy/blob/3399ea0235ca3961e30ed9cb3a0ad52ddc8398c3/link.go#L177-L178)
	// The bandwidth toxic would send all remaining data if it gets interrupted (see
	// https://github.com/Shopify/toxiproxy/blob/3399ea0235ca3961e30ed9cb3a0ad52ddc8398c3/toxics/bandwidth.go#L51-L53),
	// which means tusd would receive more data than intended. Adding the noop toxic
	// solves this problem because the bandwidth toxic stays uninterrupted.
	proxy.AddToxic("", "bandwidth", "upstream", 1, toxiproxy.Attributes{
		"rate": 5,
	})
	proxy.AddToxic("", "noop", "upstream", 1, toxiproxy.Attributes{})

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

	// Begin uploading data, but after 2s the upstream connection is interrupted silently.
	// The TCP connection stays open but tusd does not receive any more data.
	req, err = http.NewRequest("PATCH", uploadUrl, bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}

	req.Header.Add("Tus-Resumable", "1.0.0")
	req.Header.Add("Upload-Offset", "0")
	req.Header.Add("Content-Type", "application/offset+octet-stream")

	go func() {
		<-time.After(2 * time.Second)
		proxy.AddToxic("timeout_upstream", "timeout", "upstream", 1, toxiproxy.Attributes{
			"timeout": 0,
		})
	}()

	start := time.Now()
	_, err = http.DefaultClient.Do(req)
	// TODO: Can EnableFullDuplex help to receive a response here?
	if !errors.Is(err, io.EOF) {
		t.Fatalf("unexpected error %s", err)
	}

	// TODO: Attempt to obtain the response and see if we get the timeout error
	// if err != nil {
	// 	t.Fatal(err)
	// }
	// defer res.Body.Close()

	// Assert the response to see if tusd correctly emitted a timeout.
	// In reality, clients may often not receive this message due to network issues.
	// if res.StatusCode != http.StatusInternalServerError {
	// 	t.Fatalf("invalid response code %d", res.StatusCode)
	// }

	// body, err := io.ReadAll(res.Body)
	// if err != nil {
	// 	t.Fatal(err)
	// }

	// if !strings.Contains(string(body), "ERR_READ_TIMEOUT") {
	// 	t.Fatalf("invalid response body %s", string(body))
	// }

	proxy.RemoveToxic("timeout_upstream")

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

	_, addr := spawnTusd(ctx, t)

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
	// so we can an EOF error here.
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

// 	endpoint, _ := spawnTusd(ctx, t)

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

func TestLockRelease(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// We configure tusd with low poll intervals for the filelocker to get
	// a quick test run and more predictable results
	_, addr := spawnTusd(ctx, t, "-filelock-holder-poll-interval=1s", "-filelock-acquirer-poll-interval=1s")

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
	// TODO: What happens if there is no data for the PATCH incoming? Just radio silence?
	// Is the handler able to unblock the read calls?
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

	// start := time.Now()
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

	// TODO: Assert the request duration as an additional test. However, we see
	// delays in unblocking the Read calls so some unknown reason right now.
	// Let's enable this once we figure those out.
	// duration := time.Since(start)
	// if !isApprox(duration, 7*time.Second, 0.1) {
	// 	t.Fatalf("invalid request duration %v", duration)
	// }
}

// TODO: This should be an env var
const TUSD_BINARY = "../../tusd"

var TUSD_ENDPOINT_RE = regexp.MustCompile(`You can now upload files to: (https?://([^/]+)/\S*)`)

func spawnTusd(ctx context.Context, t *testing.T, args ...string) (endpoint string, address string) {
	args = append([]string{"-port=0"}, args...)
	cmd := exec.CommandContext(ctx, TUSD_BINARY, args...)
	cmd.Stderr = os.Stderr
	// cmd.Stdout = os.Stdout

	// cmd.Start()

	// <-time.After(100 * time.Millisecond)
	// return "http://localhost:1080/files", "localhost:1080"

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
