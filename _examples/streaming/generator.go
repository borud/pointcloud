package main

import (
	"math"
	"math/rand/v2"

	"github.com/borud/pointcloud"
)

// LidarGenerator simulates a LIDAR scanner at the origin that sweeps a
// beam continuously around 360°. Two objects float in space — a sphere
// and a box. Each Generate call advances the sweep and returns points
// only where the beam hits an object surface. Combined with the buffer's
// time-based eviction this produces the effect of one object fading as
// the beam sweeps past it toward the other.
type LidarGenerator struct {
	azimuth float64 // current horizontal angle (radians), increases forever
}

// NewLidarGenerator creates a new LIDAR sweep generator.
func NewLidarGenerator() *LidarGenerator {
	return &LidarGenerator{}
}

// Scene objects — positioned so the sweep spends time on each, then
// crosses empty space where nothing is hit and old points decay.
var (
	// Sphere: center and radius.
	sphereCenter = [3]float64{0.45, 0.1, 0.0}
	sphereRadius = 0.25

	// Box: center, half-extents.
	boxCenter = [3]float64{-0.35, -0.05, -0.25}
	boxHalf   = [3]float64{0.15, 0.2, 0.15}
)

const (
	beamsPerColumn = 32    // vertical resolution
	elevMin        = -0.6  // vertical sweep range (radians)
	elevMax        = 0.6
	azStep         = 0.008 // azimuth advance per column — slow sweep
	maxRange       = 1.5
	noiseAmount    = 0.004
)

// Generate produces up to n points for the next sweep sector.
func (g *LidarGenerator) Generate(n int) []pointcloud.Point3D {
	pts := make([]pointcloud.Point3D, 0, n)

	for len(pts) < n {
		az := g.azimuth
		g.azimuth += azStep

		cosAz := math.Cos(az)
		sinAz := math.Sin(az)

		for beam := range beamsPerColumn {
			if len(pts) >= n {
				break
			}

			elev := elevMin + (elevMax-elevMin)*float64(beam)/float64(beamsPerColumn-1)
			cosEl := math.Cos(elev)
			sinEl := math.Sin(elev)

			// Ray from origin.
			dx := cosEl * cosAz
			dy := sinEl
			dz := cosEl * sinAz

			hit, dist := raycast(dx, dy, dz)
			if !hit {
				continue
			}

			hx := dx*dist + (rand.Float64()-0.5)*noiseAmount
			hy := dy*dist + (rand.Float64()-0.5)*noiseAmount
			hz := dz*dist + (rand.Float64()-0.5)*noiseAmount

			r, gr, b := objectColor(hx, hy, hz)
			pts = append(pts, pointcloud.Point3D{
				X: hx, Y: hy, Z: hz,
				R: r, G: gr, B: b,
				HasColor: true,
			})
		}
	}
	return pts
}

// raycast tests a ray from the origin against the sphere and box,
// returning whether it hit and the nearest distance.
func raycast(dx, dy, dz float64) (hit bool, dist float64) {
	dist = maxRange + 1

	if t, ok := raySphere(dx, dy, dz); ok && t < dist {
		dist = t
		hit = true
	}
	if t, ok := rayBox(dx, dy, dz); ok && t < dist {
		dist = t
		hit = true
	}
	return
}

// raySphere intersects a ray from origin with the sphere.
func raySphere(dx, dy, dz float64) (float64, bool) {
	// Ray: P = t*(dx,dy,dz), Sphere: |P - C|^2 = r^2
	ox := -sphereCenter[0]
	oy := -sphereCenter[1]
	oz := -sphereCenter[2]

	a := dx*dx + dy*dy + dz*dz
	b := 2 * (ox*dx + oy*dy + oz*dz)
	c := ox*ox + oy*oy + oz*oz - sphereRadius*sphereRadius

	disc := b*b - 4*a*c
	if disc < 0 {
		return 0, false
	}

	sqrtDisc := math.Sqrt(disc)
	t1 := (-b - sqrtDisc) / (2 * a)
	t2 := (-b + sqrtDisc) / (2 * a)

	if t1 > 0.01 {
		return t1, true
	}
	if t2 > 0.01 {
		return t2, true
	}
	return 0, false
}

// rayBox intersects a ray from origin with an axis-aligned box using slab method.
func rayBox(dx, dy, dz float64) (float64, bool) {
	dir := [3]float64{dx, dy, dz}
	bmin := [3]float64{
		boxCenter[0] - boxHalf[0],
		boxCenter[1] - boxHalf[1],
		boxCenter[2] - boxHalf[2],
	}
	bmax := [3]float64{
		boxCenter[0] + boxHalf[0],
		boxCenter[1] + boxHalf[1],
		boxCenter[2] + boxHalf[2],
	}

	tmin := 0.0
	tmax := maxRange

	for i := range 3 {
		if math.Abs(dir[i]) < 1e-12 {
			// Ray parallel to slab — miss if origin outside.
			if 0 < bmin[i] || 0 > bmax[i] {
				return 0, false
			}
			continue
		}
		invD := 1.0 / dir[i]
		t1 := bmin[i] * invD
		t2 := bmax[i] * invD
		if invD < 0 {
			t1, t2 = t2, t1
		}
		tmin = math.Max(tmin, t1)
		tmax = math.Min(tmax, t2)
		if tmin > tmax {
			return 0, false
		}
	}

	if tmin > 0.01 {
		return tmin, true
	}
	return 0, false
}

// objectColor returns a color based on which object was hit.
// Sphere gets a warm orange/yellow gradient, box gets a cool blue/cyan.
func objectColor(x, y, z float64) (r, g, b uint8) {
	// Check if point is closer to sphere or box.
	sdx := x - sphereCenter[0]
	sdy := y - sphereCenter[1]
	sdz := z - sphereCenter[2]
	sphereDist := math.Sqrt(sdx*sdx + sdy*sdy + sdz*sdz)

	if sphereDist < sphereRadius+0.05 {
		// Sphere: warm gradient based on surface normal (latitude).
		ny := sdy / sphereRadius
		t := (ny + 1) / 2 // 0 at bottom, 1 at top
		return uint8(200 + 55*t), uint8(120 + 100*t), uint8(30 + 40*t)
	}

	// Box: cool gradient based on height.
	t := (y - (boxCenter[1] - boxHalf[1])) / (2 * boxHalf[1])
	t = clamp(t, 0, 1)
	return uint8(40 + 60*t), uint8(140 + 80*t), uint8(200 + 55*t)
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
