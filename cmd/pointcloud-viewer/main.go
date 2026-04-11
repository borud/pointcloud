// Package main implements the Point Cloud Viewer application.
package main

import (
	"flag"
	"fmt"
	"image/color"
	"path/filepath"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/borud/pointcloud"
)

// --- defaults ---

var (
	defaultBgColor        = color.RGBA{0, 0, 0, 255}
	defaultPointColor     = color.RGBA{255, 150, 255, 255}
	defaultInfoLabelColor = color.RGBA{255, 255, 255, 255}
	defaultInfoLabelStyle = fyne.TextStyle{Monospace: true}
)

// colorSwatch creates a clickable colored rectangle that opens a color picker.
func colorSwatch(c color.RGBA, w fyne.Window, onChange func(color.RGBA)) *fyne.Container {
	rect := canvas.NewRectangle(c)
	rect.SetMinSize(fyne.NewSize(24, 24))
	rect.CornerRadius = 4
	rect.StrokeColor = color.RGBA{180, 180, 180, 255}
	rect.StrokeWidth = 1

	btn := newTappable(func() {
		dlg := dialog.NewColorPicker("Pick Color", "", func(picked color.Color) {
			r, g, b, a := picked.RGBA()
			rgba := color.RGBA{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8), uint8(a >> 8)}
			rect.FillColor = rgba
			rect.Refresh()
			onChange(rgba)
		}, w)
		dlg.Advanced = true
		dlg.SetColor(rect.FillColor)
		dlg.Show()
	})

	return container.NewStack(rect, btn)
}

// colorRow creates a label + color swatch row.
func colorRow(label string, c color.RGBA, w fyne.Window, onChange func(color.RGBA)) (*fyne.Container, *canvas.Rectangle) {
	swatch := colorSwatch(c, w, onChange)
	rect := swatch.Objects[0].(*canvas.Rectangle)
	return container.NewHBox(swatch, widget.NewLabel(label)), rect
}

// tappable is a transparent widget that handles Tapped events.
type tappable struct {
	widget.BaseWidget
	onTap func()
}

func newTappable(onTap func()) *tappable {
	t := &tappable{onTap: onTap}
	t.ExtendBaseWidget(t)
	return t
}

func (t *tappable) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(canvas.NewRectangle(color.Transparent))
}

func (t *tappable) Tapped(_ *fyne.PointEvent) {
	if t.onTap != nil {
		t.onTap()
	}
}

