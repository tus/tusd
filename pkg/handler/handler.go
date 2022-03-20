package handler

import (
	"net/http"
	"strings"
)

// Handler is a ready to use handler with routing (using pat)
type Handler struct {
	*UnroutedHandler
	http.Handler
	allowedMethods []string
}

// NewHandler creates a routed tus protocol handler. This is the simplest
// way to use tusd but may not be as configurable as you require. If you are
// integrating this into an existing app you may like to use tusd.NewUnroutedHandler
// instead. Using tusd.NewUnroutedHandler allows the tus handlers to be combined into
// your existing router (aka mux) directly. It also allows the GET and DELETE
// endpoints to be customized. These are not part of the protocol so can be
// changed depending on your needs.
func NewHandler(config Config) (*Handler, error) {
	if err := config.validate(); err != nil {
		return nil, err
	}

	unroutedHandler, err := NewUnroutedHandler(config)
	if err != nil {
		return nil, err
	}

	allowed := []string{http.MethodPost, http.MethodHead, http.MethodPatch}
	if config.StoreComposer.UsesTerminater && !config.DisableDelete {
		allowed = append(allowed, http.MethodDelete)
	}
	if !config.DisableDownload {
		allowed = append(allowed, http.MethodGet)
	}
	routedHandler := &Handler{
		UnroutedHandler: unroutedHandler,
		allowedMethods:  allowed,
	}

	// This madness made only for saving other code from changes after rid of https://github.com/bmizerany/pat
	routedHandler.Handler = unroutedHandler.Middleware(&router{routedHandler})

	return routedHandler, nil
}

type router struct {
	routedHandler *Handler
}

func (router *router) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		http.HandlerFunc(router.routedHandler.UnroutedHandler.PostFile).ServeHTTP(w, r)
	case http.MethodHead:
		http.HandlerFunc(router.routedHandler.UnroutedHandler.HeadFile).ServeHTTP(w, r)
	case http.MethodPatch:
		http.HandlerFunc(router.routedHandler.UnroutedHandler.PatchFile).ServeHTTP(w, r)
	case http.MethodGet:
		if !router.routedHandler.config.DisableDownload {
			http.HandlerFunc(router.routedHandler.UnroutedHandler.GetFile).ServeHTTP(w, r)
		} else {
			router.NotAllowed(w, r)
		}
	case http.MethodDelete:
		if router.routedHandler.config.StoreComposer.UsesTerminater && !router.routedHandler.config.DisableDelete {
			// Only attach the DELETE handler if the Terminate() method is provided
			http.HandlerFunc(router.routedHandler.DelFile).ServeHTTP(w, r)
		} else {
			router.NotAllowed(w, r)
		}
	default:
		router.NotAllowed(w, r)
	}
}

func (router *router) NotAllowed(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("Allow", strings.Join(router.routedHandler.allowedMethods, ", "))
	http.Error(w, "Method Not Allowed", 405)
}
