package pointcloud

import (
	"image"
	"image/color"
	"math"
	"runtime"
	"sync"
	"time"
	"unsafe"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
)

// Pixel packing note: image.RGBA stores bytes as R,G,B,A in memory order.
// On little-endian systems (all platforms Go currently targets: amd64, arm64,
// etc.) the uint32 layout is A<<24 | B<<16 | G<<8 | R. The projectChunk
// inner loop uses this layout for single-store pixel writes via unsafe.

// hasColorBit is the high bit in a packed RGBA uint32, indicating the point
// has color data. The lower 24 bits store R (23:16), G (15:8), B (7:0).
const hasColorBit = 1 << 31

// canvas3d is the internal 3D point cloud renderer with arcball rotation,
// pan, and zoom. It is not exported; the public Viewer widget wraps it.
type canvas3d struct {
	widget.BaseWidget
	raster *canvas.Raster

	mu                 sync.Mutex
	orientation        Quat
	cachedMatrix       [9]float64
	matrixDirty        bool
	zoom               float64
	panX               float64
	panY               float64
	up                 UpAxis
	bgColor            color.RGBA
	defaultPointColor  color.RGBA
	homeOrientation    Quat
	maxZoomOutFraction float64

	// Flythrough mode.
	flyMode             bool
	fly                 *flythroughCamera
	grid                *spatialGrid
	onFlythroughChanged func(bool)

	// Original points kept for Tapped to return the original Point3D.
	points []Point3D
	// originalIndex maps the reordered SoA entry back to points[].
	originalIndex []int

	// SoA (Structure-of-Arrays) storage in float32 for the hot rendering
	// loop. The Z-up swap is applied at conversion time so the inner loop
	// never branches on the up axis.
	xs, ys, zs []float32
	rgba       []uint32 // packed: hasColorBit | R<<16 | G<<8 | B

	// Decimated SoA arrays for LOD during interaction. Built in convertToSoA
	// when the point count exceeds lodTargetSize.
	xsLOD, ysLOD, zsLOD []float32
	rgbaLOD             []uint32
	originalIndexLOD    []int

	// LOD configuration.
	lodEnabled    bool // whether LOD decimation is active during interaction
	lodTargetSize int  // target number of points in the decimated set

	// dragging is true during mouse interaction. When true, the draw loop
	// uses the LOD arrays for interactive frame rates.
	dragging  bool
	idleTimer *time.Timer

	dragModifier fyne.KeyModifier

	// Reusable framebuffer to avoid per-frame allocation.
	framebuffer *image.RGBA

	// Pre-filled template for fast framebuffer clear via copy().
	clearTemplate []byte

	// Last rendered pixel dimensions (set by draw, read by Tapped).
	lastPixW, lastPixH int

	onOrientationChanged func()
	onHomeView           func()
	onZoomChanged        func()
	onSpeedChanged       func(multiple float64)
	onPointTapped        func(p Point3D, screenX, screenY float64)
	onPointCleared       func()
	onFrameDrawn         func(d time.Duration) // called at end of draw with render time
}

func newCanvas3D(cfg *config) *canvas3d {
	homeOr := quatOr(cfg.homeOrientation, HomeOrientation)
	c := &canvas3d{
		orientation:        homeOr,
		matrixDirty:        true,
		zoom:               float64Or(cfg.initialZoom, 200.0),
		bgColor:            colorOr(cfg.bgColor, color.RGBA{0, 0, 0, 255}),
		defaultPointColor:  colorOr(cfg.defaultPointColor, color.RGBA{255, 150, 255, 255}),
		homeOrientation:    homeOr,
		maxZoomOutFraction: float64Or(cfg.maxZoomOutFraction, 0.2),
		lodEnabled:         true,
		lodTargetSize:      200_000,
	}
	c.raster = canvas.NewRaster(c.draw)
	c.ExtendBaseWidget(c)
	return c
}

