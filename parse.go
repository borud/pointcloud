package pointcloud

import "strconv"

type pointAccumulator struct {
	pc    *PointCloud
	empty bool
}

func newPointAccumulator(capacity int) pointAccumulator {
	return pointAccumulator{
		pc:    &PointCloud{Points: make([]Point3D, 0, capacity)},
		empty: true,
	}
}

func (a *pointAccumulator) appendPoint(p Point3D) {
	a.pc.Points = append(a.pc.Points, p)
	if a.empty {
		a.pc.MinX, a.pc.MaxX = p.X, p.X
		a.pc.MinY, a.pc.MaxY = p.Y, p.Y
		a.pc.MinZ, a.pc.MaxZ = p.Z, p.Z
		a.empty = false
		return
	}
	a.pc.MinX = min(a.pc.MinX, p.X)
	a.pc.MinY = min(a.pc.MinY, p.Y)
	a.pc.MinZ = min(a.pc.MinZ, p.Z)
	a.pc.MaxX = max(a.pc.MaxX, p.X)
	a.pc.MaxY = max(a.pc.MaxY, p.Y)
	a.pc.MaxZ = max(a.pc.MaxZ, p.Z)
}

func (a *pointAccumulator) finish(emptyErr error) (*PointCloud, error) {
	if len(a.pc.Points) == 0 {
		return nil, emptyErr
	}
	a.pc.boundsComputed = true
	return a.pc, nil
}

type lineParser struct {
	s string
	i int
}

func newLineParser(s string) lineParser {
	return lineParser{s: s}
}

func (p *lineParser) skipSeparators() {
	for p.i < len(p.s) {
		switch p.s[p.i] {
		case ' ', '\t', '\r', '\n', ',':
			p.i++
		default:
			return
		}
	}
}

func (p *lineParser) nextToken() (string, bool) {
	p.skipSeparators()
	if p.i >= len(p.s) {
		return "", false
	}
	start := p.i
	for p.i < len(p.s) {
		switch p.s[p.i] {
		case ' ', '\t', '\r', '\n', ',':
			return p.s[start:p.i], true
		default:
			p.i++
		}
	}
	return p.s[start:], true
}

func (p *lineParser) nextFloat64() (float64, bool) {
	tok, ok := p.nextToken()
	if !ok {
		return 0, false
	}
	v, err := strconv.ParseFloat(tok, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

func (p *lineParser) nextInt() (int, bool) {
	tok, ok := p.nextToken()
	if !ok {
		return 0, false
	}
	v, err := strconv.Atoi(tok)
	if err != nil {
		return 0, false
	}
	return v, true
}

func (p *lineParser) nextUint8() (uint8, bool) {
	tok, ok := p.nextToken()
	if !ok {
		return 0, false
	}
	v, err := strconv.ParseUint(tok, 10, 8)
	if err != nil {
		return 0, false
	}
	return uint8(v), true
}

func parseXYZLine(line string) (Point3D, bool) {
	p := newLineParser(line)
	x, ok := p.nextFloat64()
	if !ok {
		return Point3D{}, false
	}
	y, ok := p.nextFloat64()
	if !ok {
		return Point3D{}, false
	}
	z, ok := p.nextFloat64()
	if !ok {
		return Point3D{}, false
	}
	pt := Point3D{X: x, Y: y, Z: z}
	r, okR := p.nextUint8()
	g, okG := p.nextUint8()
	b, okB := p.nextUint8()
	if okR && okG && okB {
		pt.R, pt.G, pt.B = r, g, b
		pt.HasColor = true
	}
	return pt, true
}

func parsePTSLine(line string) (Point3D, bool) {
	p := newLineParser(line)
	x, ok := p.nextFloat64()
	if !ok {
		return Point3D{}, false
	}
	y, ok := p.nextFloat64()
	if !ok {
		return Point3D{}, false
	}
	z, ok := p.nextFloat64()
	if !ok {
		return Point3D{}, false
	}
	pt := Point3D{X: x, Y: y, Z: z}
	if _, ok := p.nextInt(); !ok {
		return pt, true
	}
	r, okR := p.nextUint8()
	g, okG := p.nextUint8()
	b, okB := p.nextUint8()
	if okR && okG && okB {
		pt.R, pt.G, pt.B = r, g, b
		pt.HasColor = true
	}
	return pt, true
}
