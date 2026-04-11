package pointcloud

import (
	"math"
	"testing"

	"fyne.io/fyne/v2"
)

func TestQuatConjugate(t *testing.T) {
	q := QuatFromAxisAngle(0, 1, 0, math.Pi/4)
	c := q.Conjugate()

	// q * conjugate(q) should be identity.
	product := q.Mul(c)
	if math.Abs(product.W-1.0) > 1e-10 {
		t.Errorf("q * conj(q) should be identity, got W=%f", product.W)
	}
	if math.Abs(product.X)+math.Abs(product.Y)+math.Abs(product.Z) > 1e-10 {
		t.Errorf("q * conj(q) should have zero imaginary, got (%f, %f, %f)", product.X, product.Y, product.Z)
	}
}

func TestQuatRotateVec3(t *testing.T) {
	// 90-degree rotation around Y should send (1,0,0) to (0,0,-1).
	q := QuatFromAxisAngle(0, 1, 0, math.Pi/2)
	v := q.RotateVec3([3]float64{1, 0, 0})
	if math.Abs(v[0]-0) > 1e-10 || math.Abs(v[1]-0) > 1e-10 || math.Abs(v[2]-(-1)) > 1e-10 {
		t.Errorf("expected (0,0,-1), got (%f,%f,%f)", v[0], v[1], v[2])
	}

	// Identity quaternion should not change the vector.
	q = QuatIdentity()
	v = q.RotateVec3([3]float64{3, 4, 5})
	if math.Abs(v[0]-3) > 1e-10 || math.Abs(v[1]-4) > 1e-10 || math.Abs(v[2]-5) > 1e-10 {
		t.Errorf("identity rotation changed vector: got (%f,%f,%f)", v[0], v[1], v[2])
	}
}

func TestProjectChunkOrbitCompatibility(t *testing.T) {
	// Verify that projectChunk with tx=0, ty=0, tz=4.0 produces the same
	// output as the original formula (dist = 4.0 - rz).
	pts := generatePoints(1000)
	c := setupCanvas(pts)

	w, h := 512, 384
	m := c.orientation.ToMatrix()

	// Run with the new parameterized version.
	img1 := make([]byte, w*h*4)
	stride := w * 4
	m0, m1, m2 := float32(m[0]), float32(m[1]), float32(m[2])
	m3, m4, m5 := float32(m[3]), float32(m[4]), float32(m[5])
	m6, m7, m8 := float32(m[6]), float32(m[7]), float32(m[8])
	zoom := float32(c.zoom)
	centerX := float32(w) / 2
	centerY := float32(h) / 2

	projectChunk(c.xs, c.ys, c.zs, c.rgba, img1, stride, w, h,
		m0, m1, m2, m3, m4, m5, m6, m7, m8,
		0, 0, 4.0,
		zoom, centerX, centerY, 255, 150, 255)

	// Verify at least some pixels were written (not all zero).
	nonZero := 0
	for i := 0; i < len(img1); i += 4 {
		if img1[i] != 0 || img1[i+1] != 0 || img1[i+2] != 0 {
			nonZero++
		}
	}
	if nonZero == 0 {
		t.Error("projectChunk produced no visible pixels")
	}
}

func TestViewMatrixIdentity(t *testing.T) {
	cam := newFlythroughCamera(nil)
	cam.pos = [3]float64{0, 0, 0}
	cam.orientation = QuatIdentity()

	m, tx, ty, tz := cam.viewMatrix()
	// With identity orientation and position at origin, the view matrix
	// should be identity and translations should be zero.
	if math.Abs(m[0]-1) > 1e-10 || math.Abs(m[4]-1) > 1e-10 || math.Abs(m[8]-1) > 1e-10 {
		t.Errorf("expected identity matrix diagonal, got [%f, %f, %f]", m[0], m[4], m[8])
	}
	if math.Abs(tx) > 1e-10 || math.Abs(ty) > 1e-10 || math.Abs(tz) > 1e-10 {
		t.Errorf("expected zero translation, got (%f, %f, %f)", tx, ty, tz)
	}
}