func main() {
	showVersion := flag.Bool("version", false, "print version and exit")
	pwd := flag.String("pwd", "", "starting directory for the file selector")
	axis := flag.String("axis", "zup", "up axis: yup or zup")
	flag.Parse()

	if *showVersion {
		fmt.Println("pointcloud", Version)
		return
	}

	myApp := app.NewWithID("no.borud.pointcloud")
	myWindow := myApp.NewWindow("Point Cloud Viewer")

	// Current state for viewer reconstruction.
	bgColor := defaultBgColor
	pointColor := defaultPointColor
	infoLabelColor := defaultInfoLabelColor
	infoLabelStyle := defaultInfoLabelStyle
	cubeColors := pointcloud.DefaultCubeColors()
	showCube := true
	showHome := true
	showZoomFit := true
	showInfo := true
	showScaleBar := true
	showFPS := false
	lodEnabled := false
	fpsColor := color.RGBA{200, 200, 200, 255}
	fpsStyle := fyne.TextStyle{Monospace: true}
	scaleBarColor := color.RGBA{200, 200, 200, 255}
	scaleUnit := ""
	scaleUnitScale := 1.0
	var currentNormScale float64

	// Points storage so we can reload after viewer rebuild.
	var currentPoints []pointcloud.Point3D
	var currentUpAxis pointcloud.UpAxis
	if strings.EqualFold(*axis, "zup") {
		currentUpAxis = pointcloud.ZUp
	}

	// Build the viewer with current settings.
	buildViewer := func() *pointcloud.Viewer {
		v := pointcloud.New(
			pointcloud.WithBackgroundColor(bgColor),
			pointcloud.WithDefaultPointColor(pointColor),
			pointcloud.WithInfoLabelColor(infoLabelColor),
			pointcloud.WithInfoLabelStyle(infoLabelStyle),
			pointcloud.WithCubeColors(cubeColors),
			pointcloud.WithOrientationCube(showCube),
			pointcloud.WithHomeButton(showHome),
			pointcloud.WithZoomFitButton(showZoomFit),
			pointcloud.WithInfoLabel(showInfo),
			pointcloud.WithScaleBar(showScaleBar),
			pointcloud.WithScaleBarColor(scaleBarColor),
			pointcloud.WithScaleUnit(scaleUnit),
			pointcloud.WithScaleUnitScale(scaleUnitScale),
			pointcloud.WithFPS(showFPS),
			pointcloud.WithFPSColor(fpsColor),
			pointcloud.WithFPSStyle(fpsStyle),
		)
		v.SetUpAxis(currentUpAxis)
		v.SetLODEnabled(lodEnabled)
		if currentNormScale > 0 {
			v.SetScale(currentNormScale)
		}
		if len(currentPoints) > 0 {
			v.SetPointsPreserveView(currentPoints)
		}
		return v
	}

	v := buildViewer()
	statusLabel := widget.NewLabel("No file loaded")

	const pad float32 = 40
	viewerArea := container.New(
		layout.NewCustomPaddedLayout(pad, pad, pad, pad),
		v,
	)

	// rebuildViewer replaces the viewer in the layout, preserving the
	// current orientation, zoom, and pan.
	rebuildViewer := func() {
		oldOrientation := v.Orientation()
		oldZoom := v.Zoom()
		oldPanX, oldPanY := v.Pan()

		v = buildViewer()

		v.SetOrientation(oldOrientation)
		v.SetZoom(oldZoom)
		v.SetPan(oldPanX, oldPanY)

		viewerArea.Objects[0] = v
		viewerArea.Refresh()
	}

	// --- Toolbar ---
	toolbar := widget.NewToolbar(
		widget.NewToolbarAction(theme.FolderOpenIcon(), func() {
			dlg := dialog.NewFileOpen(func(rc fyne.URIReadCloser, err error) {
				if err != nil {
					dialog.ShowError(err, myWindow)
					return
				}
				if rc == nil {
					return
				}
				defer rc.Close()

				path := rc.URI().Path()
				fname := filepath.Base(path)
				statusLabel.SetText(fmt.Sprintf("Loading %s...", fname))

				pc, err := pointcloud.ReadFile(path)
				if err != nil {
					dialog.ShowError(err, myWindow)
					statusLabel.SetText("Error loading file")
					return
				}

				pc.Normalize()
				currentNormScale = pc.NormScale
				currentPoints = pc.Points
				v.SetScale(pc.NormScale)
				v.SetPoints(pc.Points)
				statusLabel.SetText(fmt.Sprintf("%s - %d points", fname, len(pc.Points)))
			}, myWindow)

			exts := pointcloud.SupportedExtensions()
			dlg.SetFilter(storage.NewExtensionFileFilter(exts))
			if *pwd != "" {
				if uri, err := storage.ListerForURI(storage.NewFileURI(*pwd)); err == nil {
					dlg.SetLocation(uri)
				}
			}
			dlg.Resize(fyne.NewSize(800, 500))
			dlg.Show()
		}),
	)

	// --- Settings panel ---
	// We collect all swatch rectangles so the reset button can update them.
	type swatchEntry struct {
		rect     *canvas.Rectangle
		getColor func() color.RGBA
	}
	var swatches []swatchEntry

	trackSwatch := func(rect *canvas.Rectangle, getColor func() color.RGBA) {
		swatches = append(swatches, swatchEntry{rect, getColor})
	}

	// Canvas colors section.
	bgRow, bgRect := colorRow("WithBackgroundColor", bgColor, myWindow, func(c color.RGBA) {
		bgColor = c
		v.SetBackgroundColor(c)
	})
	trackSwatch(bgRect, func() color.RGBA { return bgColor })

	ptRow, ptRect := colorRow("WithDefaultPointColor", pointColor, myWindow, func(c color.RGBA) {
		pointColor = c
		v.SetDefaultPointColor(c)
	})
	trackSwatch(ptRect, func() color.RGBA { return pointColor })

	infoRow, infoRect := colorRow("WithInfoLabelColor", infoLabelColor, myWindow, func(c color.RGBA) {
		infoLabelColor = c
		v.SetInfoLabelColor(c)
	})
	trackSwatch(infoRect, func() color.RGBA { return infoLabelColor })

	sbColorRow, sbColorRect := colorRow("WithScaleBarColor", scaleBarColor, myWindow, func(c color.RGBA) {
		scaleBarColor = c
		v.SetScaleBarColor(c)
	})
	trackSwatch(sbColorRect, func() color.RGBA { return scaleBarColor })

	canvasSection := widget.NewCard("Canvas", "Background, point, and label colors",
		container.NewVBox(bgRow, ptRow, infoRow, sbColorRow),
	)

	// Info label style section.
	fontOptions := []string{"Regular", "Monospace", "Bold", "Italic", "Bold Italic"}
	styleFromName := func(name string) fyne.TextStyle {
		switch name {
		case "Monospace":
			return fyne.TextStyle{Monospace: true}
		case "Bold":
			return fyne.TextStyle{Bold: true}
		case "Italic":
			return fyne.TextStyle{Italic: true}
		case "Bold Italic":
			return fyne.TextStyle{Bold: true, Italic: true}
		default:
			return fyne.TextStyle{}
		}
	}
	nameFromStyle := func(s fyne.TextStyle) string {
		switch {
		case s.Monospace:
			return "Monospace"
		case s.Bold && s.Italic:
			return "Bold Italic"
		case s.Bold:
			return "Bold"
		case s.Italic:
			return "Italic"
		default:
			return "Regular"
		}
	}

	fontSelect := widget.NewSelect(fontOptions, func(name string) {
		infoLabelStyle = styleFromName(name)
		v.SetInfoLabelStyle(infoLabelStyle)
	})
	fontSelect.SetSelected(nameFromStyle(infoLabelStyle))

	fontSection := widget.NewCard("Info Label", "Style of the point info text",
		container.NewVBox(
			widget.NewLabel("WithInfoLabelStyle"),
			fontSelect,
		),
	)

	// Cube colors section.
	faceLabels := [6]string{"Faces[0] Z+", "Faces[1] Z-", "Faces[2] X+", "Faces[3] X-", "Faces[4] Y+", "Faces[5] Y-"}
	var cubeColorRows []fyne.CanvasObject
	for idx := range 6 {
		i := idx
		row, rect := colorRow(faceLabels[i], cubeColors.Faces[i], myWindow, func(c color.RGBA) {
			cubeColors.Faces[i] = c
			v.SetCubeColors(cubeColors)
		})
		trackSwatch(rect, func() color.RGBA { return cubeColors.Faces[i] })
		cubeColorRows = append(cubeColorRows, row)
	}

	edgeRow, edgeRect := colorRow("EdgeColor", cubeColors.EdgeColor, myWindow, func(c color.RGBA) {
		cubeColors.EdgeColor = c
		v.SetCubeColors(cubeColors)
	})
	trackSwatch(edgeRect, func() color.RGBA { return cubeColors.EdgeColor })
	cubeColorRows = append(cubeColorRows, edgeRow)

	lblRow, lblRect := colorRow("LabelColor", cubeColors.LabelColor, myWindow, func(c color.RGBA) {
		cubeColors.LabelColor = c
		v.SetCubeColors(cubeColors)
	})
	trackSwatch(lblRect, func() color.RGBA { return cubeColors.LabelColor })
	cubeColorRows = append(cubeColorRows, lblRow)

	axisNames := [3]string{"AxisColors[0] X", "AxisColors[1] Y", "AxisColors[2] Z"}
	for idx := range 3 {
		i := idx
		row, rect := colorRow(axisNames[i], cubeColors.AxisColors[i], myWindow, func(c color.RGBA) {
			cubeColors.AxisColors[i] = c
			v.SetCubeColors(cubeColors)
		})
		trackSwatch(rect, func() color.RGBA { return cubeColors.AxisColors[i] })
		cubeColorRows = append(cubeColorRows, row)
	}

	cubeSection := widget.NewCard("CubeColors", "Colors for the orientation cube faces, edges, and axes",
		container.NewVBox(cubeColorRows...),
	)

	// Zoom-out fraction slider.
	zoomOutFraction := 0.2
	zoomOutLabel := widget.NewLabel(fmt.Sprintf("MaxZoomOutFraction: %.0f%%", zoomOutFraction*100))
	zoomOutSlider := widget.NewSlider(0.05, 1.0)
	zoomOutSlider.Step = 0.05
	zoomOutSlider.Value = zoomOutFraction
	zoomOutSlider.OnChanged = func(val float64) {
		zoomOutFraction = val
		zoomOutLabel.SetText(fmt.Sprintf("MaxZoomOutFraction: %.0f%%", val*100))
		v.SetMaxZoomOutFraction(val)
	}

	zoomSection := widget.NewCard("Zoom", "How far the user can zoom out",
		container.NewVBox(zoomOutLabel, zoomOutSlider),
	)

	// Visibility toggles (these rebuild the viewer).
	cubeCheck := widget.NewCheck("WithOrientationCube", func(on bool) {
		showCube = on
		rebuildViewer()
	})
	cubeCheck.SetChecked(showCube)

	homeCheck := widget.NewCheck("WithHomeButton", func(on bool) {
		showHome = on
		rebuildViewer()
	})
	homeCheck.SetChecked(showHome)

	zoomFitCheck := widget.NewCheck("WithZoomFitButton", func(on bool) {
		showZoomFit = on
		rebuildViewer()
	})
	zoomFitCheck.SetChecked(showZoomFit)

	infoCheck := widget.NewCheck("WithInfoLabel", func(on bool) {
		showInfo = on
		rebuildViewer()
	})
	infoCheck.SetChecked(showInfo)

	fpsCheck := widget.NewCheck("WithFPS", func(on bool) {
		showFPS = on
		rebuildViewer()
	})
	fpsCheck.SetChecked(showFPS)

	visSection := widget.NewCard("Visibility", "Show or hide overlay elements",
		container.NewVBox(cubeCheck, homeCheck, zoomFitCheck, infoCheck, fpsCheck),
	)

	// Rendering settings.
	zupCheck := widget.NewCheck("Z-up", func(on bool) {
		if on {
			currentUpAxis = pointcloud.ZUp
		} else {
			currentUpAxis = pointcloud.YUp
		}
		v.SetUpAxis(currentUpAxis)
	})
	zupCheck.SetChecked(currentUpAxis == pointcloud.ZUp)

	lodCheck := widget.NewCheck("LOD decimation", func(on bool) {
		lodEnabled = on
		v.SetLODEnabled(on)
	})
	lodCheck.SetChecked(lodEnabled)

	renderSection := widget.NewCard("Rendering", "Controls that affect how the point cloud is rendered",
		container.NewVBox(
			zupCheck,
			widget.NewLabel("  Treat Z as up axis (typical for LiDAR/surveying data)"),
			lodCheck,
			widget.NewLabel("  Reduce point count during interaction for faster frame rates"),
		),
	)

	// FPS display settings.
	fpsColorRow, fpsColorRect := colorRow("WithFPSColor", fpsColor, myWindow, func(c color.RGBA) {
		fpsColor = c
		v.SetFPSColor(c)
	})
	trackSwatch(fpsColorRect, func() color.RGBA { return fpsColor })

	fpsFontSelect := widget.NewSelect(fontOptions, func(name string) {
		fpsStyle = styleFromName(name)
		v.SetFPSStyle(fpsStyle)
	})
	fpsFontSelect.SetSelected(nameFromStyle(fpsStyle))

	fpsSection := widget.NewCard("FPS Display", "Color and style of the FPS counter",
		container.NewVBox(
			fpsColorRow,
			widget.NewLabel("WithFPSStyle"),
			fpsFontSelect,
		),
	)

	// Scale bar settings.
	unitEntry := widget.NewEntry()
	unitEntry.SetPlaceHolder("unit (e.g. m)")
	unitEntry.SetText(scaleUnit)
	unitEntry.OnChanged = func(s string) {
		scaleUnit = s
		v.SetScaleUnit(s)
	}

	unitScaleEntry := widget.NewEntry()
	unitScaleEntry.SetPlaceHolder("multiplier (e.g. 1000)")
	unitScaleEntry.SetText(fmt.Sprintf("%g", scaleUnitScale))
	unitScaleEntry.OnChanged = func(s string) {
		var val float64
		if _, err := fmt.Sscanf(s, "%f", &val); err == nil && val > 0 {
			scaleUnitScale = val
			v.SetScaleUnitScale(val)
		}
	}

	scaleBarCheck := widget.NewCheck("WithScaleBar", func(on bool) {
		showScaleBar = on
		rebuildViewer()
	})
	scaleBarCheck.SetChecked(showScaleBar)

	scaleSection := widget.NewCard("Scale Bar", "Unit and multiplier for the scale indicator",
		container.NewVBox(
			scaleBarCheck,
			widget.NewLabel("WithScaleUnit"),
			unitEntry,
			widget.NewLabel("WithScaleUnitScale"),
			unitScaleEntry,
		),
	)

	// Reset all to defaults.
	resetBtn := widget.NewButton("Reset All to Defaults", func() {
		bgColor = defaultBgColor
		pointColor = defaultPointColor
		infoLabelColor = defaultInfoLabelColor
		infoLabelStyle = defaultInfoLabelStyle
		cubeColors = pointcloud.DefaultCubeColors()
		showCube = true
		showHome = true
		showZoomFit = true
		showInfo = true
		showScaleBar = true
		showFPS = false
		lodEnabled = false
		currentUpAxis = pointcloud.ZUp
		fpsColor = color.RGBA{200, 200, 200, 255}
		fpsStyle = fyne.TextStyle{Monospace: true}
		scaleBarColor = color.RGBA{200, 200, 200, 255}
		scaleUnit = ""
		scaleUnitScale = 1.0

		// Update swatch colors.
		for _, s := range swatches {
			s.rect.FillColor = s.getColor()
			s.rect.Refresh()
		}

		// Update font selector and zoom slider.
		fontSelect.SetSelected(nameFromStyle(infoLabelStyle))
		zoomOutFraction = 0.2
		zoomOutSlider.Value = 0.2
		zoomOutSlider.Refresh()
		zoomOutLabel.SetText(fmt.Sprintf("MaxZoomOutFraction: %.0f%%", zoomOutFraction*100))

		// Update scale bar entries.
		unitEntry.SetText("")
		unitScaleEntry.SetText("1")

		// Update checkboxes.
		cubeCheck.SetChecked(true)
		homeCheck.SetChecked(true)
		zoomFitCheck.SetChecked(true)
		infoCheck.SetChecked(true)
		scaleBarCheck.SetChecked(true)
		fpsCheck.SetChecked(false)
		fpsFontSelect.SetSelected(nameFromStyle(fpsStyle))
		zupCheck.SetChecked(true)
		lodCheck.SetChecked(false)

		rebuildViewer()
	})

	settingsContent := container.NewVBox(renderSection, canvasSection, fontSection, zoomSection, scaleSection, cubeSection, visSection, fpsSection, resetBtn)
	settingsScroll := container.NewVScroll(settingsContent)
	settingsScroll.SetMinSize(fyne.NewSize(240, 0))

	settingsPanel := container.New(
		layout.NewCustomPaddedLayout(pad, pad, pad, pad),
		settingsScroll,
	)

	top := container.NewBorder(nil, nil, nil,
		statusLabel,
		toolbar,
	)

	mainContent := container.NewBorder(top, nil, nil, settingsPanel, viewerArea)

	myWindow.SetContent(mainContent)
	myWindow.Resize(fyne.NewSize(1100, 700))
	myWindow.ShowAndRun()
}
