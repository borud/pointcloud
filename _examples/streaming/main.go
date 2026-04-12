// Package main demonstrates streaming point cloud data to the viewer widget.
//
// It maintains a sliding window buffer of timestamped batches and calls
// SetPointsPreserveView on each update — the idiomatic pattern for live data.
package main

import (
	"context"
	"fmt"
	"image/color"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	"github.com/borud/pointcloud"
)

// minWidthLayout is a Fyne layout that enforces a minimum width on its
// single child while using the child's natural height.
type minWidthLayout struct {
	minWidth float32
}

func newMinWidthLayout(minWidth float32) *minWidthLayout {
	return &minWidthLayout{minWidth: minWidth}
}

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

const (
	batchSize      = 500
	updateRate     = 100 * time.Millisecond
	maxPoints      = 50_000
	defaultDecay   = 4.0 // seconds
)

func main() {
	myApp := app.NewWithID("no.borud.pointcloud.streaming")
	myWindow := myApp.NewWindow("Streaming Point Cloud Demo")

	viewer := pointcloud.New(
		pointcloud.WithBackgroundColor(color.RGBA{15, 15, 25, 255}),
		pointcloud.WithOrientationCube(true),
		pointcloud.WithFPS(true),
		pointcloud.WithMaxZoomOutFraction(0.25),
	)

	statusLabel := widget.NewLabel("Starting...")
	statusLabel.TextStyle = fyne.TextStyle{Monospace: true}

	buf := NewStreamBuffer(maxPoints, time.Duration(defaultDecay*float64(time.Second)))
	gen := NewLidarGenerator()

	// Decay slider: 0.1s – 10s.
	decayLabel := widget.NewLabel(fmt.Sprintf("Decay: %.1fs", defaultDecay))
	decayLabel.TextStyle = fyne.TextStyle{Monospace: true}
	decaySlider := widget.NewSlider(0.1, 10.0)
	decaySlider.Step = 0.1
	decaySlider.Value = defaultDecay
	decaySlider.OnChanged = func(val float64) {
		decayLabel.SetText(fmt.Sprintf("Decay: %.1fs", val))
		buf.SetMaxAge(time.Duration(val * float64(time.Second)))
	}

	// Use a spacer-based layout to give the slider a fixed width.
	sliderSized := container.New(newMinWidthLayout(400), decaySlider)

	fpsCheck := widget.NewCheck("FPS", func(on bool) {
		viewer.SetFPSEnabled(on)
	})
	fpsCheck.SetChecked(true)

	bottomBar := container.NewBorder(
		nil, nil,
		statusLabel,
		container.NewHBox(fpsCheck, decayLabel, sliderSized),
	)

	content := container.NewBorder(
		nil,
		container.New(layout.NewCustomPaddedLayout(4, 4, 8, 8), bottomBar),
		nil, nil,
		viewer,
	)

	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		ticker := time.NewTicker(updateRate)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				pts := gen.Generate(batchSize)
				buf.Add(pts)
				flat := buf.Flatten()
				viewer.SetPointsPreserveView(flat)

				batches, points, oldest := buf.Stats()
				text := fmt.Sprintf("Batches: %d | Points: %d | Oldest: %.1fs",
					batches, points, oldest.Seconds())
				fyne.Do(func() {
					statusLabel.SetText(text)
				})
			}
		}
	}()

	myWindow.SetOnClosed(func() {
		cancel()
	})

	myWindow.SetContent(content)
	myWindow.Resize(fyne.NewSize(900, 700))
	myWindow.ShowAndRun()
}
