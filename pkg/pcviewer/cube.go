package pcviewer

import (
	"image"
	"image/color"
	"math"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/widget"

	"github.com/borud/pointcloud/pkg/raster"
)

// cubeFace defines a face of the orientation cube.
type cubeFace struct {
	verts    [4]int
	color    color.RGBA
	label    string
	snapQuat Quat
	normal   [3]float64
}

// snapTarget represents a clickable target on the cube (edge midpoint or corner).
type snapTarget struct {
	pos      [3]float64
	snapQuat Quat
}

// cube vertices for a unit cube centered at origin.
var cubeVerts = [8][3]float64{
	{-1, -1, -1}, {1, -1, -1}, {1, 1, -1}, {-1, 1, -1},
	{-1, -1, 1}, {1, -1, 1}, {1, 1, 1}, {-1, 1, 1},
}

func snapQuatFromDir(dx, dy, dz float64) Quat {
	ay := math.Atan2(-dx, -dz)
	ax := math.Atan2(dy, math.Sqrt(dx*dx+dz*dz))
	return QuatFromEulerXY(ax, ay)
}

var cubeFaces = [6]cubeFace{
	{verts: [4]int{4, 5, 6, 7}, color: color.RGBA{80, 80, 200, 200}, label: "Z+",
		snapQuat: QuatFromEulerXY(0, math.Pi), normal: [3]float64{0, 0, 1}},
	{verts: [4]int{1, 0, 3, 2}, color: color.RGBA{80, 80, 140, 200}, label: "Z-",
		snapQuat: QuatFromEulerXY(0, 0), normal: [3]float64{0, 0, -1}},
	{verts: [4]int{5, 1, 2, 6}, color: color.RGBA{200, 80, 80, 200}, label: "X+",
		snapQuat: QuatFromEulerXY(0, -math.Pi/2), normal: [3]float64{1, 0, 0}},
	{verts: [4]int{0, 4, 7, 3}, color: color.RGBA{140, 80, 80, 200}, label: "X-",
		snapQuat: QuatFromEulerXY(0, math.Pi/2), normal: [3]float64{-1, 0, 0}},
	{verts: [4]int{7, 6, 2, 3}, color: color.RGBA{80, 200, 80, 200}, label: "Y+",
		snapQuat: QuatFromEulerXY(math.Pi/2, 0), normal: [3]float64{0, 1, 0}},
	{verts: [4]int{0, 1, 5, 4}, color: color.RGBA{80, 140, 80, 200}, label: "Y-",
		snapQuat: QuatFromEulerXY(-math.Pi/2, 0), normal: [3]float64{0, -1, 0}},
}

var cubeEdges = func() [12]snapTarget {
	edges := [12][2]int{
		{0, 1}, {1, 2}, {2, 3}, {3, 0},
		{4, 5}, {5, 6}, {6, 7}, {7, 4},
		{0, 4}, {1, 5}, {2, 6}, {3, 7},
	}
	var targets [12]snapTarget
	for i, e := range edges {
		mx := (cubeVerts[e[0]][0] + cubeVerts[e[1]][0]) / 2
		my := (cubeVerts[e[0]][1] + cubeVerts[e[1]][1]) / 2
		mz := (cubeVerts[e[0]][2] + cubeVerts[e[1]][2]) / 2
		l := math.Sqrt(mx*mx + my*my + mz*mz)
		targets[i] = snapTarget{
			pos:      [3]float64{mx, my, mz},
			snapQuat: snapQuatFromDir(mx/l, my/l, mz/l),
		}
	}
	return targets
}()

var cubeCorners = func() [8]snapTarget {
	var targets [8]snapTarget
	for i, v := range cubeVerts {
		l := math.Sqrt(v[0]*v[0] + v[1]*v[1] + v[2]*v[2])
		targets[i] = snapTarget{
			pos:      v,
			snapQuat: snapQuatFromDir(v[0]/l, v[1]/l, v[2]/l),
		}
	}
	return targets
}()

