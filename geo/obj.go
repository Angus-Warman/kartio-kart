package geo

import (
	"os"
	"strconv"
	"strings"
)

// LoadOBJ parses a Wavefront .obj file and returns all triangles.
// Quads (f with 4 indices) are automatically split into two triangles.
// Only positions are read; UVs and normals in the file are ignored
// (normals are computed analytically from geometry).
func LoadOBJ(path string) ([]Triangle, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var verts []Vec3
	var tris []Triangle

	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if len(line) == 0 || line[0] == '#' {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}

		switch fields[0] {
		case "v":
			if len(fields) < 4 {
				continue
			}
			x, _ := strconv.ParseFloat(fields[1], 64)
			y, _ := strconv.ParseFloat(fields[2], 64)
			z, _ := strconv.ParseFloat(fields[3], 64)
			verts = append(verts, Vec3{x, y, z})

		case "f":
			// Each field after "f" is "vertIdx[/uvIdx[/normIdx]]" (1-based).
			indices := make([]int, 0, len(fields)-1)
			for _, f := range fields[1:] {
				parts := strings.SplitN(f, "/", 2)
				idx, _ := strconv.Atoi(parts[0])
				if idx < 0 {
					idx = len(verts) + idx + 1 // negative = relative
				}
				indices = append(indices, idx-1) // convert to 0-based
			}
			// Fan triangulation: works for convex polygons (standard for track meshes).
			for i := 1; i+1 < len(indices); i++ {
				a, b, c := indices[0], indices[i], indices[i+1]
				if a < len(verts) && b < len(verts) && c < len(verts) {
					tris = append(tris, Triangle{
						A: verts[a],
						B: verts[b],
						C: verts[c],
					})
				}
			}
		}
	}

	return tris, nil
}
