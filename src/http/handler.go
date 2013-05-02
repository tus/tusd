package http

import (
	"log"
	"net/http"
	"os"
)

type HandlerConfig struct{
	// Dir points to a filesystem path used by tus to store uploaded and partial
	// files. Will be created if does not exist yet. Required.
	Dir string

	// MaxSize defines how many bytes may be stored inside Dir. Exceeding this
	// limit will cause the oldest upload files to be deleted until enough space
	// is available again. Required.
	MaxSize int64
}

func NewHandler(config HandlerConfig) (*Handler, error) {
	// Ensure the data store directory exists
	if err := os.MkdirAll(config.Dir, 0777); err != nil {
		return nil, err
	}

	return &Handler{
		store: newDataStore(config.Dir, config.MaxSize),
	}, nil
}

type Handler struct{
	store *DataStore
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log.Printf("request: %s %s", r.Method, r.URL.RequestURI())
}
