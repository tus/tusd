package main

import (
	"log"
	tushttp "github.com/tus/tusd/src/http"
	"net/http"
	"os"
)

func main() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds)
	log.Printf("tusd started")

	addr := ":1080"
	if port := os.Getenv("TUSD_PORT"); port != "" {
		addr = ":" + port
	}
	log.Printf("servering clients at http://localhost%s", addr)

	handler := tushttp.NewHandler()
	if err := http.ListenAndServe(addr, handler); err != nil {
		panic(err)
	}
}
