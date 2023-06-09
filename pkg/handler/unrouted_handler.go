package handler

import (
	"context"
	"encoding/base64"
	"io"
	"log"
	"math"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const UploadLengthDeferred = "1"

var (
	reExtractFileID  = regexp.MustCompile(`([^/]+)\/?$`)
	reForwardedHost  = regexp.MustCompile(`host="?([^;"]+)`)
	reForwardedProto = regexp.MustCompile(`proto=(https?)`)
	reMimeType       = regexp.MustCompile(`^[a-z]+\/[a-z0-9\-\+\.]+$`)
)

var (
	ErrUnsupportedVersion               = NewError("ERR_UNSUPPORTED_VERSION", "missing, invalid or unsupported Tus-Resumable header", http.StatusPreconditionFailed)
	ErrMaxSizeExceeded                  = NewError("ERR_MAX_SIZE_EXCEEDED", "maximum size exceeded", http.StatusRequestEntityTooLarge)
	ErrInvalidContentType               = NewError("ERR_INVALID_CONTENT_TYPE", "missing or invalid Content-Type header", http.StatusBadRequest)
	ErrInvalidUploadLength              = NewError("ERR_INVALID_UPLOAD_LENGTH", "missing or invalid Upload-Length header", http.StatusBadRequest)
	ErrInvalidOffset                    = NewError("ERR_INVALID_OFFSET", "missing or invalid Upload-Offset header", http.StatusBadRequest)
	ErrNotFound                         = NewError("ERR_UPLOAD_NOT_FOUND", "upload not found", http.StatusNotFound)
	ErrFileLocked                       = NewError("ERR_UPLOAD_LOCKED", "file currently locked", http.StatusLocked)
	ErrLockTimeout                      = NewError("ERR_LOCK_TIMEOUT", "failed to acquire lock before timeout", http.StatusInternalServerError)
	ErrMismatchOffset                   = NewError("ERR_MISMATCHED_OFFSET", "mismatched offset", http.StatusConflict)
	ErrSizeExceeded                     = NewError("ERR_UPLOAD_SIZE_EXCEEDED", "upload's size exceeded", http.StatusRequestEntityTooLarge)
	ErrNotImplemented                   = NewError("ERR_NOT_IMPLEMENTED", "feature not implemented", http.StatusNotImplemented)
	ErrUploadNotFinished                = NewError("ERR_UPLOAD_NOT_FINISHED", "one of the partial uploads is not finished", http.StatusBadRequest)
	ErrInvalidConcat                    = NewError("ERR_INVALID_CONCAT", "invalid Upload-Concat header", http.StatusBadRequest)
	ErrModifyFinal                      = NewError("ERR_MODIFY_FINAL", "modifying a final upload is not allowed", http.StatusForbidden)
	ErrUploadLengthAndUploadDeferLength = NewError("ERR_AMBIGUOUS_UPLOAD_LENGTH", "provided both Upload-Length and Upload-Defer-Length", http.StatusBadRequest)
	ErrInvalidUploadDeferLength         = NewError("ERR_INVALID_UPLOAD_LENGTH_DEFER", "invalid Upload-Defer-Length header", http.StatusBadRequest)
	ErrUploadStoppedByServer            = NewError("ERR_UPLOAD_STOPPED", "upload has been stopped by server", http.StatusBadRequest)
	ErrUploadRejectedByServer           = NewError("ERR_UPLOAD_REJECTED", "upload creation has been rejected by server", http.StatusBadRequest)
	ErrUploadInterrupted                = NewError("ERR_UPLAOD_INTERRUPTED", "upload has been interrupted by another request for this upload resource", http.StatusBadRequest)

	// TODO: These two responses are 500 for backwards compatability. We should discuss
	// whether it is better to more them to 4XX status codes.
	ErrReadTimeout     = NewError("ERR_READ_TIMEOUT", "timeout while reading request body", http.StatusInternalServerError)
	ErrConnectionReset = NewError("ERR_CONNECTION_RESET", "TCP connection reset by peer", http.StatusInternalServerError)
)

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
	// completed by a user. The HookEvent will contain information about this
	// upload after it is completed. Sending to this channel will only
	// happen if the NotifyCompleteUploads field is set to true in the Config
	// structure. Notifications will also be sent for completions using the
	// Concatenation extension.
	CompleteUploads chan HookEvent
	// TerminatedUploads is used to send notifications whenever an upload is
	// terminated by a user. The HookEvent will contain information about this
	// upload gathered before the termination. Sending to this channel will only
	// happen if the NotifyTerminatedUploads field is set to true in the Config
	// structure.
	TerminatedUploads chan HookEvent
	// UploadProgress is used to send notifications about the progress of the
	// currently running uploads. For each open PATCH request, every second
	// a HookEvent instance will be send over this channel with the Offset field
	// being set to the number of bytes which have been transfered to the server.
	// Please be aware that this number may be higher than the number of bytes
	// which have been stored by the data store! Sending to this channel will only
	// happen if the NotifyUploadProgress field is set to true in the Config
	// structure.
	UploadProgress chan HookEvent
	// CreatedUploads is used to send notifications about the uploads having been
	// created. It triggers post creation and therefore has all the HookEvent incl.
	// the ID available already. It facilitates the post-create hook. Sending to
	// this channel will only happen if the NotifyCreatedUploads field is set to
	// true in the Config structure.
	CreatedUploads chan HookEvent
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
	if config.StoreComposer.UsesLengthDeferrer {
		extensions += ",creation-defer-length"
	}

	handler := &UnroutedHandler{
		config:            config,
		composer:          config.StoreComposer,
		basePath:          config.BasePath,
		isBasePathAbs:     config.isAbs,
		CompleteUploads:   make(chan HookEvent),
		TerminatedUploads: make(chan HookEvent),
		UploadProgress:    make(chan HookEvent),
		CreatedUploads:    make(chan HookEvent),
		logger:            config.Logger,
		extensions:        extensions,
		Metrics:           newMetrics(),
	}

	return handler, nil
}

