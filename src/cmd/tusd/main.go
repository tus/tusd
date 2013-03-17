package main

import (
	"log"
)

func main() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds)
	log.Printf("tusd started")

	if err := serveHttp(); err != nil {
		log.Fatal(err)
	}
}
