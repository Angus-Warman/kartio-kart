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
		Route("/g/{gameID}/lobby-qr", h.LobbyQR).
		Route("/g/{gameID}/lobby-status", h.LobbyStatus).
		Route("/g/{gameID}/join-game", h.JoinGame).
		Route("/g/{gameID}/p/{playerID}/controller", h.Controller).
		Route("/g/{gameID}/p/{playerID}/ws", h.ControllerToServer).
		Route("/g/{gameID}/match", h.Match).
		Route("/g/{gameID}/match/ws", h.ServerToMatch).
		Route("/g/{gameID}/spectate", h.Spectate).
		Route("/arena", h.Arena).
		Route("/", h.RedirectToLobby).
		Run()
}