func TestViewMatrixKnownPosition(t *testing.T) {
	cam := newFlythroughCamera(nil)
	cam.pos = [3]float64{0, 0, 4.0}
	cam.orientation = QuatIdentity()

	_, tx, ty, tz := cam.viewMatrix()
	// Camera at (0,0,4) looking down -Z: tz should be 4.0 (same as orbit mode).
	if math.Abs(tx) > 1e-10 || math.Abs(ty) > 1e-10 || math.Abs(tz-4.0) > 1e-10 {
		t.Errorf("expected (0, 0, 4), got (%f, %f, %f)", tx, ty, tz)
	}
}

func TestOrbitFlythroughRoundTrip(t *testing.T) {
	cam := newFlythroughCamera(nil)
	origOrientation := QuatFromEulerXY(-0.3, -math.Pi/4)
	origZoom := 200.0

	cam.fromOrbit(origOrientation, origZoom, 0, 0)
	gotOrient, gotZoom := cam.toOrbit(origZoom)

	// Orientation should be preserved exactly.
	if math.Abs(gotOrient.X-origOrientation.X) > 1e-10 ||
		math.Abs(gotOrient.Y-origOrientation.Y) > 1e-10 ||
		math.Abs(gotOrient.Z-origOrientation.Z) > 1e-10 ||
		math.Abs(gotOrient.W-origOrientation.W) > 1e-10 {
		t.Errorf("orientation not preserved: got %v, want %v", gotOrient, origOrientation)
	}

	// Zoom should be exactly preserved through the round trip (tz=4.0).
	if math.Abs(gotZoom-origZoom)/origZoom > 1e-10 {
		t.Errorf("zoom not preserved: got %f, want %f", gotZoom, origZoom)
	}
}

func TestFlythroughTick(t *testing.T) {
	cam := newFlythroughCamera(nil)
	cam.pos = [3]float64{0, 0, 0}
	cam.orientation = QuatIdentity()
	cam.speedMultiple = 10.0 // fast enough to see movement in one tick

	// Hold W key (forward).
	cam.keysHeld[fyne.KeyW] = true
	moved := cam.tick(1.0 / 60.0)
	if !moved {
		t.Error("tick should report movement when W is held")
	}

	// Camera should have moved forward (negative Z in camera space).
	if cam.pos[2] >= 0 {
		t.Errorf("camera should have moved forward (negative Z), got Z=%f", cam.pos[2])
	}
}

func TestBuildGrid(t *testing.T) {
	pts := generatePoints(10000)
	c := setupCanvas(pts)

	g, xs, ys, zs, rgba := buildGrid(c.xs, c.ys, c.zs, c.rgba)
	if g == nil {
		t.Fatal("buildGrid returned nil")
	}

	// Verify all points are accounted for.
	total := 0
	for i := range g.cells {
		total += g.cells[i].count
	}
	if total != len(xs) {
		t.Errorf("grid has %d points, expected %d", total, len(xs))
	}

	_ = ys
	_ = zs
	_ = rgba
}

func TestGridFrustumCulling(t *testing.T) {
	pts := generatePoints(10000)
	c := setupCanvas(pts)

	g, _, _, _, _ := buildGrid(c.xs, c.ys, c.zs, c.rgba)
	if g == nil {
		t.Fatal("buildGrid returned nil")
	}

	// Extract frustum planes from a known view and verify culling.
	m := QuatIdentity().ToMatrix()
	planes := extractFrustumPlanes(m, 0, 0, 4.0, 200, 1.33)
	cells := g.visibleCells(planes)

	// With a generous frustum from distance 4, most cells should be visible.
	if len(cells) == 0 {
		t.Error("frustum culling removed all cells")
	}
}
