package main

import (
	"kartio-kart/geo"
	"log"
	"os"
)

func main() {
	log.Println("Starting...")

	err := process()

	if err != nil {
		log.Fatalln(err)
	}

	log.Println("Done")
}

func process() error {
	input := "./assets/wario_stadium.obj"
	output := "./assets/wario_stadium.col"

	log.Println("loading", input)
	tris, err := geo.LoadOBJ(input)

	if err != nil {
		return err
	}

	log.Println("processing triangles")
	bvh := geo.NewBVH(tris)

	data, err := bvh.MarshalBinary()

	if err != nil {
		return err
	}

	err = os.WriteFile(output, data, 0666)

	if err != nil {
		return err
	}

	return nil
}

// ------------------------------------------------------------
// Usage example (remove or move to main package)
// ------------------------------------------------------------

// Example shows the intended usage. Not compiled unless called.
//
//	tris, err := collision.LoadOBJ("assets/track_collision.obj")
//	if err != nil { log.Fatal(err) }
//
//	bvh := collision.BuildBVH(tris)
//
//	// Every physics tick, per car:
//	hit, result := bvh.RaycastDown(car.Pos, 2.0)
//	if hit {
//	    // Snap car to track surface
//	    car.Pos.Y = result.Point.Y + rideHeight
//	    // Align car orientation to surface normal
//	    car.SurfaceNormal = result.Normal
//	}
