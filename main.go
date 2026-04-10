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

	"github.com/borud/pointcloud/pkg/pcviewer"
	"github.com/borud/pointcloud/pkg/pointcloud"
)

// tealTheme wraps the default dark theme with a teal background.
type tealTheme struct{}

func (t *tealTheme) Color(name fyne.ThemeColorName, variant fyne.ThemeVariant) color.Color {
	if name == theme.ColorNameBackground {
		return color.RGBA{52, 58, 64, 255}
	}
	return theme.DefaultTheme().Color(name, variant)
}

func (t *tealTheme) Font(style fyne.TextStyle) fyne.Resource {
	return theme.DefaultTheme().Font(style)
}

func (t *tealTheme) Icon(name fyne.ThemeIconName) fyne.Resource {
	return theme.DefaultTheme().Icon(name)
}

func (t *tealTheme) Size(name fyne.ThemeSizeName) float32 {
	return theme.DefaultTheme().Size(name)
}

func main() {
	pwd := flag.String("pwd", "", "starting directory for the file selector")
	axis := flag.String("axis", "zup", "up axis: yup or zup")
	flag.Parse()

	myApp := app.NewWithID("no.borud.pointcloud")
	myApp.Settings().SetTheme(&tealTheme{})
	myWindow := myApp.NewWindow("Point Cloud Viewer")

	v := pcviewer.New()
	if strings.EqualFold(*axis, "zup") {
		v.SetUpAxis(pcviewer.ZUp)
	}

	statusLabel := widget.NewLabel("No file loaded")

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

	// Inner rounded rectangle sits behind the viewer.
	innerBg := canvas.NewRectangle(color.RGBA{0, 0, 0, 255})
	innerBg.CornerRadius = 12

	const pad float32 = 40
	const radius float32 = 12
	viewerArea := container.NewStack(
		container.New(
			layout.NewCustomPaddedLayout(pad-radius, pad-radius, pad-radius, pad-radius),
			innerBg,
		),
		container.New(
			layout.NewCustomPaddedLayout(pad, pad, pad, pad),
			v,
		),
	)

	top := container.NewBorder(nil, nil, nil, statusLabel, toolbar)
	content := container.NewBorder(top, nil, nil, nil, viewerArea)

	myWindow.SetContent(content)
	myWindow.Resize(fyne.NewSize(800, 600))
	myWindow.ShowAndRun()
}
