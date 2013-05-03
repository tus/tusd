package http

import (
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
)

// HandlerConfig holds the configuration for a tus Handler.
type HandlerConfig struct {
	// Dir points to a filesystem path used by tus to store uploaded and partial
	// files. Will be created if does not exist yet. Required.
	Dir string

	// MaxSize defines how many bytes may be stored inside Dir. Exceeding this
	// limit will cause the oldest upload files to be deleted until enough space
	// is available again. Required.
	MaxSize int64

	// BasePath defines the url path used for handling uploads, e.g. "/files/".
	// Must contain a trailling "/". Requests not matching this base path will
	// cause a 404, so make sure you dispatch only appropriate requests to the
	// handler. Required.
	BasePath string
}

// NewHandler returns an initialized Handler. An error may occur if the
// config.Dir is not writable.
func NewHandler(config HandlerConfig) (*Handler, error) {
	// Ensure the data store directory exists
	if err := os.MkdirAll(config.Dir, 0777); err != nil {
		return nil, err
	}

	errChan := make(chan error)

	return &Handler{
		store:     newDataStore(config.Dir, config.MaxSize),
		basePath:  config.BasePath,
		Error:     errChan,
		sendError: errChan,
	}, nil
}

// Handler is a http.Handler that implements tus resumable upload protocol.
type Handler struct {
	store    *DataStore
	basePath string

	// Error provides error events for logging purposes.
	Error <-chan error
	// same chan as Error, used for sending.
	sendError chan<- error
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	absPath := r.URL.Path
	if !strings.HasPrefix(absPath, h.basePath) {
		err := errors.New("invalid url path: " + absPath + " - does not match basePath: " + h.basePath)
		h.err(err, w, http.StatusNotFound)
		return
	}

	relPath := absPath[len(h.basePath)-1:]

	// File creation request
	if relPath == "/" {
		// Must use POST method according to tus protocol
		if r.Method != "POST" {
			w.Header().Set("Allow", "POST")
			err := errors.New(r.Method + " used against file creation url. Only POST is allowed.")
			h.err(err, w, http.StatusMethodNotAllowed)
			return
		}
	}

	err := errors.New("invalid url path: " + absPath + " - does not match file pattern")
	h.err(err, w, http.StatusNotFound)
}

// err sends a http error response and publishes to the Error channel.
func (h *Handler) err(err error, w http.ResponseWriter, status int) {
	w.WriteHeader(status)
	io.WriteString(w, err.Error()+"\n")

	// non-blocking send
	select {
	case h.sendError <- err:
	default:
	}
}
