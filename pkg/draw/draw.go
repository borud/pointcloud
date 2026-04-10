// Package draw provides 2D raster drawing primitives.
package draw

import (
	"image"
	"image/color"
	"math"
)

// Vec2 is a 2D point.
type Vec2 struct {
	X, Y float64
}

// BlendPixel alpha-blends a color onto an existing pixel.
func BlendPixel(img *image.RGBA, x, y int, c color.RGBA) {
	off := img.PixOffset(x, y)
	if off < 0 || off+3 >= len(img.Pix) {
		return
	}
	alpha := float64(c.A) / 255.0
	img.Pix[off] = uint8(float64(c.R)*alpha + float64(img.Pix[off])*(1-alpha))
	img.Pix[off+1] = uint8(float64(c.G)*alpha + float64(img.Pix[off+1])*(1-alpha))
	img.Pix[off+2] = uint8(float64(c.B)*alpha + float64(img.Pix[off+2])*(1-alpha))
	img.Pix[off+3] = 255
}

// LineAA draws an anti-aliased line using Xiaolin Wu's algorithm.
func LineAA(img *image.RGBA, x0, y0, x1, y1 float64, c color.RGBA) {
	w, h := img.Bounds().Dx(), img.Bounds().Dy()

	steep := math.Abs(y1-y0) > math.Abs(x1-x0)
	if steep {
		x0, y0 = y0, x0
		x1, y1 = y1, x1
	}
	if x0 > x1 {
		x0, x1 = x1, x0
		y0, y1 = y1, y0
	}

	dx := x1 - x0
	dy := y1 - y0
	gradient := 0.0
	if dx != 0 {
		gradient = dy / dx
	}

	plot := func(px, py int, brightness float64) {
		if steep {
			px, py = py, px
		}
		if px >= 0 && px < w && py >= 0 && py < h {
			a := uint8(brightness * float64(c.A))
			BlendPixel(img, px, py, color.RGBA{c.R, c.G, c.B, a})
		}
	}

	xend := math.Round(x0)
	yend := y0 + gradient*(xend-x0)
	xpxl1 := int(xend)
	ypxl1 := int(math.Floor(yend))
	plot(xpxl1, ypxl1, 1.0)
	plot(xpxl1, ypxl1+1, 1.0)
	intery := yend + gradient

	xend = math.Round(x1)
	xpxl2 := int(xend)
	yend2 := y1 + gradient*(xend-x1)
	ypxl2 := int(math.Floor(yend2))
	plot(xpxl2, ypxl2, 1.0)
	plot(xpxl2, ypxl2+1, 1.0)

	for x := xpxl1 + 1; x < xpxl2; x++ {
		fpart := intery - math.Floor(intery)
		iy := int(math.Floor(intery))
		plot(x, iy, 1.0-fpart)
		plot(x, iy+1, fpart)
		intery += gradient
	}
}

// Line draws an anti-aliased line between integer coordinates.
func Line(img *image.RGBA, x0, y0, x1, y1 int, c color.RGBA) {
	LineAA(img, float64(x0), float64(y0), float64(x1), float64(y1), c)
}

// Dot draws a small filled circle.
func Dot(img *image.RGBA, cx, cy, r int, c color.RGBA) {
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	for dy := -r; dy <= r; dy++ {
		for dx := -r; dx <= r; dx++ {
			if dx*dx+dy*dy <= r*r {
				px, py := cx+dx, cy+dy
				if px >= 0 && px < w && py >= 0 && py < h {
					BlendPixel(img, px, py, c)
				}
			}
		}
	}
}

