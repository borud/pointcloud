package pcviewer

import (
	"fmt"
	"image"
	"image/color"
	"math"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"github.com/borud/pointcloud/pkg/raster"
)

// scaleBar is an overlay widget that displays a map-style ruler showing the
// distance in original data units at the current zoom level.
type scaleBar struct {
	widget.BaseWidget
	raster    *canvas.Raster
	label     *canvas.Text
	content   *fyne.Container
	canvas3d  *canvas3d
	color     color.RGBA
	unit      string
	unitScale float64
	normScale float64
}

func newScaleBar(c3d *canvas3d, barColor color.RGBA, unit string, unitScale float64) *scaleBar {
	if unitScale <= 0 {
		unitScale = 1.0
	}
	sb := &scaleBar{
		canvas3d:  c3d,
		color:     barColor,
		unit:      unit,
		unitScale: unitScale,
	}
	sb.label = canvas.NewText("", barColor)
	sb.label.TextSize = 11
	sb.label.TextStyle = fyne.TextStyle{Monospace: true}
	sb.label.Alignment = fyne.TextAlignCenter

	sb.raster = canvas.NewRaster(sb.draw)
	sb.raster.SetMinSize(fyne.NewSize(180, 16))

	sb.content = container.NewVBox(sb.label, sb.raster)
	sb.ExtendBaseWidget(sb)
	return sb
}

// CreateRenderer implements fyne.Widget.
func (sb *scaleBar) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(sb.content)
}

// MinSize implements fyne.Widget.
func (sb *scaleBar) MinSize() fyne.Size {
	return fyne.NewSize(180, 30)
}

func (sb *scaleBar) draw(w, h int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	if sb.normScale == 0 || w < 10 || h < 4 {
		return img
	}

	sb.canvas3d.mu.Lock()
	zoom := sb.canvas3d.zoom
	sb.canvas3d.mu.Unlock()

	// Compute pixels per display unit.
	// From projectChunk: screenX = (rx / (4.0 - rz)) * zoom + centerX
	// At center depth (rz=0): 1 normalized unit = zoom/4 pixels.
	// 1 original unit = normScale normalized units, so 1 original unit = (zoom/4)*normScale pixels.
	// With unitScale: 1 display unit = 1/unitScale original units = (zoom/4)*normScale/unitScale pixels.
	pixelsPerDisplayUnit := (zoom / 4.0) * sb.normScale / sb.unitScale
	if pixelsPerDisplayUnit < 1e-9 {
		return img
	}

	// Target bar width ~150 logical pixels. Account for DPI scale.
	targetPixels := float64(w) * 0.85
	rawLength := targetPixels / pixelsPerDisplayUnit
	barLength := niceScaleNumber(rawLength)
	barPixels := barLength * pixelsPerDisplayUnit

	if barPixels < 2 || barPixels > float64(w)*2 {
		return img
	}

	// Update the label text.
	labelText := formatScaleLabel(barLength, sb.unit)
	sb.label.Text = labelText
	sb.label.Color = sb.color
	sb.label.Refresh()

	// Draw the ruler centered in the raster.
	barColor := sb.color
	cx := float64(w) / 2.0
	startX := cx - barPixels/2.0
	endX := cx + barPixels/2.0
	baseY := float64(h) * 0.5

	// Horizontal line.
	raster.LineAA(img, startX, baseY, endX, baseY, barColor)

	// Subdivision ticks: 4 divisions = 5 tick marks.
	const nDivisions = 4
	tallTick := float64(h) * 0.45
	shortTick := float64(h) * 0.25
	for i := range nDivisions + 1 {
		frac := float64(i) / float64(nDivisions)
		tx := startX + frac*barPixels
		tickH := shortTick
		if i == 0 || i == nDivisions {
			tickH = tallTick
		}
		raster.LineAA(img, tx, baseY-tickH, tx, baseY+tickH, barColor)
	}

	return img
}

// niceScaleNumber rounds x to a "nice" number (1, 2, 5 × 10^n).
func niceScaleNumber(x float64) float64 {
	if x <= 0 {
		return 1
	}
	exp := math.Floor(math.Log10(x))
	frac := x / math.Pow(10, exp)
	switch {
	case frac < 1.5:
		return math.Pow(10, exp)
	case frac < 3.5:
		return 2 * math.Pow(10, exp)
	case frac < 7.5:
		return 5 * math.Pow(10, exp)
	default:
		return 10 * math.Pow(10, exp)
	}
}

// formatScaleLabel formats the scale bar label.
func formatScaleLabel(length float64, unit string) string {
	if unit == "" {
		return fmt.Sprintf("%g", length)
	}
	return fmt.Sprintf("%g %s", length, unit)
}