// orientationCube draws a 3D cube gizmo and handles clicks to snap orientation.
type orientationCube struct {
	widget.BaseWidget
	raster *canvas.Raster
	canvas *canvas3d
	size   float64
	onSnap func()
}

func newOrientationCube(c *canvas3d, onSnap func()) *orientationCube {
	oc := &orientationCube{
		canvas: c,
		size:   105,
		onSnap: onSnap,
	}
	oc.raster = canvas.NewRaster(oc.draw)
	oc.ExtendBaseWidget(oc)
	return oc
}

// CreateRenderer implements fyne.Widget.
func (oc *orientationCube) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(oc.raster)
}

// MinSize implements fyne.Widget.
func (oc *orientationCube) MinSize() fyne.Size {
	return fyne.NewSize(float32(oc.size), float32(oc.size))
}

func rotatePoint(p [3]float64, m [9]float64) (float64, float64, float64) {
	rx := m[0]*p[0] + m[1]*p[1] + m[2]*p[2]
	ry := m[3]*p[0] + m[4]*p[1] + m[5]*p[2]
	rz := m[6]*p[0] + m[7]*p[1] + m[8]*p[2]
	return rx, ry, rz
}

func projectPoint(px, py, _ float64, cx, cy, scale float64) raster.Vec2 {
	return raster.Vec2{
		X: px*scale + cx,
		Y: py*scale + cy,
	}
}

func (oc *orientationCube) draw(w, h int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i+3] = 0
	}

	oc.canvas.mu.Lock()
	m := oc.canvas.orientation.ToMatrix()
	up := oc.canvas.up
	oc.canvas.mu.Unlock()

	// Build label remap for Z-up mode: swap Y and Z labels.
	labelRemap := map[string]string{
		"X+": "X+", "X-": "X-",
		"Y+": "Y+", "Y-": "Y-",
		"Z+": "Z+", "Z-": "Z-",
	}
	axisLabels := [3]string{"X", "Y", "Z"}
	if up == ZUp {
		labelRemap["Y+"] = "Z+"
		labelRemap["Y-"] = "Z-"
		labelRemap["Z+"] = "Y+"
		labelRemap["Z-"] = "Y-"
		axisLabels = [3]string{"X", "Z", "Y"}
	}

	cx, cy := float64(w)/2, float64(h)/2
	scale := float64(w) * 0.28

	type faceZ struct {
		idx int
		z   float64
	}
	var visibleFaces []faceZ

	for i, f := range cubeFaces {
		_, _, nz := rotatePoint(f.normal, m)
		if nz > -0.1 {
			continue
		}
		avgZ := 0.0
		for _, vi := range f.verts {
			_, _, rz := rotatePoint(cubeVerts[vi], m)
			avgZ += rz
		}
		visibleFaces = append(visibleFaces, faceZ{i, avgZ / 4})
	}

	for i := 0; i < len(visibleFaces); i++ {
		for j := i + 1; j < len(visibleFaces); j++ {
			if visibleFaces[j].z < visibleFaces[i].z {
				visibleFaces[i], visibleFaces[j] = visibleFaces[j], visibleFaces[i]
			}
		}
	}

	for _, fz := range visibleFaces {
		f := cubeFaces[fz.idx]
		var projected [4]raster.Vec2
		for i, vi := range f.verts {
			rx, ry, rz := rotatePoint(cubeVerts[vi], m)
			projected[i] = projectPoint(rx, ry, rz, cx, cy, scale)
		}
		raster.FillQuad(img, projected, f.color)
		raster.QuadOutline(img, projected, color.RGBA{200, 200, 200, 255})

		fcx := (projected[0].X + projected[1].X + projected[2].X + projected[3].X) / 4
		fcy := (projected[0].Y + projected[1].Y + projected[2].Y + projected[3].Y) / 4
		raster.Label(img, int(fcx), int(fcy), labelRemap[f.label], color.RGBA{255, 255, 255, 255})
	}

	axisLen := 1.4
	axes := [3][3]float64{{axisLen, 0, 0}, {0, axisLen, 0}, {0, 0, axisLen}}
	axisColors := [3]color.RGBA{{255, 80, 80, 255}, {80, 255, 80, 255}, {80, 80, 255, 255}}
	ox, oy, oz := rotatePoint([3]float64{0, 0, 0}, m)
	origin := projectPoint(ox, oy, oz, cx, cy, scale)

	for i, a := range axes {
		rx, ry, rz := rotatePoint(a, m)
		end := projectPoint(rx, ry, rz, cx, cy, scale)
		raster.Line(img, int(origin.X), int(origin.Y), int(end.X), int(end.Y), axisColors[i])
		raster.Label(img, int(end.X), int(end.Y)-6, axisLabels[i], axisColors[i])
	}

	return img
}

