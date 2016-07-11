package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"

	"github.com/tus/tusd"
	"github.com/tus/tusd/filestore"
	"github.com/tus/tusd/limitedstore"
	"github.com/tus/tusd/memorylocker"
	"github.com/tus/tusd/prometheuscollector"
	"github.com/tus/tusd/s3store"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"

	"github.com/prometheus/client_golang/prometheus"
)

var VersionName = "n/a"
var GitCommit = "n/a"
var BuildDate = "n/a"

var httpHost string
var httpPort string
var maxSize int64
var dir string
var storeSize int64
var basepath string
var timeout int64
var s3Bucket string
var hooksDir string
var version bool
var exposeMetrics bool
var behindProxy bool

var stdout = log.New(os.Stdout, "[tusd] ", 0)
var stderr = log.New(os.Stderr, "[tusd] ", 0)

var hookInstalled bool

var greeting string

var openConnections = prometheus.NewGauge(prometheus.GaugeOpts{
	Name: "connections_open_total",
	Help: "Current number of open connections.",
})

func init() {
	flag.StringVar(&httpHost, "host", "0.0.0.0", "Host to bind HTTP server to")
	flag.StringVar(&httpPort, "port", "1080", "Port to bind HTTP server to")
	flag.Int64Var(&maxSize, "max-size", 0, "Maximum size of a single upload in bytes")
	flag.StringVar(&dir, "dir", "./data", "Directory to store uploads in")
	flag.Int64Var(&storeSize, "store-size", 0, "Size of space allowed for storage")
	flag.StringVar(&basepath, "base-path", "/files/", "Basepath of the HTTP server")
	flag.Int64Var(&timeout, "timeout", 30*1000, "Read timeout for connections in milliseconds.  A zero value means that reads will not timeout")
	flag.StringVar(&s3Bucket, "s3-bucket", "", "Use AWS S3 with this bucket as storage backend (requires the AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY and AWS_REGION environment variables to be set)")
	flag.StringVar(&hooksDir, "hooks-dir", "", "Directory to search for available hooks scripts")
	flag.BoolVar(&version, "version", false, "Print tusd version information")
	flag.BoolVar(&exposeMetrics, "expose-metrics", true, "Expose metrics about tusd usage")
	flag.BoolVar(&behindProxy, "behind-proxy", false, "Respect X-Forwarded-* and similar headers which may be set by proxies")

	flag.Parse()

	if hooksDir != "" {
		hooksDir, _ = filepath.Abs(hooksDir)
		hookInstalled = true

		stdout.Printf("Using '%s' for hooks", hooksDir)
	}

	greeting = fmt.Sprintf(
		`Welcome to tusd
===============

Congratulations for setting up tusd! You are now part of the chosen elite and
able to experience the feeling of resumable uploads! We hope you are as excited
as we are (a lot)!

However, there is something you should be aware of: While you got tusd
running (you did an awesome job!), this is the root directory of the server
and tus requests are only accepted at the %s route.

So don't waste time, head over there and experience the future!

Version = %s
GitCommit = %s
BuildDate = %s`, basepath, VersionName, GitCommit, BuildDate)
}

