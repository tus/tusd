package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strconv"
)

var fileRoute = regexp.MustCompile("^/files/([^/]+)$")

func serveHttp() error {
	http.HandleFunc("/", route)

	addr := ":1080"
	log.Printf("serving clients at %s", addr)

	return http.ListenAndServe(addr, nil)
}

func route(w http.ResponseWriter, r *http.Request) {
	log.Printf("request: %s %s", r.Method, r.URL.RequestURI())

	w.Header().Set("Server", "tusd")

	if r.Method == "POST" && r.URL.Path == "/files" {
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

	if contentRange.End != -1 {
		reply(w, http.StatusNotImplemented, "File data in initial request.")
		return
	}

	contentType := r.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	id := uid()
	if err := initFile(id, contentRange.Size, contentType); err != nil {
		reply(w, http.StatusInternalServerError, err.Error())
		return
	}

	// @TODO: Return X-Missing header

	w.Header().Set("Location", "/files/"+id)
	w.WriteHeader(http.StatusCreated)
}

func headFile(w http.ResponseWriter, r *http.Request, fileId string) {
	setFileRangeHeader(w, fileId)
}

func getFile(w http.ResponseWriter, r *http.Request, fileId string) {
	data, size, err := getFileData(fileId)
	if err != nil {
		// @TODO: Could be a 404 as well
		reply(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer data.Close()

	setFileRangeHeader(w, fileId)
	w.Header().Set("Content-Length", strconv.FormatInt(size, 10))

	if _, err := io.CopyN(w, data, size); err != nil {
		log.Printf("getFile: CopyN failed with: %s", err.Error())
		return
	}
}

func putFile(w http.ResponseWriter, r *http.Request, fileId string) {
	contentRange, err := parseContentRange(r.Header.Get("Content-Range"))
	if err != nil {
		reply(w, http.StatusBadRequest, err.Error())
		return
	}

	// @TODO: Check that file exists
	// @TODO: Make sure contentRange.Size matches file size

	if err := putFileChunk(fileId, contentRange.Start, contentRange.End, r.Body); err != nil {
		// @TODO: Could be a 404 as well
		reply(w, http.StatusInternalServerError, err.Error())
		return
	}

	setFileRangeHeader(w, fileId)
}

func setFileRangeHeader(w http.ResponseWriter, fileId string) {
	chunks, err := getReceivedChunks(fileId)
	if err != nil {
		reply(w, http.StatusInternalServerError, err.Error())
		return
	}

	received := ""
	for i, chunk := range chunks {
		received += fmt.Sprintf("%d-%d", chunk.Start, chunk.End)
		if i+1 < len(chunks) {
			received += ","
		}
	}

	if received != "" {
		w.Header().Set("Range", "bytes="+received)
	}
}