// SupportedExtensions returns a comma-separated list of the supported tus extensions.
// The availability of an extension usually depends on whether the provided data store
// implements some additional interfaces.
func (handler *UnroutedHandler) SupportedExtensions() string {
	return handler.extensions
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

		handler.log("RequestIncoming", "method", r.Method, "path", r.URL.Path, "requestId", getRequestId(r))

		handler.Metrics.incRequestsTotal(r.Method)

		header := w.Header()

		if origin := r.Header.Get("Origin"); origin != "" {
			header.Set("Access-Control-Allow-Origin", origin)

			if r.Method == "OPTIONS" {
				allowedMethods := "POST, HEAD, PATCH, OPTIONS"
				if !handler.config.DisableDownload {
					allowedMethods += ", GET"
				}

				if !handler.config.DisableTermination {
					allowedMethods += ", DELETE"
				}

				// Preflight request
				header.Add("Access-Control-Allow-Methods", allowedMethods)
				header.Add("Access-Control-Allow-Headers", "Authorization, Origin, X-Requested-With, X-Request-ID, X-HTTP-Method-Override, Content-Type, Upload-Length, Upload-Offset, Tus-Resumable, Upload-Metadata, Upload-Defer-Length, Upload-Concat")
				header.Set("Access-Control-Max-Age", "86400")

			} else {
				// Actual request
				header.Add("Access-Control-Expose-Headers", "Upload-Offset, Location, Upload-Length, Tus-Version, Tus-Resumable, Tus-Max-Size, Tus-Extension, Upload-Metadata, Upload-Defer-Length, Upload-Concat")
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
			c := newContext(w, r)
			handler.sendResp(c, HTTPResponse{
				StatusCode: http.StatusOK,
			})
			return
		}

		// Test if the version sent by the client is supported
		// GET and HEAD methods are not checked since a browser may visit this URL and does
		// not include this header. GET requests are not part of the specification.
		if r.Method != "GET" && r.Method != "HEAD" && r.Header.Get("Tus-Resumable") != "1.0.0" {
			c := newContext(w, r)
			handler.sendError(c, ErrUnsupportedVersion)
			return
		}

		// Proceed with routing the request
		h.ServeHTTP(w, r)
	})
}

