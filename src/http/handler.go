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
    "crypto/md5"
    "encoding/hex"
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
	store  *dataStore
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
		} else if r.Method == "HEAD" {
			h.headFile(w, r, id)
			return
		} else if r.Method == "GET" {
			h.getFile(w, r, id)
			return
		}

		// handle invalid method
		allowed := "HEAD,PATCH"
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

	fileType, err := getStringHeader(r, "File-Type")
	if err != nil {
		h.err(err, w, http.StatusBadRequest)
		return
	}

	finalLength, err := getPositiveIntHeader(r, "Final-Length")
	if err != nil {
		h.err(err, w, http.StatusBadRequest)
		return
	}

	// @TODO: Define meta data extension and implement it here
	// @TODO: Make max finalLength configurable, reply with error if exceeded.
	// 			  This should go into the protocol as well.
	if err := h.store.CreateFile(id, fileType, finalLength, nil); err != nil {
		h.err(err, w, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Location", h.absUrl(r, "/"+id))
	w.WriteHeader(http.StatusCreated)
}

func (h *Handler) patchFile(w http.ResponseWriter, r *http.Request, id string) {
	offset, err := getPositiveIntHeader(r, "Offset")
	//contentLength, err := getPositiveIntHeader(r, "Content-Length")
	if err != nil {
		h.err(err, w, http.StatusBadRequest)
		return
	}

	fileType, err := getStringHeader(r, "File-Type")
	if err != nil {
		h.err(err, w, http.StatusBadRequest)
		return
	}

	info, err := h.store.GetInfo(id)
	if err != nil {
		h.err(err, w, http.StatusInternalServerError)
		return
	}

	if offset > info.Offset {
		err = fmt.Errorf("Offset: %d exceeds current offset: %d", offset, info.Offset)
		h.err(err, w, http.StatusForbidden)
		return
	}

	// @TODO Test offset < current offset

	err = h.store.WriteFileChunk(id, fileType, offset, r.Body)
	if err != nil {
		// @TODO handle 404 properly (goes for all h.err calls)
		h.err(err, w, http.StatusInternalServerError)
		return
	}

	w.Header().Set("md5Value", getMD5(h.config.Dir + "/" + id + "." + fileType))
	
}

func (h *Handler) headFile(w http.ResponseWriter, r *http.Request, id string) {
	fileType, err := getStringHeader(r, "File-Type")
	if err != nil {
		h.err(err, w, http.StatusBadRequest)
		return
	}

	info, err := h.store.GetInfo(id)
	if err != nil {
		w.Header().Set("Content-Length", "0")
		w.WriteHeader(http.StatusNotFound)
		return
	}

	w.Header().Set("Offset", fmt.Sprintf("%d", info.Offset))

	if info.Offset == info.FinalLength {
		w.Header().Set("md5Value", getMD5(h.config.Dir + "/" + id + "." + fileType))
	}

}

// GET requests on files aren't part of the protocol yet,
// but it is implemented here anyway for the demo. It still lacks the meta data
// extension in order to send the proper content type header.
func (h *Handler) getFile(w http.ResponseWriter, r *http.Request, fileId string) {
	fileType, err := getStringHeader(r, "File-Type")
	if err != nil {
		h.err(err, w, http.StatusBadRequest)
		return
	}

	info, err := h.store.GetInfo(fileId)
	if os.IsNotExist(err) {
		h.err(err, w, http.StatusNotFound)
		return
	}
	if err != nil {
		h.err(err, w, http.StatusInternalServerError)
		return
	}

	data, err := h.store.ReadFile(fileId,fileType)
	if os.IsNotExist(err) {
		h.err(err, w, http.StatusNotFound)
		return
	}
	if err != nil {
		h.err(err, w, http.StatusInternalServerError)
		return
	}

	defer data.Close()

	w.Header().Set("Offset", strconv.FormatInt(info.Offset, 10))

	// @TODO: Once the meta extension is done, send the proper content type here
	//w.Header().Set("Content-Type", info.Meta.ContentType)

	w.Header().Set("Content-Length", strconv.FormatInt(info.FinalLength, 10))

	if _, err := io.CopyN(w, data, info.FinalLength); err != nil {
		return
	}
}

func getMD5(filename string) (string) {
    fi, err := os.Open(filename)
    if err != nil { panic(err) }
    defer func() {
        if err := fi.Close(); err != nil {
            panic(err)
        }
    }()

    buf := make([]byte, 1024)
    hash := md5.New()
    for {
        n, err := fi.Read(buf)
        if err != nil && err != io.EOF { panic(err) }
        if n == 0 { break }

        if _, err := io.WriteString(hash, string(buf[:n])); err != nil {
            panic(err)
        }
    }

    return hex.EncodeToString(hash.Sum(nil))
}

func getPositiveIntHeader(r *http.Request, key string) (int64, error) {
	val := r.Header.Get(key)
	if val == "" {
		return 0, errors.New(key + " header must not be empty")
	}

	intVal, err := strconv.ParseInt(val, 10, 64)
	if err != nil {
		return 0, errors.New("invalid " + key + " header: " + err.Error())
	} else if intVal < 0 {
		return 0, errors.New(key + " header must be > 0")
	}
	return intVal, nil
}

func getStringHeader(r *http.Request, key string) (string, error) {
	val := r.Header.Get(key)
	if val == "" {
		return "", errors.New(key + " header must not be empty")
	}

	return val, nil
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
