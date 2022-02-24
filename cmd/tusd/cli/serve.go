package cli

import (
	"crypto/tls"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/tus/tusd/pkg/handler"
)

const (
	TLS13       = "tls13"
	TLS12       = "tls12"
	TLS12STRONG = "tls12-strong"
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
		DisableDownload:         Flags.DisableDownload,
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
		http.Handle("/", http.StripPrefix("/", handler))
	} else {
		// If a custom basepath is defined, we show a greeting at the root path...
		if Flags.ShowGreeting {
			http.HandleFunc("/", DisplayGreeting)
		}

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

	protocol := "http"
	if Flags.TLSCertFile != "" && Flags.TLSKeyFile != "" {
		protocol = "https"
	}

	if Flags.HttpSock == "" {
		stdout.Printf("You can now upload files to: %s://%s%s", protocol, address, basepath)
	}

	// If we're not using TLS just start the server and, if http.Serve() returns, just return.
	if protocol == "http" {
		if err = http.Serve(listener, nil); err != nil {
			stderr.Fatalf("Unable to serve: %s", err)
		}
		return
	}

	// Fall-through for TLS mode.
	server := &http.Server{}
	switch Flags.TLSMode {
	case TLS13:
		server.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS13}

	case TLS12:
		// Ciphersuite selection comes from
		// https://ssl-config.mozilla.org/#server=go&version=1.14.4&config=intermediate&guideline=5.6
		// 128-bit AES modes remain as TLSv1.3 is enabled in this mode, and TLSv1.3 compatibility requires an AES-128 ciphersuite.
		server.TLSConfig = &tls.Config{
			MinVersion:               tls.VersionTLS12,
			PreferServerCipherSuites: true,
			CipherSuites: []uint16{
				tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
				tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
				tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
				tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
				tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
				tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
			},
		}

	case TLS12STRONG:
		// Ciphersuite selection as above, but intersected with
		// https://github.com/denji/golang-tls#perfect-ssl-labs-score-with-go
		// TLSv1.3 is disabled as it requires an AES-128 ciphersuite.
		server.TLSConfig = &tls.Config{
			MinVersion:               tls.VersionTLS12,
			MaxVersion:               tls.VersionTLS12,
			PreferServerCipherSuites: true,
			CipherSuites: []uint16{
				tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
				tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			},
		}

	default:
		stderr.Fatalf("Invalid TLS mode chosen. Recommended valid modes are tls13, tls12 (default), and tls12-strong")
	}

	// Disable HTTP/2; the default non-TLS mode doesn't support it
	server.TLSNextProto = make(map[string]func(*http.Server, *tls.Conn, http.Handler), 0)

	if err = server.ServeTLS(listener, Flags.TLSCertFile, Flags.TLSKeyFile); err != nil {
		stderr.Fatalf("Unable to serve: %s", err)
	}
}