// PostFile creates a new file upload using the datastore after validating the
// length and parsing the metadata.
func (handler *UnroutedHandler) PostFile(w http.ResponseWriter, r *http.Request) {
	c := newContext(w, r)

	// Check for presence of application/offset+octet-stream. If another content
	// type is defined, it will be ignored and treated as none was set because
	// some HTTP clients may enforce a default value for this header.
	containsChunk := r.Header.Get("Content-Type") == "application/offset+octet-stream"

	// Only use the proper Upload-Concat header if the concatenation extension
	// is even supported by the data store.
	var concatHeader string
	if handler.composer.UsesConcater {
		concatHeader = r.Header.Get("Upload-Concat")
	}

	// Parse Upload-Concat header
	isPartial, isFinal, partialUploadIDs, err := parseConcat(concatHeader)
	if err != nil {
		handler.sendError(c, err)
		return
	}

	// If the upload is a final upload created by concatenation multiple partial
	// uploads the size is sum of all sizes of these files (no need for
	// Upload-Length header)
	var size int64
	var sizeIsDeferred bool
	var partialUploads []Upload
	if isFinal {
		// A final upload must not contain a chunk within the creation request
		if containsChunk {
			handler.sendError(c, ErrModifyFinal)
			return
		}

		partialUploads, size, err = handler.sizeOfUploads(c, partialUploadIDs)
		if err != nil {
			handler.sendError(c, err)
			return
		}
	} else {
		uploadLengthHeader := r.Header.Get("Upload-Length")
		uploadDeferLengthHeader := r.Header.Get("Upload-Defer-Length")
		size, sizeIsDeferred, err = handler.validateNewUploadLengthHeaders(uploadLengthHeader, uploadDeferLengthHeader)
		if err != nil {
			handler.sendError(c, err)
			return
		}
	}

	// Test whether the size is still allowed
	if handler.config.MaxSize > 0 && size > handler.config.MaxSize {
		handler.sendError(c, ErrMaxSizeExceeded)
		return
	}

	// Parse metadata
	meta := ParseMetadataHeader(r.Header.Get("Upload-Metadata"))

	info := FileInfo{
		Size:           size,
		SizeIsDeferred: sizeIsDeferred,
		MetaData:       meta,
		IsPartial:      isPartial,
		IsFinal:        isFinal,
		PartialUploads: partialUploadIDs,
	}

	resp := HTTPResponse{
		StatusCode: http.StatusCreated,
		Headers:    HTTPHeaders{},
	}

	if handler.config.PreUploadCreateCallback != nil {
		resp2, err := handler.config.PreUploadCreateCallback(newHookEvent(info, r))
		if err != nil {
			handler.sendError(c, err)
			return
		}
		resp = resp.MergeWith(resp2)
	}

	upload, err := handler.composer.Core.NewUpload(c, info)
	if err != nil {
		handler.sendError(c, err)
		return
	}

	info, err = upload.GetInfo(c)
	if err != nil {
		handler.sendError(c, err)
		return
	}

	id := info.ID

	// Add the Location header directly after creating the new resource to even
	// include it in cases of failure when an error is returned
	url := handler.absFileURL(r, id)
	resp.Headers["Location"] = url

	handler.Metrics.incUploadsCreated()
	handler.log("UploadCreated", "id", id, "size", i64toa(size), "url", url)

	if handler.config.NotifyCreatedUploads {
		handler.CreatedUploads <- newHookEvent(info, r)
	}

	if isFinal {
		concatableUpload := handler.composer.Concater.AsConcatableUpload(upload)
		if err := concatableUpload.ConcatUploads(c, partialUploads); err != nil {
			handler.sendError(c, err)
			return
		}
		info.Offset = size

		if handler.config.NotifyCompleteUploads {
			handler.CompleteUploads <- newHookEvent(info, r)
		}
	}

	if containsChunk {
		if handler.composer.UsesLocker {
			lock, err := handler.lockUpload(c, id)
			if err != nil {
				handler.sendError(c, err)
				return
			}

			defer lock.Unlock()
		}

		resp, err = handler.writeChunk(c, resp, upload, info)
		if err != nil {
			handler.sendError(c, err)
			return
		}
	} else if !sizeIsDeferred && size == 0 {
		// Directly finish the upload if the upload is empty (i.e. has a size of 0).
		// This statement is in an else-if block to avoid causing duplicate calls
		// to finishUploadIfComplete if an upload is empty and contains a chunk.
		resp, err = handler.finishUploadIfComplete(c, resp, upload, info)
		if err != nil {
			handler.sendError(c, err)
			return
		}
	}

	handler.sendResp(c, resp)
}

// HeadFile returns the length and offset for the HEAD request
func (handler *UnroutedHandler) HeadFile(w http.ResponseWriter, r *http.Request) {
	c := newContext(w, r)

	id, err := extractIDFromPath(r.URL.Path)
	if err != nil {
		handler.sendError(c, err)
		return
	}

	if handler.composer.UsesLocker {
		lock, err := handler.lockUpload(c, id)
		if err != nil {
			handler.sendError(c, err)
			return
		}

		defer lock.Unlock()
	}

	upload, err := handler.composer.Core.GetUpload(c, id)
	if err != nil {
		handler.sendError(c, err)
		return
	}

	info, err := upload.GetInfo(c)
	if err != nil {
		handler.sendError(c, err)
		return
	}

	resp := HTTPResponse{
		StatusCode: http.StatusOK,
		Headers:    make(HTTPHeaders),
	}

	// Add Upload-Concat header if possible
	if info.IsPartial {
		resp.Headers["Upload-Concat"] = "partial"
	}

	if info.IsFinal {
		v := "final;"
		for _, uploadID := range info.PartialUploads {
			v += handler.absFileURL(r, uploadID) + " "
		}
		// Remove trailing space
		v = v[:len(v)-1]

		resp.Headers["Upload-Concat"] = v
	}

	if len(info.MetaData) != 0 {
		resp.Headers["Upload-Metadata"] = SerializeMetadataHeader(info.MetaData)
	}

	if info.SizeIsDeferred {
		resp.Headers["Upload-Defer-Length"] = UploadLengthDeferred
	} else {
		resp.Headers["Upload-Length"] = strconv.FormatInt(info.Size, 10)
		resp.Headers["Content-Length"] = strconv.FormatInt(info.Size, 10)
	}

	resp.Headers["Cache-Control"] = "no-store"
	resp.Headers["Upload-Offset"] = strconv.FormatInt(info.Offset, 10)
	handler.sendResp(c, resp)
}

