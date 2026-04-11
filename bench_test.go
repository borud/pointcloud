package pointcloud

import (
	"image"
	"image/color"
	"math/rand/v2"
	"testing"
)

// generatePoints creates a deterministic synthetic point cloud with a mix
// of colored and uncolored points (50/50).
func generatePoints(n int) []Point3D {
	rng := rand.New(rand.NewPCG(42, 0))

	pts := make([]Point3D, n)
	for i := range pts {
		pts[i] = Point3D{
			X: rng.Float64()*2 - 1,
			Y: rng.Float64()*2 - 1,
			Z: rng.Float64()*2 - 1,
		}
		if i%2 == 0 {
			pts[i].HasColor = true
			pts[i].R = uint8(rng.IntN(256))
			pts[i].G = uint8(rng.IntN(256))
			pts[i].B = uint8(rng.IntN(256))
		}
	}
	return pts
}

// testOrientation returns a fixed orientation that exercises all 9 matrix
// elements (not axis-aligned, not home).
var testOrientation = QuatFromEulerXY(0.7, 1.2)

func setupCanvas(pts []Point3D) *canvas3d {
	c := &canvas3d{
		orientation: testOrientation,
		matrixDirty: true,
		zoom:        200.0,
		bgColor:     color.RGBA{30, 30, 30, 255},
	}
	c.points = pts
	c.convertToSoA()
	return c
}

func benchDraw(b *testing.B, nPoints, w, h int) {
	b.Helper()
	pts := generatePoints(nPoints)
	c := setupCanvas(pts)

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		c.draw(w, h)
	}
}

func BenchmarkDraw_100k(b *testing.B) { benchDraw(b, 100_000, 1024, 768) }
func BenchmarkDraw_500k(b *testing.B) { benchDraw(b, 500_000, 1024, 768) }
func BenchmarkDraw_1M(b *testing.B)   { benchDraw(b, 1_000_000, 1024, 768) }
func BenchmarkDraw_5M(b *testing.B)   { benchDraw(b, 5_000_000, 1920, 1080) }

// BenchmarkClear_1080p measures framebuffer clear via copy() from a template.
func BenchmarkClear_1080p(b *testing.B) {
	w, h := 1920, 1080
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	bg := color.RGBA{30, 30, 30, 255}

	// Build the template once (same as the optimized draw path).
	tmpl := make([]byte, len(img.Pix))
	for i := 0; i < len(tmpl); i += 4 {
		tmpl[i] = bg.R
		tmpl[i+1] = bg.G
		tmpl[i+2] = bg.B
		tmpl[i+3] = bg.A
	}

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		copy(img.Pix, tmpl)
	}
}

// BenchmarkDraw_Flythrough_1M measures draw with flythrough camera inside cloud.
func BenchmarkDraw_Flythrough_1M(b *testing.B) {
	pts := generatePoints(1_000_000)
	c := setupCanvas(pts)
	c.flyMode = true
	c.fly = newFlythroughCamera(c)
	c.fly.pos = [3]float64{0, 0, 0} // camera at center of cloud
	c.fly.orientation = QuatIdentity()
	w, h := 1024, 768

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		c.draw(w, h)
	}
}

// BenchmarkProjection_1M measures only the projection math (no pixel writes).
func BenchmarkProjection_1M(b *testing.B) {
	pts := generatePoints(1_000_000)
	c := setupCanvas(pts)
	w, h := 1024, 768

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		m := c.orientation.ToMatrix()
		zoom := c.zoom
		centerX, centerY := float64(w)/2, float64(h)/2

		for _, p := range c.points {
			rx := m[0]*p.X + m[1]*p.Y + m[2]*p.Z
			ry := m[3]*p.X + m[4]*p.Y + m[5]*p.Z
			rz2 := m[6]*p.X + m[7]*p.Y + m[8]*p.Z

			dist := 4.0 - rz2
			if dist < 0.1 {
				continue
			}
			_ = (rx/dist)*zoom + centerX
			_ = (ry/dist)*zoom + centerY
		}
	}
}
