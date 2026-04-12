// Package main loads a point cloud model and continuously rotates it.
//
// By default the model completes one full revolution every two seconds.
// Controls let you adjust the rotation speed, axis, direction, and model.
package main

import (
	"fmt"
	"image/color"
	"math"
	"os"
	"sync/atomic"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	"github.com/borud/pointcloud"
)

const (
	defaultRPM   = 30.0 // 1 revolution per 2 seconds = 30 RPM
	tickInterval = 16 * time.Millisecond
)

var models = map[string]string{
	"Goat Skull": "../../data/goatscull.ply",
	"Chair":      "../../data/chair.ply",
}

func main() {
	myApp := app.NewWithID("no.borud.pointcloud.rotate")
	myWindow := myApp.NewWindow("Point Cloud — Rotation Demo")

	viewer := pointcloud.New(
		pointcloud.WithBackgroundColor(color.RGBA{20, 20, 30, 255}),
		pointcloud.WithOrientationCube(true),
		pointcloud.WithFPS(true),
	)
	viewer.SetLODEnabled(false)

	statusLabel := widget.NewLabel("Loading...")
	statusLabel.TextStyle = fyne.TextStyle{Monospace: true}

	rpm := defaultRPM
	axisX, axisY, axisZ := 0.0, 0.0, 1.0 // default: Z axis
	direction := 1.0                       // 1.0 = CCW, -1.0 = CW
	pitch := -0.3                          // camera pitch (radians, matches HomeOrientation)
	yaw := -math.Pi / 4                   // camera yaw (radians, matches HomeOrientation)
	var loading atomic.Bool

	loadModel := func(name string) {
		path, ok := models[name]
		if !ok || !loading.CompareAndSwap(false, true) {
			return
		}
		fyne.Do(func() {
			statusLabel.SetText(fmt.Sprintf("Loading %s...", name))
		})
		go func() {
			defer loading.Store(false)
			pc, err := pointcloud.ReadFile(path)
			if err != nil {
				fyne.Do(func() {
					statusLabel.SetText(fmt.Sprintf("Error: %v", err))
				})
				fmt.Fprintf(os.Stderr, "failed to load %s: %v\n", path, err)
				return
			}
			pc.ComputeBounds()
			pc.Normalize()
			viewer.SetPoints(pc.Points)
			fyne.Do(func() {
				statusLabel.SetText(fmt.Sprintf("%s — %d points", name, len(pc.Points)))
			})
		}()
	}

	// Model selector.
	modelNames := []string{"Goat Skull", "Chair"}
	modelSelect := widget.NewSelect(modelNames, func(val string) {
		loadModel(val)
	})

	// Speed controls.
	speedLabel := widget.NewLabel(fmt.Sprintf("Speed: %.0f RPM", rpm))
	speedLabel.TextStyle = fyne.TextStyle{Monospace: true}

	speedSlider := widget.NewSlider(0, 600)
	speedSlider.Step = 1
	speedSlider.Value = rpm
	speedSlider.OnChanged = func(val float64) {
		rpm = val
		speedLabel.SetText(fmt.Sprintf("Speed: %.0f RPM", val))
	}

	sliderSized := container.New(newMinWidthLayout(300), speedSlider)

	// Axis and direction selectors.
	axisSelect := widget.NewSelect([]string{"X", "Y", "Z"}, func(val string) {
		switch val {
		case "X":
			axisX, axisY, axisZ = 1, 0, 0
		case "Y":
			axisX, axisY, axisZ = 0, 1, 0
		case "Z":
			axisX, axisY, axisZ = 0, 0, 1
		}
	})
	axisSelect.SetSelected("Z")

	dirSelect := widget.NewSelect([]string{"CCW", "CW"}, func(val string) {
		if val == "CW" {
			direction = -1.0
		} else {
			direction = 1.0
		}
	})
	dirSelect.SetSelected("CCW")

	fpsCheck := widget.NewCheck("FPS", func(on bool) {
		viewer.SetFPSEnabled(on)
	})
	fpsCheck.SetChecked(true)

	// Camera orientation sliders.
	pitchLabel := widget.NewLabel(fmt.Sprintf("Pitch: %+.0f°", pitch*180/math.Pi))
	pitchLabel.TextStyle = fyne.TextStyle{Monospace: true}
	pitchSlider := widget.NewSlider(-90, 90)
	pitchSlider.Step = 1
	pitchSlider.Value = pitch * 180 / math.Pi
	pitchSlider.OnChanged = func(val float64) {
		pitch = val * math.Pi / 180
		pitchLabel.SetText(fmt.Sprintf("Pitch: %+.0f°", val))
	}
	pitchSized := container.New(newMinWidthLayout(200), pitchSlider)

	yawLabel := widget.NewLabel(fmt.Sprintf("Yaw: %+.0f°", yaw*180/math.Pi))
	yawLabel.TextStyle = fyne.TextStyle{Monospace: true}
	yawSlider := widget.NewSlider(-180, 180)
	yawSlider.Step = 1
	yawSlider.Value = yaw * 180 / math.Pi
	yawSlider.OnChanged = func(val float64) {
		yaw = val * math.Pi / 180
		yawLabel.SetText(fmt.Sprintf("Yaw: %+.0f°", val))
	}
	yawSized := container.New(newMinWidthLayout(200), yawSlider)

	topRow := container.NewHBox(
		widget.NewLabel("Model:"), modelSelect,
		widget.NewLabel("Axis:"), axisSelect,
		widget.NewLabel("Dir:"), dirSelect,
		fpsCheck,
		speedLabel, sliderSized,
	)

	bottomRow := container.NewBorder(
		nil, nil,
		statusLabel,
		container.NewHBox(
			pitchLabel, pitchSized,
			yawLabel, yawSized,
		),
	)

	controls := container.NewVBox(topRow, bottomRow)

	content := container.NewBorder(
		nil,
		container.New(layout.NewCustomPaddedLayout(4, 4, 8, 8), controls),
		nil, nil,
		viewer,
	)

	myWindow.SetContent(content)
	myWindow.Resize(fyne.NewSize(1100, 750))

	// Load default model and start rotation.
	modelSelect.SetSelected("Goat Skull")

	go func() {
		ticker := time.NewTicker(tickInterval)
		defer ticker.Stop()

		var angle float64
		last := time.Now()

		for range ticker.C {
			now := time.Now()
			dt := now.Sub(last).Seconds()
			last = now

			currentRPM := rpm
			radiansPerSec := currentRPM * 2 * math.Pi / 60.0
			angle += radiansPerSec * dt

			spin := pointcloud.QuatFromAxisAngle(axisX, axisY, axisZ, angle*direction)
			camera := pointcloud.QuatFromEulerXY(pitch, yaw)
			viewer.SetOrientation(camera.Mul(spin))
		}
	}()

	myWindow.ShowAndRun()
}

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