// PatchFile adds a chunk to an upload. This operation is only allowed
// if enough space in the upload is left.
func (handler *UnroutedHandler) PatchFile(w http.ResponseWriter, r *http.Request) {
	c := newContext(w, r)

	// Check for presence of application/offset+octet-stream
	if r.Header.Get("Content-Type") != "application/offset+octet-stream" {
		handler.sendError(c, ErrInvalidContentType)
		return
	}

	// Check for presence of a valid Upload-Offset Header
	offset, err := strconv.ParseInt(r.Header.Get("Upload-Offset"), 10, 64)
	if err != nil || offset < 0 {
		handler.sendError(c, ErrInvalidOffset)
		return
	}

	id, err := extractIDFromPath(r.URL.Path)
	if err != nil {
		handler.sendError(c, err)
		return
	}

	if handler.composer.UsesLocker {
		lock, err := handler.lockUpload(c, id)
		if err != nil {
			handler.sendError(c, err)
			return
		}

		defer lock.Unlock()
	}

	upload, err := handler.composer.Core.GetUpload(c, id)
	if err != nil {
		handler.sendError(c, err)
		return
	}

	info, err := upload.GetInfo(c)
	if err != nil {
		handler.sendError(c, err)
		return
	}

	// Modifying a final upload is not allowed
	if info.IsFinal {
		handler.sendError(c, ErrModifyFinal)
		return
	}

	if offset != info.Offset {
		handler.sendError(c, ErrMismatchOffset)
		return
	}

	resp := HTTPResponse{
		StatusCode: http.StatusNoContent,
		Headers:    make(HTTPHeaders, 1), // Initialize map, so writeChunk can set the Upload-Offset header.
	}

	// Do not proxy the call to the data store if the upload is already completed
	if !info.SizeIsDeferred && info.Offset == info.Size {
		resp.Headers["Upload-Offset"] = strconv.FormatInt(offset, 10)
		handler.sendResp(c, resp)
		return
	}

	if r.Header.Get("Upload-Length") != "" {
		if !handler.composer.UsesLengthDeferrer {
			handler.sendError(c, ErrNotImplemented)
			return
		}
		if !info.SizeIsDeferred {
			handler.sendError(c, ErrInvalidUploadLength)
			return
		}
		uploadLength, err := strconv.ParseInt(r.Header.Get("Upload-Length"), 10, 64)
		if err != nil || uploadLength < 0 || uploadLength < info.Offset || (handler.config.MaxSize > 0 && uploadLength > handler.config.MaxSize) {
			handler.sendError(c, ErrInvalidUploadLength)
			return
		}

		lengthDeclarableUpload := handler.composer.LengthDeferrer.AsLengthDeclarableUpload(upload)
		if err := lengthDeclarableUpload.DeclareLength(c, uploadLength); err != nil {
			handler.sendError(c, err)
			return
		}

		info.Size = uploadLength
		info.SizeIsDeferred = false
	}

	resp, err = handler.writeChunk(c, resp, upload, info)
	if err != nil {
		handler.sendError(c, err)
		return
	}

	handler.sendResp(c, resp)
}

