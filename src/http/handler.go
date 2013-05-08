package http

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"regexp"
	"strconv"
	"strings"
)

var fileUrlMatcher = regexp.MustCompile("^/([a-z0-9]{32})$")

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
		config:    config,
		Error:     errChan,
		sendError: errChan,
	}, nil
}

// Handler is a http.Handler that implements tus resumable upload protocol.
type Handler struct {
	store  *DataStore
	config HandlerConfig

	// Error provides error events for logging purposes.
	Error <-chan error
	// same chan as Error, used for sending.
	sendError chan<- error
}

// ServeHTTP processes an incoming request according to the tus protocol.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Verify that url matches BasePath
	absPath := r.URL.Path
	if !strings.HasPrefix(absPath, h.config.BasePath) {
		err := errors.New("unknown url: " + absPath + " - does not match BasePath: " + h.config.BasePath)
		h.err(err, w, http.StatusNotFound)
		return
	}

	// example relPath results: "/", "/f81d4fae7dec11d0a76500a0c91e6bf6", etc.
	relPath := absPath[len(h.config.BasePath)-1:]

	// file creation request
	if relPath == "/" {
		if r.Method == "POST" {
			h.createFile(w, r)
			return
		}

		// handle invalid method
		w.Header().Set("Allow", "POST")
		err := errors.New(r.Method + " used against file creation url. Only POST is allowed.")
		h.err(err, w, http.StatusMethodNotAllowed)
		return
	}

	if matches := fileUrlMatcher.FindStringSubmatch(relPath); matches != nil {
		id := matches[1]
		if r.Method == "PATCH" {
			h.patchFile(w, r, id)
			return
		}

		// handle invalid method
		allowed := "PATCH"
		w.Header().Set("Allow", allowed)
		err := errors.New(r.Method + " used against file creation url. Allowed: " + allowed)
		h.err(err, w, http.StatusMethodNotAllowed)
		return
	}

	// handle unknown url
	err := errors.New("unknown url: " + absPath + " - does not match file pattern")
	h.err(err, w, http.StatusNotFound)
}

func (h *Handler) createFile(w http.ResponseWriter, r *http.Request) {
	id := uid()

	finalLength, err := getPositiveIntHeader(r, "Final-Length")
	if err != nil {
		h.err(err, w, http.StatusBadRequest)
		return
	}

	// @TODO: Define meta data extension and implement it here
	// @TODO: Make max finalLength configurable, reply with error if exceeded.
	// 			  This should go into the protocol as well.
	if err := h.store.CreateFile(id, finalLength, nil); err != nil {
		h.err(err, w, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Location", h.absUrl(r, "/"+id))
	w.WriteHeader(http.StatusCreated)
}

func (h *Handler) patchFile(w http.ResponseWriter, r *http.Request, id string) {
	offset, err := getPositiveIntHeader(r, "Offset")
	if err != nil {
		h.err(err, w, http.StatusBadRequest)
		return
	}

	err = h.store.WriteFileChunk(id, offset, r.Body)
	if err != nil {
		h.err(err, w, http.StatusInternalServerError)
		return
	}

	fmt.Printf("success\n")
}

func getPositiveIntHeader(r *http.Request, key string) (int64, error) {
	val := r.Header.Get(key)
	if val == "" {
		return 0, errors.New(key+" header must not be empty")
	}

	intVal, err := strconv.ParseInt(val, 10, 64)
	if err != nil {
		return 0, errors.New("invalid " + key + " header: " + err.Error())
	} else if intVal < 0 {
		return 0, errors.New(key + " header must be > 0")
	}
	return intVal, nil
}

// absUrl turn a relPath (e.g. "/foo") into an absolute url (e.g.
// "http://example.com/foo").
//
// @TODO: Look at r.TLS to determine the url scheme.
// @TODO: Make url prefix user configurable (optional) to deal with reverse
// 				proxies. This could be done by turning BasePath into BaseURL that
//				that could be relative or absolute.
func (h *Handler) absUrl(r *http.Request, relPath string) string {
	return "http://" + r.Host + path.Clean(h.config.BasePath+relPath)
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
