package tusd

import (
	"encoding/base64"
	"errors"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
)

var reExtractFileID = regexp.MustCompile(`([^/]+)\/?$`)

var (
	ErrUnsupportedVersion  = errors.New("unsupported version")
	ErrMaxSizeExceeded     = errors.New("maximum size exceeded")
	ErrInvalidContentType  = errors.New("missing or invalid Content-Type header")
	ErrInvalidUploadLength = errors.New("missing or invalid Upload-Length header")
	ErrInvalidOffset       = errors.New("missing or invalid Upload-Offset header")
	ErrNotFound            = errors.New("upload not found")
	ErrFileLocked          = errors.New("file currently locked")
	ErrMismatchOffset      = errors.New("mismatched offset")
	ErrSizeExceeded        = errors.New("resource's size exceeded")
	ErrNotImplemented      = errors.New("feature not implemented")
	ErrUploadNotFinished   = errors.New("one of the partial uploads is not finished")
	ErrInvalidConcat       = errors.New("invalid Upload-Concat header")
	ErrModifyFinal         = errors.New("modifying a final upload is not allowed")
)

// HTTP status codes sent in the response when the specific error is returned.
var ErrStatusCodes = map[error]int{
	ErrUnsupportedVersion:  http.StatusPreconditionFailed,
	ErrMaxSizeExceeded:     http.StatusRequestEntityTooLarge,
	ErrInvalidContentType:  http.StatusBadRequest,
	ErrInvalidUploadLength: http.StatusBadRequest,
	ErrInvalidOffset:       http.StatusBadRequest,
	ErrNotFound:            http.StatusNotFound,
	ErrFileLocked:          423, // Locked (WebDAV) (RFC 4918)
	ErrMismatchOffset:      http.StatusConflict,
	ErrSizeExceeded:        http.StatusRequestEntityTooLarge,
	ErrNotImplemented:      http.StatusNotImplemented,
	ErrUploadNotFinished:   http.StatusBadRequest,
	ErrInvalidConcat:       http.StatusBadRequest,
	ErrModifyFinal:         http.StatusForbidden,
}

// Config provides a way to configure the Handler depending on your needs.
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
	// Initiate the CompleteUploads channel in the Handler struct in order to
	// be notified about complete uploads
	NotifyCompleteUploads bool
	// Logger the logger to use internally
	Logger *log.Logger
}

// UnroutedHandler exposes methods to handle requests as part of the tus protocol,
// such as PostFile, HeadFile, PatchFile and DelFile. In addition the GetFile method
// is provided which is, however, not part of the specification.
type UnroutedHandler struct {
	config        Config
	dataStore     DataStore
	isBasePathAbs bool
	basePath      string
	locks         map[string]bool
	logger        *log.Logger

	// For each finished upload the corresponding info object will be sent using
	// this unbuffered channel. The NotifyCompleteUploads property in the Config
	// struct must be set to true in order to work.
	CompleteUploads chan FileInfo
}

// NewUnroutedHandler creates a new handler without routing using the given
// configuration. It exposes the http handlers which need to be combined with
// a router (aka mux) of your choice. If you are looking for preconfigured
// handler see NewHandler.
func NewUnroutedHandler(config Config) (*UnroutedHandler, error) {
	logger := config.Logger
	if logger == nil {
		logger = log.New(os.Stdout, "[tusd] ", 0)
	}
	base := config.BasePath
	uri, err := url.Parse(base)
	if err != nil {
		return nil, err
	}

	// Ensure base path ends with slash to remove logic from absFileURL
	if base != "" && string(base[len(base)-1]) != "/" {
		base += "/"
	}

	// Ensure base path begins with slash if not absolute (starts with scheme)
	if !uri.IsAbs() && len(base) > 0 && string(base[0]) != "/" {
		base = "/" + base
	}

	handler := &UnroutedHandler{
		config:          config,
		dataStore:       config.DataStore,
		basePath:        base,
		isBasePathAbs:   uri.IsAbs(),
		locks:           make(map[string]bool),
		CompleteUploads: make(chan FileInfo),
		logger:          logger,
	}

	return handler, nil
}