// writeChunk reads the body from the requests r and appends it to the upload
// with the corresponding id. Afterwards, it will set the necessary response
// headers but will not send the response.
func (handler *UnroutedHandler) writeChunk(c *httpContext, resp HTTPResponse, upload Upload, info FileInfo) (HTTPResponse, error) {
	// Get Content-Length if possible
	r := c.req
	length := r.ContentLength
	offset := info.Offset
	id := info.ID

	// Test if this upload fits into the file's size
	if !info.SizeIsDeferred && offset+length > info.Size {
		return resp, ErrSizeExceeded
	}

	maxSize := info.Size - offset
	// If the upload's length is deferred and the PATCH request does not contain the Content-Length
	// header (which is allowed if 'Transfer-Encoding: chunked' is used), we still need to set limits for
	// the body size.
	if info.SizeIsDeferred {
		if handler.config.MaxSize > 0 {
			// Ensure that the upload does not exceed the maximum upload size
			maxSize = handler.config.MaxSize - offset
		} else {
			// If no upload limit is given, we allow arbitrary sizes
			maxSize = math.MaxInt64
		}
	}
	if length > 0 {
		maxSize = length
	}

	handler.log("ChunkWriteStart", "id", id, "maxSize", i64toa(maxSize), "offset", i64toa(offset))

	var bytesWritten int64
	var err error
	// Prevent a nil pointer dereference when accessing the body which may not be
	// available in the case of a malicious request.
	if r.Body != nil {
		// Limit the data read from the request's body to the allowed maximum
		c.body = newBodyReader(r.Body, maxSize)

		// We use a context object to allow the hook system to cancel an upload
		uploadCtx, stopUpload := context.WithCancel(context.Background())
		info.stopUpload = stopUpload
		// terminateUpload specifies whether the upload should be deleted after
		// the write has finished
		terminateUpload := false
		// Cancel the context when the function exits to ensure that the goroutine
		// is properly cleaned up
		defer stopUpload()

		go func() {
			// Interrupt the Read() call from the request body
			<-uploadCtx.Done()
			// TODO: Consider using CloseWithError function from BodyReader
			terminateUpload = true
			r.Body.Close()
		}()

		if handler.config.NotifyUploadProgress {
			stopProgressEvents := handler.sendProgressMessages(newHookEvent(info, r), c.body)
			defer close(stopProgressEvents)
		}

		bytesWritten, err = upload.WriteChunk(c, offset, c.body)
		if terminateUpload && handler.composer.UsesTerminater {
			if terminateErr := handler.terminateUpload(c, upload, info); terminateErr != nil {
				// We only log this error and not show it to the user since this
				// termination error is not relevant to the uploading client
				handler.log("UploadStopTerminateError", "id", id, "error", terminateErr.Error())
			}
		}

		// If we encountered an error while reading the body from the HTTP request, log it, but only include
		// it in the response, if the store did not also return an error.
		if bodyErr := c.body.hasError(); bodyErr != nil {
			handler.log("BodyReadError", "id", id, "error", bodyErr.Error())
			if err == nil {
				err = bodyErr
			}
		}

		// If the upload was stopped by the server, send an error response indicating this.
		// TODO: Include a custom reason for the end user why the upload was stopped.
		if terminateUpload {
			err = ErrUploadStoppedByServer
		}
	}

	handler.log("ChunkWriteComplete", "id", id, "bytesWritten", i64toa(bytesWritten))

	if err != nil {
		return resp, err
	}

	// Send new offset to client
	newOffset := offset + bytesWritten
	resp.Headers["Upload-Offset"] = strconv.FormatInt(newOffset, 10)
	handler.Metrics.incBytesReceived(uint64(bytesWritten))
	info.Offset = newOffset

	return handler.finishUploadIfComplete(c, resp, upload, info)
}

// finishUploadIfComplete checks whether an upload is completed (i.e. upload offset
// matches upload size) and if so, it will call the data store's FinishUpload
// function and send the necessary message on the CompleteUpload channel.
func (handler *UnroutedHandler) finishUploadIfComplete(c *httpContext, resp HTTPResponse, upload Upload, info FileInfo) (HTTPResponse, error) {
	r := c.req

	// If the upload is completed, ...
	if !info.SizeIsDeferred && info.Offset == info.Size {
		// ... allow the data storage to finish and cleanup the upload
		if err := upload.FinishUpload(c); err != nil {
			return resp, err
		}

		// ... allow the hook callback to run before sending the response
		if handler.config.PreFinishResponseCallback != nil {
			resp2, err := handler.config.PreFinishResponseCallback(newHookEvent(info, r))
			if err != nil {
				return resp, err
			}
			resp = resp.MergeWith(resp2)
		}

		handler.log("UploadFinished", "id", info.ID, "size", strconv.FormatInt(info.Size, 10))
		handler.Metrics.incUploadsFinished()

		// ... send the info out to the channel
		if handler.config.NotifyCompleteUploads {
			handler.CompleteUploads <- newHookEvent(info, r)
		}
	}

	return resp, nil
}

