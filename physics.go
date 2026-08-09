package main

import (
	_ "embed"
	"kartio-kart/geo"
)

//go:embed assets/wario_stadium.col
var stadiumColBytes []byte

const (
	gravity     = 9.81 // m/s², applied along -Z (the physics up axis)
	trackOffset = 20.0 // collision mesh is Y-up; track sits at mesh Y = physics Z + trackOffset
	castAbove   = 5.0  // raycast origin height above the racer, in mesh units
	surfaceK    = 60.0 // contact spring stiffness (1/s²)
	surfaceC    = 4.0  // contact damping (1/s)
)

type Engine struct {
	bvh *geo.BVH
}

func NewEngine() (*Engine, error) {
	bvh := &geo.BVH{}
	err := bvh.UnmarshalBinary(stadiumColBytes)
	if err != nil {
		return nil, err
	}

	return &Engine{
		bvh: bvh,
	}, nil
}

// Step applies gravity and track contact forces to a racer for one tick.
//
// Physics uses X/Y as the ground plane with Z up, while the collision mesh is
// Y-up and translated so the track sits at mesh Y = trackOffset. A ray is cast
// straight down from the racer; while the racer penetrates the surface a
// spring-damper contact force resists the penetration, then any remainder is
// resolved so the racer rests exactly on the track.
func (e *Engine) Step(racer *Racer, dt float32) {
	ph := &racer.Physics

	// Gravity pulls the racer down along -Z.
	ph.velZ -= gravity * dt

	// Convert the racer into mesh space and cast down onto the surface.
	meshPos := geo.Vec3{
		X: float64(ph.X),
		Y: float64(ph.Z) + trackOffset,
		Z: float64(ph.Y),
	}

	hit, res := e.bvh.RaycastDown(meshPos, castAbove)
	if !hit {
		return // no surface below (e.g. over a gap); gravity keeps it falling
	}

	surfaceZ := float32(res.Point.Y) - trackOffset
	depth := surfaceZ - ph.Z

	// Contact force: a spring-damper that resists penetration below the
	// surface, then resolves the remaining overlap so it rests on top.
	if depth > 0 {
		ph.velZ += (depth*surfaceK - ph.velZ*surfaceC) * dt
		if ph.Z < surfaceZ {
			ph.Z = surfaceZ
		}
	}

	// While grounded, don't let gravity accumulate downward velocity.
	if depth >= 0 && ph.velZ < 0 {
		ph.velZ = 0
	}
}