// minZoom returns the minimum zoom level that keeps the point cloud visible
// as at least maxZoomOutFraction of the largest canvas dimension. The
// calculation is rotation-invariant because it uses the fixed normalized
// bounding sphere radius (1.0) at center depth.
func (c *canvas3d) minZoom() float64 {
	size := c.Size()
	maxDim := float64(max(size.Width, size.Height))
	if maxDim < 1 {
		return 1.0
	}
	return 2.0 * c.maxZoomOutFraction * maxDim
}

func (c *canvas3d) setPoints(pts []Point3D) {
	c.mu.Lock()
	c.points = pts
	c.convertToSoA()
	c.mu.Unlock()
	c.zoomToExtents()
}

// convertToSoA converts the AoS points into SoA float32 arrays. The Z-up
// swap is applied here so the inner loop never branches on the up axis.
// Also builds a decimated LOD copy if LOD is enabled and the cloud is large
// enough. Must be called with c.mu held.
func (c *canvas3d) convertToSoA() {
	n := len(c.points)

	c.xs = make([]float32, n)
	c.ys = make([]float32, n)
	c.zs = make([]float32, n)
	c.rgba = make([]uint32, n)
	c.originalIndex = make([]int, n)

	zup := c.up == ZUp
	for i, p := range c.points {
		c.originalIndex[i] = i
		if zup {
			c.xs[i] = float32(p.X)
			c.ys[i] = float32(p.Z)
			c.zs[i] = float32(-p.Y)
		} else {
			c.xs[i] = float32(p.X)
			c.ys[i] = float32(p.Y)
			c.zs[i] = float32(p.Z)
		}

		if p.HasColor {
			c.rgba[i] = hasColorBit | uint32(p.R)<<16 | uint32(p.G)<<8 | uint32(p.B)
		} else {
			c.rgba[i] = 0
		}
	}

	// Build spatial grid for flythrough frustum culling.
	c.grid, c.xs, c.ys, c.zs, c.rgba, c.originalIndex =
		buildGrid(c.xs, c.ys, c.zs, c.rgba, c.originalIndex)

	c.buildLOD()
}

// buildLOD builds the decimated LOD arrays from the current full SoA arrays.
// Must be called with c.mu held.
func (c *canvas3d) buildLOD() {
	n := len(c.xs)
	target := c.lodTargetSize

	if !c.lodEnabled || target <= 0 || n <= target {
		c.xsLOD = nil
		c.ysLOD = nil
		c.zsLOD = nil
		c.rgbaLOD = nil
		c.originalIndexLOD = nil
		return
	}

	// Take every Nth point to get approximately target points.
	step := n / target
	if step < 2 {
		step = 2
	}
	lodN := (n + step - 1) / step
	c.xsLOD = make([]float32, lodN)
	c.ysLOD = make([]float32, lodN)
	c.zsLOD = make([]float32, lodN)
	c.rgbaLOD = make([]uint32, lodN)
	c.originalIndexLOD = make([]int, lodN)
	j := 0
	for i := 0; i < n; i += step {
		c.xsLOD[j] = c.xs[i]
		c.ysLOD[j] = c.ys[i]
		c.zsLOD[j] = c.zs[i]
		c.rgbaLOD[j] = c.rgba[i]
		c.originalIndexLOD[j] = c.originalIndex[i]
		j++
	}
	c.xsLOD = c.xsLOD[:j]
	c.ysLOD = c.ysLOD[:j]
	c.zsLOD = c.zsLOD[:j]
	c.rgbaLOD = c.rgbaLOD[:j]
	c.originalIndexLOD = c.originalIndexLOD[:j]
}

// startInteraction marks the canvas as actively interacting, switching to
// LOD rendering. An idle timer restores full detail after 100ms of inactivity.
func (c *canvas3d) startInteraction() {
	c.mu.Lock()
	// Nothing to do if there are no LOD arrays to switch to.
	if c.xsLOD == nil {
		c.mu.Unlock()
		return
	}
	c.dragging = true
	if c.idleTimer != nil {
		c.idleTimer.Stop()
	}
	c.idleTimer = time.AfterFunc(100*time.Millisecond, func() {
		c.mu.Lock()
		c.dragging = false
		c.mu.Unlock()
		// Refresh must happen on the Fyne main thread since AfterFunc
		// fires on an arbitrary goroutine.
		fyne.Do(func() {
			c.raster.Refresh()
		})
	})
	c.mu.Unlock()
}

