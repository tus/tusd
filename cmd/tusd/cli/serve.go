package cli

import (
	"net"
	"net/http"
	"time"

	"github.com/tus/tusd"
)

// Setups the different components, starts a Listener and give it to
// http.Serve().
//
// By default it will bind to the specified host/port, unless a UNIX socket is
// specified, in which case a different socket creation and binding mechanism
// is put in place.
func Serve() {
	SetupPreHooks(Composer)

	handler, err := tusd.NewHandler(tusd.Config{
		MaxSize:                 Flags.MaxSize,
		BasePath:                Flags.Basepath,
		RespectForwardedHeaders: Flags.BehindProxy,
		StoreComposer:           Composer,
		NotifyCompleteUploads:   true,
		NotifyTerminatedUploads: true,
		NotifyUploadProgress:    true,
		NotifyCreatedUploads:    true,
	})
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

	stdout.Printf(Composer.Capabilities())

	// Do not display the greeting if the tusd handler will be mounted at the root
	// path. Else this would cause a "multiple registrations for /" panic.
	if basepath != "/" {
		http.HandleFunc("/", DisplayGreeting)
	}

	http.Handle(basepath, http.StripPrefix(basepath, handler))

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

	if err = http.Serve(listener, nil); err != nil {
		stderr.Fatalf("Unable to serve: %s", err)
	}
}
