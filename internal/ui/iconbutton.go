package ui

import (
	"image"
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/widget"

	"github.com/borud/pointcloud/internal/raster"
)

// IconButton is a small, transparent, tappable widget that renders custom
// content via a draw function. Used for the home and zoom-fit buttons.
type IconButton struct {
	widget.BaseWidget
	Raster   *canvas.Raster
	onTap    func()
	w, h     float32
	drawFunc func(*image.RGBA, int, int)
}

// NewIconButton creates a new icon button with the given size, draw function,
// and tap callback.
func NewIconButton(w, h float32, drawFunc func(*image.RGBA, int, int), onTap func()) *IconButton {
	ib := &IconButton{
		onTap:    onTap,
		w:        w,
		h:        h,
		drawFunc: drawFunc,
	}
	ib.Raster = canvas.NewRaster(func(pw, ph int) image.Image {
		img := image.NewRGBA(image.Rect(0, 0, pw, ph))
		ib.drawFunc(img, pw, ph)
		return img
	})
	ib.ExtendBaseWidget(ib)
	return ib
}

// CreateRenderer implements fyne.Widget.
func (ib *IconButton) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(ib.Raster)
}

// MinSize implements fyne.Widget.
func (ib *IconButton) MinSize() fyne.Size {
	return fyne.NewSize(ib.w, ib.h)
}

// Tapped implements fyne.Tappable.
func (ib *IconButton) Tapped(_ *fyne.PointEvent) {
	if ib.onTap != nil {
		ib.onTap()
	}
}

// DrawZoomFitIcon draws corner brackets with inward arrows suggesting "fit to view".
func DrawZoomFitIcon(img *image.RGBA, w, h int) {
	outline := color.RGBA{200, 200, 200, 200}
	cx, cy := float64(w)/2, float64(h)/2
	s := float64(w) * 0.30

	// Draw corner brackets to suggest "fit to view".
	bLen := s * 0.8 // bracket arm length

	// Top-left corner
	raster.LineAA(img, cx-s, cy-s, cx-s+bLen, cy-s, outline)
	raster.LineAA(img, cx-s, cy-s, cx-s, cy-s+bLen, outline)
	// Top-right corner
	raster.LineAA(img, cx+s, cy-s, cx+s-bLen, cy-s, outline)
	raster.LineAA(img, cx+s, cy-s, cx+s, cy-s+bLen, outline)
	// Bottom-left corner
	raster.LineAA(img, cx-s, cy+s, cx-s+bLen, cy+s, outline)
	raster.LineAA(img, cx-s, cy+s, cx-s, cy+s-bLen, outline)
	// Bottom-right corner
	raster.LineAA(img, cx+s, cy+s, cx+s-bLen, cy+s, outline)
	raster.LineAA(img, cx+s, cy+s, cx+s, cy+s-bLen, outline)

	// Small inward arrows (dots at center of each edge).
	arrowColor := color.RGBA{180, 180, 180, 180}
	aOff := s * 0.35
	raster.LineAA(img, cx-aOff, cy, cx+aOff, cy, arrowColor)
	raster.LineAA(img, cx, cy-aOff, cx, cy+aOff, arrowColor)
}

// DrawHomeIcon draws a simple house icon.
func DrawHomeIcon(img *image.RGBA, w, h int) {
	fill := color.RGBA{160, 160, 160, 160}
	outline := color.RGBA{200, 200, 200, 200}
	cx, cy := float64(w)/2, float64(h)/2
	s := float64(w) * 0.30

	// Filled roof (triangle).
	roof := [4]raster.Vec2{
		{X: cx, Y: cy - s*1.3},
		{X: cx + s*1.2, Y: cy - s*0.05},
		{X: cx - s*1.2, Y: cy - s*0.05},
		{X: cx, Y: cy - s*1.3},
	}
	raster.FillQuad(img, roof, fill)
	raster.LineAA(img, roof[0].X, roof[0].Y, roof[1].X, roof[1].Y, outline)
	raster.LineAA(img, roof[0].X, roof[0].Y, roof[2].X, roof[2].Y, outline)
	raster.LineAA(img, roof[1].X, roof[1].Y, roof[2].X, roof[2].Y, outline)

	// Filled body (rectangle).
	body := [4]raster.Vec2{
		{X: cx - s*0.7, Y: cy - s*0.05},
		{X: cx + s*0.7, Y: cy - s*0.05},
		{X: cx + s*0.7, Y: cy + s*0.95},
		{X: cx - s*0.7, Y: cy + s*0.95},
	}
	raster.FillQuad(img, body, fill)
	raster.QuadOutline(img, body, outline)

	// Door cutout (darker rectangle).
	door := color.RGBA{60, 60, 60, 180}
	doorQ := [4]raster.Vec2{
		{X: cx - s*0.25, Y: cy + s*0.3},
		{X: cx + s*0.25, Y: cy + s*0.3},
		{X: cx + s*0.25, Y: cy + s*0.95},
		{X: cx - s*0.25, Y: cy + s*0.95},
	}
	raster.FillQuad(img, doorQ, door)
}