func (c *canvas3d) zoomToExtents() {
	if c.flyMode && c.fly != nil {
		// In flythrough mode, move camera to see the full cloud.
		// Place camera at (0, 0, 4) looking toward origin — the same
		// implicit position as orbit mode's default.
		c.fly.mu.Lock()
		c.fly.pos = c.fly.orientation.RotateVec3([3]float64{0, 0, 4.0})
		c.fly.mu.Unlock()
		fyne.Do(func() { c.raster.Refresh() })
		return
	}

	size := c.Size()
	w, h := float64(size.Width), float64(size.Height)
	if w < 1 || h < 1 {
		c.mu.Lock()
		c.zoom = 200
		c.panX = 0
		c.panY = 0
		c.mu.Unlock()
		fyne.Do(func() { c.raster.Refresh() })
		c.fireZoomChanged()
		return
	}
	c.mu.Lock()
	c.zoom = math.Min(w, h) * 0.8 * 2
	c.panX = 0
	c.panY = 0
	c.mu.Unlock()
	fyne.Do(func() { c.raster.Refresh() })
	c.fireZoomChanged()
}

func (c *canvas3d) fireZoomChanged() {
	if c.onZoomChanged != nil {
		c.onZoomChanged()
	}
}

func (c *canvas3d) homeView() {
	if c.flyMode && c.fly != nil {
		// In flythrough mode, reset orientation and move camera to
		// the home view position without leaving flythrough.
		c.fly.mu.Lock()
		c.fly.orientation = c.homeOrientation
		c.fly.pos = c.homeOrientation.RotateVec3([3]float64{0, 0, 4.0})
		c.fly.mu.Unlock()
		fyne.Do(func() { c.raster.Refresh() })
		return
	}

	c.mu.Lock()
	c.orientation = c.homeOrientation
	c.matrixDirty = true
	c.mu.Unlock()
	c.zoomToExtents()
}

// CreateRenderer implements fyne.Widget.
func (c *canvas3d) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(c.raster)
}

