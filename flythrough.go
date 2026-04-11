package pointcloud

import (
	"math"
	"sync"
	"time"

	"fyne.io/fyne/v2"
)

// flythroughBaseSpeed is the default movement speed in normalized units per tick.
const flythroughBaseSpeed = 0.08

// flythroughCamera implements a first-person camera for flying through
// a point cloud with WASD+mouse controls.
type flythroughCamera struct {
	mu            sync.Mutex
	pos           [3]float64 // camera position in normalized space
	orientation   Quat       // camera look direction
	speedMultiple float64    // speed as multiplier of base speed (1.0 = 1x)
	shiftHeld     bool       // when true, WASD/arrows rotate instead of move
	keysHeld      map[fyne.KeyName]bool
	ticker        *time.Ticker
	canvas        *canvas3d // back-reference for refresh
}

func newFlythroughCamera(c *canvas3d) *flythroughCamera {
	return &flythroughCamera{
		orientation:   QuatIdentity(),
		speedMultiple: 1.0,
		keysHeld:      make(map[fyne.KeyName]bool),
		canvas:        c,
	}
}

// start begins the movement ticker goroutine.
func (f *flythroughCamera) start() {
	if f.ticker != nil {
		return
	}
	f.ticker = time.NewTicker(16 * time.Millisecond)
	go func() {
		for range f.ticker.C {
			if f.tick(0.016) {
				f.canvas.startInteraction()
				fyne.Do(func() {
					f.canvas.raster.Refresh()
				})
			}
		}
	}()
}

// stop stops the movement ticker.
func (f *flythroughCamera) stop() {
	if f.ticker != nil {
		f.ticker.Stop()
		f.ticker = nil
	}
}

// hasKeysHeld returns true if any movement keys are currently held.
func (f *flythroughCamera) hasKeysHeld() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, held := range f.keysHeld {
		if held {
			return true
		}
	}
	return false
}

// tick updates the camera based on held keys. With shift held, WASD/arrows
// rotate the camera (pitch/roll). Without shift, they move. Returns true
// if anything changed.
func (f *flythroughCamera) tick(dt float64) bool {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.shiftHeld {
		return f.tickRotate(dt)
	}
	return f.tickMove(dt)
}

// tickMove handles translation. Must be called with f.mu held.
func (f *flythroughCamera) tickMove(dt float64) bool {
	var dx, dy, dz float64
	if f.keysHeld[fyne.KeyW] || f.keysHeld[fyne.KeyUp] {
		dz++
	}
	if f.keysHeld[fyne.KeyS] || f.keysHeld[fyne.KeyDown] {
		dz--
	}
	if f.keysHeld[fyne.KeyA] || f.keysHeld[fyne.KeyLeft] {
		dx--
	}
	if f.keysHeld[fyne.KeyD] || f.keysHeld[fyne.KeyRight] {
		dx++
	}
	if f.keysHeld[fyne.KeySpace] {
		dy++
	}
	if f.keysHeld[fyne.KeyQ] {
		dy--
	}

	if dx == 0 && dy == 0 && dz == 0 {
		return false
	}

	length := math.Sqrt(dx*dx + dy*dy + dz*dz)
	if length > 0 {
		dx /= length
		dy /= length
		dz /= length
	}

	speed := flythroughBaseSpeed * f.speedMultiple * dt * 60

	right := f.orientation.RotateVec3([3]float64{1, 0, 0})
	up := [3]float64{0, 1, 0}
	forward := f.orientation.RotateVec3([3]float64{0, 0, -1})

	f.pos[0] += (right[0]*dx + up[0]*dy + forward[0]*dz) * speed
	f.pos[1] += (right[1]*dx + up[1]*dy + forward[1]*dz) * speed
	f.pos[2] += (right[2]*dx + up[2]*dy + forward[2]*dz) * speed

	return true
}

const rotateRate = 1.5 // radians per second

// tickRotate handles pitch and roll. Must be called with f.mu held.
func (f *flythroughCamera) tickRotate(dt float64) bool {
	var pitch, roll float64

	// W/S and Up/Down: pitch (rotate around camera-local X).
	if f.keysHeld[fyne.KeyW] || f.keysHeld[fyne.KeyUp] {
		pitch--
	}
	if f.keysHeld[fyne.KeyS] || f.keysHeld[fyne.KeyDown] {
		pitch++
	}

	// A/D and Left/Right: roll (rotate around camera-local Z).
	if f.keysHeld[fyne.KeyA] || f.keysHeld[fyne.KeyLeft] {
		roll++
	}
	if f.keysHeld[fyne.KeyD] || f.keysHeld[fyne.KeyRight] {
		roll--
	}

	if pitch == 0 && roll == 0 {
		return false
	}

	angle := rotateRate * dt
	if pitch != 0 {
		q := QuatFromAxisAngle(1, 0, 0, pitch*angle)
		f.orientation = f.orientation.Mul(q).Normalize()
	}
	if roll != 0 {
		q := QuatFromAxisAngle(0, 0, 1, roll*angle)
		f.orientation = f.orientation.Mul(q).Normalize()
	}

	return true
}

