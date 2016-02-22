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
	"github.com/tus/tusd/s3store"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
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

var stdout = log.New(os.Stdout, "[tusd] ", 0)
var stderr = log.New(os.Stderr, "[tusd] ", 0)

var hookInstalled bool

func init() {
	flag.StringVar(&httpHost, "host", "0.0.0.0", "Host to bind HTTP server to")
	flag.StringVar(&httpPort, "port", "1080", "Port to bind HTTP server to")
	flag.Int64Var(&maxSize, "max-size", 0, "Maximum size of uploads in bytes")
	flag.StringVar(&dir, "dir", "./data", "Directory to store uploads in")
	flag.Int64Var(&storeSize, "store-size", 0, "Size of space allowed for storage")
	flag.StringVar(&basepath, "base-path", "/files/", "Basepath of the HTTP server")
	flag.Int64Var(&timeout, "timeout", 30*1000, "Read timeout for connections in milliseconds")
	flag.StringVar(&s3Bucket, "s3-bucket", "", "Use AWS S3 with this bucket as storage backend (requires the AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY and AWS_REGION environment variables to be set)")
	flag.StringVar(&hooksDir, "hooks-dir", "", "")
	flag.BoolVar(&version, "version", false, "Print tusd version information")

	flag.Parse()

	if hooksDir != "" {
		hooksDir, _ = filepath.Abs(hooksDir)
		hookInstalled = true

		stdout.Printf("Using '%s' for hooks", hooksDir)
	}
}

func main() {
	if version {
		fmt.Printf("Version: %s\nCommit: %s\nDate: %s\n", VersionName, GitCommit, BuildDate)

		return
	}

	var store tusd.TerminaterDataStore
	if s3Bucket == "" {
		stdout.Printf("Using '%s' as directory storage.\n", dir)
		if err := os.MkdirAll(dir, os.FileMode(0775)); err != nil {
			stderr.Fatalf("Unable to ensure directory exists: %s", err)
		}

		store = filestore.New(dir)
	} else {
		stdout.Printf("Using 's3://%s' as S3 bucket for storage.\n", s3Bucket)

		// Derive credentials from AWS_SECRET_ACCESS_KEY, AWS_ACCESS_KEY_ID and
		// AWS_REGION environment variables.
		credentials := aws.NewConfig().WithCredentials(credentials.NewEnvCredentials())
		store = s3store.New(s3Bucket, s3.New(session.New(), credentials))
	}

	if storeSize > 0 {
		store = limitedstore.New(storeSize, store)
		stdout.Printf("Using %.2fMB as storage size.\n", float64(storeSize)/1024/1024)

		// We need to ensure that a single upload can fit into the storage size
		if maxSize > storeSize || maxSize == 0 {
			maxSize = storeSize
		}
	}

	stdout.Printf("Using %.2fMB as maximum size.\n", float64(maxSize)/1024/1024)

	handler, err := tusd.NewHandler(tusd.Config{
		MaxSize:               maxSize,
		BasePath:              basepath,
		DataStore:             store,
		NotifyCompleteUploads: true,
	})
	if err != nil {
		stderr.Fatalf("Unable to create handler: %s", err)
	}

	address := httpHost + ":" + httpPort
	stdout.Printf("Using %s as address to listen.\n", address)

	go func() {
		for {
			select {
			case info := <-handler.CompleteUploads:
				invokeHook(info)
			}
		}
	}()

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

func invokeHook(info tusd.FileInfo) {
	stdout.Printf("Upload %s (%d bytes) finished\n", info.ID, info.Size)

	if !hookInstalled {
		return
	}

	stdout.Println("Invoking hooks…")

	cmd := exec.Command(hooksDir + "/postfinish")
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
			stderr.Printf("Error running postfinish hook for %s: %s", info.ID, err)
		}
	}()
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
	err := c.Conn.SetReadDeadline(time.Now().Add(c.ReadTimeout))
	if err != nil {
		return 0, err
	}
	return c.Conn.Read(b)
}

func (c *Conn) Write(b []byte) (int, error) {
	err := c.Conn.SetWriteDeadline(time.Now().Add(c.WriteTimeout))
	if err != nil {
		return 0, err
	}
	return c.Conn.Write(b)
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
