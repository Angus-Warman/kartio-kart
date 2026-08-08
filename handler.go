package main

import (
	"embed"
	"encoding/json"
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
	ID    string
	state controllerState
}

type controllerState struct {
	JsX  float32 `json:"js_x"`
	JsY  float32 `json:"js_y"`
	BtnA bool    `json:"btn_a"`
	BtnB bool    `json:"btn_b"`
	BtnX bool    `json:"btn_x"`
	BtnY bool    `json:"btn_y"`
}

type packet struct {
	PlayerID string          `json:"playerID"`
	State    controllerState `json:"state"`
}

type Game struct {
	mu           sync.Mutex
	id           string
	players      map[string]*Player
	matchSockets []*response.WebSocketConnection
	playerJoined chan struct{}
}

func (g *Game) addPlayer(id string) {
	g.mu.Lock()

	g.players[id] = &Player{
		ID: id,
	}

	g.mu.Unlock()

	select {
	case g.playerJoined <- struct{}{}:
	default:
	}
}

func (g *Game) findPlayer(id string) (*Player, bool) {
	g.mu.Lock()
	defer g.mu.Unlock()

	p, ok := g.players[id]

	return p, ok
}

func (g *Game) playerExists(id string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()

	_, ok := g.players[id]

	return ok
}

func (g *Game) handleMessage(playerID string, state controllerState) {
	g.mu.Lock()

	p, ok := g.players[playerID]

	if !ok || p.state == state {
		g.mu.Unlock()
		return
	}

	p.state = state

	data, err := json.Marshal(packet{
		PlayerID: playerID,
		State:    state,
	})

	sockets := g.matchSockets
	g.mu.Unlock()

	if err != nil {
		log.Println(err)
		return
	}

	for _, socket := range sockets {
		if err := socket.Send(string(data)); err != nil {
			log.Println(err)
		}
	}
}

func (g *Game) AddMatchSocket(socket *response.WebSocketConnection) {
	g.mu.Lock()
	defer g.mu.Unlock()

	g.matchSockets = append(g.matchSockets, socket)
}

func (g *Game) RemoveMatchSocket(socket *response.WebSocketConnection) {
	g.mu.Lock()
	defer g.mu.Unlock()

	for i, s := range g.matchSockets {
		if s == socket {
			g.matchSockets = append(g.matchSockets[:i], g.matchSockets[i+1:]...)
			return
		}
	}
}

type lobbyData struct {
	GameID  string
	Players []Player
}

type controllerData struct {
	SocketURL string
}

type matchData struct {
	SocketURL string
}

type Handler struct {
	game      *Game
	templates map[string]*template.Template
}

func NewHandler() (*Handler, error) {
	pages := []string{"lobby", "controller", "match"}

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
			id:           "4321",
			players:      make(map[string]*Player),
			playerJoined: make(chan struct{}, 1),
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
		GameID: g.id,
	}
}

func (h *Handler) Lobby(w http.ResponseWriter, r *http.Request) {
	h.render(w, "lobby", h.game.lobbyData())
}

func (h *Handler) LobbyQR(w http.ResponseWriter, r *http.Request) {
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

func (g *Game) lobbyStatusMessage() string {
	var msg string

	switch numPlayers := len(g.players); numPlayers {
	case 0:
		msg = "Waiting for players to join..."
	case 1:
		msg = "1 player in lobby"
	default:
		msg = fmt.Sprintf("%v players in lobby", numPlayers)
	}

	return msg
}

func (h *Handler) LobbyStatus(w http.ResponseWriter, r *http.Request) {
	stream := response.Stream(w, r)

	if err := stream.Send(h.game.lobbyStatusMessage()); err != nil {
		log.Println(err)
		return
	}

	for {
		select {
		case <-h.game.playerJoined:
		case <-r.Context().Done():
			return
		}

		if err := stream.Send(h.game.lobbyStatusMessage()); err != nil {
			log.Println(err)
			return
		}
	}
}

func (h *Handler) JoinGame(w http.ResponseWriter, r *http.Request) {
	gameID := r.PathValue("gameID")
	playerID := fmt.Sprint(11111 + rand.Intn(88888))
	h.game.addPlayer(playerID)

	redirectTo := fmt.Sprintf("http://%v/g/%v/p/%v/controller", r.Host, gameID, playerID)
	http.Redirect(w, r, redirectTo, http.StatusSeeOther)
}

func (h *Handler) Controller(w http.ResponseWriter, r *http.Request) {
	gameID := r.PathValue("gameID")
	playerID := r.PathValue("playerID")

	if !h.game.playerExists(playerID) {
		http.NotFound(w, r)
		return
	}

	socketURL := fmt.Sprintf("ws://%v/g/%v/p/%v/ws", r.Host, gameID, playerID)

	h.render(w, "controller", controllerData{
		SocketURL: socketURL,
	})
}

func (h *Handler) ControllerSocket(w http.ResponseWriter, r *http.Request) {
	log.Println("connecting socket")

	gameID := r.PathValue("gameID")
	playerID := r.PathValue("playerID")
	_ = gameID

	g := h.game

	if !g.playerExists(playerID) {
		http.NotFound(w, r)
		return
	}

	response.WebSocket(func(socket *response.WebSocketConnection) {
		log.Println("connecting to match socket...")

		for {
			msg, err := socket.Read()

			if err != nil {
				log.Println(err)
				return
			}

			var state controllerState

			if err := json.Unmarshal([]byte(msg), &state); err != nil {
				log.Println(err)
				continue
			}

			g.handleMessage(playerID, state)
		}
	}).ServeHTTP(w, r)
}

func (h *Handler) Match(w http.ResponseWriter, r *http.Request) {
	gameID := r.PathValue("gameID")

	socketURL := fmt.Sprintf("ws://%v/g/%v/match/ws", r.Host, gameID)

	h.render(w, "match", matchData{
		SocketURL: socketURL,
	})
}

func (h *Handler) MatchSocket(w http.ResponseWriter, r *http.Request) {
	g := h.game

	response.WebSocket(func(socket *response.WebSocketConnection) {
		log.Println("connecting to match socket...")

		g.AddMatchSocket(socket)
		defer g.RemoveMatchSocket(socket)

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