// GetFile handles requests to download a file using a GET request. This is not
// part of the specification.
func (handler *UnroutedHandler) GetFile(w http.ResponseWriter, r *http.Request) {
	c := newContext(w, r)

	id, err := extractIDFromPath(r.URL.Path)
	if err != nil {
		handler.sendError(c, err)
		return
	}

	if handler.composer.UsesLocker {
		lock, err := handler.lockUpload(c, id)
		if err != nil {
			handler.sendError(c, err)
			return
		}

		defer lock.Unlock()
	}

	upload, err := handler.composer.Core.GetUpload(c, id)
	if err != nil {
		handler.sendError(c, err)
		return
	}

	info, err := upload.GetInfo(c)
	if err != nil {
		handler.sendError(c, err)
		return
	}

	contentType, contentDisposition := filterContentType(info)
	resp := HTTPResponse{
		StatusCode: http.StatusOK,
		Headers: HTTPHeaders{
			"Content-Length":      strconv.FormatInt(info.Offset, 10),
			"Content-Type":        contentType,
			"Content-Disposition": contentDisposition,
		},
		Body: "", // Body is intentionally left empty, and we copy it manually in later.
	}

	// If no data has been uploaded yet, respond with an empty "204 No Content" status.
	if info.Offset == 0 {
		resp.StatusCode = http.StatusNoContent
		handler.sendResp(c, resp)
		return
	}

	src, err := upload.GetReader(c)
	if err != nil {
		handler.sendError(c, err)
		return
	}

	handler.sendResp(c, resp)
	io.Copy(w, src)

	src.Close()
}

// mimeInlineBrowserWhitelist is a map containing MIME types which should be
// allowed to be rendered by browser inline, instead of being forced to be
// downloaded. For example, HTML or SVG files are not allowed, since they may
// contain malicious JavaScript. In a similiar fashion PDF is not on this list
// as their parsers commonly contain vulnerabilities which can be exploited.
// The values of this map does not convey any meaning and are therefore just
// empty structs.
var mimeInlineBrowserWhitelist = map[string]struct{}{
	"text/plain": {},

	"image/png":  {},
	"image/jpeg": {},
	"image/gif":  {},
	"image/bmp":  {},
	"image/webp": {},

	"audio/wave":      {},
	"audio/wav":       {},
	"audio/x-wav":     {},
	"audio/x-pn-wav":  {},
	"audio/webm":      {},
	"video/webm":      {},
	"audio/ogg":       {},
	"video/ogg":       {},
	"application/ogg": {},
}

// filterContentType returns the values for the Content-Type and
// Content-Disposition headers for a given upload. These values should be used
// in responses for GET requests to ensure that only non-malicious file types
// are shown directly in the browser. It will extract the file name and type
// from the "fileame" and "filetype".
// See https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers/Content-Disposition
func filterContentType(info FileInfo) (contentType string, contentDisposition string) {
	filetype := info.MetaData["filetype"]

	if reMimeType.MatchString(filetype) {
		// If the filetype from metadata is well formed, we forward use this
		// for the Content-Type header. However, only whitelisted mime types
		// will be allowed to be shown inline in the browser
		contentType = filetype
		if _, isWhitelisted := mimeInlineBrowserWhitelist[filetype]; isWhitelisted {
			contentDisposition = "inline"
		} else {
			contentDisposition = "attachment"
		}
	} else {
		// If the filetype from the metadata is not well formed, we use a
		// default type and force the browser to download the content.
		contentType = "application/octet-stream"
		contentDisposition = "attachment"
	}

	// Add a filename to Content-Disposition if one is available in the metadata
	if filename, ok := info.MetaData["filename"]; ok {
		contentDisposition += ";filename=" + strconv.Quote(filename)
	}

	return contentType, contentDisposition
}

// DelFile terminates an upload permanently.
func (handler *UnroutedHandler) DelFile(w http.ResponseWriter, r *http.Request) {
	c := newContext(w, r)

	// Abort the request handling if the required interface is not implemented
	if !handler.composer.UsesTerminater {
		handler.sendError(c, ErrNotImplemented)
		return
	}

	id, err := extractIDFromPath(r.URL.Path)
	if err != nil {
		handler.sendError(c, err)
		return
	}

	if handler.composer.UsesLocker {
		lock, err := handler.lockUpload(c, id)
		if err != nil {
			handler.sendError(c, err)
			return
		}

		defer lock.Unlock()
	}

	upload, err := handler.composer.Core.GetUpload(c, id)
	if err != nil {
		handler.sendError(c, err)
		return
	}

	var info FileInfo
	if handler.config.NotifyTerminatedUploads {
		info, err = upload.GetInfo(c)
		if err != nil {
			handler.sendError(c, err)
			return
		}
	}

	err = handler.terminateUpload(c, upload, info)
	if err != nil {
		handler.sendError(c, err)
		return
	}

	handler.sendResp(c, HTTPResponse{
		StatusCode: http.StatusNoContent,
	})
}

