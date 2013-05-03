package main

import (
	tushttp "github.com/tus/tusd/src/http"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
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

	http.Handle(basePath, tusHandler)

	go handleUploads(tusHandler)

	log.Printf("servering clients at http://localhost%s", addr)
	if err := http.ListenAndServe(addr, nil); err != nil {
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
