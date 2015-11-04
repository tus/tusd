package tusd

import (
	"net/http"

	"github.com/bmizerany/pat"
)

type RoutedHandler struct {
	handler         *Handler
	routeHandler    http.Handler
	CompleteUploads chan FileInfo
}

func NewRoutedHandler(config Config) (*RoutedHandler, error) {
	handler, err := NewHandler(config)
	if err != nil {
		return nil, err
	}

	routedHandler := &RoutedHandler{
		handler:         handler,
		CompleteUploads: handler.CompleteUploads,
	}

	mux := pat.New()

	routedHandler.routeHandler = handler.TusMiddleware(mux)

	mux.Post("", http.HandlerFunc(handler.postFile))
	mux.Head(":id", http.HandlerFunc(handler.headFile))
	mux.Get(":id", http.HandlerFunc(handler.getFile))
	mux.Del(":id", http.HandlerFunc(handler.delFile))
	mux.Add("PATCH", ":id", http.HandlerFunc(handler.patchFile))

	return routedHandler, nil
}

// ServeHTTP Implements the http.Handler interface.
func (rHandler *RoutedHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	rHandler.routeHandler.ServeHTTP(w, r)
}