func (c *canvas3d) draw(w, h int) image.Image {
	drawStart := time.Now()
	c.lastPixW, c.lastPixH = w, h

	// Reuse the framebuffer across frames; only reallocate on resize.
	if c.framebuffer == nil ||
		c.framebuffer.Rect.Dx() != w ||
		c.framebuffer.Rect.Dy() != h {
		c.framebuffer = image.NewRGBA(image.Rect(0, 0, w, h))
		c.clearTemplate = nil
	}
	img := c.framebuffer

	c.mu.Lock()
	if c.matrixDirty {
		c.cachedMatrix = c.orientation.ToMatrix()
		c.matrixDirty = false
	}
	m64 := c.cachedMatrix
	zoom := c.zoom
	panX, panY := c.panX, c.panY
	bg := c.bgColor

	// Use LOD arrays during interaction for responsive frame rates.
	// Use LOD arrays during interaction for responsive frame rates.
	// In flythrough mode, skip LOD — the grid-reordered full arrays
	// are needed and frustum culling provides the perf benefit instead.
	xs := c.xs
	ys := c.ys
	zs := c.zs
	rgba := c.rgba
	if c.dragging && c.xsLOD != nil && !c.flyMode {
		xs = c.xsLOD
		ys = c.ysLOD
		zs = c.zsLOD
		rgba = c.rgbaLOD
	}
	defR := float32(c.defaultPointColor.R)
	defG := float32(c.defaultPointColor.G)
	defB := float32(c.defaultPointColor.B)
	c.mu.Unlock()

	// Rebuild the clear template when the framebuffer size or bg color changes.
	if c.clearTemplate == nil || len(c.clearTemplate) != len(img.Pix) {
		c.clearTemplate = make([]byte, len(img.Pix))
		for i := 0; i < len(c.clearTemplate); i += 4 {
			c.clearTemplate[i] = bg.R
			c.clearTemplate[i+1] = bg.G
			c.clearTemplate[i+2] = bg.B
			c.clearTemplate[i+3] = bg.A
		}
	}
	copy(img.Pix, c.clearTemplate)

	// Convert projection parameters to float32 for the inner loop.
	m0, m1, m2 := float32(m64[0]), float32(m64[1]), float32(m64[2])
	m3, m4, m5 := float32(m64[3]), float32(m64[4]), float32(m64[5])
	m6, m7, m8 := float32(m64[6]), float32(m64[7]), float32(m64[8])
	zoomF := float32(zoom)

	// panX/panY are in Fyne DIP; scale to pixel space.
	size := c.Size()
	scaleX, scaleY := float32(1.0), float32(1.0)
	if size.Width > 0 {
		scaleX = float32(w) / float32(size.Width)
	}
	if size.Height > 0 {
		scaleY = float32(h) / float32(size.Height)
	}
	centerX := float32(w)/2 + float32(panX)*scaleX
	centerY := float32(h)/2 + float32(panY)*scaleY

	stride := img.Stride
	pix := img.Pix

	// Parallelize by partitioning points among workers. Each worker projects
	// and writes its chunk of points directly to the shared framebuffer.
	// Pixel races (two points mapping to the same pixel) are benign — the
	// last writer wins, which is visually irrelevant for point cloud rendering.
	// This means -race will flag writes to pix[], but the race is harmless:
	// each write is a 1-byte store and the "wrong" value is still a valid color.
	nWorkers := runtime.GOMAXPROCS(0)
	n := len(xs)
	if n < 50_000 {
		nWorkers = 1
	}

	// Compute camera translation. In orbit mode these are the fixed values
	// that produce identical output to the old `dist = 4.0 - rz` formula.
	// In flythrough mode, they encode the actual camera transform.
	var txCam, tyCam, tzCam float32
	var visible []gridCell
	useGridCulling := false
	if c.flyMode && c.fly != nil {
		var vm [9]float64
		var vtx, vty, vtz float64
		vm, vtx, vty, vtz = c.fly.viewMatrix()
		// Replace the rotation matrix with the flythrough view matrix.
		m0, m1, m2 = float32(vm[0]), float32(vm[1]), float32(vm[2])
		m3, m4, m5 = float32(vm[3]), float32(vm[4]), float32(vm[5])
		m6, m7, m8 = float32(vm[6]), float32(vm[7]), float32(vm[8])
		txCam = float32(vtx)
		tyCam = float32(vty)
		tzCam = float32(vtz)
		if c.grid != nil {
			useGridCulling = true
			aspect := 1.0
			if h > 0 {
				aspect = float64(w) / float64(h)
			}
			visible = c.grid.visibleCells(extractFrustumPlanes(vm, vtx, vty, vtz, zoom, aspect))
		}
	} else {
		txCam = 0
		tyCam = 0
		tzCam = 4.0
	}

	if useGridCulling {
		if nWorkers <= 1 || len(visible) < 2 {
			for _, cell := range visible {
				hi := cell.start + cell.count
				projectChunk(xs[cell.start:hi], ys[cell.start:hi], zs[cell.start:hi], rgba[cell.start:hi], pix, stride, w, h,
					m0, m1, m2, m3, m4, m5, m6, m7, m8,
					txCam, tyCam, tzCam,
					zoomF, centerX, centerY, defR, defG, defB)
			}
		} else {
			var wg sync.WaitGroup
			workerCount := min(nWorkers, len(visible))
			wg.Add(workerCount)
			for worker := 0; worker < workerCount; worker++ {
				worker := worker
				go func() {
					defer wg.Done()
					for i := worker; i < len(visible); i += workerCount {
						cell := visible[i]
						hi := cell.start + cell.count
						projectChunk(xs[cell.start:hi], ys[cell.start:hi], zs[cell.start:hi], rgba[cell.start:hi], pix, stride, w, h,
							m0, m1, m2, m3, m4, m5, m6, m7, m8,
							txCam, tyCam, tzCam,
							zoomF, centerX, centerY, defR, defG, defB)
					}
				}()
			}
			wg.Wait()
		}
	} else if nWorkers <= 1 {
		projectChunk(xs, ys, zs, rgba, pix, stride, w, h,
			m0, m1, m2, m3, m4, m5, m6, m7, m8,
			txCam, tyCam, tzCam,
			zoomF, centerX, centerY, defR, defG, defB)
	} else {
		var wg sync.WaitGroup
		wg.Add(nWorkers)
		chunkSize := (n + nWorkers - 1) / nWorkers
		for t := range nWorkers {
			lo := t * chunkSize
			hi := lo + chunkSize
			if hi > n {
				hi = n
			}
			go func() {
				defer wg.Done()
				projectChunk(xs[lo:hi], ys[lo:hi], zs[lo:hi], rgba[lo:hi],
					pix, stride, w, h,
					m0, m1, m2, m3, m4, m5, m6, m7, m8,
					txCam, tyCam, tzCam,
					zoomF, centerX, centerY, defR, defG, defB)
			}()
		}
		wg.Wait()
	}

	if c.onFrameDrawn != nil {
		c.onFrameDrawn(time.Since(drawStart))
	}

	return img
}

