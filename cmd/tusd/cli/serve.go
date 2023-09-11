package cli

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"syscall"

	tushandler "github.com/tus/tusd/v2/pkg/handler"
	"github.com/tus/tusd/v2/pkg/hooks"
	"github.com/tus/tusd/v2/pkg/hooks/plugin"
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
	config := tushandler.Config{
		MaxSize:                          Flags.MaxSize,
		BasePath:                         Flags.Basepath,
		Cors:                             getCorsConfig(),
		RespectForwardedHeaders:          Flags.BehindProxy,
		EnableExperimentalProtocol:       Flags.ExperimentalProtocol,
		DisableDownload:                  Flags.DisableDownload,
		DisableTermination:               Flags.DisableTermination,
		StoreComposer:                    Composer,
		UploadProgressInterval:           Flags.ProgressHooksInterval,
		AcquireLockTimeout:               Flags.AcquireLockTimeout,
		GracefulRequestCompletionTimeout: Flags.GracefulRequestCompletionTimeout,
		NetworkTimeout:                   Flags.NetworkTimeout,
	}

	var handler *tushandler.Handler
	var err error
	hookHandler := getHookHandler(&config)
	if hookHandler != nil {
		handler, err = hooks.NewHandlerWithHooks(&config, hookHandler, Flags.EnabledHooks)

		var enabledHooksString []string
		for _, h := range Flags.EnabledHooks {
			enabledHooksString = append(enabledHooksString, string(h))
		}

		stdout.Printf("Enabled hook events: %s", strings.Join(enabledHooksString, ", "))

	} else {
		handler, err = tushandler.NewHandler(config)
	}
	if err != nil {
		stderr.Fatalf("Unable to create handler: %s", err)
	}

	stdout.Printf("Supported tus extensions: %s\n", handler.SupportedExtensions())

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

	mux := http.NewServeMux()
	if basepath == "/" {
		// If the basepath is set to the root path, only install the tusd handler
		// and do not show a greeting.
		mux.Handle("/", http.StripPrefix("/", handler))
	} else {
		// If a custom basepath is defined, we show a greeting at the root path...
		if Flags.ShowGreeting {
			mux.HandleFunc("/", DisplayGreeting)
		}

		// ... and register a route with and without the trailing slash, so we can
		// handle uploads for /files/ and /files, for example.
		basepathWithoutSlash := strings.TrimSuffix(basepath, "/")
		basepathWithSlash := basepathWithoutSlash + "/"

		mux.Handle(basepathWithSlash, http.StripPrefix(basepathWithSlash, handler))
		mux.Handle(basepathWithoutSlash, http.StripPrefix(basepathWithoutSlash, handler))
	}

	if Flags.ExposeMetrics {
		SetupMetrics(mux, handler)
		hooks.SetupHookMetrics()
	}

	if Flags.ExposePprof {
		SetupPprof(mux)
	}

	var listener net.Listener
	if Flags.HttpSock != "" {
		listener, err = NewUnixListener(address)
	} else {
		listener, err = NewListener(address)
	}

	if err != nil {
		stderr.Fatalf("Unable to create listener: %s", err)
	}

	protocol := "http"
	if Flags.TLSCertFile != "" && Flags.TLSKeyFile != "" {
		protocol = "https"
	}

	if Flags.HttpSock == "" {
		stdout.Printf("You can now upload files to: %s://%s%s", protocol, listener.Addr(), basepath)
	}

	server := &http.Server{
		Handler:           mux,
		ReadTimeout:       0,
		ReadHeaderTimeout: Flags.NetworkTimeout,
		WriteTimeout:      Flags.NetworkTimeout,
		IdleTimeout:       Flags.NetworkTimeout,
		MaxHeaderBytes:    http.DefaultMaxHeaderBytes,
		// TODO: Track open (and/or active) connections using ConnState
		// See https://stackoverflow.com/questions/51317122/how-to-get-number-of-idle-and-active-connections-in-go
		// go MetricsOpenConnections.Inc()
	}

	shutdownComplete := setupSignalHandler(server, handler)

	if protocol == "http" {
		// Non-TLS mode
		err = server.Serve(listener)
	} else {
		// TLS mode
		err = serveTLS(server, listener)
	}

	// Note: http.Server.Serve and http.Server.ServeTLS (in serveTLS) always return a non-nil error code. So
	// we can assume from here that `err != nil`
	if err == http.ErrServerClosed {
		// ErrServerClosed means that http.Server.Shutdown was called due to an interruption signal.
		// We wait until the interruption procedure is complete or times out and then exit main.
		<-shutdownComplete
	} else {
		// Any other error is relayed to the user.
		stderr.Fatalf("Unable to serve: %s", err)
	}
}

func serveTLS(server *http.Server, listener net.Listener) error {
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

	return server.ServeTLS(listener, Flags.TLSCertFile, Flags.TLSKeyFile)
}

func setupSignalHandler(server *http.Server, handler *tushandler.Handler) <-chan struct{} {
	shutdownComplete := make(chan struct{})

	// We read up to two signals, so use a capacity of 2 here to not miss any signal
	c := make(chan os.Signal, 2)

	// os.Interrupt is mapped to SIGINT on Unix and to the termination instructions on Windows.
	// On Unix we also listen to SIGTERM.
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	// Signal to the handler that it should stop all long running requests if we shut down
	server.RegisterOnShutdown(handler.InterruptRequestHandling)

	go func() {
		// First interrupt signal
		<-c
		stdout.Println("Received interrupt signal. Shutting down tusd...")

		// Wait for second interrupt signal, while also shutting down the existing server
		go func() {
			<-c
			stdout.Println("Received second interrupt signal. Exiting immediately!")
			os.Exit(1)
		}()

		// Shutdown the server, but with a user-specified timeout
		ctx, cancel := context.WithTimeout(context.Background(), Flags.ShutdownTimeout)
		defer cancel()

		err := server.Shutdown(ctx)

		if err == nil {
			stdout.Println("Shutdown completed. Goodbye!")
		} else if errors.Is(err, context.DeadlineExceeded) {
			stderr.Println("Shutdown timeout exceeded. Exiting immediately!")
		} else {
			stderr.Printf("Failed to shutdown gracefully: %s\n", err)
		}

		// Make sure that the plugins exit properly.
		plugin.CleanupPlugins()

		close(shutdownComplete)
	}()

	return shutdownComplete
}

func getCorsConfig() *tushandler.CorsConfig {
	config := tushandler.DefaultCorsConfig
	config.Disable = Flags.DisableCors
	config.AllowCredentials = Flags.CorsAllowCredentials
	config.MaxAge = Flags.CorsMaxAge

	var err error
	config.AllowOrigin, err = regexp.Compile(Flags.CorsAllowOrigin)
	if err != nil {
		stderr.Fatalf("Invalid regular expression for -cors-allow-origin flag: %s", err)
	}

	if Flags.CorsAllowHeaders != "" {
		config.AllowHeaders += ", " + Flags.CorsAllowHeaders
	}

	if Flags.CorsAllowMethods != "" {
		config.AllowMethods += ", " + Flags.CorsAllowMethods
	}

	if Flags.CorsExposeHeaders != "" {
		config.ExposeHeaders += ", " + Flags.CorsExposeHeaders
	}

	return &config
}
