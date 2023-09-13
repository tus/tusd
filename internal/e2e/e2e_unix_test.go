//go:build unix

package e2e_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	toxiproxy "github.com/Shopify/toxiproxy/v2/client"
)

// TestShutdown asserts that tusd closes all ongoing upload requests and shuts down
// cleanly on its own when receiving a signal to stop.
// This test is not run on Windows where sending an interrupt signal is not supported.
// See https://github.com/golang/go/issues/46345.
func TestShutdown(t *testing.T) {
	t.Parallel()

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