// projectChunk projects a contiguous slice of points and writes pixels to the
// shared framebuffer. Called from one goroutine per chunk during parallel draw.
//
// The tx, ty, tz parameters encode the camera translation. In orbit mode
// these are (0, 0, 4.0), which produces the same result as the original
// `dist = 4.0 - rz` formula. In flythrough mode they encode the full
// camera transform.
func projectChunk(
	xs, ys, zs []float32, rgba []uint32, pix []byte, stride, w, h int,
	m0, m1, m2, m3, m4, m5, m6, m7, m8 float32,
	tx, ty, tz float32,
	zoomF, centerX, centerY float32,
	defR, defG, defB float32,
) {
	for i, px := range xs {
		py := ys[i]
		pz := zs[i]

		rx := m0*px + m1*py + m2*pz + tx
		ry := m3*px + m4*py + m5*pz + ty
		rz := m6*px + m7*py + m8*pz

		dist := tz - rz
		if dist < 0.1 {
			continue
		}
		invDist := 1.0 / dist
		projX := rx*invDist*zoomF + centerX
		projY := ry*invDist*zoomF + centerY

		ix, iy := int(projX), int(projY)
		if ix < 0 || ix >= w || iy < 0 || iy >= h {
			continue
		}

		off := iy*stride + ix*4
		packed := rgba[i]

		// Depth-based shading: use camera-space depth for consistent
		// shading in both orbit and flythrough modes.
		shade := 1.0 - (tz-dist)*0.15
		if shade < 0.3 {
			shade = 0.3
		} else if shade > 1.0 {
			shade = 1.0
		}

		var r, g, b uint8
		if packed&hasColorBit != 0 {
			r = uint8(float32(packed>>16&0xFF) * shade)
			g = uint8(float32(packed>>8&0xFF) * shade)
			b = uint8(float32(packed&0xFF) * shade)
		} else {
			r = uint8(defR * shade)
			g = uint8(defG * shade)
			b = uint8(defB * shade)
		}

		// Single 32-bit store (little-endian: R at low byte).
		*(*uint32)(unsafe.Pointer(&pix[off])) =
			uint32(r) | uint32(g)<<8 | uint32(b)<<16 | 0xFF000000
	}
}

func arcballVector(mx, my, w, h float64) [3]float64 {
	size := math.Min(w, h)
	x := (2*mx - w) / size
	y := (2*my - h) / size
	d := x*x + y*y
	if d <= 1.0 {
		return [3]float64{x, y, math.Sqrt(1 - d)}
	}
	s := 1.0 / math.Sqrt(d)
	return [3]float64{x * s, y * s, 0}
}

