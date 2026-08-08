package main

import (
	"embed"
	"fmt"
	"html/template"
	"log"
	"math/rand"
	"net/http"
	"sync"

	"github.com/Angus-Warman/httpmin/response"
	"github.com/skip2/go-qrcode"
)

//go:embed templates
var templatesFS embed.FS

const numPlayers = 4

type Player struct {
	ID        string
	Connected bool
}

type Game struct {
	mu      sync.Mutex
	id      string
	players []*Player
}

func (g *Game) findPlayer(id string) *Player {
	g.mu.Lock()
	defer g.mu.Unlock()

	for _, p := range g.players {
		if p.ID == id {
			return p
		}
	}

	return nil
}

func (g *Game) setConnected(id string, connected bool) {
	g.mu.Lock()
	defer g.mu.Unlock()

	for _, p := range g.players {
		if p.ID == id {
			p.Connected = connected
			return
		}
	}
}

func (g *Game) snapshot() []Player {
	g.mu.Lock()
	defer g.mu.Unlock()

	players := make([]Player, len(g.players))

	for i, p := range g.players {
		players[i] = Player{ID: p.ID, Connected: p.Connected}
	}

	return players
}

type lobbyData struct {
	GameID  string
	Players []Player
}

type controllerData struct {
	GameID   string
	PlayerID string
}

type Handler struct {
	game      *Game
	templates map[string]*template.Template
}

func NewHandler() (*Handler, error) {
	pages := []string{"lobby", "controller"}

	templates := make(map[string]*template.Template, len(pages))

	for _, page := range pages {
		t, err := template.ParseFS(templatesFS, "templates/layout.tmpl", "templates/"+page+".tmpl")

		if err != nil {
			return nil, err
		}

		templates[page] = t
	}

	return &Handler{
		game: &Game{
			id: "4321",
		},
		templates: templates,
	}, nil
}

func (h *Handler) render(w http.ResponseWriter, page string, data any) {
	t := h.templates[page]

	if t == nil {
		http.Error(w, "template not found", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html")

	if err := t.ExecuteTemplate(w, "layout", data); err != nil {
		log.Println(err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (h *Handler) RedirectToLobby(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/" {
		url := fmt.Sprintf("/g/%v/lobby", h.game.id)
		http.Redirect(w, r, url, http.StatusSeeOther)
		return
	}

	http.Error(w, "not found", 404)
}

func (g *Game) lobbyData() lobbyData {
	return lobbyData{
		GameID:  g.id,
		Players: g.snapshot(),
	}
}

func (h *Handler) Lobby(w http.ResponseWriter, r *http.Request) {
	h.render(w, "lobby", h.game.lobbyData())
}

func (h *Handler) JoinQR(w http.ResponseWriter, r *http.Request) {
	png, err := qrcode.Encode(
		fmt.Sprintf("http://%v/g/%v/join-game", r.Host, h.game.id),
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

func (h *Handler) JoinGame(w http.ResponseWriter, r *http.Request) {
	gameID := r.PathValue("gameID")
	playerID := fmt.Sprint(11111 + rand.Intn(88888))

	redirectTo := fmt.Sprintf("http://%v/g/%v/p/%v/controller", r.Host, gameID, playerID)
	http.Redirect(w, r, redirectTo, http.StatusSeeOther)
}

func (h *Handler) Controller(w http.ResponseWriter, r *http.Request) {
	gameID := r.PathValue("gameID")
	playerID := r.PathValue("playerID")

	// if h.game.findPlayer(playerID) == nil {
	// 	http.NotFound(w, r)
	// 	return
	// }

	h.render(w, "controller", controllerData{
		GameID:   gameID,
		PlayerID: playerID,
	})
}

func (h *Handler) ControllerSocket(w http.ResponseWriter, r *http.Request) {
	playerID := r.PathValue("playerID")

	g := h.game

	if g.findPlayer(playerID) == nil {
		http.NotFound(w, r)
		return
	}

	response.WebSocket(func(socket *response.WebSocketConnection) {
		g.setConnected(playerID, true)
		defer g.setConnected(playerID, false)

		socket.Send("connected")

		for {
			_, err := socket.ReadMessage()

			if err != nil {
				log.Println(err)
				return
			}
		}
	}).ServeHTTP(w, r)
}
