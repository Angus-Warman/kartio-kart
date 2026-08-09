package geo

import (
	"encoding/binary"
	"fmt"
	"math"
)

// ------------------------------------------------------------
// Vec3
// ------------------------------------------------------------

type Vec3 struct{ X, Y, Z float64 }

func (a Vec3) Add(b Vec3) Vec3      { return Vec3{a.X + b.X, a.Y + b.Y, a.Z + b.Z} }
func (a Vec3) Sub(b Vec3) Vec3      { return Vec3{a.X - b.X, a.Y - b.Y, a.Z - b.Z} }
func (a Vec3) Scale(s float64) Vec3 { return Vec3{a.X * s, a.Y * s, a.Z * s} }
func (a Vec3) Dot(b Vec3) float64   { return a.X*b.X + a.Y*b.Y + a.Z*b.Z }
func (a Vec3) Cross(b Vec3) Vec3 {
	return Vec3{
		a.Y*b.Z - a.Z*b.Y,
		a.Z*b.X - a.X*b.Z,
		a.X*b.Y - a.Y*b.X,
	}
}
func (a Vec3) Len() float64 { return math.Sqrt(a.Dot(a)) }
func (a Vec3) Norm() Vec3 {
	l := a.Len()
	if l == 0 {
		return Vec3{}
	}
	return a.Scale(1 / l)
}
func minF(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
func maxF(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

// ------------------------------------------------------------
// Triangle
// ------------------------------------------------------------

type Triangle struct {
	A, B, C Vec3
}

func (t Triangle) Centroid() Vec3 {
	return Vec3{
		(t.A.X + t.B.X + t.C.X) / 3,
		(t.A.Y + t.B.Y + t.C.Y) / 3,
		(t.A.Z + t.B.Z + t.C.Z) / 3,
	}
}

func (t Triangle) Normal() Vec3 {
	return t.B.Sub(t.A).Cross(t.C.Sub(t.A)).Norm()
}

// RayHit holds the result of a successful ray-triangle intersection.
type RayHit struct {
	Point  Vec3
	Normal Vec3
	T      float64 // distance along ray
}

// IntersectRay tests the Möller–Trumbore intersection.
// Returns (hit, RayHit). Normal always points "up" (positive Y component preferred).
func (tri Triangle) IntersectRay(origin, dir Vec3) (bool, RayHit) {
	const eps = 1e-8

	edge1 := tri.B.Sub(tri.A)
	edge2 := tri.C.Sub(tri.A)

	h := dir.Cross(edge2)
	a := edge1.Dot(h)
	if a > -eps && a < eps {
		return false, RayHit{} // ray parallel to triangle
	}

	f := 1.0 / a
	s := origin.Sub(tri.A)
	u := f * s.Dot(h)
	if u < 0 || u > 1 {
		return false, RayHit{}
	}

	q := s.Cross(edge1)
	v := f * dir.Dot(q)
	if v < 0 || u+v > 1 {
		return false, RayHit{}
	}

	t := f * edge2.Dot(q)
	if t < eps {
		return false, RayHit{} // intersection behind origin
	}

	point := origin.Add(dir.Scale(t))
	normal := tri.Normal()

	// Flip normal so it opposes the ray direction (face the caster).
	if normal.Dot(dir) > 0 {
		normal = normal.Scale(-1)
	}

	return true, RayHit{Point: point, Normal: normal, T: t}
}

// ------------------------------------------------------------
// AABB
// ------------------------------------------------------------

type AABB struct {
	Min, Max Vec3
}

func aabbFromTriangles(tris []Triangle) AABB {
	box := AABB{
		Min: Vec3{math.MaxFloat64, math.MaxFloat64, math.MaxFloat64},
		Max: Vec3{-math.MaxFloat64, -math.MaxFloat64, -math.MaxFloat64},
	}
	for _, tri := range tris {
		for _, v := range []Vec3{tri.A, tri.B, tri.C} {
			box.Min.X = minF(box.Min.X, v.X)
			box.Min.Y = minF(box.Min.Y, v.Y)
			box.Min.Z = minF(box.Min.Z, v.Z)
			box.Max.X = maxF(box.Max.X, v.X)
			box.Max.Y = maxF(box.Max.Y, v.Y)
			box.Max.Z = maxF(box.Max.Z, v.Z)
		}
	}
	return box
}

// IntersectsRay uses the slab method. Returns (hit, tMin).
func (b AABB) IntersectsRay(origin, dir Vec3) (bool, float64) {
	tMin := -math.MaxFloat64
	tMax := math.MaxFloat64

	axes := [3][2]float64{
		{origin.X, dir.X},
		{origin.Y, dir.Y},
		{origin.Z, dir.Z},
	}
	mins := [3]float64{b.Min.X, b.Min.Y, b.Min.Z}
	maxs := [3]float64{b.Max.X, b.Max.Y, b.Max.Z}

	for i := 0; i < 3; i++ {
		o, d := axes[i][0], axes[i][1]
		if math.Abs(d) < 1e-8 {
			if o < mins[i] || o > maxs[i] {
				return false, 0
			}
			continue
		}
		t1 := (mins[i] - o) / d
		t2 := (maxs[i] - o) / d
		if t1 > t2 {
			t1, t2 = t2, t1
		}
		tMin = maxF(tMin, t1)
		tMax = minF(tMax, t2)
		if tMin > tMax {
			return false, 0
		}
	}
	return tMax >= 0, tMin
}

// LongestAxis returns the index of the longest axis (0=X, 1=Y, 2=Z).
func (b AABB) LongestAxis() int {
	dx := b.Max.X - b.Min.X
	dy := b.Max.Y - b.Min.Y
	dz := b.Max.Z - b.Min.Z
	if dx >= dy && dx >= dz {
		return 0
	}
	if dy >= dz {
		return 1
	}
	return 2
}

// ------------------------------------------------------------
// BVH
// ------------------------------------------------------------

const bvhLeafThreshold = 8 // triangles per leaf

// BVHNode is a node in the bounding volume hierarchy.
// Leaf nodes have Tris set; internal nodes have Left and Right set.
type BVHNode struct {
	Bounds AABB
	Left   *BVHNode
	Right  *BVHNode
	Tris   []Triangle
}

// BuildBVH constructs a BVH from a slice of triangles using
// surface-area-heuristic-free median split (fast, good enough for static tracks).
func BuildBVH(tris []Triangle) *BVHNode {
	if len(tris) == 0 {
		return nil
	}
	node := &BVHNode{Bounds: aabbFromTriangles(tris)}

	if len(tris) <= bvhLeafThreshold {
		node.Tris = tris
		return node
	}

	axis := node.Bounds.LongestAxis()

	// Sort triangles by centroid along the longest axis.
	sortTrisByAxis(tris, axis)

	mid := len(tris) / 2
	node.Left = BuildBVH(tris[:mid])
	node.Right = BuildBVH(tris[mid:])
	return node
}

// sortTrisByAxis is an in-place insertion sort (fast for small slices,
// simple to understand). For very large meshes swap to sort.Slice.
func sortTrisByAxis(tris []Triangle, axis int) {
	centroidVal := func(t Triangle) float64 {
		c := t.Centroid()
		switch axis {
		case 0:
			return c.X
		case 1:
			return c.Y
		default:
			return c.Z
		}
	}
	// simple sort.Slice equivalent without importing sort for clarity
	n := len(tris)
	for i := 1; i < n; i++ {
		key := tris[i]
		kv := centroidVal(key)
		j := i - 1
		for j >= 0 && centroidVal(tris[j]) > kv {
			tris[j+1] = tris[j]
			j--
		}
		tris[j+1] = key
	}
}

// ------------------------------------------------------------
// BVH — top-level handle, implements encoding.Binary{Marshaler,Unmarshaler}
// ------------------------------------------------------------

// BVH is the public handle for a built bounding volume hierarchy.
// Use New to construct one, then call Raycast / RaycastDown at runtime.
// It round-trips through binary via MarshalBinary / UnmarshalBinary.
type BVH struct {
	root *BVHNode
}

// NewBVH builds a BVH from a triangle slice.
func NewBVH(tris []Triangle) *BVH {
	return &BVH{root: BuildBVH(tris)}
}

// ------------------------------------------------------------
// Binary format (little-endian throughout)
//
// Header — 16 bytes
//   [0:4]   magic   "BVH1"
//   [4:8]   uint32  node count
//   [8:12]  uint32  triangle count
//   [12:16] uint32  reserved (0)
//
// Node × nodeCount — 40 bytes each
//   float32×6  AABB min XYZ, max XYZ
//   int32      left child index  (-1 = none)
//   int32      right child index (-1 = none)
//   int32      tri start index   (-1 = internal node)
//   int32      tri count         ( 0 = internal node)
//
// Triangle × triCount — 36 bytes each
//   float32×9  A.XYZ  B.XYZ  C.XYZ
// ------------------------------------------------------------

const bvhMagic = "BVH1"

type bvhFlatNode struct {
	MinX, MinY, MinZ float32
	MaxX, MaxY, MaxZ float32
	Left             int32
	Right            int32
	TriStart         int32
	TriCount         int32
}

type bvhFlatTri struct {
	AX, AY, AZ float32
	BX, BY, BZ float32
	CX, CY, CZ float32
}

// MarshalBinary implements encoding.BinaryMarshaler.
func (b *BVH) MarshalBinary() ([]byte, error) {
	nodes, tris := flattenBVH(b.root)

	const headerSize = 16
	const nodeSize = 40
	const triSize = 36
	total := headerSize + len(nodes)*nodeSize + len(tris)*triSize

	buf := make([]byte, total)
	le := binary.LittleEndian
	off := 0

	// Header
	copy(buf[off:], bvhMagic)
	off += 4
	le.PutUint32(buf[off:], uint32(len(nodes)))
	off += 4
	le.PutUint32(buf[off:], uint32(len(tris)))
	off += 4
	le.PutUint32(buf[off:], 0) // reserved
	off += 4

	// Nodes
	for _, n := range nodes {
		for _, f := range []float32{n.MinX, n.MinY, n.MinZ, n.MaxX, n.MaxY, n.MaxZ} {
			le.PutUint32(buf[off:], math.Float32bits(f))
			off += 4
		}
		for _, i := range []int32{n.Left, n.Right, n.TriStart, n.TriCount} {
			le.PutUint32(buf[off:], uint32(i))
			off += 4
		}
	}

	// Triangles
	for _, t := range tris {
		for _, f := range []float32{t.AX, t.AY, t.AZ, t.BX, t.BY, t.BZ, t.CX, t.CY, t.CZ} {
			le.PutUint32(buf[off:], math.Float32bits(f))
			off += 4
		}
	}

	return buf, nil
}

// UnmarshalBinary implements encoding.BinaryUnmarshaler.
func (b *BVH) UnmarshalBinary(data []byte) error {
	const headerSize = 16
	const nodeSize = 40
	const triSize = 36

	if len(data) < headerSize {
		return fmt.Errorf("bvh: data too short for header (%d bytes)", len(data))
	}

	le := binary.LittleEndian
	off := 0

	// Header
	if string(data[off:off+4]) != bvhMagic {
		return fmt.Errorf("bvh: bad magic %q", data[off:off+4])
	}
	off += 4
	nodeCount := int(le.Uint32(data[off:]))
	off += 4
	triCount := int(le.Uint32(data[off:]))
	off += 4
	off += 4 // reserved

	want := headerSize + nodeCount*nodeSize + triCount*triSize
	if len(data) < want {
		return fmt.Errorf("bvh: data too short: need %d bytes, have %d", want, len(data))
	}

	// Flat nodes
	flatNodes := make([]bvhFlatNode, nodeCount)
	for i := range flatNodes {
		readF32 := func() float32 {
			v := math.Float32frombits(le.Uint32(data[off:]))
			off += 4
			return v
		}
		readI32 := func() int32 {
			v := int32(le.Uint32(data[off:]))
			off += 4
			return v
		}
		flatNodes[i] = bvhFlatNode{
			MinX: readF32(), MinY: readF32(), MinZ: readF32(),
			MaxX: readF32(), MaxY: readF32(), MaxZ: readF32(),
			Left: readI32(), Right: readI32(),
			TriStart: readI32(), TriCount: readI32(),
		}
	}

	// Flat triangles
	flatTris := make([]bvhFlatTri, triCount)
	for i := range flatTris {
		f := func() float32 {
			v := math.Float32frombits(le.Uint32(data[off:]))
			off += 4
			return v
		}
		flatTris[i] = bvhFlatTri{
			AX: f(), AY: f(), AZ: f(),
			BX: f(), BY: f(), BZ: f(),
			CX: f(), CY: f(), CZ: f(),
		}
	}

	// Reconstruct tree from flat arrays.
	b.root = unflattenBVH(flatNodes, flatTris, 0)
	return nil
}

// ------------------------------------------------------------
// flatten / unflatten helpers
// ------------------------------------------------------------

func flattenBVH(root *BVHNode) ([]bvhFlatNode, []bvhFlatTri) {
	var nodes []bvhFlatNode
	var tris []bvhFlatTri

	var recurse func(n *BVHNode) int32
	recurse = func(n *BVHNode) int32 {
		if n == nil {
			return -1
		}
		idx := int32(len(nodes))
		nodes = append(nodes, bvhFlatNode{}) // reserve

		fn := bvhFlatNode{
			MinX: float32(n.Bounds.Min.X), MinY: float32(n.Bounds.Min.Y), MinZ: float32(n.Bounds.Min.Z),
			MaxX: float32(n.Bounds.Max.X), MaxY: float32(n.Bounds.Max.Y), MaxZ: float32(n.Bounds.Max.Z),
			Left: -1, Right: -1, TriStart: -1, TriCount: 0,
		}

		if n.Left == nil && n.Right == nil {
			fn.TriStart = int32(len(tris))
			fn.TriCount = int32(len(n.Tris))
			for _, t := range n.Tris {
				tris = append(tris, bvhFlatTri{
					AX: float32(t.A.X), AY: float32(t.A.Y), AZ: float32(t.A.Z),
					BX: float32(t.B.X), BY: float32(t.B.Y), BZ: float32(t.B.Z),
					CX: float32(t.C.X), CY: float32(t.C.Y), CZ: float32(t.C.Z),
				})
			}
		} else {
			fn.Left = recurse(n.Left)
			fn.Right = recurse(n.Right)
		}

		nodes[idx] = fn
		return idx
	}

	recurse(root)
	return nodes, tris
}

func unflattenBVH(nodes []bvhFlatNode, tris []bvhFlatTri, idx int32) *BVHNode {
	if idx < 0 || int(idx) >= len(nodes) {
		return nil
	}
	fn := nodes[idx]
	node := &BVHNode{
		Bounds: AABB{
			Min: Vec3{float64(fn.MinX), float64(fn.MinY), float64(fn.MinZ)},
			Max: Vec3{float64(fn.MaxX), float64(fn.MaxY), float64(fn.MaxZ)},
		},
	}

	if fn.TriStart >= 0 {
		// Leaf
		start := int(fn.TriStart)
		count := int(fn.TriCount)
		node.Tris = make([]Triangle, count)
		for i, ft := range tris[start : start+count] {
			node.Tris[i] = Triangle{
				A: Vec3{float64(ft.AX), float64(ft.AY), float64(ft.AZ)},
				B: Vec3{float64(ft.BX), float64(ft.BY), float64(ft.BZ)},
				C: Vec3{float64(ft.CX), float64(ft.CY), float64(ft.CZ)},
			}
		}
	} else {
		node.Left = unflattenBVH(nodes, tris, fn.Left)
		node.Right = unflattenBVH(nodes, tris, fn.Right)
	}

	return node
}

// ------------------------------------------------------------
// Raycast
// ------------------------------------------------------------

// Raycast traverses the BVH and returns the closest hit along the ray.
// origin and dir should be in the same coordinate space as the mesh.
// dir does NOT need to be normalised (but normals in the result will be unit length).
func (b *BVH) Raycast(origin, dir Vec3) (bool, RayHit) {
	if b.root == nil {
		return false, RayHit{}
	}
	return b.root.raycast(origin, dir)
}

// RaycastDown is a convenience wrapper for the common case of casting straight
// down (negative Y) from a position above the track.
// startHeight is added to pos.Y so you can cast from safely above the car.
func (b *BVH) RaycastDown(pos Vec3, startHeight float64) (bool, RayHit) {
	origin := Vec3{pos.X, pos.Y + startHeight, pos.Z}
	return b.Raycast(origin, Vec3{0, -1, 0})
}

// raycast is the internal recursive traversal on BVHNode.
func (node *BVHNode) raycast(origin, dir Vec3) (bool, RayHit) {
	ok, _ := node.Bounds.IntersectsRay(origin, dir)
	if !ok {
		return false, RayHit{}
	}

	// Leaf — test all triangles, return closest.
	if node.Left == nil && node.Right == nil {
		bestT := math.MaxFloat64
		var bestHit RayHit
		found := false
		for _, tri := range node.Tris {
			if hit, h := tri.IntersectRay(origin, dir); hit && h.T < bestT {
				bestT = h.T
				bestHit = h
				found = true
			}
		}
		return found, bestHit
	}

	// Internal — recurse into both children, keep nearest.
	hitL, hL := node.Left.raycast(origin, dir)
	hitR, hR := node.Right.raycast(origin, dir)

	switch {
	case hitL && hitR:
		if hL.T <= hR.T {
			return true, hL
		}
		return true, hR
	case hitL:
		return true, hL
	case hitR:
		return true, hR
	default:
		return false, RayHit{}
	}
}
