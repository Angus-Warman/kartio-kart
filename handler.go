package main

import (
	"embed"
	"net/http"
)

//go:embed templates
var templatesFS embed.FS

type Handler struct {
}

func NewHandler() (*Handler, error) {
	return &Handler{}, nil
}

func (h *Handler) Lobby(w http.ResponseWriter, r *http.Request) {

}

func (h *Handler) Controller(w http.ResponseWriter, r *http.Request) {

}