// Tapped implements fyne.Tappable.
func (oc *orientationCube) Tapped(ev *fyne.PointEvent) {
	w := oc.Size().Width
	h := oc.Size().Height

	oc.canvas.mu.Lock()
	m := oc.canvas.orientation.ToMatrix()
	oc.canvas.mu.Unlock()

	cx, cy := float64(w)/2, float64(h)/2
	scale := float64(w) * 0.28

	clickX, clickY := float64(ev.Position.X), float64(ev.Position.Y)

	bestFace := -1
	bestFaceDist := math.MaxFloat64
	for i, f := range cubeFaces {
		_, _, nz := rotatePoint(f.normal, m)
		if nz > -0.1 {
			continue
		}
		var projected [4]raster.Vec2
		fcx, fcy := 0.0, 0.0
		for j, vi := range f.verts {
			rx, ry, rz := rotatePoint(cubeVerts[vi], m)
			projected[j] = projectPoint(rx, ry, rz, cx, cy, scale)
			fcx += projected[j].X
			fcy += projected[j].Y
		}
		fcx /= 4
		fcy /= 4
		if raster.PointInQuad(clickX, clickY, projected) {
			dist := math.Hypot(clickX-fcx, clickY-fcy)
			if dist < bestFaceDist {
				bestFaceDist = dist
				bestFace = i
			}
		}
	}

	hitRadius := float64(w) * 0.12
	var bestSnapQuat Quat
	bestPointDist := math.MaxFloat64
	foundPoint := false

	for _, t := range cubeCorners {
		rx, ry, rz := rotatePoint(t.pos, m)
		if rz >= 0 {
			continue
		}
		p := projectPoint(rx, ry, rz, cx, cy, scale)
		d := math.Hypot(clickX-p.X, clickY-p.Y)
		if d < hitRadius && d < bestPointDist {
			bestPointDist = d
			bestSnapQuat = t.snapQuat
			foundPoint = true
		}
	}
	for _, t := range cubeEdges {
		rx, ry, rz := rotatePoint(t.pos, m)
		if rz >= 0 {
			continue
		}
		p := projectPoint(rx, ry, rz, cx, cy, scale)
		d := math.Hypot(clickX-p.X, clickY-p.Y)
		if d < hitRadius && d < bestPointDist {
			bestPointDist = d
			bestSnapQuat = t.snapQuat
			foundPoint = true
		}
	}

	var snapQ Quat
	hit := false
	if foundPoint && bestPointDist < 8 {
		snapQ = bestSnapQuat
		hit = true
	} else if bestFace >= 0 {
		snapQ = cubeFaces[bestFace].snapQuat
		hit = true
	} else if foundPoint {
		snapQ = bestSnapQuat
		hit = true
	}

	if hit {
		oc.canvas.mu.Lock()
		oc.canvas.orientation = snapQ
		oc.canvas.mu.Unlock()
		oc.canvas.raster.Refresh()
		oc.raster.Refresh()
		if oc.onSnap != nil {
			oc.onSnap()
		}
	}
}
