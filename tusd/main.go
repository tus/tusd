package main

import (
	"github.com/tus/tusd"
	"github.com/tus/tusd/filestore"
	"net/http"
	"os"
)

func main() {

	if err := os.MkdirAll("./data/", os.FileMode(0666)); err != nil {
		panic(err)
	}

	store := filestore.FileStore{
		Path: "./data/",
	}

	handler, err := tusd.NewHandler(tusd.Config{
		MaxSize:   1024 * 1024 * 1024,
		BasePath:  "files/",
		DataStore: store,
	})
	if err != nil {
		panic(err)
	}

	http.Handle("/files/", http.StripPrefix("/files/", handler))
	err = http.ListenAndServe(":1080", nil)
	if err != nil {
		panic(err)
	}
}