// MouseDown implements desktop.Mouseable.
func (c *canvas3d) MouseDown(ev *desktop.MouseEvent) {
	c.dragModifier = ev.Modifier
	// Request focus on any mouse interaction so keyboard shortcuts work.
	if fyneCanvas := fyne.CurrentApp().Driver().CanvasForObject(c); fyneCanvas != nil {
		fyneCanvas.Focus(c)
	}
}

// MouseUp implements desktop.Mouseable.
func (c *canvas3d) MouseUp(_ *desktop.MouseEvent) {
	c.dragModifier = 0
}

// Dragged implements fyne.Draggable.
func (c *canvas3d) Dragged(ev *fyne.DragEvent) {
	c.startInteraction()

	// In flythrough mode, dragging controls the look direction.
	if c.flyMode && c.fly != nil {
		c.fly.handleMouseLook(float64(ev.Dragged.DX), float64(ev.Dragged.DY))
		c.raster.Refresh()
		return
	}

	panning := c.dragModifier&fyne.KeyModifierShift != 0
	if panning {
		c.mu.Lock()
		c.panX += float64(ev.Dragged.DX)
		c.panY += float64(ev.Dragged.DY)
		c.mu.Unlock()
		c.raster.Refresh()
		return
	}

	size := c.Size()
	w, h := float64(size.Width), float64(size.Height)
	if w < 1 || h < 1 {
		return
	}

	curX := float64(ev.Position.X)
	curY := float64(ev.Position.Y)
	prevX := curX - float64(ev.Dragged.DX)
	prevY := curY - float64(ev.Dragged.DY)

	p0 := arcballVector(prevX, prevY, w, h)
	p1 := arcballVector(curX, curY, w, h)

	cx := p0[1]*p1[2] - p0[2]*p1[1]
	cy := p0[2]*p1[0] - p0[0]*p1[2]
	cz := p0[0]*p1[1] - p0[1]*p1[0]
	dot := p0[0]*p1[0] + p0[1]*p1[1] + p0[2]*p1[2]

	dq := Quat{X: cx, Y: cy, Z: cz, W: dot}.Normalize()

	c.mu.Lock()
	c.orientation = dq.Mul(c.orientation).Normalize()
	c.matrixDirty = true
	c.mu.Unlock()
	c.raster.Refresh()
	if c.onOrientationChanged != nil {
		c.onOrientationChanged()
	}
}

// DragEnd implements fyne.Draggable.
func (c *canvas3d) DragEnd() {}

// Scrolled implements fyne.Scrollable.
func (c *canvas3d) Scrolled(ev *fyne.ScrollEvent) {
	c.startInteraction()

	// In flythrough mode, scroll adjusts movement speed multiplier.
	if c.flyMode && c.fly != nil {
		c.fly.mu.Lock()
		c.fly.speedMultiple *= 1.0 + float64(ev.Scrolled.DY)*0.02
		if c.fly.speedMultiple < 0.25 {
			c.fly.speedMultiple = 0.25
		}
		if c.fly.speedMultiple > 16.0 {
			c.fly.speedMultiple = 16.0
		}
		mult := c.fly.speedMultiple
		c.fly.mu.Unlock()
		if c.onSpeedChanged != nil {
			c.onSpeedChanged(mult)
		}
		return
	}

	c.mu.Lock()
	c.zoom *= 1.0 + float64(ev.Scrolled.DY)*0.02
	if mz := c.minZoom(); c.zoom < mz {
		c.zoom = mz
	}
	c.mu.Unlock()
	c.raster.Refresh()
	c.fireZoomChanged()
}

// FocusGained implements fyne.Focusable.
func (c *canvas3d) FocusGained() {}

// FocusLost implements fyne.Focusable.
func (c *canvas3d) FocusLost() {}

