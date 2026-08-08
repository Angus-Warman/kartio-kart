package main

import (
	"fmt"
	"sync"
)

type Player struct {
	ID         string
	Controller ControllerState
	Physics    PhysicsState
}

type ControllerState struct {
	JsX  float32 `json:"js_x"`
	JsY  float32 `json:"js_y"`
	BtnA bool    `json:"btn_a"`
	BtnB bool    `json:"btn_b"`
	BtnX bool    `json:"btn_x"`
	BtnY bool    `json:"btn_y"`
}

type PhysicsState struct {
	X      float32 `json:"x"`
	Y      float32 `json:"y"`
	Z      float32 `json:"z"`
	velX   float32
	velY   float32
	velZ   float32
	accelX float32
	accelY float32
	accelZ float32
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

func (g *Game) AddPlayer(id string) {
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

func (g *Game) FindPlayer(id string) (*Player, bool) {
	g.mu.Lock()
	defer g.mu.Unlock()

	p, ok := g.players[id]

	return p, ok
}

func (g *Game) PlayerExists(id string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()

	_, ok := g.players[id]

	return ok
}

func (g *Game) HandleMessage(playerID string, state ControllerState) {
	g.mu.Lock()
	defer g.mu.Unlock()

	p, ok := g.players[playerID]

	if !ok {
		return
	}

	p.Controller = state
}

func (g *Game) StatusMessage() string {
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

func (g *Game) Start() {
	go g.Run()
}

func (g *Game) Run() {
	for {
	}
}
