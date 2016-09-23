package tusd

import (
	"encoding/base64"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
)

var (
	reExtractFileID  = regexp.MustCompile(`([^/]+)\/?$`)
	reForwardedHost  = regexp.MustCompile(`host=([^,]+)`)
	reForwardedProto = regexp.MustCompile(`proto=(https?)`)
)

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

// UnroutedHandler exposes methods to handle requests as part of the tus protocol,
// such as PostFile, HeadFile, PatchFile and DelFile. In addition the GetFile method
// is provided which is, however, not part of the specification.
type UnroutedHandler struct {
	config        Config
	composer      *StoreComposer
	isBasePathAbs bool
	basePath      string
	logger        *log.Logger
	extensions    string

	// CompleteUploads is used to send notifications whenever an upload is
	// completed by a user. The FileInfo will contain information about this
	// upload after it is completed. Sending to this channel will only
	// happen if the NotifyCompleteUploads field is set to true in the Config
	// structure. Notifications will also be sent for completions using the
	// Concatenation extension.
	CompleteUploads chan FileInfo
	// TerminatedUploads is used to send notifications whenever an upload is
	// terminated by a user. The FileInfo will contain information about this
	// upload gathered before the termination. Sending to this channel will only
	// happen if the NotifyTerminatedUploads field is set to true in the Config
	// structure.
	TerminatedUploads chan FileInfo
	// Metrics provides numbers of the usage for this handler.
	Metrics Metrics
}

