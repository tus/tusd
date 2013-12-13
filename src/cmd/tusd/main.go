package main

import (
	tushttp "github.com/tus/tusd/src/http"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

const basePath = "/files/"

func main() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds)
	log.Printf("tusd started")

	addr := ":1080"
	if envPort := os.Getenv("TUSD_PORT"); envPort != "" {
		addr = ":" + envPort
	}

	maxSize := int64(1024 * 1024 * 1024)
	if envMaxSize := os.Getenv("TUSD_DATA_STORE_MAXSIZE"); envMaxSize != "" {
		parsed, err := strconv.ParseInt(envMaxSize, 10, 64)
		if err != nil {
			panic("bad TUSD_DATA_STORE_MAXSIZE: " + err.Error())
		}
		maxSize = parsed
	}

	dir := os.Getenv("TUSD_DATA_DIR")
	if dir == "" {
		if workingDir, err := os.Getwd(); err != nil {
			panic(err)
		} else {
			dir = filepath.Join(workingDir, "tus_data")
		}
	}

	tusConfig := tushttp.HandlerConfig{
		Dir:      dir,
		MaxSize:  maxSize,
		BasePath: basePath,
	}

	log.Printf("handler config: %+v", tusConfig)

	tusHandler, err := tushttp.NewHandler(tusConfig)
	if err != nil {
		panic(err)
	}

	http.HandleFunc(basePath, func(w http.ResponseWriter, r *http.Request) {
		// Allow CORS for almost everything. This needs to be revisted / limited to
		// routes and methods that need it.

		// Domains allowed to make requests
		w.Header().Add("Access-Control-Allow-Origin", "*")
		// Methods clients are allowed to use
		w.Header().Add("Access-Control-Allow-Methods", "HEAD,GET,PUT,POST,PATCH,DELETE")
		// Headers clients are allowed to send
		w.Header().Add("Access-Control-Allow-Headers", "Origin, X-Requested-With, Content-Type, Accept, Content-Disposition, Final-Length, Offset")
		// Headers clients are allowed to receive
		w.Header().Add("Access-Control-Expose-Headers", "Location, Range, Content-Disposition, Offset")

		if r.Method == "OPTIONS" {
			return
		}

		tusHandler.ServeHTTP(w, r)
	})

	go handleUploads(tusHandler)

	// On http package's default action, a broken http connection will cause io.Copy() stuck because it always suppose more data will coming and wait for them infinitely
	// To prevent it happen, we should set a specific timeout value on http server
	s := &http.Server{
		Addr:           addr,
		Handler:        nil,
		ReadTimeout:    8 * time.Second,
		WriteTimeout:   8 * time.Second,
		MaxHeaderBytes: 0,
	}

	log.Printf("servering clients at http://localhost%s", addr)
	if err := s.ListenAndServe(); err != nil {
		panic(err)
	}
}

func handleUploads(tus *tushttp.Handler) {
	for {
		select {
		case err := <-tus.Error:
			log.Printf("error: %s", err)
		}
	}
}
