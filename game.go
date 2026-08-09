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
	accelScale   = 20.0                  // joystick-to-acceleration multiplier
	drag         = 0.97                  // velocity damping per tick (0=instant stop, 1=no drag)
	autoSpeed    = 4.0                   // autopilot target speed in m/s
	autoGain     = 4.0                   // autopilot steering gain (1/s)
)

type Player struct {
	ID         string
	Racer      *Racer
	Controller ControllerState
}

type Racer struct {
	Index   int    `json:"Index"`
	Colour  string `json:"colour"`
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

// Autopilot drives a racer that no human controls.
type Autopilot struct {
	Racer *Racer
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
	racers       []*Racer
	autopilots   []*Autopilot
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
		players:      make(map[string]*Player),
		playerJoined: make(chan struct{}, 1),
		dataCh:       make(chan string, 1),
		racers:       racers,
		autopilots:   autopilots,
		engine:       engine,
	}, nil
}

func (g *Game) AddPlayer(id string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()

	if len(g.players) >= g.maxPlayers {
		return false
	}

	rIdx := len(g.players)

	racer := g.racers[rIdx]

	g.removeAutopilot(rIdx)

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

// removeAutopilot drops the autopilot controlling the racer at rIdx so a
// human can take over that slot.
func (g *Game) removeAutopilot(rIdx int) {
	for i, a := range g.autopilots {
		if a.Racer.Index == rIdx {
			g.autopilots = append(g.autopilots[:i], g.autopilots[i+1:]...)
			return
		}
	}
}

// Drive steers the racer along a circle of spawnRadius around the origin,
// chasing the tangent velocity so it converges onto the circular path.
func (a *Autopilot) Drive(dt float32) {
	ph := &a.Racer.Physics

	r := float32(math.Sqrt(float64(ph.X*ph.X + ph.Y*ph.Y)))

	if r < 0.001 {
		r = 0.001
	}

	// Counter-clockwise tangent to the circle at the racer's position.
	tx := -ph.Y / r
	ty := ph.X / r

	ph.velX += (tx*autoSpeed - ph.velX) * autoGain * dt
	ph.velY += (ty*autoSpeed - ph.velY) * autoGain * dt
	ph.velZ = 0

	ph.X += ph.velX * dt
	ph.Y += ph.velY * dt
	ph.Z += ph.velZ * dt
}

// updateHeading faces the racer in the direction of travel. Physics use X/Y
// as the ground plane with Z up, but the quaternion is expressed for the
// renderer, which uses X/Z as the ground plane with Y up: a yaw of
// atan2(velY, velX) around the game's up axis maps to a rotation around +Y
// by its negation. Only steer while actually moving so the racer keeps its
// last heading when stopped.
func updateHeading(ph *PhysicsState) {
	if vx, vy := ph.velX, ph.velY; vx*vx+vy*vy > 1e-4 {
		half := math.Atan2(float64(vy), float64(vx)) / 2
		ph.RotW = float32(math.Cos(half))
		ph.RotY = float32(-math.Sin(half))
		ph.RotX = 0
		ph.RotZ = 0
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

			updateHeading(ph)
		}

		for _, a := range g.autopilots {
			a.Drive(dt)
			updateHeading(&a.Racer.Physics)
		}

		for _, racer := range g.racers {
			g.engine.Step(racer, dt)
		}

		gameData := encode(g.racers)

		g.dataCh <- gameData

		g.mu.Unlock()
	}
}

func (g *Game) Step(r *Racer) {

}
