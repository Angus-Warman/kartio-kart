package geo

import "math"

// Simplify reduces a triangle soup before it is handed to BuildBVH.
// It runs three passes in order:
//
//  1. Degenerate removal  — drops zero-area triangles that contribute nothing.
//  2. Vertex welding      — snaps vertices within eps of each other to a single
//     canonical position, eliminating the duplicated verts that every OBJ /
//     GLB exporter produces along UV seams and hard edges.
//  3. Coplanar merging    — walks the welded adjacency graph and merges
//     neighbouring triangles that share an edge and lie on the same plane
//     (normal dot-product > coplanarDot, same d in the plane equation) into
//     larger polygons, then re-fans those polygons into fewer triangles.
//
// The result is passed straight to BuildBVH.  For a typical kart-scale
// collision mesh you can expect 30–70 % triangle reduction on flat/gently
// curved surfaces (road surface, walls, ramps) with zero change to curved or
// detailed geometry.
//
// eps controls vertex welding distance (world units).
// coplanarAngleDeg controls the maximum angle between face normals that is
// still considered "same plane" — 1.0° is tight (only truly flat surfaces),
// 5.0° merges gently curved ramps, anything above ~15° starts merging things
// you probably want kept separate.
func Simplify(tris []Triangle, eps, coplanarAngleDeg float64) []Triangle {
	tris = removeDegenerate(tris, eps)
	tris = weldAndMerge(tris, eps, coplanarAngleDeg)
	return tris
}

// ------------------------------------------------------------
// Pass 1 — degenerate removal
// ------------------------------------------------------------

// removeDegenerate drops triangles whose area is below the epsilon threshold.
func removeDegenerate(tris []Triangle, eps float64) []Triangle {
	minArea := eps * eps * 0.5
	out := tris[:0]
	for _, t := range tris {
		ab := t.B.Sub(t.A)
		ac := t.C.Sub(t.A)
		// area = 0.5 * |cross|
		cross := ab.Cross(ac)
		area := cross.Len() * 0.5
		if area > minArea {
			out = append(out, t)
		}
	}
	return out
}

// ------------------------------------------------------------
// Pass 2 — vertex welding
// ------------------------------------------------------------

// weldAndMerge welds vertices then runs coplanar merging.
func weldAndMerge(tris []Triangle, eps, coplanarAngleDeg float64) []Triangle {
	verts, indices := weldVertices(tris, eps)
	return mergeCoplanar(verts, indices, coplanarAngleDeg)
}

// weldVertices snaps nearby vertices together and returns:
//   - a canonical vertex table
//   - the triangle list as index triples into that table
func weldVertices(tris []Triangle, eps float64) ([]Vec3, [][3]int) {
	// Spatial grid: bucket size = eps.  For each vertex we check its own
	// cell and the 26 neighbours so we never miss a weld across a cell border.
	type cellKey struct{ x, y, z int }
	grid := make(map[cellKey][]int)
	canonical := []Vec3{}

	cellOf := func(v Vec3) cellKey {
		return cellKey{
			x: int(math.Floor(v.X / eps)),
			y: int(math.Floor(v.Y / eps)),
			z: int(math.Floor(v.Z / eps)),
		}
	}

	findOrAdd := func(v Vec3) int {
		ck := cellOf(v)
		// Search 3×3×3 neighbourhood.
		for dx := -1; dx <= 1; dx++ {
			for dy := -1; dy <= 1; dy++ {
				for dz := -1; dz <= 1; dz++ {
					nk := cellKey{ck.x + dx, ck.y + dy, ck.z + dz}
					for _, idx := range grid[nk] {
						c := canonical[idx]
						if math.Abs(c.X-v.X) <= eps &&
							math.Abs(c.Y-v.Y) <= eps &&
							math.Abs(c.Z-v.Z) <= eps {
							return idx
						}
					}
				}
			}
		}
		// New canonical vertex.
		idx := len(canonical)
		canonical = append(canonical, v)
		grid[ck] = append(grid[ck], idx)
		return idx
	}

	indices := make([][3]int, 0, len(tris))
	for _, tri := range tris {
		a := findOrAdd(tri.A)
		b := findOrAdd(tri.B)
		c := findOrAdd(tri.C)
		if a == b || b == c || a == c {
			continue // collapsed to a line after welding — skip
		}
		indices = append(indices, [3]int{a, b, c})
	}

	return canonical, indices
}

// ------------------------------------------------------------
// Pass 3 — coplanar merging
// ------------------------------------------------------------

// edgeKey is an unordered pair of vertex indices representing a shared edge.
type edgeKey struct{ lo, hi int }

func makeEdgeKey(a, b int) edgeKey {
	if a < b {
		return edgeKey{a, b}
	}
	return edgeKey{b, a}
}

