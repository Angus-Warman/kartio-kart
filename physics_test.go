package main

import (
	"math"
	"testing"
)

// SUGGESTION ONLY
// The tests in this file are suggestions, not hard requirements. Making them
// pass is not a mandatory part of the task. If a test fails, investigate the
// cause; whether to fix it is a judgement call and can be deferred.
//
// TestStepSettlesRacerOnTrack simulates the game loop (position integration in
// Run + gravity/contact in Step) and checks each racer lands on the track.
func TestStepSettlesRacerOnTrack(t *testing.T) {
	e, err := NewEngine()
	if err != nil {
		t.Fatal(err)
	}

	dt := float32(16e-3)

	for i := 0; i < numRacers; i++ {
		angle := 2 * math.Pi * float64(i) / float64(numRacers)
		r := &Racer{
			Physics: PhysicsState{
				X: float32(4 * math.Cos(angle)),
				Y: float32(4 * math.Sin(angle)),
				Z: 0,
			},
		}

		minZ := r.Physics.Z
		settled := 0
		steps := 600
		for range steps {
			r.Physics.Z += r.Physics.velZ * dt
			e.Step(r, dt)
			if r.Physics.Z < minZ {
				minZ = r.Physics.Z
			}
			if math.Abs(float64(r.Physics.velZ)) < 0.1 {
				settled++
			} else {
				settled = 0
			}
		}

		if settled < 60 {
			t.Errorf("racer %d did not settle: Z=%.3f velZ=%.3f", i, r.Physics.Z, r.Physics.velZ)
		} else {
			t.Logf("racer %d fell from Z=0 to min %.3f, settled at Z=%.3f velZ=%.3f",
				i, minZ, r.Physics.Z, r.Physics.velZ)
		}
	}
}
