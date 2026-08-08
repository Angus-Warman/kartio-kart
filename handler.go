package main

import (
	"embed"
	"log"
	"net/http"

	"github.com/Angus-Warman/httpmin/response"
	"github.com/skip2/go-qrcode"
)

//go:embed templates
var templatesFS embed.FS

func (h *Handler) ControllerQR(w http.ResponseWriter, r *http.Request) {
	png, err := qrcode.Encode(
		"http://"+r.Host+"/game/"+r.PathValue("gameID")+"/controller/"+r.PathValue("playerID"),
		qrcode.Medium,
		200,
	)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "image/png")
	w.Write(png)
}

type Handler struct {
}

func NewHandler() (*Handler, error) {
	return &Handler{}, nil
}

func (h *Handler) Lobby(w http.ResponseWriter, r *http.Request) {
	// TODO render template
}

func (h *Handler) Controller(w http.ResponseWriter, r *http.Request) {
	// If gameID doesn't exist or playerID does not exist, return 404

	// TODO render template
}

func (h *Handler) ControllerSocket(w http.ResponseWriter, r *http.Request) {
	// If gameID doesn't exist or playerID does not exist, return 404

	response.WebSocket(func(socket *response.WebSocketConnection) {
		socket.Send("connected")

		for {
			_, err := socket.ReadMessage()
			if err != nil {
				log.Println(err)
				return
			}
		}
	})
}
