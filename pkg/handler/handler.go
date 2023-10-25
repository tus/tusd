package handler

import (
	"net/http"
)

// Handler is a ready to use handler with routing (using pat)
type Handler struct {
	*UnroutedHandler
	http.Handler
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

	handler, err := NewUnroutedHandler(config)
	if err != nil {
		return nil, err
	}

	routedHandler := &Handler{
		UnroutedHandler: handler,
	}

	mux := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "POST" && r.URL.Path == "":
			handler.PostFile(w, r)
		case r.Method == "HEAD" && r.URL.Path != "":
			handler.HeadFile(w, r)
		case r.Method == "PATCH" && r.URL.Path != "":
			handler.PatchFile(w, r)
		case r.Method == "GET" && r.URL.Path != "" && !config.DisableDownload:
			handler.GetFile(w, r)
		case r.Method == "DELETE" && r.URL.Path != "" && config.StoreComposer.UsesTerminater && !config.DisableTermination:
			handler.DelFile(w, r)
		default:
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`combination of path and method are not recognized`))
		}
	})

	routedHandler.Handler = handler.Middleware(mux)

	return routedHandler, nil
}