// Middleware checks various aspects of the request and ensures that it
// conforms with the spec. Also handles method overriding for clients which
// cannot make PATCH AND DELETE requests. If you are using the tusd handlers
// directly you will need to wrap at least the POST and PATCH endpoints in
// this middleware.
func (handler *UnroutedHandler) Middleware(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Allow overriding the HTTP method. The reason for this is
		// that some libraries/environments to not support PATCH and
		// DELETE requests, e.g. Flash in a browser and parts of Java
		if newMethod := r.Header.Get("X-HTTP-Method-Override"); newMethod != "" {
			r.Method = newMethod
		}

		go handler.logger.Println(r.Method, r.URL.Path)

		header := w.Header()

		if origin := r.Header.Get("Origin"); origin != "" {
			header.Set("Access-Control-Allow-Origin", origin)

			if r.Method == "OPTIONS" {
				// Preflight request
				header.Set("Access-Control-Allow-Methods", "POST, GET, HEAD, PATCH, DELETE, OPTIONS")
				header.Set("Access-Control-Allow-Headers", "Origin, X-Requested-With, Content-Type, Upload-Length, Upload-Offset, Tus-Resumable, Upload-Metadata")
				header.Set("Access-Control-Max-Age", "86400")

			} else {
				// Actual request
				header.Set("Access-Control-Expose-Headers", "Upload-Offset, Location, Upload-Length, Tus-Version, Tus-Resumable, Tus-Max-Size, Tus-Extension, Upload-Metadata")
			}
		}

		// Set current version used by the server
		header.Set("Tus-Resumable", "1.0.0")

		// Add nosniff to all responses https://golang.org/src/net/http/server.go#L1429
		header.Set("X-Content-Type-Options", "nosniff")

		// Set appropriated headers in case of OPTIONS method allowing protocol
		// discovery and end with an 204 No Content
		if r.Method == "OPTIONS" {
			if handler.config.MaxSize > 0 {
				header.Set("Tus-Max-Size", strconv.FormatInt(handler.config.MaxSize, 10))
			}

			header.Set("Tus-Version", "1.0.0")
			header.Set("Tus-Extension", "creation,concatenation,termination")

			w.WriteHeader(http.StatusNoContent)
			return
		}

		// Test if the version sent by the client is supported
		// GET methods are not checked since a browser may visit this URL and does
		// not include this header. This request is not part of the specification.
		if r.Method != "GET" && r.Header.Get("Tus-Resumable") != "1.0.0" {
			handler.sendError(w, r, ErrUnsupportedVersion)
			return
		}

		// Proceed with routing the request
		h.ServeHTTP(w, r)
	})
}

// PostFile creates a new file upload using the datastore after validating the
// length and parsing the metadata.
func (handler *UnroutedHandler) PostFile(w http.ResponseWriter, r *http.Request) {
	// Parse Upload-Concat header
	isPartial, isFinal, partialUploads, err := parseConcat(r.Header.Get("Upload-Concat"))
	if err != nil {
		handler.sendError(w, r, err)
		return
	}

	// If the upload is a final upload created by concatenation multiple partial
	// uploads the size is sum of all sizes of these files (no need for
	// Upload-Length header)
	var size int64
	if isFinal {
		size, err = handler.sizeOfUploads(partialUploads)
		if err != nil {
			handler.sendError(w, r, err)
			return
		}
	} else {
		size, err = strconv.ParseInt(r.Header.Get("Upload-Length"), 10, 64)
		if err != nil || size < 0 {
			handler.sendError(w, r, ErrInvalidUploadLength)
			return
		}
	}

	// Test whether the size is still allowed
	if handler.config.MaxSize > 0 && size > handler.config.MaxSize {
		handler.sendError(w, r, ErrMaxSizeExceeded)
		return
	}

	// Parse metadata
	meta := parseMeta(r.Header.Get("Upload-Metadata"))

	info := FileInfo{
		Size:           size,
		MetaData:       meta,
		IsPartial:      isPartial,
		IsFinal:        isFinal,
		PartialUploads: partialUploads,
	}

	id, err := handler.dataStore.NewUpload(info)
	if err != nil {
		handler.sendError(w, r, err)
		return
	}

	if isFinal {
		if err := handler.fillFinalUpload(id, partialUploads); err != nil {
			handler.sendError(w, r, err)
			return
		}
	}

	url := handler.absFileURL(r, id)
	w.Header().Set("Location", url)
	w.WriteHeader(http.StatusCreated)
}

