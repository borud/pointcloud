// Package main generates a synthetic seabed point cloud and displays it.
//
// The terrain is built from layered Perlin-style noise to simulate
// rolling hills, ridges, a trench, scattered rocks, and sandy ripples.
// Points are colored by depth using a bathymetric palette.
package main

import (
	"fmt"
	"image/color"
	"math"
	"math/rand/v2"
	"sync"
	"sync/atomic"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	"github.com/borud/pointcloud"
)

const (
	noiseScale = 0.003 // measurement noise amplitude
	worldHalf  = 9.0   // half-extent of the XY world

	defaultHeight = 0.05 // default height scale — fairly flat
	defaultPoints = 160000
	minPoints     = 100000
	maxPoints     = 50000000
)

// minWidthLayout enforces a minimum width on its single child.
type minWidthLayout struct {
	minWidth float32
}

func newMinWidthLayout(w float32) *minWidthLayout { return &minWidthLayout{minWidth: w} }

func (l *minWidthLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	if len(objects) == 0 {
		return fyne.NewSize(l.minWidth, 0)
	}
	return fyne.NewSize(l.minWidth, objects[0].MinSize().Height)
}

func (l *minWidthLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	for _, o := range objects {
		o.Resize(size)
		o.Move(fyne.NewPos(0, 0))
	}
}

// seabedState holds the raw terrain heights so the height slider can
// rescale Z without regenerating the terrain.
type seabedState struct {
	mu       sync.Mutex
	gridSize int
	heights  []float64 // raw heights, len == gridSize*gridSize
	noiseZ   []float64 // per-point noise, len == gridSize*gridSize
	maxAbsZ  float64   // max |height| at generation time
}

// buildPoints maps raw heights to viewer coordinates. The heightScale
// controls how much of the [-0.9, 0.9] Z range the terrain actually
// occupies — low values produce a flat surface, 1.0 fills the range.
func (s *seabedState) buildPoints(heightScale float64) []pointcloud.Point3D {
	s.mu.Lock()
	defer s.mu.Unlock()

	n := len(s.heights)
	if n == 0 {
		return nil
	}
	gs := s.gridSize

	// Scale Z so that at heightScale=1.0 the full range maps to [-0.9, 0.9].
	// At lower values, the terrain occupies proportionally less Z range.
	divisor := s.maxAbsZ
	if divisor < 1e-12 {
		divisor = 1
	}

	pts := make([]pointcloud.Point3D, n)
	for iy := range gs {
		for ix := range gs {
			x := float64(ix)/float64(gs-1)*1.8 - 0.9
			y := float64(iy)/float64(gs-1)*1.8 - 0.9

			idx := iy*gs + ix
			// Map raw height to [-0.9, 0.9] scaled by heightScale.
			zNorm := (s.heights[idx] / divisor) * 0.9 * heightScale
			zNorm += s.noiseZ[idx]

			r, g, b := bathyColor(zNorm)
			pts[idx] = pointcloud.Point3D{
				X: x, Y: y, Z: clamp(zNorm, -0.9, 0.9),
				R: r, G: g, B: b,
				HasColor: true,
			}
		}
	}
	return pts
}

