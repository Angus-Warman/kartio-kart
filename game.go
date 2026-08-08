package main

import (
	"fmt"
	"math"
	"sync"
	"time"
)

const (
	tickInterval = 16 * time.Millisecond // ~60 Hz
	spawnRadius  = 2.0                   // distance from origin where players spawn
	accelScale   = 10.0                  // joystick-to-acceleration multiplier
	drag         = 0.85                  // velocity damping per tick (0=instant stop, 1=no drag)
)

type Player struct {
	ID         string
	Racer      *Racer
	Controller ControllerState
}

type Racer struct {
	Index   int // 0-3
	Physics PhysicsState
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

type GameTick []*Racer

type Game struct {
	mu           sync.Mutex
	id           string
	players      map[string]*Player
	racers       []*Racer
	maxPlayers   int
	playerJoined chan struct{}
	tickCh       chan GameTick
	started      bool
}

func NewGame() *Game {
	racers := []*Racer{
		{
			Index: 0,
		},
		{
			Index: 1,
		},
		{
			Index: 2,
		},
		{
			Index: 3,
		},
	}

	return &Game{
		id:           "4321",
		maxPlayers:   4,
		players:      make(map[string]*Player),
		playerJoined: make(chan struct{}, 1),
		tickCh:       make(chan GameTick, 1),
		racers:       racers,
	}
}

func (g *Game) AddPlayer(id string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()

	if len(g.players) >= g.maxPlayers {
		return false
	}

	rIdx := len(g.players)

	racer := g.racers[rIdx]

	p := &Player{
		ID:    id,
		Racer: racer,
	}

	g.players[id] = p

	select {
	case g.playerJoined <- struct{}{}:
	default:
	}

	return true
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

func (g *Game) spawnRacers() {
	g.mu.Lock()
	defer g.mu.Unlock()

	n := len(g.players)
	if n == 0 {
		return
	}

	for i, r := range g.racers {
		angle := 2 * math.Pi * float64(i) / float64(n)

		r.Physics = PhysicsState{
			X: float32(spawnRadius * math.Cos(angle)),
			Y: float32(spawnRadius * math.Sin(angle)),
			Z: 0,
		}
	}
}

func (g *Game) Start() {
	if g.started {
		return
	}

	g.started = true
	g.spawnRacers()
	go g.Run()
}

func (g *Game) Run() {
	ticker := time.NewTicker(tickInterval)
	defer ticker.Stop()

	dt := float32(tickInterval.Seconds())

	for range ticker.C {
		g.mu.Lock()
		for _, p := range g.players {
			ph := &p.Racer.Physics
			ctrl := p.Controller

			// Joystick drives acceleration on X/Y axes.
			ph.accelX = ctrl.JsX * accelScale
			ph.accelY = ctrl.JsY * accelScale
			// accelZ stays zero unless something else sets it (e.g. jump via BtnA).

			// Integrate acceleration into velocity, then apply drag.
			ph.velX = (ph.velX + ph.accelX*dt) * drag
			ph.velY = (ph.velY + ph.accelY*dt) * drag
			ph.velZ = (ph.velZ + ph.accelZ*dt) * drag

			// Integrate velocity into position.
			ph.X += ph.velX * dt
			ph.Y += ph.velY * dt
			ph.Z += ph.velZ * dt
		}

		g.tickCh <- g.racers

		g.mu.Unlock()
	}
}
