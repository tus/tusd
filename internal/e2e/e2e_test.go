package e2e_test

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
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

func Test_SuccessfulUpload(t *testing.T) {
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

func Test_NetworkReadTimeout(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// We configure tusd with a read timeout of 5s, meaning that if no data
	// is received for 5s, it terminates the connection.
	_, addr := spawnTusd(ctx, t, "-read-timeout=5s")

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

	start := time.Now()
	go func() {
		<-time.After(2 * time.Second)
		proxy.AddToxic("timeout_upstream", "timeout", "upstream", 1, toxiproxy.Attributes{
			"timeout": 0,
		})
	}()

	req.Header.Add("Tus-Resumable", "1.0.0")
	req.Header.Add("Upload-Offset", "0")
	req.Header.Add("Content-Type", "application/offset+octet-stream")

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

// TODO: This should be an env var
const TUSD_BINARY = "../../tusd"

var TUSD_ENDPOINT_RE = regexp.MustCompile(`You can now upload files to: (https?://([^/]+)/\S*)`)

func spawnTusd(ctx context.Context, t *testing.T, args ...string) (endpoint string, address string) {
	args = append([]string{"-port=0"}, args...)
	cmd := exec.CommandContext(ctx, TUSD_BINARY, args...)
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
		fmt.Println(scanner.Text()) // Println will add back the final '\n'
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
