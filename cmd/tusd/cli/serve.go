package cli

import (
	"net/http"
	"time"

	"github.com/tus/tusd"
)

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
	})
	if err != nil {
		stderr.Fatalf("Unable to create handler: %s", err)
	}

	address := Flags.HttpHost + ":" + Flags.HttpPort
	basepath := Flags.Basepath

	stdout.Printf("Using %s as address to listen.\n", address)
	stdout.Printf("Using %s as the base path.\n", basepath)

	SetupPostHooks(handler)

	if Flags.ExposeMetrics {
		SetupMetrics(handler)
	}

	stdout.Printf(Composer.Capabilities())

	// Do not display the greeting if the tusd handler will be mounted at the root
	// path. Else this would cause a "multiple registrations for /" panic.
	if basepath != "/" {
		http.HandleFunc("/", DisplayGreeting)
	}

	http.Handle(basepath, http.StripPrefix(basepath, handler))

	timeoutDuration := time.Duration(Flags.Timeout) * time.Millisecond
	listener, err := NewListener(address, timeoutDuration, timeoutDuration)
	if err != nil {
		stderr.Fatalf("Unable to create listener: %s", err)
	}

	if err = http.Serve(listener, nil); err != nil {
		stderr.Fatalf("Unable to serve: %s", err)
	}
}
