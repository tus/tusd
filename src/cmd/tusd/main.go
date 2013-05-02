package main

import (
	tushttp "github.com/tus/tusd/src/http"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
)

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

	config := tushttp.HandlerConfig{
		Dir:          dir,
		MaxSize:      maxSize,
	}

	log.Printf("handler config: %+v", config)

	handler, err := tushttp.NewHandler(config)
	if err != nil {
		panic(err)
	}

	log.Printf("servering clients at http://localhost%s", addr)
	if err := http.ListenAndServe(addr, handler); err != nil {
		panic(err)
	}
}
