package http

import (
	"net/http"
)

func NewHandler() *Handler {
	return &Handler{}
}

type Handler struct {}

func (h *Handler) ServeHTTP(http.ResponseWriter, *http.Request) {
}
