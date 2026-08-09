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

type packet struct {
	PlayerID string          `json:"playerID"`
	State    ControllerState `json:"state"`
}

type lobbyData struct {
	GameID string
}

type controllerData struct {
	SocketURL string
	Colour    string
}

type matchData struct {
	PlayerIDs   []string
	SocketURL   string
	JSONColours string
}

type Handler struct {
	g            *Game
	templates    map[string]*template.Template
	mu           sync.Mutex
	matchSockets []*response.WebSocketConnection
}

func NewHandler() (*Handler, error) {
	pages := []string{"lobby", "controller", "match", "arena", "spectate"}

	templates := make(map[string]*template.Template, len(pages))

	for _, page := range pages {
		files := []string{"templates/layout.tmpl", "templates/" + page + ".tmpl"}

		if page == "match" || page == "spectate" {
			files = append(files, "templates/scene.tmpl")
		}

		t, err := template.ParseFS(templatesFS, files...)

		if err != nil {
			return nil, err
		}

		templates[page] = t
	}

	g, err := NewGame()

	if err != nil {
		return nil, err
	}

	return &Handler{
		g:         g,
		templates: templates,
	}, nil
}

func (h *Handler) broadcastPacket(p packet) {
	data, err := json.Marshal(p)

	if err != nil {
		log.Println(err)
		return
	}

	h.mu.Lock()
	sockets := h.matchSockets
	h.mu.Unlock()

	for _, socket := range sockets {
		if err := socket.Send(string(data)); err != nil {
			log.Println(err)
		}
	}
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
		url := fmt.Sprintf("/g/%v/lobby", h.g.id)
		http.Redirect(w, r, url, http.StatusSeeOther)
		return
	}

	http.Error(w, "not found", 404)
}

func (h *Handler) Lobby(w http.ResponseWriter, r *http.Request) {
	h.render(w, "lobby", lobbyData{GameID: h.g.id})
}

func (h *Handler) LobbyQR(w http.ResponseWriter, r *http.Request) {
	png, err := qrcode.Encode(
		fmt.Sprintf("http://%v/g/%v/join-game", r.Host, h.g.id),
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

func (h *Handler) LobbyStatus(w http.ResponseWriter, r *http.Request) {
	stream := response.Stream(w, r)

	if err := stream.Send(h.g.StatusMessage()); err != nil {
		log.Println(err)
		return
	}

	for {
		select {
		case <-h.g.playerJoined:
		case <-r.Context().Done():
			return
		}

		if err := stream.Send(h.g.StatusMessage()); err != nil {
			log.Println(err)
			return
		}
	}
}

func (h *Handler) JoinGame(w http.ResponseWriter, r *http.Request) {
	gameID := r.PathValue("gameID")
	playerID := fmt.Sprint(11111 + rand.Intn(88888))
	canJoin := h.g.AddPlayer(playerID)

	if !canJoin {
		http.Error(w, "game is full", http.StatusConflict)
		return
	}

	redirectTo := fmt.Sprintf("http://%v/g/%v/p/%v/controller", r.Host, gameID, playerID)
	http.Redirect(w, r, redirectTo, http.StatusSeeOther)
}

func (h *Handler) Arena(w http.ResponseWriter, r *http.Request) {
	h.render(w, "arena", nil)
}

func (h *Handler) Controller(w http.ResponseWriter, r *http.Request) {
	gameID := r.PathValue("gameID")
	playerID := r.PathValue("playerID")

	if !h.g.PlayerExists(playerID) {
		http.NotFound(w, r)
		return
	}

	colour := ""
	if p, ok := h.g.FindPlayer(playerID); ok {
		colour = p.Racer.Colour
	}

	socketURL := fmt.Sprintf("ws://%v/g/%v/p/%v/ws", r.Host, gameID, playerID)

	h.render(w, "controller", controllerData{
		SocketURL: socketURL,
		Colour:    colour,
	})
}

func (h *Handler) ControllerToServer(w http.ResponseWriter, r *http.Request) {
	log.Println("connecting socket")

	gameID := r.PathValue("gameID")
	playerID := r.PathValue("playerID")
	_ = gameID

	g := h.g

	if !g.PlayerExists(playerID) {
		http.NotFound(w, r)
		return
	}

	response.WebSocket(func(socket *response.WebSocketConnection) {
		log.Println("connecting to controller socket...")

		for {
			msg, err := socket.Read()

			if err != nil {
				log.Println(err) // Disconnect
				return
			}

			var state ControllerState

			if err := json.Unmarshal([]byte(msg), &state); err != nil {
				log.Println(err)
				continue
			}

			g.HandleMessage(playerID, state)
		}
	}).ServeHTTP(w, r)
}

func (h *Handler) Match(w http.ResponseWriter, r *http.Request) {
	gameID := r.PathValue("gameID")

	h.g.Start()

	socketURL := fmt.Sprintf("ws://%v/g/%v/match/ws", r.Host, gameID)

	h.render(w, "match", matchData{
		SocketURL:   socketURL,
		JSONColours: h.coloursJSON(),
	})
}

func (h *Handler) Spectate(w http.ResponseWriter, r *http.Request) {
	gameID := r.PathValue("gameID")

	h.g.Start()

	socketURL := fmt.Sprintf("ws://%v/g/%v/match/ws", r.Host, gameID)

	h.render(w, "spectate", matchData{
		SocketURL:   socketURL,
		JSONColours: h.coloursJSON(),
	})
}

func (h *Handler) coloursJSON() string {
	colours := make([]string, len(h.g.racers))
	for i, rc := range h.g.racers {
		colours[i] = rc.Colour
	}

	b, err := json.Marshal(colours)
	if err != nil {
		log.Println(err)
		return "[]"
	}

	return string(b)
}

func (h *Handler) ServerToMatch(w http.ResponseWriter, r *http.Request) {
	response.WebSocket(func(socket *response.WebSocketConnection) {
		h.addMatchSocket(socket)
		defer h.removeMatchSocket(socket)

		for {
			msg := <-h.g.dataCh

			err := socket.Send(msg)

			if err != nil {
				log.Println(err)
				return
			}
		}
	}).ServeHTTP(w, r)
}

func encode(data any) string {
	b, err := json.Marshal(data)

	if err != nil {
		log.Println(err)
		return err.Error()
	}

	return string(b)
}

func (h *Handler) addMatchSocket(socket *response.WebSocketConnection) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.matchSockets = append(h.matchSockets, socket)
}

func (h *Handler) removeMatchSocket(socket *response.WebSocketConnection) {
	h.mu.Lock()
	defer h.mu.Unlock()

	for i, s := range h.matchSockets {
		if s == socket {
			h.matchSockets = append(h.matchSockets[:i], h.matchSockets[i+1:]...)
			return
		}
	}
}
