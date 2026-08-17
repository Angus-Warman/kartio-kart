package main

import (
	"fmt"
	"math"
	"sync"
	"time"
)

const (
	tickInterval = 16 * time.Millisecond // ~60 Hz
	numRacers    = 4                     // total racer slots (players + autopilots)
	spawnRadius  = 4.0                   // distance from origin where players spawn
	accelScale   = 20.0                  // forward thrust multiplier (m/s²)
	turnSpeed    = 3.0                   // yaw rate when joystick is fully left/right (rad/s)
	drag         = 0.97                  // velocity damping per tick (0=instant stop, 1=no drag)
	autoThrust   = 6.0                   // autopilot forward thrust (m/s²)
	autoTurnRate = 0.45                  // autopilot yaw rate (rad/s), produces a gentle right-hand circle
)

type Player struct {
	ID         string
	Racer      *Racer
	Controller ControllerState
}

type Racer struct {
	Index   int     `json:"Index"`
	Colour  string  `json:"colour"`
	Heading float64 // current yaw angle in radians (ground plane)
	Thrust  float32 // forward acceleration this tick (m/s²)
	Physics PhysicsState
}

var racerColours = []string{"#ff4444", "#44ff88", "#4488ff", "#ffcc00"}

type ControllerState struct {
	JsX  float32 `json:"js_x"`
	JsY  float32 `json:"js_y"`
	BtnA bool    `json:"btn_a"`
	BtnB bool    `json:"btn_b"`
	BtnX bool    `json:"btn_x"`
	BtnY bool    `json:"btn_y"`
}

// Autopilot owns a Racer and writes Heading/Thrust directly each tick.
// Think() is the only method that needs to change when a smarter brain arrives.
type Autopilot struct {
	Racer *Racer
}

func (a *Autopilot) Drive(dt float32) {
	a.Racer.Heading += float64(autoTurnRate * dt)
	a.Racer.Thrust = autoThrust
}

type PhysicsState struct {
	X      float32 `json:"x"`
	Y      float32 `json:"y"`
	Z      float32 `json:"z"`
	RotX   float32 `json:"rot_x"`
	RotY   float32 `json:"rot_y"`
	RotZ   float32 `json:"rot_z"`
	RotW   float32 `json:"rot_w"`
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
	autopilots   []*Autopilot
	racers       []*Racer
	maxPlayers   int
	playerJoined chan struct{}
	dataCh       chan string
	started      bool
	engine       *Engine
}

func NewGame() (*Game, error) {
	engine, err := NewEngine()

	if err != nil {
		return nil, err
	}

	racers := make([]*Racer, numRacers)
	players := make(map[string]*Player)
	autopilots := make([]*Autopilot, numRacers)

	for i := range numRacers {
		racer := &Racer{
			Index:  i,
			Colour: racerColours[i],
		}

		angle := 2 * math.Pi * float64(i) / float64(numRacers)

		racer.Physics.X = float32(spawnRadius * math.Cos(angle))
		racer.Physics.Y = float32(spawnRadius * math.Sin(angle))
		racer.Physics.RotW = 1 // identity quaternion

		racers[i] = racer
		autopilots[i] = &Autopilot{Racer: racer}
	}

	return &Game{
		id:           "4321",
		maxPlayers:   numRacers,
		players:      players,
		autopilots:   autopilots,
		playerJoined: make(chan struct{}, 1),
		dataCh:       make(chan string, 1),
		racers:       racers,
		engine:       engine,
	}, nil
}

func (g *Game) AddPlayer(id string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()

	if len(g.players) >= g.maxPlayers {
		return false
	}

	// Claim the next racer slot and retire its autopilot.
	rIdx := len(g.players)
	racer := g.racers[rIdx]

	for i, a := range g.autopilots {
		if a.Racer.Index == rIdx {
			g.autopilots = append(g.autopilots[:i], g.autopilots[i+1:]...)
			break
		}
	}

	g.players[id] = &Player{
		ID:    id,
		Racer: racer,
	}

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

func (g *Game) Start() {
	if g.started {
		return
	}

	g.started = true
	go g.Run()
}

func (g *Game) Run() {
	ticker := time.NewTicker(tickInterval)
	defer ticker.Stop()

	dt := float32(tickInterval.Seconds())

	for range ticker.C {
		g.mu.Lock()

		// Autopilots write Heading/Thrust directly onto their Racer.
		for _, a := range g.autopilots {
			a.Drive(dt)
		}

		// Players translate joystick input into Heading/Thrust on their Racer.
		for _, p := range g.players {
			// JsX yaws the heading; JsY sets forward thrust (negative = forward).
			p.Racer.Heading += float64(p.Controller.JsX) * turnSpeed * float64(dt)
			p.Racer.Thrust = float32(-float64(p.Controller.JsY) * accelScale)
		}

		// Unified physics pass — same for every racer regardless of input source.
		for _, r := range g.racers {
			ph := &r.Physics

			// Apply heading to quaternion.
			half := r.Heading / 2
			ph.RotW = float32(math.Cos(half))
			ph.RotY = float32(-math.Sin(half))
			ph.RotX = 0
			ph.RotZ = 0

			// Project thrust along the heading vector.
			ph.accelX = float32(float64(r.Thrust) * math.Cos(r.Heading))
			ph.accelY = float32(float64(r.Thrust) * math.Sin(r.Heading))

			// Integrate acceleration → velocity → position.
			ph.velX = (ph.velX + ph.accelX*dt) * drag
			ph.velY = (ph.velY + ph.accelY*dt) * drag
			ph.velZ = (ph.velZ + ph.accelZ*dt) * drag

			ph.X += ph.velX * dt
			ph.Y += ph.velY * dt
			ph.Z += ph.velZ * dt

			g.engine.Step(r, dt)
		}

		gameData := encode(g.racers)

		g.dataCh <- gameData

		g.mu.Unlock()
	}
}

func (g *Game) Step(r *Racer) {

}
