package cli

import (
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/tus/tusd/pkg/handler"
)

// Setups the different components, starts a Listener and give it to
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

	if Flags.ExposeMetrics {
		SetupMetrics(handler)
		SetupHookMetrics()
	}

	stdout.Printf("Supported tus extensions: %s\n", handler.SupportedExtensions())

	if basepath == "/" {
		// If the basepath is set to the root path, only install the tusd handler
		// and do not show a greeting.
		http.Handle("/", handler)
	} else {
		// If a custom basepath is defined, we show a greeting at the root path...
		http.HandleFunc("/", DisplayGreeting)

		// ... and register a route with and without the trailing slash, so we can
		// handle uploads for /files/ and /files, for example.
		basepathWithoutSlash := strings.TrimSuffix(basepath, "/")
		basepathWithSlash := basepathWithoutSlash + "/"

		http.Handle(basepathWithSlash, http.StripPrefix(basepathWithSlash, handler))
		http.Handle(basepathWithoutSlash, http.StripPrefix(basepathWithoutSlash, handler))
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

	if err = http.Serve(listener, nil); err != nil {
		stderr.Fatalf("Unable to serve: %s", err)
	}
}