// HeadFile returns the length and offset for the HEAD request
func (handler *UnroutedHandler) HeadFile(w http.ResponseWriter, r *http.Request) {

	id := r.URL.Query().Get(":id")
	info, err := handler.dataStore.GetInfo(id)
	if err != nil {
		handler.sendError(w, r, err)
		return
	}

	// Add Upload-Concat header if possible
	if info.IsPartial {
		w.Header().Set("Upload-Concat", "partial")
	}

	if info.IsFinal {
		v := "final;"
		for _, uploadID := range info.PartialUploads {
			v += " " + handler.absFileURL(r, uploadID)
		}
		w.Header().Set("Upload-Concat", v)
	}

	if len(info.MetaData) != 0 {
		w.Header().Set("Upload-Metadata", serializeMeta(info.MetaData))
	}

	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Upload-Length", strconv.FormatInt(info.Size, 10))
	w.Header().Set("Upload-Offset", strconv.FormatInt(info.Offset, 10))
	w.WriteHeader(http.StatusNoContent)
}

// PatchFile adds a chunk to an upload. Only allowed if the upload is not
// locked and enough space is left.
func (handler *UnroutedHandler) PatchFile(w http.ResponseWriter, r *http.Request) {

	//Check for presence of application/offset+octet-stream
	if r.Header.Get("Content-Type") != "application/offset+octet-stream" {
		handler.sendError(w, r, ErrInvalidContentType)
		return
	}

	//Check for presence of a valid Upload-Offset Header
	offset, err := strconv.ParseInt(r.Header.Get("Upload-Offset"), 10, 64)
	if err != nil || offset < 0 {
		handler.sendError(w, r, ErrInvalidOffset)
		return
	}

	id := extractIDFromPath(r.URL.Path)

	// Ensure file is not locked
	if _, ok := handler.locks[id]; ok {
		handler.sendError(w, r, ErrFileLocked)
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
		handler.sendError(w, r, err)
		return
	}

	// Modifying a final upload is not allowed
	if info.IsFinal {
		handler.sendError(w, r, ErrModifyFinal)
		return
	}

	if offset != info.Offset {
		handler.sendError(w, r, ErrMismatchOffset)
		return
	}

	// Get Content-Length if possible
	length := r.ContentLength

	// Test if this upload fits into the file's size
	if offset+length > info.Size {
		handler.sendError(w, r, ErrSizeExceeded)
		return
	}

	maxSize := info.Size - offset
	if length > 0 {
		maxSize = length
	}

	// Limit the
	reader := io.LimitReader(r.Body, maxSize)

	bytesWritten, err := handler.dataStore.WriteChunk(id, offset, reader)
	if err != nil {
		handler.sendError(w, r, err)
		return
	}

	// Send new offset to client
	newOffset := offset + bytesWritten
	w.Header().Set("Upload-Offset", strconv.FormatInt(newOffset, 10))

	// If the upload is completed, send the info out to the channel
	if handler.config.NotifyCompleteUploads && newOffset == info.Size {
		info.Size = newOffset
		handler.CompleteUploads <- info
	}

	w.WriteHeader(http.StatusNoContent)
}

// GetFile handles requests to download a file using a GET request. This is not
// part of the specification.
func (handler *UnroutedHandler) GetFile(w http.ResponseWriter, r *http.Request) {
	id := extractIDFromPath(r.URL.Path)

	// Ensure file is not locked
	if _, ok := handler.locks[id]; ok {
		handler.sendError(w, r, ErrFileLocked)
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
		handler.sendError(w, r, err)
		return
	}

	// Do not do anything if no data is stored yet.
	if info.Offset == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	// Get reader
	src, err := handler.dataStore.GetReader(id)
	if err != nil {
		handler.sendError(w, r, err)
		return
	}

	w.Header().Set("Content-Length", strconv.FormatInt(info.Offset, 10))
	w.WriteHeader(http.StatusOK)
	io.Copy(w, src)

	// Try to close the reader if the io.Closer interface is implemented
	if closer, ok := src.(io.Closer); ok {
		closer.Close()
	}
}

