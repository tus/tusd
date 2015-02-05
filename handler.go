package tusd

import (
	"encoding/base64"
	"errors"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/bmizerany/pat"
)

var logger = log.New(os.Stdout, "[tusd] ", 0)

var (
	ErrUnsupportedVersion  = errors.New("unsupported version")
	ErrMaxSizeExceeded     = errors.New("maximum size exceeded")
	ErrInvalidEntityLength = errors.New("missing or invalid Entity-Length header")
	ErrInvalidOffset       = errors.New("missing or invalid Offset header")
	ErrNotFound            = errors.New("upload not found")
	ErrFileLocked          = errors.New("file currently locked")
	ErrIllegalOffset       = errors.New("illegal offset")
	ErrSizeExceeded        = errors.New("resource's size exceeded")
)

// HTTP status codes sent in the response when the specific error is returned.
var ErrStatusCodes = map[error]int{
	ErrUnsupportedVersion:  http.StatusPreconditionFailed,
	ErrMaxSizeExceeded:     http.StatusRequestEntityTooLarge,
	ErrInvalidEntityLength: http.StatusBadRequest,
	ErrInvalidOffset:       http.StatusBadRequest,
	ErrNotFound:            http.StatusNotFound,
	ErrFileLocked:          423, // Locked (WebDAV) (RFC 4918)
	ErrIllegalOffset:       http.StatusConflict,
	ErrSizeExceeded:        http.StatusRequestEntityTooLarge,
}

type Config struct {
	// DataStore implementation used to store and retrieve the single uploads.
	// Must no be nil.
	DataStore DataStore
	// MaxSize defines how many bytes may be stored in one single upload. If its
	// value is is 0 or smaller no limit will be enforced.
	MaxSize int64
	// BasePath defines the URL path used for handling uploads, e.g. "/files/".
	// If no trailing slash is presented it will be added. You may specify an
	// absolute URL containing a scheme, e.g. "http://tus.io"
	BasePath string
}

type Handler struct {
	config        Config
	dataStore     DataStore
	isBasePathAbs bool
	basePath      string
	routeHandler  http.Handler
	locks         map[string]bool
}

// Create a new handler using the given configuration.
func NewHandler(config Config) (*Handler, error) {
	base := config.BasePath
	uri, err := url.Parse(base)
	if err != nil {
		return nil, err
	}

	// Ensure base path ends with slash to remove logic from absFileUrl
	if base != "" && string(base[len(base)-1]) != "/" {
		base += "/"
	}

	// Ensure base path begins with slash if not absolute (starts with scheme)
	if !uri.IsAbs() && len(base) > 0 && string(base[0]) != "/" {
		base = "/" + base
	}

	mux := pat.New()

	handler := &Handler{
		config:        config,
		dataStore:     config.DataStore,
		basePath:      base,
		isBasePathAbs: uri.IsAbs(),
		routeHandler:  mux,
		locks:         make(map[string]bool),
	}

	mux.Post("", http.HandlerFunc(handler.postFile))
	mux.Head(":id", http.HandlerFunc(handler.headFile))
	mux.Add("PATCH", ":id", http.HandlerFunc(handler.patchFile))

	return handler, nil
}

// Implement the http.Handler interface.
func (handler *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	go logger.Println(r.Method, r.URL.Path)

	header := w.Header()

	if origin := r.Header.Get("Origin"); origin != "" {
		header.Set("Access-Control-Allow-Origin", origin)

		if r.Method == "OPTIONS" {
			// Preflight request
			header.Set("Access-Control-Allow-Methods", "POST, HEAD, PATCH, OPTIONS")
			header.Set("Access-Control-Allow-Headers", "Origin, X-Requested-With, Content-Type, Entity-Length, Offset, TUS-Resumable")
			header.Set("Access-Control-Max-Age", "86400")

		} else {
			// Actual request
			header.Set("Access-Control-Expose-Headers", "Offset, Location, Entity-Length, TUS-Version, TUS-Resumable, TUS-Max-Size, TUS-Extension")
		}
	}

	// Set current version used by the server
	header.Set("TUS-Resumable", "1.0.0")

	// Set appropriated headers in case of OPTIONS method allowing protocol
	// discovery and end with an 204 No Content
	if r.Method == "OPTIONS" {
		if handler.config.MaxSize > 0 {
			header.Set("TUS-Max-Size", strconv.FormatInt(handler.config.MaxSize, 10))
		}

		header.Set("TUS-Version", "1.0.0")
		header.Set("TUS-Extension", "file-creation,metadata")

		w.WriteHeader(http.StatusNoContent)
		return
	}

	// Test if the version sent by the client is supported
	if r.Header.Get("TUS-Resumable") != "1.0.0" {
		handler.sendError(w, ErrUnsupportedVersion)
		return
	}

	// Proceed with routing the request
	handler.routeHandler.ServeHTTP(w, r)
}