func main() {
	// Print version and other information and exit if the -version flag has been
	// passed.
	if version {
		fmt.Printf("Version: %s\nCommit: %s\nDate: %s\n", VersionName, GitCommit, BuildDate)

		return
	}

	// Attempt to use S3 as a backend if the -s3-bucket option has been supplied.
	// If not, we default to storing them locally on disk.
	composer := tusd.NewStoreComposer()
	if s3Bucket == "" {
		stdout.Printf("Using '%s' as directory storage.\n", dir)
		if err := os.MkdirAll(dir, os.FileMode(0775)); err != nil {
			stderr.Fatalf("Unable to ensure directory exists: %s", err)
		}

		store := filestore.New(dir)
		store.UseIn(composer)
	} else {
		stdout.Printf("Using 's3://%s' as S3 bucket for storage.\n", s3Bucket)

		// Derive credentials from AWS_SECRET_ACCESS_KEY, AWS_ACCESS_KEY_ID and
		// AWS_REGION environment variables.
		credentials := aws.NewConfig().WithCredentials(credentials.NewEnvCredentials())
		store := s3store.New(s3Bucket, s3.New(session.New(), credentials))
		store.UseIn(composer)

		locker := memorylocker.New()
		locker.UseIn(composer)
	}

	if storeSize > 0 {
		limitedstore.New(storeSize, composer.Core, composer.Terminater).UseIn(composer)
		stdout.Printf("Using %.2fMB as storage size.\n", float64(storeSize)/1024/1024)

		// We need to ensure that a single upload can fit into the storage size
		if maxSize > storeSize || maxSize == 0 {
			maxSize = storeSize
		}
	}

	stdout.Printf("Using %.2fMB as maximum size.\n", float64(maxSize)/1024/1024)

	handler, err := tusd.NewHandler(tusd.Config{
		MaxSize:                 maxSize,
		BasePath:                basepath,
		StoreComposer:           composer,
		RespectForwardedHeaders: behindProxy,
		NotifyCompleteUploads:   true,
		NotifyTerminatedUploads: true,
	})
	if err != nil {
		stderr.Fatalf("Unable to create handler: %s", err)
	}

	address := httpHost + ":" + httpPort
	stdout.Printf("Using %s as address to listen.\n", address)

	stdout.Printf("Using %s as the base path.\n", basepath)

	stdout.Printf(composer.Capabilities())

	go func() {
		for {
			select {
			case info := <-handler.CompleteUploads:
				invokeHook("post-finish", info)
			case info := <-handler.TerminatedUploads:
				invokeHook("post-terminate", info)
			}
		}
	}()

	if exposeMetrics {
		prometheus.MustRegister(openConnections)
		prometheus.MustRegister(prometheuscollector.New(handler.Metrics))

		http.Handle("/metrics", prometheus.Handler())
	}

	// Do not display the greeting if the tusd handler will be mounted at the root
	// path. Else this would cause a "multiple registrations for /" panic.
	if basepath != "/" {
		http.HandleFunc("/", displayGreeting)
	}

	http.Handle(basepath, http.StripPrefix(basepath, handler))

	timeoutDuration := time.Duration(timeout) * time.Millisecond
	listener, err := NewListener(address, timeoutDuration, timeoutDuration)
	if err != nil {
		stderr.Fatalf("Unable to create listener: %s", err)
	}

	if err = http.Serve(listener, nil); err != nil {
		stderr.Fatalf("Unable to serve: %s", err)
	}
}

func invokeHook(name string, info tusd.FileInfo) {
	switch name {
	case "post-finish":
		stdout.Printf("Upload %s (%d bytes) finished\n", info.ID, info.Size)
	case "post-terminate":
		stdout.Printf("Upload %s terminated\n", info.ID)
	}

	if !hookInstalled {
		return
	}

	stdout.Printf("Invoking %s hook…\n", name)

	cmd := exec.Command(hooksDir + "/" + name)
	env := os.Environ()
	env = append(env, "TUS_ID="+info.ID)
	env = append(env, "TUS_SIZE="+strconv.FormatInt(info.Size, 10))

	jsonInfo, err := json.Marshal(info)
	if err != nil {
		stderr.Printf("Error encoding JSON for hook: %s", err)
	}

	reader := bytes.NewReader(jsonInfo)
	cmd.Stdin = reader

	cmd.Env = env
	cmd.Dir = hooksDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	go func() {
		err := cmd.Run()
		if err != nil {
			stderr.Printf("Error running %s hook for %s: %s", name, info.ID, err)
		}
	}()
}

func displayGreeting(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte(greeting))
}

// Listener wraps a net.Listener, and gives a place to store the timeout
// parameters. On Accept, it will wrap the net.Conn with our own Conn for us.
// Original implementation taken from https://gist.github.com/jbardin/9663312
// Thanks! <3
type Listener struct {
	net.Listener
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
}

func (l *Listener) Accept() (net.Conn, error) {
	c, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}

	go openConnections.Inc()

	tc := &Conn{
		Conn:         c,
		ReadTimeout:  l.ReadTimeout,
		WriteTimeout: l.WriteTimeout,
	}
	return tc, nil
}

// Conn wraps a net.Conn, and sets a deadline for every read
// and write operation.
type Conn struct {
	net.Conn
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
}

func (c *Conn) Read(b []byte) (int, error) {
	var err error
	if c.ReadTimeout > 0 {
		err = c.Conn.SetReadDeadline(time.Now().Add(c.ReadTimeout))
	} else {
		err = c.Conn.SetReadDeadline(time.Time{})
	}

	if err != nil {
		return 0, err
	}

	return c.Conn.Read(b)
}

func (c *Conn) Write(b []byte) (int, error) {
	var err error
	if c.WriteTimeout > 0 {
		err = c.Conn.SetWriteDeadline(time.Now().Add(c.WriteTimeout))
	} else {
		err = c.Conn.SetWriteDeadline(time.Time{})
	}

	if err != nil {
		return 0, err
	}

	return c.Conn.Write(b)
}

func (c *Conn) Close() error {
	go openConnections.Dec()
	return c.Conn.Close()
}

func NewListener(addr string, readTimeout, writeTimeout time.Duration) (net.Listener, error) {
	l, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}

	tl := &Listener{
		Listener:     l,
		ReadTimeout:  readTimeout,
		WriteTimeout: writeTimeout,
	}
	return tl, nil
}