func main() {
	myApp := app.NewWithID("no.borud.pointcloud.seabed")
	myWindow := myApp.NewWindow("Seabed Point Cloud")

	viewer := pointcloud.New(
		pointcloud.WithBackgroundColor(color.RGBA{5, 10, 30, 255}),
		pointcloud.WithOrientationCube(true),
		pointcloud.WithFPS(true),
		pointcloud.WithMaxZoomOutFraction(0.25),
	)
	viewer.SetUpAxis(pointcloud.ZUp)

	statusLabel := widget.NewLabel("Generating seabed...")
	statusLabel.TextStyle = fyne.TextStyle{Monospace: true}

	var state seabedState
	var generating atomic.Bool
	heightScale := defaultHeight
	numPoints := defaultPoints

	// --- Height slider: 0.01 – 1.0 ---
	heightLabel := widget.NewLabel(fmt.Sprintf("Height: %.2f", heightScale))
	heightLabel.TextStyle = fyne.TextStyle{Monospace: true}
	heightSlider := widget.NewSlider(0.01, 1.0)
	heightSlider.Step = 0.01
	heightSlider.Value = heightScale
	heightSlider.OnChanged = func(val float64) {
		heightScale = val
		heightLabel.SetText(fmt.Sprintf("Height: %.2f", val))
		pts := state.buildPoints(heightScale)
		if pts != nil {
			viewer.SetPointsPreserveView(pts)
		}
	}

	// --- Points slider: 100k – 50M (logarithmic) ---
	logMin := math.Log10(float64(minPoints))
	logMax := math.Log10(float64(maxPoints))

	pointsLabel := widget.NewLabel(fmt.Sprintf("Points: %s", formatCount(numPoints)))
	pointsLabel.TextStyle = fyne.TextStyle{Monospace: true}
	pointsSlider := widget.NewSlider(logMin, logMax)
	pointsSlider.Step = 0.01
	pointsSlider.Value = math.Log10(float64(numPoints))
	pointsSlider.OnChanged = func(val float64) {
		n := int(math.Round(math.Pow(10, val)))
		numPoints = n
		pointsLabel.SetText(fmt.Sprintf("Points: %s", formatCount(n)))
	}

	generate := func() {
		if !generating.CompareAndSwap(false, true) {
			return
		}
		n := numPoints
		fyne.Do(func() {
			statusLabel.SetText(fmt.Sprintf("Generating %s points...", formatCount(n)))
		})
		go func() {
			defer generating.Store(false)
			generateTerrain(&state, n)
			pts := state.buildPoints(heightScale)
			viewer.SetPoints(pts)
			fyne.Do(func() {
				statusLabel.SetText(fmt.Sprintf("Seabed — %s points", formatCount(len(pts))))
			})
		}()
	}

	regenBtn := widget.NewButton("Regenerate", func() { generate() })

	fpsCheck := widget.NewCheck("FPS", func(on bool) {
		viewer.SetFPSEnabled(on)
	})
	fpsCheck.SetChecked(true)

	heightSized := container.New(newMinWidthLayout(250), heightSlider)
	pointsSized := container.New(newMinWidthLayout(250), pointsSlider)

	bottomBar := container.NewBorder(
		nil, nil,
		statusLabel,
		container.NewHBox(
			fpsCheck,
			heightLabel, heightSized,
			pointsLabel, pointsSized,
			regenBtn,
		),
	)

	content := container.NewBorder(
		nil,
		container.New(layout.NewCustomPaddedLayout(4, 4, 8, 8), bottomBar),
		nil, nil,
		viewer,
	)

	myWindow.SetContent(content)
	myWindow.Resize(fyne.NewSize(1100, 750))

	generate()

	myWindow.ShowAndRun()
}

// formatCount formats a number with K/M suffixes.
func formatCount(n int) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	case n >= 1_000:
		return fmt.Sprintf("%.0fK", float64(n)/1_000)
	default:
		return fmt.Sprintf("%d", n)
	}
}