// terminateUpload passes a given upload to the DataStore's Terminater,
// send the corresponding upload info on the TerminatedUploads channnel
// and updates the statistics.
// Note the the info argument is only needed if the terminated uploads
// notifications are enabled.
func (handler *UnroutedHandler) terminateUpload(c *httpContext, upload Upload, info FileInfo) error {
	terminatableUpload := handler.composer.Terminater.AsTerminatableUpload(upload)

	err := terminatableUpload.Terminate(c)
	if err != nil {
		return err
	}

	if handler.config.NotifyTerminatedUploads {
		handler.TerminatedUploads <- newHookEvent(info, c.req)
	}

	handler.log("UploadTerminated", "id", info.ID)
	handler.Metrics.incUploadsTerminated()

	return nil
}

// Send the error in the response body. The status code will be looked up in
// ErrStatusCodes. If none is found 500 Internal Error will be used.
func (handler *UnroutedHandler) sendError(c *httpContext, err error) {
	// Errors for read timeouts contain too much information which is not
	// necessary for us and makes grouping for the metrics harder. The error
	// message looks like: read tcp 127.0.0.1:1080->127.0.0.1:53673: i/o timeout
	// Therefore, we use a common error message for all of them.
	if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
		err = ErrReadTimeout
	}

	// Errors for connnection resets also contain TCP details, we don't need, e.g:
	// read tcp 127.0.0.1:1080->127.0.0.1:10023: read: connection reset by peer
	// Therefore, we also trim those down.
	if strings.HasSuffix(err.Error(), "read: connection reset by peer") {
		err = ErrConnectionReset
	}

	// TODO: Decide if we should handle this in here, in body_reader or not at all.
	// If the HTTP PATCH request gets interrupted in the middle (e.g. because
	// the user wants to pause the upload), Go's net/http returns an io.ErrUnexpectedEOF.
	// However, for the handler it's not important whether the stream has ended
	// on purpose or accidentally.
	//if err == io.ErrUnexpectedEOF {
	//	err = nil
	//}

	// TODO: Decide if we want to ignore connection reset errors all together.
	// In some cases, the HTTP connection gets reset by the other peer. This is not
	// necessarily the tus client but can also be a proxy in front of tusd, e.g. HAProxy 2
	// is known to reset the connection to tusd, when the tus client closes the connection.
	// To avoid erroring out in this case and loosing the uploaded data, we can ignore
	// the error here without causing harm.
	//if strings.Contains(err.Error(), "read: connection reset by peer") {
	//	err = nil
	//}

	r := c.req

	detailedErr, ok := err.(Error)
	if !ok {
		handler.log("InternalServerError", "message", err.Error(), "method", r.Method, "path", r.URL.Path, "requestId", getRequestId(r))
		detailedErr = NewError("ERR_INTERNAL_SERVER_ERROR", err.Error(), http.StatusInternalServerError)
	}

	// If we are sending the response for a HEAD request, ensure that we are not including
	// any response body.
	if r.Method == "HEAD" {
		detailedErr.HTTPResponse.Body = ""
	}

	handler.sendResp(c, detailedErr.HTTPResponse)
	handler.Metrics.incErrorsTotal(detailedErr)
}

