package pointcloud

import (
	"bytes"
	"fmt"
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

func BenchmarkBuildGrid_1M(b *testing.B) {
	pts := generatePoints(1_000_000)
	c := &canvas3d{points: pts}
	c.xs = make([]float32, len(pts))
	c.ys = make([]float32, len(pts))
	c.zs = make([]float32, len(pts))
	c.rgba = make([]uint32, len(pts))
	c.originalIndex = make([]int, len(pts))
	for i, p := range pts {
		c.xs[i] = float32(p.X)
		c.ys[i] = float32(p.Y)
		c.zs[i] = float32(p.Z)
		c.originalIndex[i] = i
		if p.HasColor {
			c.rgba[i] = hasColorBit | uint32(p.R)<<16 | uint32(p.G)<<8 | uint32(p.B)
		}
	}

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		_, _, _, _, _, _ = buildGrid(c.xs, c.ys, c.zs, c.rgba, c.originalIndex)
	}
}

func BenchmarkFrustumCulling_1M(b *testing.B) {
	pts := generatePoints(1_000_000)
	c := setupCanvas(pts)
	vm := QuatIdentity().ToMatrix()
	planes := extractFrustumPlanes(vm, 0, 0, 4.0, 200, 1024.0/768.0)

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		_ = c.grid.visibleCells(planes)
	}
}

func BenchmarkReadXYZ_100k(b *testing.B) {
	var buf bytes.Buffer
	for _, p := range generatePoints(100_000) {
		if p.HasColor {
			fmt.Fprintf(&buf, "%f %f %f %d %d %d\n", p.X, p.Y, p.Z, p.R, p.G, p.B)
		} else {
			fmt.Fprintf(&buf, "%f %f %f\n", p.X, p.Y, p.Z)
		}
	}
	data := buf.Bytes()

	b.ReportAllocs()
	b.SetBytes(int64(len(data)))
	b.ResetTimer()
	for range b.N {
		if _, err := ReadXYZ(bytes.NewReader(data)); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkReadPTS_100k(b *testing.B) {
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "%d\n", 100_000)
	for _, p := range generatePoints(100_000) {
		if p.HasColor {
			fmt.Fprintf(&buf, "%f %f %f 1 %d %d %d\n", p.X, p.Y, p.Z, p.R, p.G, p.B)
		} else {
			fmt.Fprintf(&buf, "%f %f %f 1\n", p.X, p.Y, p.Z)
		}
	}
	data := buf.Bytes()

	b.ReportAllocs()
	b.SetBytes(int64(len(data)))
	b.ResetTimer()
	for range b.N {
		if _, err := ReadPTS(bytes.NewReader(data)); err != nil {
			b.Fatal(err)
		}
	}
}
