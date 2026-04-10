package pcviewer

import (
	"image"
	"image/color"
	"math"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"

	"github.com/borud/pointcloud/pkg/pointcloud"
)

// canvas3d is the internal 3D point cloud renderer with arcball rotation,
// pan, and zoom. It is not exported; the public Viewer widget wraps it.
type canvas3d struct {
	widget.BaseWidget
	raster *canvas.Raster

	mu          sync.Mutex
	orientation Quat
	zoom        float64
	panX        float64
	panY        float64
	up          UpAxis
	bgColor     color.RGBA
	points      []pointcloud.Point3D

	dragModifier fyne.KeyModifier

	// Last rendered pixel dimensions (set by draw, read by Tapped).
	lastPixW, lastPixH int

	onOrientationChanged func()
	onHomeView           func()
	onPointTapped        func(p pointcloud.Point3D, screenX, screenY float64)
}

func newCanvas3D() *canvas3d {
	c := &canvas3d{
		orientation: HomeOrientation,
		zoom:        200.0,
		bgColor:     color.RGBA{0, 0, 0, 255},
	}
	c.raster = canvas.NewRaster(c.draw)
	c.ExtendBaseWidget(c)
	return c
}

func (c *canvas3d) setPoints(pts []pointcloud.Point3D) {
	c.mu.Lock()
	c.points = pts
	c.mu.Unlock()
	c.zoomToExtents()
}

func (c *canvas3d) zoomToExtents() {
	size := c.Size()
	w, h := float64(size.Width), float64(size.Height)
	if w < 1 || h < 1 {
		c.mu.Lock()
		c.zoom = 200
		c.panX = 0
		c.panY = 0
		c.mu.Unlock()
		c.raster.Refresh()
		return
	}
	c.mu.Lock()
	c.zoom = math.Min(w, h) * 0.8 * 2
	c.panX = 0
	c.panY = 0
	c.mu.Unlock()
	c.raster.Refresh()
}

func (c *canvas3d) homeView() {
	c.mu.Lock()
	c.orientation = HomeOrientation
	c.mu.Unlock()
	c.zoomToExtents()
}

// CreateRenderer implements fyne.Widget.
func (c *canvas3d) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(c.raster)
}

func (c *canvas3d) draw(w, h int) image.Image {
	c.lastPixW, c.lastPixH = w, h
	img := image.NewRGBA(image.Rect(0, 0, w, h))

	c.mu.Lock()
	m := c.orientation.ToMatrix()
	zoom := c.zoom
	panX, panY := c.panX, c.panY
	up := c.up
	pts := c.points
	bg := c.bgColor
	c.mu.Unlock()

	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i] = bg.R
		img.Pix[i+1] = bg.G
		img.Pix[i+2] = bg.B
		img.Pix[i+3] = bg.A
	}

	centerX, centerY := float64(w)/2+panX, float64(h)/2+panY

	for _, p := range pts {
		px, py, pz := p.X, p.Y, p.Z
		if up == ZUp {
			py, pz = p.Z, -p.Y
		}

		rx := m[0]*px + m[1]*py + m[2]*pz
		ry := m[3]*px + m[4]*py + m[5]*pz
		rz2 := m[6]*px + m[7]*py + m[8]*pz

		dist := 4.0 - rz2
		if dist < 0.1 {
			continue
		}
		projX := (rx/dist)*zoom + centerX
		projY := (ry/dist)*zoom + centerY

		ix, iy := int(projX), int(projY)
		if ix < 0 || ix >= w || iy < 0 || iy >= h {
			continue
		}
		var clr color.RGBA
		if p.HasColor {
			shade := math.Min(1.0, math.Max(0.3, 1.0-rz2*0.15))
			clr = color.RGBA{
				uint8(float64(p.R) * shade),
				uint8(float64(p.G) * shade),
				uint8(float64(p.B) * shade),
				255,
			}
		} else {
			brightness := uint8(math.Min(255, math.Max(50, 200-rz2*80)))
			clr = color.RGBA{brightness, 150, 255, 255}
		}
		off := img.PixOffset(ix, iy)
		img.Pix[off] = clr.R
		img.Pix[off+1] = clr.G
		img.Pix[off+2] = clr.B
		img.Pix[off+3] = clr.A
	}
	return img
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
	c.mu.Lock()
	c.zoom *= 1.0 + float64(ev.Scrolled.DY)*0.02
	if c.zoom < 1 {
		c.zoom = 1
	}
	c.mu.Unlock()
	c.raster.Refresh()
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
	case '-':
		c.mu.Lock()
		c.zoom /= 1.1
		if c.zoom < 1 {
			c.zoom = 1
		}
		c.mu.Unlock()
		c.raster.Refresh()
	case 'h':
		c.homeView()
		if c.onHomeView != nil {
			c.onHomeView()
		}
	case 'f':
		c.zoomToExtents()
	}
}

// TypedKey implements fyne.Focusable.
func (c *canvas3d) TypedKey(ev *fyne.KeyEvent) {
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
	c.mu.Unlock()
	c.raster.Refresh()
	if c.onOrientationChanged != nil {
		c.onOrientationChanged()
	}
}

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
	clickPX := float64(ev.Position.X) * scaleX
	clickPY := float64(ev.Position.Y) * scaleY

	c.mu.Lock()
	m := c.orientation.ToMatrix()
	zoom := c.zoom
	panX, panY := c.panX, c.panY
	up := c.up
	pts := c.points
	c.mu.Unlock()

	// Use pixel center — same as draw().
	centerX, centerY := pixW/2+panX, pixH/2+panY

	bestDist := math.MaxFloat64
	bestIdx := -1
	// Pick radius in pixels. Points are single-pixel, so be generous.
	maxPickRadius := 10.0 * scaleX

	for i, p := range pts {
		px, py, pz := p.X, p.Y, p.Z
		if up == ZUp {
			py, pz = p.Z, -p.Y
		}

		rx := m[0]*px + m[1]*py + m[2]*pz
		ry := m[3]*px + m[4]*py + m[5]*pz
		rz2 := m[6]*px + m[7]*py + m[8]*pz

		dist := 4.0 - rz2
		if dist < 0.1 {
			continue
		}
		sx := (rx/dist)*zoom + centerX
		sy := (ry/dist)*zoom + centerY

		dx := sx - clickPX
		dy := sy - clickPY
		d := math.Sqrt(dx*dx + dy*dy)
		if d < bestDist {
			bestDist = d
			bestIdx = i
		}
	}

	if bestIdx >= 0 && bestDist <= maxPickRadius {
		c.onPointTapped(pts[bestIdx], float64(ev.Position.X), float64(ev.Position.Y))
	}
}