// generateTerrain fills state with raw (unscaled) heights and noise.
func generateTerrain(state *seabedState, numPoints int) {
	gs := int(math.Ceil(math.Sqrt(float64(numPoints))))
	if gs < 2 {
		gs = 2
	}
	total := gs * gs

	seed := rand.Uint64()
	rng := rand.New(rand.NewPCG(seed, seed^0xdeadbeef))

	type octave struct {
		freqX, freqY, phase, amp float64
	}
	octaves := []octave{
		{1.0, 1.0, rng.Float64() * 2 * math.Pi, 0.30},
		{2.3, 1.8, rng.Float64() * 2 * math.Pi, 0.15},
		{4.7, 5.1, rng.Float64() * 2 * math.Pi, 0.07},
		{9.3, 8.7, rng.Float64() * 2 * math.Pi, 0.03},
		{18.0, 20.0, rng.Float64() * 2 * math.Pi, 0.015},
	}

	type rock struct {
		cx, cy, radius, height float64
	}
	numRocks := 15 + rng.IntN(20)
	rocks := make([]rock, numRocks)
	for i := range rocks {
		rocks[i] = rock{
			cx:     rng.Float64()*16 - 8,
			cy:     rng.Float64()*16 - 8,
			radius: 0.2 + rng.Float64()*0.6,
			height: 0.03 + rng.Float64()*0.08,
		}
	}

	trenchPhase := rng.Float64() * 2 * math.Pi
	trenchDir := rng.Float64()*0.6 - 0.3

	heights := make([]float64, total)
	noiseZ := make([]float64, total)
	maxAbsZ := 0.0

	for iy := range gs {
		for ix := range gs {
			x := float64(ix)/float64(gs-1)*2*worldHalf - worldHalf
			y := float64(iy)/float64(gs-1)*2*worldHalf - worldHalf

			z := 0.0
			for _, o := range octaves {
				z += o.amp * math.Sin(o.freqX*x*math.Pi/worldHalf+o.phase) *
					math.Cos(o.freqY*y*math.Pi/worldHalf+o.phase*1.3)
			}

			z += 0.05 * math.Sin(3.1*x*math.Pi/worldHalf+1.7*y*math.Pi/worldHalf+octaves[0].phase)

			trenchCenter := 1.5*math.Sin(2.0*y*math.Pi/(2*worldHalf)+trenchPhase) + trenchDir*y
			trenchDist := math.Abs(x - trenchCenter)
			trenchWidth := 1.2
			if trenchDist < trenchWidth {
				depth := 0.20 * (1.0 - (trenchDist/trenchWidth)*(trenchDist/trenchWidth))
				z -= depth
			}

			for _, r := range rocks {
				dx := x - r.cx
				dy := y - r.cy
				d2 := (dx*dx + dy*dy) / (r.radius * r.radius)
				if d2 < 9 {
					z += r.height * math.Exp(-d2/2)
				}
			}

			idx := iy*gs + ix
			heights[idx] = z
			noiseZ[idx] = (rng.Float64() - 0.5) * noiseScale * 2

			if abs := math.Abs(z); abs > maxAbsZ {
				maxAbsZ = abs
			}
		}
	}

	state.mu.Lock()
	state.gridSize = gs
	state.heights = heights
	state.noiseZ = noiseZ
	state.maxAbsZ = maxAbsZ
	state.mu.Unlock()
}

// bathyColor maps a normalized depth z in [-0.9, 0.9] to a bathymetric
// color scheme: deep blue -> medium blue -> teal -> sandy tan -> light sand.
func bathyColor(z float64) (r, g, b uint8) {
	t := (z + 0.9) / 1.8
	t = clamp(t, 0, 1)

	switch {
	case t < 0.2:
		f := t / 0.2
		return uint8(15 + 20*f), uint8(10 + 30*f), uint8(60 + 60*f)
	case t < 0.4:
		f := (t - 0.2) / 0.2
		return uint8(35 + 10*f), uint8(40 + 50*f), uint8(120 + 40*f)
	case t < 0.6:
		f := (t - 0.4) / 0.2
		return uint8(45 + 30*f), uint8(90 + 60*f), uint8(160 - 20*f)
	case t < 0.8:
		f := (t - 0.6) / 0.2
		return uint8(75 + 80*f), uint8(150 + 40*f), uint8(140 - 50*f)
	default:
		f := (t - 0.8) / 0.2
		return uint8(155 + 60*f), uint8(190 + 30*f), uint8(90 + 40*f)
	}
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