// FillQuad fills a convex quadrilateral using scanline.
func FillQuad(img *image.RGBA, q [4]Vec2, c color.RGBA) {
	bounds := img.Bounds()
	minY, maxY := q[0].Y, q[0].Y
	for _, v := range q {
		if v.Y < minY {
			minY = v.Y
		}
		if v.Y > maxY {
			maxY = v.Y
		}
	}
	iy0 := int(math.Max(minY, float64(bounds.Min.Y)))
	iy1 := int(math.Min(maxY, float64(bounds.Max.Y-1)))

	edges := [4][2]Vec2{{q[0], q[1]}, {q[1], q[2]}, {q[2], q[3]}, {q[3], q[0]}}

	for y := iy0; y <= iy1; y++ {
		fy := float64(y) + 0.5
		xMin, xMax := math.MaxFloat64, -math.MaxFloat64
		for _, e := range edges {
			a, b := e[0], e[1]
			if (a.Y <= fy && b.Y > fy) || (b.Y <= fy && a.Y > fy) {
				t := (fy - a.Y) / (b.Y - a.Y)
				ix := a.X + t*(b.X-a.X)
				if ix < xMin {
					xMin = ix
				}
				if ix > xMax {
					xMax = ix
				}
			}
		}
		if xMin > xMax {
			continue
		}
		x0 := int(math.Max(xMin, float64(bounds.Min.X)))
		x1 := int(math.Min(xMax, float64(bounds.Max.X-1)))
		for x := x0; x <= x1; x++ {
			BlendPixel(img, x, y, c)
		}
	}
}

// QuadOutline draws the outline of a quadrilateral with anti-aliased lines.
func QuadOutline(img *image.RGBA, q [4]Vec2, c color.RGBA) {
	for i := 0; i < 4; i++ {
		a, b := q[i], q[(i+1)%4]
		LineAA(img, a.X, a.Y, b.X, b.Y, c)
	}
}

// Label draws a bitmap label (3x5 font, scaled 2x).
func Label(img *image.RGBA, cx, cy int, text string, c color.RGBA) {
	glyphs := map[byte][5]uint8{
		'X': {0b101, 0b101, 0b010, 0b101, 0b101},
		'Y': {0b101, 0b101, 0b010, 0b010, 0b010},
		'Z': {0b111, 0b001, 0b010, 0b100, 0b111},
		'+': {0b000, 0b010, 0b111, 0b010, 0b000},
		'-': {0b000, 0b000, 0b111, 0b000, 0b000},
		'^': {0b010, 0b111, 0b101, 0b000, 0b000},
	}

	const s = 2
	totalW := len(text)*4*s - s
	startX := cx - totalW/2
	startY := cy - 5

	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	for ci, ch := range text {
		glyph, ok := glyphs[byte(ch)]
		if !ok {
			continue
		}
		ox := startX + ci*4*s
		for row := 0; row < 5; row++ {
			for col := 0; col < 3; col++ {
				if glyph[row]&(1<<(2-col)) != 0 {
					for dy := 0; dy < s; dy++ {
						for dx := 0; dx < s; dx++ {
							px, py := ox+col*s+dx, startY+row*s+dy
							if px >= 0 && px < w && py >= 0 && py < h {
								img.Set(px, py, c)
							}
						}
					}
				}
			}
		}
	}
}

// PointInQuad checks if point (px, py) is inside a quadrilateral.
func PointInQuad(px, py float64, q [4]Vec2) bool {
	return pointInTriangle(px, py, q[0], q[1], q[2]) ||
		pointInTriangle(px, py, q[0], q[2], q[3])
}

func pointInTriangle(px, py float64, a, b, c Vec2) bool {
	d1 := cross2D(px, py, a, b)
	d2 := cross2D(px, py, b, c)
	d3 := cross2D(px, py, c, a)
	hasNeg := (d1 < 0) || (d2 < 0) || (d3 < 0)
	hasPos := (d1 > 0) || (d2 > 0) || (d3 > 0)
	return !(hasNeg && hasPos)
}

func cross2D(px, py float64, a, b Vec2) float64 {
	return (b.X-a.X)*(py-a.Y) - (b.Y-a.Y)*(px-a.X)
}
