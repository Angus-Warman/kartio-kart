package main

import (
	"embed"

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
		ServeEmbedded(publicFiles).
		Route("/game/{gameID}/lobby", h.Lobby).
		Route("/game/{gameID}/controller/{playerID}", h.Controller).
		Route("/game/{gameID}/controller/{playerID}/qr", h.ControllerQR).
		Route("/game/{gameID}/controller/{playerID}/ws", h.ControllerSocket).
		Run()
}
