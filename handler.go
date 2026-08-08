package main

import (
	"embed"
	"html/template"
	"log"
	"net/http"
	"strconv"
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
	mu        sync.Mutex
	games     map[string]*Game
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
		games:     make(map[string]*Game),
		templates: templates,
	}, nil
}

func (h *Handler) game(gameID string) (*Game, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()

	g, ok := h.games[gameID]

	return g, ok
}

func (h *Handler) getOrCreateGame(gameID string) *Game {
	h.mu.Lock()
	defer h.mu.Unlock()

	if g, ok := h.games[gameID]; ok {
		return g
	}

	players := make([]*Player, numPlayers)

	for i := range players {
		players[i] = &Player{ID: strconv.Itoa(i)}
	}

	g := &Game{players: players}

	h.games[gameID] = g

	return g
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

func (h *Handler) Lobby(w http.ResponseWriter, r *http.Request) {
	gameID := r.PathValue("gameID")

	g := h.getOrCreateGame(gameID)

	h.render(w, "lobby", lobbyData{
		GameID:  gameID,
		Players: g.snapshot(),
	})
}

func (h *Handler) Controller(w http.ResponseWriter, r *http.Request) {
	gameID := r.PathValue("gameID")
	playerID := r.PathValue("playerID")

	g, ok := h.game(gameID)

	if !ok {
		http.NotFound(w, r)
		return
	}

	if g.findPlayer(playerID) == nil {
		http.NotFound(w, r)
		return
	}

	h.render(w, "controller", controllerData{
		GameID:   gameID,
		PlayerID: playerID,
	})
}

func (h *Handler) ControllerQR(w http.ResponseWriter, r *http.Request) {
	gameID := r.PathValue("gameID")
	playerID := r.PathValue("playerID")

	png, err := qrcode.Encode(
		"http://"+r.Host+"/game/"+gameID+"/controller/"+playerID,
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

func (h *Handler) ControllerSocket(w http.ResponseWriter, r *http.Request) {
	gameID := r.PathValue("gameID")
	playerID := r.PathValue("playerID")

	g, ok := h.game(gameID)

	if !ok {
		http.NotFound(w, r)
		return
	}

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