// sendResp writes the header to w with the specified status code.
func (handler *UnroutedHandler) sendResp(c *httpContext, resp HTTPResponse) {
	resp.writeTo(c.res)

	handler.log("ResponseOutgoing", "status", strconv.Itoa(resp.StatusCode), "method", c.req.Method, "path", c.req.URL.Path, "requestId", getRequestId(c.req), "body", resp.Body)
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

// sendProgressMessage will send a notification over the UploadProgress channel
// every second, indicating how much data has been transfered to the server.
// It will stop sending these instances once the returned channel has been
// closed.
func (handler *UnroutedHandler) sendProgressMessages(hook HookEvent, reader *bodyReader) chan<- struct{} {
	previousOffset := int64(0)
	originalOffset := hook.Upload.Offset
	stop := make(chan struct{}, 1)

	go func() {
		for {
			select {
			case <-stop:
				hook.Upload.Offset = originalOffset + reader.bytesRead()
				if hook.Upload.Offset != previousOffset {
					handler.UploadProgress <- hook
					previousOffset = hook.Upload.Offset
				}
				return
			case <-time.After(handler.config.UploadProgressInterval):
				hook.Upload.Offset = originalOffset + reader.bytesRead()
				if hook.Upload.Offset != previousOffset {
					handler.UploadProgress <- hook
					previousOffset = hook.Upload.Offset
				}
			}
		}
	}()

	return stop
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
func (handler *UnroutedHandler) sizeOfUploads(ctx context.Context, ids []string) (partialUploads []Upload, size int64, err error) {
	partialUploads = make([]Upload, len(ids))

	for i, id := range ids {
		upload, err := handler.composer.Core.GetUpload(ctx, id)
		if err != nil {
			return nil, 0, err
		}

		info, err := upload.GetInfo(ctx)
		if err != nil {
			return nil, 0, err
		}

		if info.SizeIsDeferred || info.Offset != info.Size {
			err = ErrUploadNotFinished
			return nil, 0, err
		}

		size += info.Size
		partialUploads[i] = upload
	}

	return
}

// Verify that the Upload-Length and Upload-Defer-Length headers are acceptable for creating a
// new upload
func (handler *UnroutedHandler) validateNewUploadLengthHeaders(uploadLengthHeader string, uploadDeferLengthHeader string) (uploadLength int64, uploadLengthDeferred bool, err error) {
	haveBothLengthHeaders := uploadLengthHeader != "" && uploadDeferLengthHeader != ""
	haveInvalidDeferHeader := uploadDeferLengthHeader != "" && uploadDeferLengthHeader != UploadLengthDeferred
	lengthIsDeferred := uploadDeferLengthHeader == UploadLengthDeferred

	if lengthIsDeferred && !handler.composer.UsesLengthDeferrer {
		err = ErrNotImplemented
	} else if haveBothLengthHeaders {
		err = ErrUploadLengthAndUploadDeferLength
	} else if haveInvalidDeferHeader {
		err = ErrInvalidUploadDeferLength
	} else if lengthIsDeferred {
		uploadLengthDeferred = true
	} else {
		uploadLength, err = strconv.ParseInt(uploadLengthHeader, 10, 64)
		if err != nil || uploadLength < 0 {
			err = ErrInvalidUploadLength
		}
	}

	return
}

// lockUpload creates a new lock for the given upload ID and attempts to lock it.
// The created lock is returned if it was aquired successfully.
func (handler *UnroutedHandler) lockUpload(c *httpContext, id string) (Lock, error) {
	lock, err := handler.composer.Locker.NewLock(id)
	if err != nil {
		return nil, err
	}

	// TODO: Make lock timeout configurable
	ctx, cancelContext := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancelContext()
	releaseLock := func() {
		if c.body != nil {
			handler.log("UploadInterrupted", "id", id, "requestId", getRequestId(c.req))
			c.body.closeWithError(ErrUploadInterrupted)
		}
	}

	if err := lock.Lock(ctx, releaseLock); err != nil {
		return nil, err
	}

	return lock, nil
}

// ParseMetadataHeader parses the Upload-Metadata header as defined in the
// File Creation extension.
// e.g. Upload-Metadata: name bHVucmpzLnBuZw==,type aW1hZ2UvcG5n
func ParseMetadataHeader(header string) map[string]string {
	meta := make(map[string]string)

	for _, element := range strings.Split(header, ",") {
		element := strings.TrimSpace(element)

		parts := strings.Split(element, " ")

		if len(parts) > 2 {
			continue
		}

		key := parts[0]
		if key == "" {
			continue
		}

		value := ""
		if len(parts) == 2 {
			// Ignore current element if the value is no valid base64
			dec, err := base64.StdEncoding.DecodeString(parts[1])
			if err != nil {
				continue
			}

			value = string(dec)
		}

		meta[key] = value
	}

	return meta
}

// SerializeMetadataHeader serializes a map of strings into the Upload-Metadata
// header format used in the response for HEAD requests.
// e.g. Upload-Metadata: name bHVucmpzLnBuZw==,type aW1hZ2UvcG5n
func SerializeMetadataHeader(meta map[string]string) string {
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
// Upload-Concat: final;http://tus.io/files/a /files/b/
func parseConcat(header string) (isPartial bool, isFinal bool, partialUploads []string, err error) {
	if len(header) == 0 {
		return
	}

	if header == "partial" {
		isPartial = true
		return
	}

	l := len("final;")
	if strings.HasPrefix(header, "final;") && len(header) > l {
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

// getRequestId returns the value of the X-Request-ID header, if available,
// and also takes care of truncating the input.
func getRequestId(r *http.Request) string {
	reqId := r.Header.Get("X-Request-ID")
	if reqId == "" {
		return ""
	}

	// Limit the length of the request ID to 36 characters, which is enough
	// to fit a UUID.
	if len(reqId) > 36 {
		reqId = reqId[:36]
	}

	return reqId
}
