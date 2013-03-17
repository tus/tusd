package main

import (
	"fmt"
	"net/http"
	"log"
)

func main() {
	http.HandleFunc("/", route)
	err := http.ListenAndServe(":1080", nil)
	if err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}

func route(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Server", "tusd")

	if r.Method == "POST" && r.URL.Path == "/files" {
		createFile(w, r)
		return
	}

	reply(w, http.StatusNotFound, "No matching route")
}

func reply(w http.ResponseWriter, code int, message string) {
	w.WriteHeader(code)
	fmt.Fprintf(w, "%d - %s: %s\n", code, http.StatusText(code), message)
}

func createFile(w http.ResponseWriter, r *http.Request) {
	contentRange := r.Header.Get("Content-Range")
	if contentRange == "" {
		reply(w, http.StatusBadRequest, "Content-Range header is required")
		return
	}

	contentType := r.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	log.Printf("contentType: %s", contentType)
}
