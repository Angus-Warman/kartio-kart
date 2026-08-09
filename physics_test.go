package main

import (
	"math"
	"testing"

	"kartio-kart/geo"
)

// SUGGESTION ONLY
// The tests in this file are suggestions, not hard requirements. Making them
// pass is not a mandatory part of the task. If a test fails, investigate the
// cause; whether to fix it is a judgement call and can be deferred.
//
// TestStepSettlesRacerOnTrack simulates the game loop for an autopilot
// (position integration in Autopilot.Drive + gravity/contact in Engine.Step).
// It checks that gravity drops each racer from its spawn height onto the track
// surface, and that the ground contact never lets one sink below the surface.
func TestStepSettlesRacerOnTrack(t *testing.T) {
	e, err := NewEngine()
	if err != nil {
		t.Fatal(err)
	}

	dt := float32(16e-3)
	const settleTicks = 600 // ~10 s

	for i := 0; i < numRacers; i++ {
		angle := 2 * math.Pi * float64(i) / float64(numRacers)
		r := &Racer{
			Index: i,
			Physics: PhysicsState{
				X: float32(4 * math.Cos(angle)),
				Y: float32(4 * math.Sin(angle)),
				Z: 0,
			},
		}
		a := &Autopilot{Racer: r}

		minZ := r.Physics.Z
		sunkTicks := 0
		for tick := 0; tick < settleTicks; tick++ {
			a.Drive(dt)
			e.Step(r, dt)

			if r.Physics.Z < minZ {
				minZ = r.Physics.Z
			}

			// Once the racer has been driving a while it should ride the
			// surface, never falling far below it.
			if tick > 150 {
				mesh := geo.Vec3{
					X: float64(r.Physics.X),
					Y: float64(r.Physics.Z) + trackOffset,
					Z: float64(r.Physics.Y),
				}
				if hit, res := e.bvh.RaycastDown(mesh, castAbove); hit {
					if surfaceZ := float32(res.Point.Y) - trackOffset; r.Physics.Z < surfaceZ-1.0 {
						sunkTicks++
					}
				}
			}
		}

		if minZ > -2.0 {
			t.Errorf("racer %d never fell to the track (minZ=%.3f)", i, minZ)
		} else if sunkTicks > 5 {
			t.Errorf("racer %d sank below the track surface on %d ticks", i, sunkTicks)
		} else {
			t.Logf("racer %d fell from Z=0 to min %.3f, riding the track (sunkTicks=%d)",
				i, minZ, sunkTicks)
		}
	}
}
