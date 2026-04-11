// Package pcviewer provides a reusable Fyne widget for interactive 3D point
// cloud visualization.
//
// The [Viewer] widget combines a 3D rendering canvas with an orientation cube,
// home and zoom-to-fit buttons, and a point info label. It supports arcball
// rotation (drag), panning (Shift+drag), and scroll-wheel zoom.
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
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
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
//   - Home and zoom-to-fit buttons
type Viewer struct {
	widget.BaseWidget

	canvas    *canvas3d
	cube      *orientationCube
	home      *iconButton
	zoomFit   *iconButton
	infoLabel *canvas.Text
	fpsLabel  *canvas.Text
	scaleBar  *scaleBar
	content   *fyne.Container

	// FPS tracking state.
	fpsFrameCount int
	fpsLastUpdate time.Time
}

// New creates a new point cloud viewer widget with default settings.
// The default up-axis is YUp. Use [Viewer.SetUpAxis] to change it.
// Functional options can be provided to configure the viewer at creation time.
func New(opts ...Option) *Viewer {
	cfg := &config{}
	for _, o := range opts {
		o(cfg)
	}

	v := &Viewer{}
	v.canvas = newCanvas3D(cfg)

	showCube := boolOr(cfg.showCube, true)
	showHome := boolOr(cfg.showHome, true)
	showZoomFit := boolOr(cfg.showZoomFit, true)
	showInfo := boolOr(cfg.showInfoLabel, true)

	if showHome {
		v.home = newIconButton(28, 28, drawHomeIcon, func() {
			v.canvas.homeView()
			if v.cube != nil {
				v.cube.raster.Refresh()
			}
		})
	}

	if showZoomFit {
		v.zoomFit = newIconButton(28, 28, drawZoomFitIcon, func() {
			v.canvas.zoomToExtents()
		})
	}

	if showCube {
		cubeColors := DefaultCubeColors()
		if cfg.cubeColors != nil {
			cubeColors = *cfg.cubeColors
		}
		v.cube = newOrientationCube(v.canvas, cubeColors, func() {
			// cube snapped orientation — no extra action needed
		})
	}

	v.canvas.onOrientationChanged = func() {
		if v.cube != nil {
			v.cube.raster.Refresh()
		}
	}

	v.canvas.onHomeView = func() {
		if v.cube != nil {
			v.cube.raster.Refresh()
		}
	}

	if showInfo {
		infoColor := colorOr(cfg.infoLabelColor, color.RGBA{})

		v.infoLabel = canvas.NewText("", color.RGBA{0, 0, 0, 0})
		if cfg.infoLabelColor != nil {
			v.infoLabel.Color = infoColor
		} else {
			v.infoLabel.Color = theme.DefaultTheme().Color(theme.ColorNameForeground, theme.VariantDark)
		}
		if cfg.infoLabelStyle != nil {
			v.infoLabel.TextStyle = *cfg.infoLabelStyle
		} else {
			v.infoLabel.TextStyle = fyne.TextStyle{Monospace: true}
		}
		v.infoLabel.TextSize = float32Or(cfg.infoLabelSize, 12)
	}

	showScaleBar := boolOr(cfg.showScaleBar, true)
	if showScaleBar {
		sbColor := colorOr(cfg.scaleBarColor, color.RGBA{200, 200, 200, 255})
		sbUnit := stringOr(cfg.scaleUnit, "")
		sbUnitScale := float64Or(cfg.scaleUnitScale, 1.0)
		v.scaleBar = newScaleBar(v.canvas, sbColor, sbUnit, sbUnitScale)
	}

	if boolOr(cfg.showFPS, false) {
		fpsColor := colorOr(cfg.fpsColor, color.RGBA{200, 200, 200, 255})
		v.fpsLabel = canvas.NewText("", fpsColor)
		if cfg.fpsStyle != nil {
			v.fpsLabel.TextStyle = *cfg.fpsStyle
		} else {
			v.fpsLabel.TextStyle = fyne.TextStyle{Monospace: true}
		}
		v.fpsLabel.TextSize = float32Or(cfg.fpsSize, 14)
		v.fpsLastUpdate = time.Now()
		v.canvas.onFrameDrawn = func(_ time.Duration) {
			v.fpsFrameCount++
			elapsed := time.Since(v.fpsLastUpdate)
			if elapsed >= time.Second {
				fps := float64(v.fpsFrameCount) / elapsed.Seconds()
				v.fpsFrameCount = 0
				v.fpsLastUpdate = time.Now()
				text := fmt.Sprintf("%.0f FPS", fps)
				fyne.Do(func() {
					v.fpsLabel.Text = text
					v.fpsLabel.Refresh()
				})
			}
		}
	}

	v.canvas.onZoomChanged = func() {
		if v.scaleBar != nil {
			fyne.Do(func() { v.scaleBar.raster.Refresh() })
		}
	}

	v.canvas.onPointTapped = func(p pointcloud.Point3D, _, _ float64) {
		if v.infoLabel == nil {
			return
		}
		if p.HasColor {
			v.infoLabel.Text = fmt.Sprintf("X:%.4f  Y:%.4f  Z:%.4f  RGB:%d,%d,%d", p.X, p.Y, p.Z, p.R, p.G, p.B)
		} else {
			v.infoLabel.Text = fmt.Sprintf("X:%.4f  Y:%.4f  Z:%.4f", p.X, p.Y, p.Z)
		}
		v.infoLabel.Refresh()
	}

	v.canvas.onPointCleared = func() {
		if v.infoLabel == nil {
			return
		}
		v.infoLabel.Text = ""
		v.infoLabel.Refresh()
	}

	// Build the overlay controls.
	var btnItems []fyne.CanvasObject
	btnItems = append(btnItems, layout.NewSpacer())
	if v.zoomFit != nil {
		btnItems = append(btnItems, container.New(layout.NewGridWrapLayout(fyne.NewSize(28, 28)), v.zoomFit))
	}
	if v.home != nil {
		btnItems = append(btnItems, container.New(layout.NewGridWrapLayout(fyne.NewSize(28, 28)), v.home))
	}
	btnRow := container.NewHBox(btnItems...)

	var controlItems []fyne.CanvasObject
	controlItems = append(controlItems, btnRow)
	if v.cube != nil {
		controlItems = append(controlItems, container.New(layout.NewGridWrapLayout(fyne.NewSize(105, 105)), v.cube))
	}
	controls := container.NewVBox(controlItems...)

	var bottomItems []fyne.CanvasObject
	if v.infoLabel != nil {
		bottomItems = append(bottomItems, v.infoLabel)
	}
	bottomItems = append(bottomItems, layout.NewSpacer())
	if v.scaleBar != nil {
		bottomItems = append(bottomItems, v.scaleBar)
	}
	var bottom fyne.CanvasObject
	if len(bottomItems) > 1 { // more than just a spacer
		bottom = container.NewHBox(bottomItems...)
	}

	var topItems []fyne.CanvasObject
	if v.fpsLabel != nil {
		topItems = append(topItems, v.fpsLabel)
	}
	topItems = append(topItems, layout.NewSpacer())
	topItems = append(topItems, controls)
	top := container.NewHBox(topItems...)

	overlay := container.New(
		layout.NewCustomPaddedLayout(15, 15, 15, 15),
		container.NewBorder(
			top,
			bottom,
			nil, nil,
		),
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

// SetPointsPreserveView replaces the displayed point cloud without resetting
// the orientation, zoom, or pan.
func (v *Viewer) SetPointsPreserveView(pts []pointcloud.Point3D) {
	v.canvas.mu.Lock()
	v.canvas.points = pts
	v.canvas.convertToSoA()
	v.canvas.mu.Unlock()
	fyne.Do(func() { v.canvas.raster.Refresh() })
}

// SetUpAxis sets the up-axis convention used for rendering.
// Use [YUp] for data where Y is up, or [ZUp] for data where Z is up
// (typical for LiDAR and surveying data).
func (v *Viewer) SetUpAxis(up UpAxis) {
	v.canvas.mu.Lock()
	v.canvas.up = up
	v.canvas.convertToSoA()
	v.canvas.mu.Unlock()
	fyne.Do(func() { v.canvas.raster.Refresh() })
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
	fyne.Do(func() {
		if v.cube != nil {
			v.cube.raster.Refresh()
		}
	})
}

// SetOrientation sets the view orientation to the given quaternion.
func (v *Viewer) SetOrientation(q Quat) {
	v.canvas.mu.Lock()
	v.canvas.orientation = q
	v.canvas.matrixDirty = true
	v.canvas.mu.Unlock()
	fyne.Do(func() {
		v.canvas.raster.Refresh()
		if v.cube != nil {
			v.cube.raster.Refresh()
		}
	})
}

// SetDefaultPointColor sets the fallback color for points without RGB data.
func (v *Viewer) SetDefaultPointColor(c color.RGBA) {
	v.canvas.mu.Lock()
	v.canvas.defaultPointColor = c
	v.canvas.mu.Unlock()
	fyne.Do(func() { v.canvas.raster.Refresh() })
}

// SetCubeColors updates the orientation cube colors at runtime.
// Has no effect if the cube was hidden via WithOrientationCube(false).
func (v *Viewer) SetCubeColors(cc CubeColors) {
	if v.cube != nil {
		v.cube.colors = cc
		fyne.Do(func() { v.cube.raster.Refresh() })
	}
}

// SetInfoLabelColor sets the info label text color at runtime.
// Has no effect if the info label was hidden via WithInfoLabel(false).
func (v *Viewer) SetInfoLabelColor(c color.RGBA) {
	if v.infoLabel != nil {
		v.infoLabel.Color = c
		fyne.Do(func() { v.infoLabel.Refresh() })
	}
}

// SetInfoLabelSize sets the info label font size at runtime.
// Has no effect if the info label was hidden via WithInfoLabel(false).
func (v *Viewer) SetInfoLabelSize(size float32) {
	if v.infoLabel != nil {
		v.infoLabel.TextSize = size
		fyne.Do(func() { v.infoLabel.Refresh() })
	}
}

// SetInfoLabelStyle sets the info label text style at runtime.
// Has no effect if the info label was hidden via WithInfoLabel(false).
func (v *Viewer) SetInfoLabelStyle(s fyne.TextStyle) {
	if v.infoLabel != nil {
		v.infoLabel.TextStyle = s
		fyne.Do(func() { v.infoLabel.Refresh() })
	}
}

// SetScale sets the normalization scale factor for the scale bar.
// This is typically the NormScale value from a PointCloud after Normalize().
func (v *Viewer) SetScale(normScale float64) {
	if v.scaleBar != nil {
		v.scaleBar.normScale = normScale
		fyne.Do(func() { v.scaleBar.raster.Refresh() })
	}
}

// SetScaleUnit sets the unit label for the scale bar (e.g. "m").
func (v *Viewer) SetScaleUnit(unit string) {
	if v.scaleBar != nil {
		v.scaleBar.unit = unit
		fyne.Do(func() { v.scaleBar.raster.Refresh() })
	}
}

// SetScaleUnitScale sets the unit multiplier for the scale bar.
func (v *Viewer) SetScaleUnitScale(multiplier float64) {
	if v.scaleBar != nil {
		if multiplier <= 0 {
			multiplier = 1.0
		}
		v.scaleBar.unitScale = multiplier
		fyne.Do(func() { v.scaleBar.raster.Refresh() })
	}
}

// SetScaleBarColor sets the scale bar color at runtime.
func (v *Viewer) SetScaleBarColor(c color.RGBA) {
	if v.scaleBar != nil {
		v.scaleBar.color = c
		v.scaleBar.label.Color = c
		fyne.Do(func() {
			v.scaleBar.label.Refresh()
			v.scaleBar.raster.Refresh()
		})
	}
}

// Zoom returns the current zoom level.
func (v *Viewer) Zoom() float64 {
	v.canvas.mu.Lock()
	defer v.canvas.mu.Unlock()
	return v.canvas.zoom
}

// SetZoom sets the zoom level.
func (v *Viewer) SetZoom(z float64) {
	v.canvas.mu.Lock()
	v.canvas.zoom = z
	v.canvas.mu.Unlock()
	fyne.Do(func() { v.canvas.raster.Refresh() })
}

// Pan returns the current pan offset in Fyne DIP units.
func (v *Viewer) Pan() (x, y float64) {
	v.canvas.mu.Lock()
	defer v.canvas.mu.Unlock()
	return v.canvas.panX, v.canvas.panY
}

// SetPan sets the pan offset in Fyne DIP units.
func (v *Viewer) SetPan(x, y float64) {
	v.canvas.mu.Lock()
	v.canvas.panX = x
	v.canvas.panY = y
	v.canvas.mu.Unlock()
	fyne.Do(func() { v.canvas.raster.Refresh() })
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
	v.canvas.clearTemplate = nil // force rebuild on next draw
	v.canvas.mu.Unlock()
	fyne.Do(func() { v.canvas.raster.Refresh() })
}

// SetLODEnabled enables or disables LOD (level-of-detail) decimation during
// mouse interaction. When enabled, a decimated point set is rendered while
// dragging or scrolling for responsive frame rates. The full cloud is
// restored after a short idle period. LOD is enabled by default.
func (v *Viewer) SetLODEnabled(enabled bool) {
	v.canvas.mu.Lock()
	v.canvas.lodEnabled = enabled
	v.canvas.buildLOD()
	v.canvas.mu.Unlock()
}

// LODEnabled returns whether LOD decimation is currently enabled.
func (v *Viewer) LODEnabled() bool {
	v.canvas.mu.Lock()
	defer v.canvas.mu.Unlock()
	return v.canvas.lodEnabled
}

// SetLODTargetSize sets the target number of points in the decimated LOD set.
// Smaller values give faster interaction at the cost of visual fidelity during
// drag. The default is 200,000. The LOD arrays are rebuilt immediately.
func (v *Viewer) SetLODTargetSize(n int) {
	v.canvas.mu.Lock()
	v.canvas.lodTargetSize = n
	v.canvas.buildLOD()
	v.canvas.mu.Unlock()
}

// SetFPSColor sets the FPS counter text color.
func (v *Viewer) SetFPSColor(c color.RGBA) {
	if v.fpsLabel != nil {
		v.fpsLabel.Color = c
		fyne.Do(func() { v.fpsLabel.Refresh() })
	}
}

// SetFPSStyle sets the FPS counter text style.
func (v *Viewer) SetFPSStyle(s fyne.TextStyle) {
	if v.fpsLabel != nil {
		v.fpsLabel.TextStyle = s
		fyne.Do(func() { v.fpsLabel.Refresh() })
	}
}

// SetFPSSize sets the FPS counter font size.
func (v *Viewer) SetFPSSize(size float32) {
	if v.fpsLabel != nil {
		v.fpsLabel.TextSize = size
		fyne.Do(func() { v.fpsLabel.Refresh() })
	}
}

// SetOnFrameDrawn registers a callback that is invoked at the end of each
// frame render with the time the draw call took. Useful for benchmarking.
// If the FPS display is enabled, both the FPS counter and this callback
// will be called.
func (v *Viewer) SetOnFrameDrawn(fn func(time.Duration)) {
	if v.fpsLabel != nil {
		fpsFn := v.canvas.onFrameDrawn // the FPS tracking callback
		v.canvas.onFrameDrawn = func(d time.Duration) {
			fpsFn(d)
			if fn != nil {
				fn(d)
			}
		}
	} else {
		v.canvas.onFrameDrawn = fn
	}
}

// LODTargetSize returns the current LOD target point count.
func (v *Viewer) LODTargetSize() int {
	v.canvas.mu.Lock()
	defer v.canvas.mu.Unlock()
	return v.canvas.lodTargetSize
}

// SetMaxZoomOutFraction sets the minimum visible fraction of the largest
// canvas dimension that the point cloud must occupy when zooming out.
// For example, 0.2 means zoom-out stops when the cloud covers ~20% of the
// viewport. The default is 0.2.
func (v *Viewer) SetMaxZoomOutFraction(f float64) {
	v.canvas.mu.Lock()
	v.canvas.maxZoomOutFraction = f
	v.canvas.mu.Unlock()
}

// MaxZoomOutFraction returns the current zoom-out fraction limit.
func (v *Viewer) MaxZoomOutFraction() float64 {
	v.canvas.mu.Lock()
	defer v.canvas.mu.Unlock()
	return v.canvas.maxZoomOutFraction
}