// NewUnroutedHandler creates a new handler without routing using the given
// configuration. It exposes the http handlers which need to be combined with
// a router (aka mux) of your choice. If you are looking for preconfigured
// handler see NewHandler.
func NewUnroutedHandler(config Config) (*UnroutedHandler, error) {
	if err := config.validate(); err != nil {
		return nil, err
	}

	// Only promote extesions using the Tus-Extension header which are implemented
	extensions := "creation,creation-with-upload"
	if config.StoreComposer.UsesTerminater {
		extensions += ",termination"
	}
	if config.StoreComposer.UsesConcater {
		extensions += ",concatenation"
	}

	handler := &UnroutedHandler{
		config:            config,
		composer:          config.StoreComposer,
		basePath:          config.BasePath,
		isBasePathAbs:     config.isAbs,
		CompleteUploads:   make(chan FileInfo),
		TerminatedUploads: make(chan FileInfo),
		logger:            config.Logger,
		extensions:        extensions,
		Metrics:           newMetrics(),
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

		handler.log("RequestIncoming", "method", r.Method, "path", r.URL.Path)

		go handler.Metrics.incRequestsTotal(r.Method)

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
			header.Set("Tus-Extension", handler.extensions)

			// Although the 204 No Content status code is a better fit in this case,
			// since we do not have a response body included, we cannot use it here
			// as some browsers only accept 200 OK as successful response to a
			// preflight request. If we send them the 204 No Content the response
			// will be ignored or interpreted as a rejection.
			// For example, the Presto engine, which is used in older versions of
			// Opera, Opera Mobile and Opera Mini, handles CORS this way.
			handler.sendResp(w, r, http.StatusOK)
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
	// Check for presence of application/offset+octet-stream
	containsChunk := false
	if contentType := r.Header.Get("Content-Type"); contentType != "" {
		if contentType != "application/offset+octet-stream" {
			handler.sendError(w, r, ErrInvalidContentType)
			return
		}
		containsChunk = true
	}

	// Only use the proper Upload-Concat header if the concatenation extension
	// is even supported by the data store.
	var concatHeader string
	if handler.composer.UsesConcater {
		concatHeader = r.Header.Get("Upload-Concat")
	}

	// Parse Upload-Concat header
	isPartial, isFinal, partialUploads, err := parseConcat(concatHeader)
	if err != nil {
		handler.sendError(w, r, err)
		return
	}

	// If the upload is a final upload created by concatenation multiple partial
	// uploads the size is sum of all sizes of these files (no need for
	// Upload-Length header)
	var size int64
	if isFinal {
		// A final upload must not contain a chunk within the creation request
		if containsChunk {
			handler.sendError(w, r, ErrModifyFinal)
			return
		}

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

	id, err := handler.composer.Core.NewUpload(info)
	if err != nil {
		handler.sendError(w, r, err)
		return
	}

	// Add the Location header directly after creating the new resource to even
	// include it in cases of failure when an error is returned
	url := handler.absFileURL(r, id)
	w.Header().Set("Location", url)

	go handler.Metrics.incUploadsCreated()
	handler.log("UploadCreated", "id", id, "size", i64toa(size), "url", url)

	if isFinal {
		if err := handler.composer.Concater.ConcatUploads(id, partialUploads); err != nil {
			handler.sendError(w, r, err)
			return
		}
		info.Offset = size

		if handler.config.NotifyCompleteUploads {
			info.ID = id
			handler.CompleteUploads <- info
		}
	}

	if containsChunk {
		if handler.composer.UsesLocker {
			locker := handler.composer.Locker
			if err := locker.LockUpload(id); err != nil {
				handler.sendError(w, r, err)
				return
			}

			defer locker.UnlockUpload(id)
		}

		if err := handler.writeChunk(id, info, w, r); err != nil {
			handler.sendError(w, r, err)
			return
		}
	}

	handler.sendResp(w, r, http.StatusCreated)
}

// HeadFile returns the length and offset for the HEAD request
func (handler *UnroutedHandler) HeadFile(w http.ResponseWriter, r *http.Request) {

	id, err := extractIDFromPath(r.URL.Path)
	if err != nil {
		handler.sendError(w, r, err)
		return
	}

	if handler.composer.UsesLocker {
		locker := handler.composer.Locker
		if err := locker.LockUpload(id); err != nil {
			handler.sendError(w, r, err)
			return
		}

		defer locker.UnlockUpload(id)
	}

	info, err := handler.composer.Core.GetInfo(id)
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
	handler.sendResp(w, r, http.StatusOK)
}

// PatchFile adds a chunk to an upload. Only allowed enough space is left.
func (handler *UnroutedHandler) PatchFile(w http.ResponseWriter, r *http.Request) {

	// Check for presence of application/offset+octet-stream
	if r.Header.Get("Content-Type") != "application/offset+octet-stream" {
		handler.sendError(w, r, ErrInvalidContentType)
		return
	}

	// Check for presence of a valid Upload-Offset Header
	offset, err := strconv.ParseInt(r.Header.Get("Upload-Offset"), 10, 64)
	if err != nil || offset < 0 {
		handler.sendError(w, r, ErrInvalidOffset)
		return
	}

	id, err := extractIDFromPath(r.URL.Path)
	if err != nil {
		handler.sendError(w, r, err)
		return
	}

	if handler.composer.UsesLocker {
		locker := handler.composer.Locker
		if err := locker.LockUpload(id); err != nil {
			handler.sendError(w, r, err)
			return
		}

		defer locker.UnlockUpload(id)
	}

	info, err := handler.composer.Core.GetInfo(id)
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

	// Do not proxy the call to the data store if the upload is already completed
	if info.Offset == info.Size {
		w.Header().Set("Upload-Offset", strconv.FormatInt(offset, 10))
		handler.sendResp(w, r, http.StatusNoContent)
		return
	}

	if err := handler.writeChunk(id, info, w, r); err != nil {
		handler.sendError(w, r, err)
		return
	}

	handler.sendResp(w, r, http.StatusNoContent)
}

// PatchFile adds a chunk to an upload. Only allowed enough space is left.
func (handler *UnroutedHandler) writeChunk(id string, info FileInfo, w http.ResponseWriter, r *http.Request) error {
	// Get Content-Length if possible
	length := r.ContentLength
	offset := info.Offset

	// Test if this upload fits into the file's size
	if offset+length > info.Size {
		return ErrSizeExceeded
	}

	maxSize := info.Size - offset
	if length > 0 {
		maxSize = length
	}

	handler.log("ChunkWriteStart", "id", id, "maxSize", i64toa(maxSize), "offset", i64toa(offset))

	var bytesWritten int64
	// Prevent a nil pointer derefernce when accessing the body which may not be
	// available in the case of a malicious request.
	if r.Body != nil {
		// Limit the data read from the request's body to the allowed maxiumum
		reader := io.LimitReader(r.Body, maxSize)

		var err error
		bytesWritten, err = handler.composer.Core.WriteChunk(id, offset, reader)
		if err != nil {
			return err
		}
	}

	handler.log("ChunkWriteComplete", "id", id, "bytesWritten", i64toa(bytesWritten))

	// Send new offset to client
	newOffset := offset + bytesWritten
	w.Header().Set("Upload-Offset", strconv.FormatInt(newOffset, 10))
	go handler.Metrics.incBytesReceived(uint64(bytesWritten))

	// If the upload is completed, ...
	if newOffset == info.Size {
		// ... allow custom mechanism to finish and cleanup the upload
		if handler.composer.UsesFinisher {
			if err := handler.composer.Finisher.FinishUpload(id); err != nil {
				return err
			}
		}

		// ... send the info out to the channel
		if handler.config.NotifyCompleteUploads {
			info.Offset = newOffset
			handler.CompleteUploads <- info
		}

		go handler.Metrics.incUploadsFinished()
	}

	return nil
}

// GetFile handles requests to download a file using a GET request. This is not
// part of the specification.
func (handler *UnroutedHandler) GetFile(w http.ResponseWriter, r *http.Request) {
	if !handler.composer.UsesGetReader {
		handler.sendError(w, r, ErrNotImplemented)
		return
	}

	id, err := extractIDFromPath(r.URL.Path)
	if err != nil {
		handler.sendError(w, r, err)
		return
	}

	if handler.composer.UsesLocker {
		locker := handler.composer.Locker
		if err := locker.LockUpload(id); err != nil {
			handler.sendError(w, r, err)
			return
		}

		defer locker.UnlockUpload(id)
	}

	info, err := handler.composer.Core.GetInfo(id)
	if err != nil {
		handler.sendError(w, r, err)
		return
	}

	// Do not do anything if no data is stored yet.
	if info.Offset == 0 {
		handler.sendResp(w, r, http.StatusNoContent)
		return
	}

	// Get reader
	src, err := handler.composer.GetReader.GetReader(id)
	if err != nil {
		handler.sendError(w, r, err)
		return
	}

	if filename, ok := info.MetaData["filename"]; ok {
		w.Header().Set("Content-Disposition", "inline;filename="+strconv.Quote(filename))
	}

	w.Header().Set("Content-Length", strconv.FormatInt(info.Offset, 10))
	handler.sendResp(w, r, http.StatusOK)
	io.Copy(w, src)

	// Try to close the reader if the io.Closer interface is implemented
	if closer, ok := src.(io.Closer); ok {
		closer.Close()
	}
}

// DelFile terminates an upload permanently.
func (handler *UnroutedHandler) DelFile(w http.ResponseWriter, r *http.Request) {
	// Abort the request handling if the required interface is not implemented
	if !handler.composer.UsesTerminater {
		handler.sendError(w, r, ErrNotImplemented)
		return
	}

	id, err := extractIDFromPath(r.URL.Path)
	if err != nil {
		handler.sendError(w, r, err)
		return
	}

	if handler.composer.UsesLocker {
		locker := handler.composer.Locker
		if err := locker.LockUpload(id); err != nil {
			handler.sendError(w, r, err)
			return
		}

		defer locker.UnlockUpload(id)
	}

	var info FileInfo
	if handler.config.NotifyTerminatedUploads {
		info, err = handler.composer.Core.GetInfo(id)
		if err != nil {
			handler.sendError(w, r, err)
			return
		}
	}

	err = handler.composer.Terminater.Terminate(id)
	if err != nil {
		handler.sendError(w, r, err)
		return
	}

	handler.sendResp(w, r, http.StatusNoContent)

	if handler.config.NotifyTerminatedUploads {
		handler.TerminatedUploads <- info
	}

	go handler.Metrics.incUploadsTerminated()
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

	handler.log("ResponseOutgoing", "status", strconv.Itoa(status), "method", r.Method, "path", r.URL.Path, "error", err.Error())

	go handler.Metrics.incErrorsTotal(err)
}

// sendResp writes the header to w with the specified status code.
func (handler *UnroutedHandler) sendResp(w http.ResponseWriter, r *http.Request, status int) {
	w.WriteHeader(status)

	handler.log("ResponseOutgoing", "status", strconv.Itoa(status), "method", r.Method, "path", r.URL.Path)
}

// Make an absolute URLs to the given upload id. If the base path is absolute
// it will be prepended else the host and protocol from the request is used.
func (handler *UnroutedHandler) absFileURL(r *http.Request, id string) string {
	if handler.isBasePathAbs {
		return handler.basePath + id
	}

	// Read origin and protocol from request
	host, proto := getHostAndProtocol(r, handler.config.RespectForwardedHeaders)

	url := proto + "://" + host + handler.basePath + id

	return url
}

// getHostAndProtocol extracts the host and used protocol (either HTTP or HTTPS)
// from the given request. If `allowForwarded` is set, the X-Forwarded-Host,
// X-Forwarded-Proto and Forwarded headers will also be checked to
// support proxies.
func getHostAndProtocol(r *http.Request, allowForwarded bool) (host, proto string) {
	if r.TLS != nil {
		proto = "https"
	} else {
		proto = "http"
	}

	host = r.Host

	if !allowForwarded {
		return
	}

	if h := r.Header.Get("X-Forwarded-Host"); h != "" {
		host = h
	}

	if h := r.Header.Get("X-Forwarded-Proto"); h == "http" || h == "https" {
		proto = h
	}

	if h := r.Header.Get("Forwarded"); h != "" {
		if r := reForwardedHost.FindStringSubmatch(h); len(r) == 2 {
			host = r[1]
		}

		if r := reForwardedProto.FindStringSubmatch(h); len(r) == 2 {
			proto = r[1]
		}
	}

	return
}

// The get sum of all sizes for a list of upload ids while checking whether
// all of these uploads are finished yet. This is used to calculate the size
// of a final resource.
func (handler *UnroutedHandler) sizeOfUploads(ids []string) (size int64, err error) {
	for _, id := range ids {
		info, err := handler.composer.Core.GetInfo(id)
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

			id, extractErr := extractIDFromPath(value)
			if extractErr != nil {
				err = extractErr
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
func extractIDFromPath(url string) (string, error) {
	result := reExtractFileID.FindStringSubmatch(url)
	if len(result) != 2 {
		return "", ErrNotFound
	}
	return result[1], nil
}

func i64toa(num int64) string {
	return strconv.FormatInt(num, 10)
}