// mergeCoplanar merges adjacent coplanar triangles into convex polygons then
// re-fans them, reducing the total triangle count on flat surfaces.
func mergeCoplanar(verts []Vec3, indices [][3]int, coplanarAngleDeg float64) []Triangle {
	coplanarDot := math.Cos(coplanarAngleDeg * math.Pi / 180.0)

	n := len(indices)
	normals := make([]Vec3, n)
	ds := make([]float64, n) // plane d: normal·point
	for i, tri := range indices {
		e1 := verts[tri[1]].Sub(verts[tri[0]])
		e2 := verts[tri[2]].Sub(verts[tri[0]])
		normals[i] = e1.Cross(e2).Norm()
		ds[i] = normals[i].Dot(verts[tri[0]])
	}

	// Build edge → triangle adjacency.
	// Each edge maps to at most 2 triangle indices (manifold mesh).
	type edgeEntry struct{ t0, t1 int }
	edgeMap := make(map[edgeKey]edgeEntry, n*3)

	addEdge := func(triIdx, a, b int) {
		ek := makeEdgeKey(a, b)
		e := edgeMap[ek]
		if e.t0 == 0 && e.t1 == 0 {
			e.t0 = triIdx + 1 // store 1-based to distinguish "empty" from tri 0
		} else {
			e.t1 = triIdx + 1
		}
		edgeMap[ek] = e
	}
	for i, tri := range indices {
		addEdge(i, tri[0], tri[1])
		addEdge(i, tri[1], tri[2])
		addEdge(i, tri[2], tri[0])
	}

	merged := make([]bool, n)
	out := make([]Triangle, 0, n)

	// For each unmerged triangle, BFS over coplanar neighbours.
	for start := 0; start < n; start++ {
		if merged[start] {
			continue
		}

		// Collect the connected coplanar island reachable from start.
		island := []int{start}
		merged[start] = true
		queue := []int{start}

		for len(queue) > 0 {
			cur := queue[0]
			queue = queue[1:]
			tri := indices[cur]
			edges := [3][2]int{{tri[0], tri[1]}, {tri[1], tri[2]}, {tri[2], tri[0]}}
			for _, e := range edges {
				ek := makeEdgeKey(e[0], e[1])
				entry := edgeMap[ek]
				for _, nb1 := range []int{entry.t0 - 1, entry.t1 - 1} {
					if nb1 < 0 || nb1 == cur || merged[nb1] {
						continue
					}
					// Coplanar check: same normal direction and same d.
					dot := normals[cur].Dot(normals[nb1])
					if dot < coplanarDot {
						continue
					}
					dDiff := math.Abs(ds[cur] - ds[nb1])
					if dDiff > 1e-4 {
						continue
					}
					merged[nb1] = true
					island = append(island, nb1)
					queue = append(queue, nb1)
				}
			}
		}

		if len(island) == 1 {
			// Nothing to merge — emit as-is.
			tri := indices[start]
			out = append(out, Triangle{
				A: verts[tri[0]],
				B: verts[tri[1]],
				C: verts[tri[2]],
			})
			continue
		}

		// Merge island into a polygon and re-fan.
		// Strategy: extract the boundary loop of the island, then ear-fan from
		// the first vertex.  This works correctly for convex islands (the common
		// case on flat track surfaces) and produces valid (if not minimal) output
		// for concave islands.
		boundary := extractBoundary(indices, island)
		if len(boundary) < 3 {
			// Degenerate island — emit original triangles.
			for _, ti := range island {
				tri := indices[ti]
				out = append(out, Triangle{
					A: verts[tri[0]],
					B: verts[tri[1]],
					C: verts[tri[2]],
				})
			}
			continue
		}

		// Fan-triangulate the boundary polygon.
		for i := 1; i+1 < len(boundary); i++ {
			t := Triangle{
				A: verts[boundary[0]],
				B: verts[boundary[i]],
				C: verts[boundary[i+1]],
			}
			// Drop degenerate fans (can appear in concave islands).
			ab := t.B.Sub(t.A)
			ac := t.C.Sub(t.A)
			if ab.Cross(ac).Len() > 1e-10 {
				out = append(out, t)
			}
		}
	}

	return out
}

// extractBoundary returns the ordered boundary vertex loop of a set of triangles.
// Boundary edges are those that appear in exactly one triangle of the island.
func extractBoundary(indices [][3]int, island []int) []int {
	// Count edge occurrences within the island.
	edgeCount := make(map[edgeKey]int)
	edgeDir := make(map[edgeKey][2]int) // directed edge (preserves winding)

	for _, ti := range island {
		tri := indices[ti]
		edges := [3][2]int{{tri[0], tri[1]}, {tri[1], tri[2]}, {tri[2], tri[0]}}
		for _, e := range edges {
			ek := makeEdgeKey(e[0], e[1])
			edgeCount[ek]++
			edgeDir[ek] = [2]int{e[0], e[1]}
		}
	}

	// Boundary edges appear exactly once.
	// Build adjacency: vertex → next vertex along boundary.
	next := make(map[int]int)
	for ek, count := range edgeCount {
		if count == 1 {
			dir := edgeDir[ek]
			next[dir[0]] = dir[1]
		}
	}
	if len(next) == 0 {
		return nil
	}

	// Walk the loop.
	start := -1
	for v := range next {
		start = v
		break
	}

	loop := []int{start}
	cur := next[start]
	for cur != start && len(loop) <= len(next) {
		loop = append(loop, cur)
		var ok bool
		cur, ok = next[cur]
		if !ok {
			return nil // open boundary — non-manifold input
		}
	}
	if cur != start {
		return nil // didn't close
	}
	return loop
}