// DelFile terminates an upload permanently.
func (handler *UnroutedHandler) DelFile(w http.ResponseWriter, r *http.Request) {
	id := extractIDFromPath(r.URL.Path)

	// Ensure file is not locked
	if _, ok := handler.locks[id]; ok {
		handler.sendError(w, r, ErrFileLocked)
		return
	}

	// Lock file for further writes (heads are allowed)
	handler.locks[id] = true

	// File will be unlocked regardless of an error or success
	defer func() {
		delete(handler.locks, id)
	}()

	err := handler.dataStore.Terminate(id)
	if err != nil {
		handler.sendError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// Send the error in the response body. The status code will be looked up in
// ErrStatusCodes. If none is found 500 Internal Error will be used.
func (handler *UnroutedHandler) sendError(w http.ResponseWriter, r *http.Request, err error) {
	// Interpret os.ErrNotExist as 404 Not Found
	if os.IsNotExist(err) {
		err = ErrNotFound
	}

	status, ok := ErrStatusCodes[err]
	if !ok {
		status = 500
	}

	reason := err.Error() + "\n"
	if r.Method == "HEAD" {
		reason = ""
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Length", strconv.Itoa(len(reason)))
	w.WriteHeader(status)
	w.Write([]byte(reason))
}

// Make an absolute URLs to the given upload id. If the base path is absolute
// it will be prepended else the host and protocol from the request is used.
func (handler *UnroutedHandler) absFileURL(r *http.Request, id string) string {
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

// The get sum of all sizes for a list of upload ids while checking whether
// all of these uploads are finished yet. This is used to calculate the size
// of a final resource.
func (handler *UnroutedHandler) sizeOfUploads(ids []string) (size int64, err error) {
	for _, id := range ids {
		info, err := handler.dataStore.GetInfo(id)
		if err != nil {
			return size, err
		}

		if info.Offset != info.Size {
			err = ErrUploadNotFinished
			return size, err
		}

		size += info.Size
	}

	return
}

// Fill an empty upload with the content of the uploads by their ids. The data
// will be written in the order as they appear in the slice
func (handler *UnroutedHandler) fillFinalUpload(id string, uploads []string) error {
	readers := make([]io.Reader, len(uploads))

	for index, uploadID := range uploads {
		reader, err := handler.dataStore.GetReader(uploadID)
		if err != nil {
			return err
		}
		readers[index] = reader
	}

	reader := io.MultiReader(readers...)
	_, err := handler.dataStore.WriteChunk(id, 0, reader)

	return err
}

// Parse the Upload-Metadata header as defined in the File Creation extension.
// e.g. Upload-Metadata: name bHVucmpzLnBuZw==,type aW1hZ2UvcG5n
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

// Serialize a map of strings into the Upload-Metadata header format used in the
// response for HEAD requests.
// e.g. Upload-Metadata: name bHVucmpzLnBuZw==,type aW1hZ2UvcG5n
func serializeMeta(meta map[string]string) string {
	header := ""
	for key, value := range meta {
		valueBase64 := base64.StdEncoding.EncodeToString([]byte(value))
		header += key + " " + valueBase64 + ","
	}

	// Remove trailing comma
	if len(header) > 0 {
		header = header[:len(header)-1]
	}

	return header
}

// Parse the Upload-Concat header, e.g.
// Upload-Concat: partial
// Upload-Concat: final; http://tus.io/files/a /files/b/
func parseConcat(header string) (isPartial bool, isFinal bool, partialUploads []string, err error) {
	if len(header) == 0 {
		return
	}

	if header == "partial" {
		isPartial = true
		return
	}

	l := len("final; ")
	if strings.HasPrefix(header, "final; ") && len(header) > l {
		isFinal = true

		list := strings.Split(header[l:], " ")
		for _, value := range list {
			value := strings.TrimSpace(value)
			if value == "" {
				continue
			}

			id := extractIDFromPath(value)
			if id == "" {
				err = ErrInvalidConcat
				return
			}

			partialUploads = append(partialUploads, id)
		}
	}

	// If no valid partial upload ids are extracted this is not a final upload.
	if len(partialUploads) == 0 {
		isFinal = false
		err = ErrInvalidConcat
	}

	return
}

// extractIDFromPath pulls the last segment from the url provided
func extractIDFromPath(url string) string {
	result := reExtractFileID.FindStringSubmatch(url)
	if len(result) != 2 {
		return ""
	}
	return result[1]
}
