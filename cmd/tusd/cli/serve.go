package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"net"
	"net/http"
	"strings"
	"time"

	"github.com/oklog/run"
	"github.com/tus/tusd/pkg/handler"
)

// Serve sets up the different components, starts a Listener and give it to
// http.Serve().
//
// By default it will bind to the specified host/port, unless a UNIX socket is
// specified, in which case a different socket creation and binding mechanism
// is put in place.
func Serve() {
	config := handler.Config{
		MaxSize:                 Flags.MaxSize,
		BasePath:                Flags.Basepath,
		RespectForwardedHeaders: Flags.BehindProxy,
		StoreComposer:           Composer,
		NotifyCompleteUploads:   true,
		NotifyTerminatedUploads: true,
		NotifyUploadProgress:    true,
		NotifyCreatedUploads:    true,
	}

	if err := SetupPreHooks(&config); err != nil {
		stderr.Fatalf("Unable to setup hooks for handler: %s", err)
	}

	handler, err := handler.NewHandler(config)
	if err != nil {
		stderr.Fatalf("Unable to create handler: %s", err)
	}

	basepath := Flags.Basepath
	address := ""

	if Flags.HttpSock != "" {
		address = Flags.HttpSock
		stdout.Printf("Using %s as socket to listen.\n", address)
	} else {
		address = Flags.HttpHost + ":" + Flags.HttpPort
		stdout.Printf("Using %s as address to listen.\n", address)
	}

	stdout.Printf("Using %s as the base path.\n", basepath)

	SetupPostHooks(handler)

	mux := http.NewServeMux()
	s := &http.Server{
		Addr:		address,
		Handler:        mux,
	}


	if Flags.ExposeMetrics {
		SetupMetrics(mux, handler)
		SetupHookMetrics()
	}

	stdout.Printf("Supported tus extensions: %s\n", handler.SupportedExtensions())

	if basepath == "/" {
		// If the basepath is set to the root path, only install the tusd handler
		// and do not show a greeting.

		mux.Handle("/", http.StripPrefix("/", handler))
	} else {
		// If a custom basepath is defined, we show a greeting at the root path...
		mux.HandleFunc("/", DisplayGreeting)

		// ... and register a route with and without the trailing slash, so we can
		// handle uploads for /files/ and /files, for example.
		basepathWithoutSlash := strings.TrimSuffix(basepath, "/")
		basepathWithSlash := basepathWithoutSlash + "/"

		mux.Handle(basepathWithSlash, http.StripPrefix(basepathWithSlash, handler))
		mux.Handle(basepathWithoutSlash, http.StripPrefix(basepathWithoutSlash, handler))
	}

	var listener net.Listener
	timeoutDuration := time.Duration(Flags.Timeout) * time.Millisecond

	if Flags.HttpSock != "" {
		listener, err = NewUnixListener(address, timeoutDuration, timeoutDuration)
	} else {
		listener, err = NewListener(address, timeoutDuration, timeoutDuration)
	}

	if err != nil {
		stderr.Fatalf("Unable to create listener: %s", err)
	}

	if Flags.HttpSock == "" {
		stdout.Printf("You can now upload files to: http://%s%s", address, basepath)
	}

	var g run.Group

	g.Add(func() error {
		if err = s.Serve(listener); err != nil {
			stderr.Fatalf("Unable to serve: %s", err)
			return err
		}
		return nil
	}, func(error) {
		// TODO(rbastic): externalize shutdown timeout? for now just 30 mins? i don't know.
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*time.Duration(Flags.ShutdownTimeout))
		defer cancel()
		stderr.Printf("httpserver shutting down %s\n", s.Shutdown(ctx))
	})

	cancel := make(chan struct{})
	g.Add(func() error {
		return interrupt(cancel)
	}, func(error) {
		close(cancel)
	})

	err = g.Run()
	if err != nil {
		stderr.Fatalf("error", err)
		os.Exit(-1)
	}
}

func interrupt(cancel <-chan struct{}) error {
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)
	select {
	case sig := <-c:
		return fmt.Errorf("received signal %s", sig)
	case <-cancel:
		return errors.New("canceled")
	}
}