// Create a new file upload using the datastore after validating the length
// and parsing the metadata.
func (handler *Handler) postFile(w http.ResponseWriter, r *http.Request) {
	size, err := strconv.ParseInt(r.Header.Get("Entity-Length"), 10, 64)
	if err != nil || size < 0 {
		handler.sendError(w, ErrInvalidEntityLength)
		return
	}

	// Test whether the size is still allowed
	if handler.config.MaxSize > 0 && size > handler.config.MaxSize {
		handler.sendError(w, ErrMaxSizeExceeded)
		return
	}

	// Parse metadata
	meta := parseMeta(r.Header.Get("Metadata"))

	id, err := handler.dataStore.NewUpload(size, meta)
	if err != nil {
		handler.sendError(w, err)
		return
	}

	url := handler.absFileUrl(r, id)
	w.Header().Set("Location", url)
	w.WriteHeader(http.StatusCreated)
}

// Returns the length and offset for the HEAD request
func (handler *Handler) headFile(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get(":id")
	info, err := handler.dataStore.GetInfo(id)
	if err != nil {
		// Interpret os.ErrNotExist as 404 Not Found
		if os.IsNotExist(err) {
			err = ErrNotFound
		}
		handler.sendError(w, err)
		return
	}

	w.Header().Set("Entity-Length", strconv.FormatInt(info.Size, 10))
	w.Header().Set("Offset", strconv.FormatInt(info.Offset, 10))
	w.WriteHeader(http.StatusNoContent)
}

// Add a chunk to an upload. Only allowed if the upload is not locked and enough
// space is left.
func (handler *Handler) patchFile(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get(":id")

	// Ensure file is not locked
	if _, ok := handler.locks[id]; ok {
		handler.sendError(w, ErrFileLocked)
		return
	}

	// Lock file for further writes (heads are allowed)
	handler.locks[id] = true

	// File will be unlocked regardless of an error or success
	defer func() {
		delete(handler.locks, id)
	}()

	info, err := handler.dataStore.GetInfo(id)
	if err != nil {
		if os.IsNotExist(err) {
			err = ErrNotFound
		}
		handler.sendError(w, err)
		return
	}

	// Ensure the offsets match
	offset, err := strconv.ParseInt(r.Header.Get("Offset"), 10, 64)
	if err != nil {
		handler.sendError(w, ErrInvalidOffset)
		return
	}

	if offset != info.Offset {
		handler.sendError(w, ErrIllegalOffset)
		return
	}

	// Get Content-Length if possible
	length := r.ContentLength

	// Test if this upload fits into the file's size
	if offset+length > info.Size {
		handler.sendError(w, ErrSizeExceeded)
		return
	}

	maxSize := info.Size - offset
	if length > 0 {
		maxSize = length
	}

	// Limit the
	reader := io.LimitReader(r.Body, maxSize)

	err = handler.dataStore.WriteChunk(id, offset, reader)
	if err != nil {
		handler.sendError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// Send the error in the response body. The status code will be looked up in
// ErrStatusCodes. If none is found 500 Internal Error will be used.
func (handler *Handler) sendError(w http.ResponseWriter, err error) {
	status, ok := ErrStatusCodes[err]
	if !ok {
		status = 500
	}
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(status)
	w.Write([]byte(err.Error() + "\n"))
}

// Make an absolute URLs to the given upload id. If the base path is absolute
// it will be prepended else the host and protocol from the request is used.
func (handler *Handler) absFileUrl(r *http.Request, id string) string {
	if handler.isBasePathAbs {
		return handler.basePath + id
	}

	// Read origin and protocol from request
	url := "http://"
	if r.TLS != nil {
		url = "https://"
	}

	url += r.Host + handler.basePath + id

	return url
}

// Parse the meatadata as defined in the Metadata extension.
// e.g. Metadata: key base64value, key2 base64value
func parseMeta(header string) map[string]string {
	meta := make(map[string]string)

	for _, element := range strings.Split(header, ",") {
		element := strings.TrimSpace(element)

		parts := strings.Split(element, " ")

		// Do not continue with this element if no key and value or presented
		if len(parts) != 2 {
			continue
		}

		// Ignore corrent element if the value is no valid base64
		key := parts[0]
		value, err := base64.StdEncoding.DecodeString(parts[1])
		if err != nil {
			continue
		}

		meta[key] = string(value)
	}

	return meta
}
