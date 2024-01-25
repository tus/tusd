package handler

import (
	"net/http"
	"strings"
)

// Handler is a ready to use handler with routing
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
		method := r.Method
		path := strings.Trim(r.URL.Path, "/")

		switch path {
		case "":
			// Root endpoint for upload creation
			switch method {
			case "POST":
				handler.PostFile(w, r)
			default:
				w.Header().Add("Allow", "POST")
				w.WriteHeader(http.StatusMethodNotAllowed)
				w.Write([]byte(`method not allowed`))
			}
		default:
			// URL points to an upload resource
			switch {
			case method == "HEAD" && r.URL.Path != "":
				// Offset retrieval
				handler.HeadFile(w, r)
			case method == "PATCH" && r.URL.Path != "":
				// Upload apppending
				handler.PatchFile(w, r)
			case method == "GET" && r.URL.Path != "" && !config.DisableDownload:
				// Upload download
				handler.GetFile(w, r)
			case method == "DELETE" && r.URL.Path != "" && config.StoreComposer.UsesTerminater && !config.DisableTermination:
				// Upload termination
				handler.DelFile(w, r)
			default:
				// TODO: Only add GET and DELETE if they are supported
				w.Header().Add("Allow", "GET, HEAD, PATCH, DELETE")
				w.WriteHeader(http.StatusMethodNotAllowed)
				w.Write([]byte(`method not allowed`))
			}
		}
	})

	routedHandler.Handler = handler.Middleware(mux)

	return routedHandler, nil
}
