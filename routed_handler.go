package tusd

import (
	"net/http"

	"github.com/bmizerany/pat"
)

// RoutedHandler is a ready to use handler with routing (using pat)
type RoutedHandler struct {
	handler         *Handler
	routeHandler    http.Handler
	CompleteUploads chan FileInfo
}

// NewRoutedHandler creates a routed tus protocol handler. This is the simplest
// way to use tusd but may not be as configurable as you require. If you are
// integrating this into an existing app you may like to use tusd.NewHandler
// instead. Using tusd.NewHandler allows the tus handlers to be combined into
// your existing router (aka mux) directly. It also allows the GET and DELETE
// endpoints to be customized. These are not part of the protocol so can be
// changed depending on your needs.
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

	mux.Post("", http.HandlerFunc(handler.PostFile))
	mux.Head(":id", http.HandlerFunc(handler.HeadFile))
	mux.Get(":id", http.HandlerFunc(handler.GetFile))
	mux.Del(":id", http.HandlerFunc(handler.DelFile))
	mux.Add("PATCH", ":id", http.HandlerFunc(handler.PatchFile))

	return routedHandler, nil
}

// ServeHTTP Implements the http.Handler interface.
func (rHandler *RoutedHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	rHandler.routeHandler.ServeHTTP(w, r)
}
