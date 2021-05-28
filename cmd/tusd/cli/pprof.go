package cli

import (
	"net/http"
	"net/http/pprof"
	"runtime"
)

func SetupPprof(mux *http.ServeMux) {
	runtime.SetBlockProfileRate(Flags.PprofBlockProfileRate)
	runtime.SetMutexProfileFraction(Flags.PprofMutexProfileRate)

	mux.HandleFunc(Flags.PprofPath, pprof.Index)
	mux.HandleFunc(Flags.PprofPath+"cmdline", pprof.Cmdline)
	mux.HandleFunc(Flags.PprofPath+"profile", pprof.Profile)
	mux.HandleFunc(Flags.PprofPath+"symbol", pprof.Symbol)
	mux.HandleFunc(Flags.PprofPath+"trace", pprof.Trace)
}