// TypedRune implements fyne.Focusable.
func (c *canvas3d) TypedRune(r rune) {
	switch r {
	case '+', '=':
		c.mu.Lock()
		c.zoom *= 1.1
		c.mu.Unlock()
		c.raster.Refresh()
		c.fireZoomChanged()
	case '-':
		c.mu.Lock()
		c.zoom /= 1.1
		if mz := c.minZoom(); c.zoom < mz {
			c.zoom = mz
		}
		c.mu.Unlock()
		c.raster.Refresh()
		c.fireZoomChanged()
	case 'h':
		c.homeView()
		if c.onHomeView != nil {
			c.onHomeView()
		}
	case 'f':
		c.zoomToExtents()
	case 'g':
		c.setFlythrough(!c.flyMode)
	}
}

// TypedKey implements fyne.Focusable.
func (c *canvas3d) TypedKey(ev *fyne.KeyEvent) {
	// Esc exits flythrough mode.
	if ev.Name == fyne.KeyEscape && c.flyMode {
		c.setFlythrough(false)
		return
	}

	// In flythrough mode, arrow keys are handled by KeyDown/KeyUp.
	if c.flyMode {
		return
	}

	const angle = 0.087 // ~5 degrees
	var dq Quat
	switch ev.Name {
	case fyne.KeyLeft:
		dq = QuatFromAxisAngle(0, 1, 0, -angle)
	case fyne.KeyRight:
		dq = QuatFromAxisAngle(0, 1, 0, angle)
	case fyne.KeyUp:
		dq = QuatFromAxisAngle(1, 0, 0, -angle)
	case fyne.KeyDown:
		dq = QuatFromAxisAngle(1, 0, 0, angle)
	default:
		return
	}
	c.mu.Lock()
	c.orientation = dq.Mul(c.orientation).Normalize()
	c.matrixDirty = true
	c.mu.Unlock()
	c.raster.Refresh()
	if c.onOrientationChanged != nil {
		c.onOrientationChanged()
	}
}

// KeyDown implements desktop.Keyable — tracks held keys for flythrough movement.
func (c *canvas3d) KeyDown(ev *fyne.KeyEvent) {
	if !c.flyMode || c.fly == nil {
		return
	}
	c.fly.mu.Lock()
	if ev.Name == desktop.KeyShiftLeft || ev.Name == desktop.KeyShiftRight {
		c.fly.shiftHeld = true
	} else {
		c.fly.keysHeld[ev.Name] = true
	}
	c.fly.mu.Unlock()

	// Start the ticker if not already running.
	if c.fly.hasKeysHeld() {
		c.fly.start()
	}
}

// KeyUp implements desktop.Keyable — releases held keys.
func (c *canvas3d) KeyUp(ev *fyne.KeyEvent) {
	if !c.flyMode || c.fly == nil {
		return
	}
	c.fly.mu.Lock()
	if ev.Name == desktop.KeyShiftLeft || ev.Name == desktop.KeyShiftRight {
		c.fly.shiftHeld = false
	} else {
		delete(c.fly.keysHeld, ev.Name)
	}
	c.fly.mu.Unlock()

	// Stop the ticker when no keys are held.
	if !c.fly.hasKeysHeld() {
		c.fly.stop()
	}
}

// setFlythrough toggles flythrough mode on or off.
func (c *canvas3d) setFlythrough(on bool) {
	if c.flyMode == on {
		return
	}

	if on {
		// Orbit → Flythrough transition.
		c.fly = newFlythroughCamera(c)
		c.mu.Lock()
		c.fly.fromOrbit(c.orientation, c.zoom, c.panX, c.panY)
		// Pan is now baked into the camera position.
		c.panX = 0
		c.panY = 0
		c.flyMode = true
		c.mu.Unlock()
	} else {
		// Flythrough → Orbit transition.
		if c.fly != nil {
			c.fly.stop()
			c.mu.Lock()
			orient, zoom := c.fly.toOrbit(c.zoom)
			c.orientation = orient
			c.matrixDirty = true
			c.zoom = zoom
			if mz := c.minZoom(); c.zoom < mz {
				c.zoom = mz
			}
			c.panX = 0
			c.panY = 0
			c.flyMode = false
			c.mu.Unlock()
		} else {
			c.mu.Lock()
			c.flyMode = false
			c.mu.Unlock()
		}
	}

	c.raster.Refresh()
	if c.onOrientationChanged != nil {
		c.onOrientationChanged()
	}
	if c.onFlythroughChanged != nil {
		c.onFlythroughChanged(on)
	}
}

