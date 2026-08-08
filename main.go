package main

import (
	"embed"
	"net/http"

	"github.com/Angus-Warman/httpmin"
)

//go:embed all:public
var publicFiles embed.FS

func main() {
	c := httpmin.New().OnPort("7888").PublicIP()

	h, err := NewHandler()

	if err != nil {
		panic(err)
	}

	c.
		RouteHandler("/public/", http.FileServerFS(publicFiles)).
		Route("/g/{gameID}/lobby", h.Lobby).
		// Route("/g/{gameID}/lobby/status", h.LobbyStatus).
		Route("/g/{gameID}/join/qr", h.JoinQR).
		Route("/g/{gameID}/join-game", h.JoinGame).
		Route("/g/{gameID}/p/{playerID}/controller", h.Controller).
		Route("/g/{gameID}/p/{playerID}/ws", h.ControllerSocket).
		Route("/", h.RedirectToLobby).
		Run()
}