// handleMouseLook applies yaw (around world Y) and pitch (around local X).
func (f *flythroughCamera) handleMouseLook(dx, dy float64) {
	const sensitivity = 0.003

	f.mu.Lock()
	defer f.mu.Unlock()

	// Yaw around world Y axis.
	yaw := QuatFromAxisAngle(0, 1, 0, -dx*sensitivity)
	// Pitch around camera-local X axis.
	pitch := QuatFromAxisAngle(1, 0, 0, -dy*sensitivity)

	f.orientation = yaw.Mul(f.orientation).Mul(pitch).Normalize()
}

// viewMatrix returns the view rotation matrix and translation for projectChunk.
func (f *flythroughCamera) viewMatrix() (m [9]float64, tx, ty, tz float64) {
	f.mu.Lock()
	pos := f.pos
	orient := f.orientation
	f.mu.Unlock()

	// The view matrix is the inverse of the camera transform.
	// For a rotation quaternion, the inverse is the conjugate.
	invOrient := orient.Conjugate()
	m = invOrient.ToMatrix()

	// Translate to camera space: multiply position by inverse rotation.
	// tx/ty are -(R^T * pos) and get added to rx/ry in the inner loop.
	// tz uses the positive sign because the inner loop computes
	// dist = tz - rz (not tz + rz), so the sign is already flipped.
	tx = -(m[0]*pos[0] + m[1]*pos[1] + m[2]*pos[2])
	ty = -(m[3]*pos[0] + m[4]*pos[1] + m[5]*pos[2])
	tz = m[6]*pos[0] + m[7]*pos[1] + m[8]*pos[2]

	return m, tx, ty, tz
}

// fromOrbit sets the flythrough camera state from the current orbit state.
//
// Key insight: orbit rotates points by orientation.ToMatrix(), while
// viewMatrix() returns orientation.Conjugate().ToMatrix(). So the flythrough
// orientation must be the conjugate of the orbit orientation to produce the
// same rotation matrix — and thus the same view.
//
// With the correct orientation, placing the camera at
// flyOrientation.RotateVec3({0,0,4.0}) produces tx=0, ty=0, tz=4.0 in the
// view matrix, exactly matching orbit's implicit camera. Zoom (c.zoom) stays
// unchanged since both modes use it identically.
func (f *flythroughCamera) fromOrbit(orientation Quat, zoom float64, panX, panY float64) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.orientation = orientation.Conjugate()

	// Place camera so viewMatrix returns tx=0, ty=0, tz=4.0.
	f.pos = f.orientation.RotateVec3([3]float64{0, 0, 4.0})

	// Bake orbit pan into camera position as a lateral offset.
	// Pan is in DIP; at center depth the world offset is pan * tz / zoom.
	if (panX != 0 || panY != 0) && zoom > 0 {
		right := f.orientation.RotateVec3([3]float64{1, 0, 0})
		up := f.orientation.RotateVec3([3]float64{0, 1, 0})
		wx := -panX * 4.0 / zoom
		wy := -panY * 4.0 / zoom
		f.pos[0] += right[0]*wx + up[0]*wy
		f.pos[1] += right[1]*wx + up[1]*wy
		f.pos[2] += right[2]*wx + up[2]*wy
	}

	f.speedMultiple = 1.0
}

// toOrbit extracts orbit parameters from the flythrough state.
// currentZoom is the unchanged c.zoom value.
// Returns orbit orientation and adjusted zoom. Pan is set to zero since
// any lateral offset is encoded in the camera position.
func (f *flythroughCamera) toOrbit(currentZoom float64) (Quat, float64) {
	f.mu.Lock()
	defer f.mu.Unlock()

	// Orbit orientation is the conjugate of flythrough orientation.
	orient := f.orientation.Conjugate()

	// Compute tz from viewMatrix to find how far the camera is from
	// the origin along the view axis. Orbit always uses tz=4.0, so
	// we scale zoom to compensate: orbit_zoom = currentZoom * 4 / tz.
	m := orient.ToMatrix()
	tz := m[6]*f.pos[0] + m[7]*f.pos[1] + m[8]*f.pos[2]
	if tz < 0.01 {
		tz = 4.0
	}
	zoom := currentZoom * 4.0 / tz

	return orient, zoom
}
