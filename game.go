package main

import (
	"fmt"
	"sync"
)

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

type Game struct {
	mu           sync.Mutex
	id           string
	players      map[string]*Player
	maxPlayers   int
	playerJoined chan struct{}
}

func NewGame() *Game {
	return &Game{
		id:           "4321",
		maxPlayers:   4,
		players:      make(map[string]*Player),
		playerJoined: make(chan struct{}, 1),
	}
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

func (g *Game) handleMessage(playerID string, state controllerState) bool {
	g.mu.Lock()
	defer g.mu.Unlock()

	p, ok := g.players[playerID]

	if !ok || p.state == state {
		return false
	}

	p.state = state

	return true
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
