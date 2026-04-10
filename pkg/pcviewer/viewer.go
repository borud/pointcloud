// Package pcviewer provides a reusable Fyne widget for interactive 3D point
// cloud visualization.
//
// The [Viewer] widget combines a 3D rendering canvas with an orientation cube,
// a home button, and a Y/Z-up axis toggle. It supports arcball rotation
// (drag), panning (Shift+drag), and scroll-wheel zoom.
//
// Basic usage:
//
//	v := pcviewer.New()
//	v.SetUpAxis(pcviewer.ZUp)
//	// ... add v to your Fyne layout ...
//	v.SetPoints(points)
package pcviewer

import (
	"fmt"
	"image/color"
	"math"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	"github.com/borud/pointcloud/pkg/pointcloud"
)

// HomeOrientation is the default home view: side-on, slightly elevated.
var HomeOrientation = QuatFromEulerXY(-0.3, -math.Pi/4)

// UpAxis selects which input data axis maps to screen-up.
type UpAxis int

const (
	// YUp treats the Y axis as up (common in computer graphics).
	YUp UpAxis = iota
	// ZUp treats the Z axis as up (common in surveying and LiDAR).
	ZUp
)

// Viewer is a self-contained point cloud viewer widget for Fyne applications.
//
// It provides:
//   - Interactive 3D rendering with perspective projection
//   - Arcball rotation via mouse drag
//   - Panning via Shift+drag
//   - Zoom via scroll wheel
//   - An orientation cube for quick view snapping
//   - A home button to reset the view
//   - A Y-up / Z-up axis toggle button
type Viewer struct {
	widget.BaseWidget

	canvas    *canvas3d
	cube      *orientationCube
	home      *iconButton
	zoomFit   *iconButton
	axisBtn   *textButton
	infoLabel *widget.Label
	content   *fyne.Container
}

// New creates a new point cloud viewer widget with default settings.
// The default up-axis is YUp. Use [Viewer.SetUpAxis] to change it.
func New() *Viewer {
	v := &Viewer{}

	v.canvas = newCanvas3D()

	v.home = newIconButton(28, 28, drawHomeIcon, func() {
		v.canvas.homeView()
		v.cube.raster.Refresh()
	})

	v.zoomFit = newIconButton(28, 28, drawZoomFitIcon, func() {
		v.canvas.zoomToExtents()
	})

	v.cube = newOrientationCube(v.canvas, func() {
		// cube snapped orientation — no extra action needed
	})

	v.canvas.onOrientationChanged = func() {
		v.cube.raster.Refresh()
	}

	v.canvas.onHomeView = func() {
		v.cube.raster.Refresh()
	}

	v.infoLabel = widget.NewLabel("")
	v.infoLabel.TextStyle = fyne.TextStyle{Monospace: true}

	v.canvas.onPointTapped = func(p pointcloud.Point3D, _, _ float64) {
		if p.HasColor {
			v.infoLabel.SetText(fmt.Sprintf("X:%.4f  Y:%.4f  Z:%.4f  RGB:%d,%d,%d", p.X, p.Y, p.Z, p.R, p.G, p.B))
		} else {
			v.infoLabel.SetText(fmt.Sprintf("X:%.4f  Y:%.4f  Z:%.4f", p.X, p.Y, p.Z))
		}
	}

	v.axisBtn = newTextButton("Y-up", 11, func() {
		v.canvas.mu.Lock()
		if v.canvas.up == YUp {
			v.canvas.up = ZUp
		} else {
			v.canvas.up = YUp
		}
		up := v.canvas.up
		v.canvas.mu.Unlock()
		if up == ZUp {
			v.axisBtn.SetText("Z-up")
		} else {
			v.axisBtn.SetText("Y-up")
		}
		v.cube.raster.Refresh()
		v.canvas.raster.Refresh()
	})

	cubeWrap := container.New(layout.NewGridWrapLayout(fyne.NewSize(105, 105)), v.cube)
	btnRow := container.NewHBox(
		container.New(layout.NewGridWrapLayout(fyne.NewSize(40, 28)), v.axisBtn),
		layout.NewSpacer(),
		container.New(layout.NewGridWrapLayout(fyne.NewSize(28, 28)), v.zoomFit),
		container.New(layout.NewGridWrapLayout(fyne.NewSize(28, 28)), v.home),
	)
	controls := container.NewVBox(btnRow, cubeWrap)

	overlay := container.NewBorder(
		container.NewHBox(layout.NewSpacer(), controls),
		container.NewHBox(v.infoLabel),
		nil, nil,
	)

	v.content = container.NewStack(v.canvas, overlay)
	v.ExtendBaseWidget(v)
	return v
}

// CreateRenderer implements fyne.Widget.
func (v *Viewer) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(v.content)
}

// SetPoints replaces the displayed point cloud and resets the view to fit
// all points. The points should be normalized (see [pointcloud.PointCloud.Normalize]).
func (v *Viewer) SetPoints(pts []pointcloud.Point3D) {
	v.canvas.setPoints(pts)
}

// SetUpAxis sets the up-axis convention used for rendering.
// Use [YUp] for data where Y is up, or [ZUp] for data where Z is up
// (typical for LiDAR and surveying data).
func (v *Viewer) SetUpAxis(up UpAxis) {
	v.canvas.mu.Lock()
	v.canvas.up = up
	v.canvas.mu.Unlock()
	if up == ZUp {
		v.axisBtn.SetText("Z-up")
	} else {
		v.axisBtn.SetText("Y-up")
	}
	v.canvas.raster.Refresh()
}

// GetUpAxis returns the current up-axis setting.
func (v *Viewer) GetUpAxis() UpAxis {
	v.canvas.mu.Lock()
	defer v.canvas.mu.Unlock()
	return v.canvas.up
}

// ZoomToExtents resets zoom and pan so the full point cloud is visible.
func (v *Viewer) ZoomToExtents() {
	v.canvas.zoomToExtents()
}

// HomeView resets the orientation, zoom, and pan to the default home view.
func (v *Viewer) HomeView() {
	v.canvas.homeView()
	v.cube.raster.Refresh()
}

// SetOrientation sets the view orientation to the given quaternion.
func (v *Viewer) SetOrientation(q Quat) {
	v.canvas.mu.Lock()
	v.canvas.orientation = q
	v.canvas.mu.Unlock()
	v.canvas.raster.Refresh()
	v.cube.raster.Refresh()
}

// Orientation returns the current view orientation as a quaternion.
func (v *Viewer) Orientation() Quat {
	v.canvas.mu.Lock()
	defer v.canvas.mu.Unlock()
	return v.canvas.orientation
}

// SetBackgroundColor sets the canvas background color.
// The default is black.
func (v *Viewer) SetBackgroundColor(c color.RGBA) {
	v.canvas.mu.Lock()
	v.canvas.bgColor = c
	v.canvas.mu.Unlock()
	v.canvas.raster.Refresh()
}