// Compile-time checks for interface implementations.
var _ desktop.Keyable = (*canvas3d)(nil)

// Tapped implements fyne.Tappable — picks the nearest point to the click.
func (c *canvas3d) Tapped(ev *fyne.PointEvent) {
	if c.onPointTapped == nil {
		return
	}

	// Event position is in Fyne points. The draw() function projects into
	// pixel coordinates. Convert click to pixel space so the projection
	// matches exactly.
	size := c.Size()
	pointW, pointH := float64(size.Width), float64(size.Height)
	if pointW < 1 || pointH < 1 {
		return
	}

	pixW, pixH := float64(c.lastPixW), float64(c.lastPixH)
	if pixW < 1 || pixH < 1 {
		return
	}
	scaleX, scaleY := pixW/pointW, pixH/pointH
	clickPX := float32(float64(ev.Position.X) * scaleX)
	clickPY := float32(float64(ev.Position.Y) * scaleY)

	c.mu.Lock()
	if c.matrixDirty {
		c.cachedMatrix = c.orientation.ToMatrix()
		c.matrixDirty = false
	}
	m64 := c.cachedMatrix
	zoom := float32(c.zoom)
	panX, panY := c.panX, c.panY
	pts := c.points
	xs := c.xs
	ys := c.ys
	zs := c.zs
	indices := c.originalIndex
	c.mu.Unlock()

	m0, m1, m2 := float32(m64[0]), float32(m64[1]), float32(m64[2])
	m3, m4, m5 := float32(m64[3]), float32(m64[4]), float32(m64[5])
	m6, m7, m8 := float32(m64[6]), float32(m64[7]), float32(m64[8])

	var txCam, tyCam, tzCam float32
	if c.flyMode && c.fly != nil {
		vm, vtx, vty, vtz := c.fly.viewMatrix()
		m0, m1, m2 = float32(vm[0]), float32(vm[1]), float32(vm[2])
		m3, m4, m5 = float32(vm[3]), float32(vm[4]), float32(vm[5])
		m6, m7, m8 = float32(vm[6]), float32(vm[7]), float32(vm[8])
		txCam = float32(vtx)
		tyCam = float32(vty)
		tzCam = float32(vtz)
	} else {
		txCam = 0
		tyCam = 0
		tzCam = 4.0
	}

	centerX := float32(pixW)/2 + float32(panX)*float32(scaleX)
	centerY := float32(pixH)/2 + float32(panY)*float32(scaleY)

	// Compare squared distances to avoid sqrt in the hot loop.
	bestDistSq := float32(math.MaxFloat32)
	bestIdx := -1
	maxPickRadiusSq := float32(10.0*scaleX) * float32(10.0*scaleX)

	for i, px := range xs {
		py := ys[i]
		pz := zs[i]

		rx := m0*px + m1*py + m2*pz + txCam
		ry := m3*px + m4*py + m5*pz + tyCam
		rz := m6*px + m7*py + m8*pz

		dist := tzCam - rz
		if dist < 0.1 {
			continue
		}
		invDist := 1.0 / dist
		sx := rx*invDist*zoom + centerX
		sy := ry*invDist*zoom + centerY

		dx := sx - clickPX
		dy := sy - clickPY
		dSq := dx*dx + dy*dy
		if dSq < bestDistSq {
			bestDistSq = dSq
			bestIdx = i
		}
	}

	if bestIdx >= 0 && bestDistSq <= maxPickRadiusSq {
		c.onPointTapped(pts[indices[bestIdx]], float64(ev.Position.X), float64(ev.Position.Y))
	} else if c.onPointCleared != nil {
		c.onPointCleared()
	}
}
