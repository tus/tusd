package main

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path"
	"regexp"
	"strconv"
)

// fileRoute matches /files/<id>. Go seems to use \r to terminate header
// values, so to ease bash scripting, the route ignores a trailing \r in the
// route. Better ideas are welcome.
var fileRoute = regexp.MustCompile("^/files/([^/\r\n]+)\r?$")

var filesRoute = regexp.MustCompile("^/files/?$")
var dataStore *DataStore

func init() {
	wd, err := os.Getwd()
	if err != nil {
		panic(err)
	}

	dataDir := path.Join(wd, "tus_data")
	if configDir := os.Getenv("TUSD_DATA_DIR"); configDir != "" {
		dataDir = configDir
	}

	// dataStoreSize limits the storage used by the data store. If exceeded, the
	// data store will start garbage collection old files until enough storage is
	// available again.
	var dataStoreSize int64
	dataStoreSize = 1024 * 1024 * 1024
	if configStoreSize := os.Getenv("TUSD_DATA_STORE_MAXSIZE"); configStoreSize != "" {
		parsed, err := strconv.ParseInt(configStoreSize, 10, 64)
		if err != nil {
			panic(errors.New("Invalid data store max size configured"))
		}
		dataStoreSize = parsed
	}

	log.Print("Datastore directory: ", dataDir)
	log.Print("Datastore max size: ", dataStoreSize)

	if err := os.MkdirAll(dataDir, 0777); err != nil {
		panic(err)
	}
	dataStore = NewDataStore(dataDir, dataStoreSize)
}

func serveHttp() error {
	http.HandleFunc("/", route)

	addr := ":1080"
	if port := os.Getenv("TUSD_PORT"); port != "" {
		addr = ":" + port
	}
	log.Printf("serving clients at %s", addr)

	return http.ListenAndServe(addr, nil)
}

func route(w http.ResponseWriter, r *http.Request) {
	log.Printf("request: %s %s", r.Method, r.URL.RequestURI())

	w.Header().Set("Server", "tusd")

	// Allow CORS for almost everything. This needs to be revisted / limited to
	// routes and methods that need it.
	w.Header().Add("Access-Control-Allow-Origin", "*")
	w.Header().Add("Access-Control-Allow-Methods", "HEAD,GET,PUT,POST,DELETE")
	w.Header().Add("Access-Control-Allow-Headers", "Origin, X-Requested-With, Content-Type, Accept, Content-Range, Content-Disposition")
	w.Header().Add("Access-Control-Expose-Headers", "Location, Range, Content-Disposition")

	if r.Method == "OPTIONS" {
		reply(w, http.StatusOK, "")
		return
	}

	if r.Method == "POST" && filesRoute.Match([]byte(r.URL.Path)) {
		postFiles(w, r)
	} else if match := fileRoute.FindStringSubmatch(r.URL.Path); match != nil {
		id := match[1]
		switch r.Method {
		case "HEAD":
			headFile(w, r, id)
		case "GET":
			getFile(w, r, id)
		case "PUT":
			putFile(w, r, id)
		default:
			reply(w, http.StatusMethodNotAllowed, "Invalid http method")
		}
	} else {
		reply(w, http.StatusNotFound, "No matching route")
	}
}

func reply(w http.ResponseWriter, code int, message string) {
	w.WriteHeader(code)
	fmt.Fprintf(w, "%d - %s: %s\n", code, http.StatusText(code), message)
}

