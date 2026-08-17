// Package httpprof exposes the standard Go pprof endpoints on custom HTTP routers.
package httpprof

import (
	"net/http"
	"net/http/pprof"
)

// Handler returns an HTTP handler for Go's standard pprof endpoints.
//
// It serves /debug/pprof/ and its standard children, such as profile,
// goroutine, heap, trace, cmdline, and symbol.
func Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	return mux
}

// Register adds the standard pprof endpoints to mux.
func Register(mux *http.ServeMux) {
	mux.Handle("/debug/pprof/", Handler())
}
