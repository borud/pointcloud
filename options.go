package pointcloud

import (
	"image/color"

	"fyne.io/fyne/v2"
)

// Option configures a Viewer during construction.
type Option func(*config)

// config holds optional overrides for Viewer construction.
// Pointer/flag fields let us distinguish "not set" from "set to zero value".
type config struct {
	bgColor           *color.RGBA
	defaultPointColor *color.RGBA
	cubeColors        *CubeColors
	showCube          *bool
	showHome          *bool
	showZoomFit       *bool
	showInfoLabel     *bool
	infoLabelColor    *color.RGBA
	infoLabelStyle    *fyne.TextStyle
	infoLabelSize     *float32
	homeOrientation      *Quat
	initialZoom          *float64
	maxZoomOutFraction   *float64
	showScaleBar         *bool
	scaleBarColor        *color.RGBA
	scaleUnit            *string
	scaleUnitScale       *float64
	showFPS              *bool
	fpsColor             *color.RGBA
	fpsStyle             *fyne.TextStyle
	fpsSize              *float32
}

// CubeColors configures the colors of the orientation cube.
type CubeColors struct {
	Faces      [6]color.RGBA // Z+, Z-, X+, X-, Y+, Y-
	EdgeColor  color.RGBA
	LabelColor color.RGBA
	AxisColors [3]color.RGBA // X, Y, Z
}

// DefaultCubeColors returns the default orientation cube colors.
func DefaultCubeColors() CubeColors {
	return CubeColors{
		Faces: [6]color.RGBA{
			{80, 80, 200, 200}, // Z+
			{80, 80, 140, 200}, // Z-
			{200, 80, 80, 200}, // X+
			{140, 80, 80, 200}, // X-
			{80, 200, 80, 200}, // Y+
			{80, 140, 80, 200}, // Y-
		},
		EdgeColor:  color.RGBA{200, 200, 200, 255},
		LabelColor: color.RGBA{255, 255, 255, 255},
		AxisColors: [3]color.RGBA{
			{255, 80, 80, 255}, // X
			{80, 255, 80, 255}, // Y
			{80, 80, 255, 255}, // Z
		},
	}
}

// WithBackgroundColor sets the canvas background color.
func WithBackgroundColor(c color.RGBA) Option {
	return func(cfg *config) { cfg.bgColor = &c }
}

// WithDefaultPointColor sets the fallback color for points without RGB data.
func WithDefaultPointColor(c color.RGBA) Option {
	return func(cfg *config) { cfg.defaultPointColor = &c }
}

// WithCubeColors sets the orientation cube colors.
func WithCubeColors(cc CubeColors) Option {
	return func(cfg *config) { cfg.cubeColors = &cc }
}

// WithOrientationCube controls whether the orientation cube is displayed.
func WithOrientationCube(show bool) Option {
	return func(cfg *config) { cfg.showCube = &show }
}

// WithHomeButton controls whether the home button is displayed.
func WithHomeButton(show bool) Option {
	return func(cfg *config) { cfg.showHome = &show }
}

// WithZoomFitButton controls whether the zoom-fit button is displayed.
func WithZoomFitButton(show bool) Option {
	return func(cfg *config) { cfg.showZoomFit = &show }
}

// WithInfoLabel controls whether the point info label is displayed.
func WithInfoLabel(show bool) Option {
	return func(cfg *config) { cfg.showInfoLabel = &show }
}

// WithInfoLabelColor sets the info label text color.
func WithInfoLabelColor(c color.RGBA) Option {
	return func(cfg *config) { cfg.infoLabelColor = &c }
}

// WithInfoLabelStyle sets the info label text style (font).
func WithInfoLabelStyle(s fyne.TextStyle) Option {
	return func(cfg *config) { cfg.infoLabelStyle = &s }
}

// WithInfoLabelSize sets the info label font size. The default is 12.
func WithInfoLabelSize(size float32) Option {
	return func(cfg *config) { cfg.infoLabelSize = &size }
}

func float32Or(p *float32, def float32) float32 {
	if p != nil {
		return *p
	}
	return def
}

// WithHomeOrientation sets the default home view orientation.
func WithHomeOrientation(q Quat) Option {
	return func(cfg *config) { cfg.homeOrientation = &q }
}

// WithInitialZoom sets the starting zoom level.
func WithInitialZoom(z float64) Option {
	return func(cfg *config) { cfg.initialZoom = &z }
}

// WithMaxZoomOutFraction sets the minimum visible fraction of the largest
// canvas dimension that the point cloud must occupy. For example, 0.2 means
// the user cannot zoom out past the point where the cloud covers less than
// 20% of the viewport. The default is 0.2.
func WithMaxZoomOutFraction(f float64) Option {
	return func(cfg *config) { cfg.maxZoomOutFraction = &f }
}

// WithFPS controls whether the FPS counter is displayed.
func WithFPS(show bool) Option {
	return func(cfg *config) { cfg.showFPS = &show }
}

// WithFPSColor sets the FPS counter text color.
func WithFPSColor(c color.RGBA) Option {
	return func(cfg *config) { cfg.fpsColor = &c }
}

// WithFPSStyle sets the FPS counter text style.
func WithFPSStyle(s fyne.TextStyle) Option {
	return func(cfg *config) { cfg.fpsStyle = &s }
}

// WithFPSSize sets the FPS counter font size. The default is 14.
func WithFPSSize(size float32) Option {
	return func(cfg *config) { cfg.fpsSize = &size }
}

// WithScaleBar controls whether the scale bar is displayed.
func WithScaleBar(show bool) Option {
	return func(cfg *config) { cfg.showScaleBar = &show }
}

// WithScaleBarColor sets the scale bar color.
func WithScaleBarColor(c color.RGBA) Option {
	return func(cfg *config) { cfg.scaleBarColor = &c }
}

// WithScaleUnit sets the unit label for the scale bar (e.g. "m").
func WithScaleUnit(unit string) Option {
	return func(cfg *config) { cfg.scaleUnit = &unit }
}

// WithScaleUnitScale sets the unit multiplier for the scale bar.
// For example, if data is in meters and you want to display millimeters,
// set this to 1000.
func WithScaleUnitScale(s float64) Option {
	return func(cfg *config) { cfg.scaleUnitScale = &s }
}

func stringOr(p *string, def string) string {
	if p != nil {
		return *p
	}
	return def
}

func boolOr(p *bool, def bool) bool {
	if p != nil {
		return *p
	}
	return def
}

func colorOr(p *color.RGBA, def color.RGBA) color.RGBA {
	if p != nil {
		return *p
	}
	return def
}

func float64Or(p *float64, def float64) float64 {
	if p != nil {
		return *p
	}
	return def
}

func quatOr(p *Quat, def Quat) Quat {
	if p != nil {
		return *p
	}
	return def
}
