package cli

import (
	"net/http"
	"net/http/pprof"
	"os"
	"runtime"
	"strings"

	"github.com/bmizerany/pat"
	"github.com/felixge/fgprof"
	"github.com/goji/httpauth"
)

func SetupPprof(globalMux *http.ServeMux) {
	runtime.SetBlockProfileRate(Flags.PprofBlockProfileRate)
	runtime.SetMutexProfileFraction(Flags.PprofMutexProfileRate)

	mux := pat.New()
	mux.Get("", http.HandlerFunc(pprof.Index))
	mux.Get("cmdline", http.HandlerFunc(pprof.Cmdline))
	mux.Get("profile", http.HandlerFunc(pprof.Profile))
	mux.Get("symbol", http.HandlerFunc(pprof.Symbol))
	mux.Get("trace", http.HandlerFunc(pprof.Trace))
	mux.Get("fgprof", fgprof.Handler())

	var handler http.Handler = mux
	auth := os.Getenv("TUSD_PPROF_AUTH")
	if auth != "" {
		parts := strings.SplitN(auth, ":", 2)
		if len(parts) != 2 {
			stderr.Fatalf("TUSD_PPROF_AUTH must be two values separated by a colon")
		}

		handler = httpauth.SimpleBasicAuth(parts[0], parts[1])(mux)
	}

	globalMux.Handle(Flags.PprofPath, http.StripPrefix(Flags.PprofPath, handler))

}