func postFiles(w http.ResponseWriter, r *http.Request) {
	contentRange, err := parseContentRange(r.Header.Get("Content-Range"))
	if err != nil {
		reply(w, http.StatusBadRequest, err.Error())
		return
	}

	if contentRange.Size == -1 {
		reply(w, http.StatusBadRequest, "Content-Range must indicate total file size.")
		return
	}

	contentType := r.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	contentDisposition := r.Header.Get("Content-Disposition")

	id := uid()
	if err := dataStore.CreateFile(id, contentRange.Size, contentType, contentDisposition); err != nil {
		reply(w, http.StatusInternalServerError, err.Error())
		return
	}

	if contentRange.End != -1 {
		err := dataStore.WriteFileChunk(id, contentRange.Start, contentRange.End, r.Body)
		if os.IsNotExist(err) {
			reply(w, http.StatusNotFound, err.Error())
			return
		} else if err != nil {
			reply(w, http.StatusInternalServerError, err.Error())
			return
		}

	}

	w.Header().Set("Location", "http://"+r.Host+"/files/"+id)
	setFileHeaders(w, id)
	w.WriteHeader(http.StatusCreated)
}

func headFile(w http.ResponseWriter, r *http.Request, fileId string) {
	// Work around a bug in Go that would cause HEAD responses to hang. Should be
	// fixed in future release, see:
	// http://code.google.com/p/go/issues/detail?id=4126
	w.Header().Set("Content-Length", "0")
	setFileHeaders(w, fileId)
}

func getFile(w http.ResponseWriter, r *http.Request, fileId string) {
	meta, err := dataStore.GetFileMeta(fileId)
	if os.IsNotExist(err) {
		reply(w, http.StatusNotFound, err.Error())
		return
	} else if err != nil {
		reply(w, http.StatusInternalServerError, err.Error())
		return
	}

	data, err := dataStore.ReadFile(fileId)
	if os.IsNotExist(err) {
		reply(w, http.StatusNotFound, err.Error())
		return
	} else if err != nil {
		reply(w, http.StatusInternalServerError, err.Error())
		return
	}

	defer data.Close()

	setFileHeaders(w, fileId)
	w.Header().Set("Content-Length", strconv.FormatInt(meta.Size, 10))

	if _, err := io.CopyN(w, data, meta.Size); err != nil {
		log.Printf("getFile: CopyN of fileId %s failed with: %s. Is the upload complete yet?", fileId, err.Error())
		return
	}
}

func putFile(w http.ResponseWriter, r *http.Request, fileId string) {
	var start int64 = 0
	var end int64 = 0

	contentRange, err := parseContentRange(r.Header.Get("Content-Range"))
	if err != nil {
		contentLength := r.Header.Get("Content-Length")
		end, err = strconv.ParseInt(contentLength, 10, 64)
		if err != nil {
			reply(w, http.StatusBadRequest, "Invalid content length provided")
		}

		// we are zero-indexed
		end = end - 1

		// @TODO: Make sure contentLength matches the content length of the initial
		// POST request
	} else {

		// @TODO: Make sure contentRange.Size matches file size

		start = contentRange.Start
		end = contentRange.End
	}

	// @TODO: Check that file exists

	err = dataStore.WriteFileChunk(fileId, start, end, r.Body)
	if os.IsNotExist(err) {
		reply(w, http.StatusNotFound, err.Error())
		return
	} else if err != nil {
		reply(w, http.StatusInternalServerError, err.Error())
		return
	}

	setFileHeaders(w, fileId)
}

func setFileHeaders(w http.ResponseWriter, fileId string) {
	meta, err := dataStore.GetFileMeta(fileId)
	if os.IsNotExist(err) {
		reply(w, http.StatusNotFound, err.Error())
		return
	} else if err != nil {
		reply(w, http.StatusInternalServerError, err.Error())
		return
	}

	rangeHeader := ""
	for i, chunk := range meta.Chunks {
		rangeHeader += fmt.Sprintf("%d-%d", chunk.Start, chunk.End)
		if i+1 < len(meta.Chunks) {
			rangeHeader += ","
		}
	}

	if rangeHeader != "" {
		w.Header().Set("Range", "bytes="+rangeHeader)
	}

	w.Header().Set("Content-Type", meta.ContentType)
	w.Header().Set("Content-Disposition", meta.ContentDisposition)
}
