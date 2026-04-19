package pointcloud

import "math"

const gridSize = 8 // 8x8x8 = 512 cells

// spatialGrid is a uniform 3D grid over normalized space for coarse
// frustum culling in flythrough mode.
type spatialGrid struct {
	cells    [gridSize * gridSize * gridSize]gridCell
	cellSize [3]float64 // size of each cell
	origin   [3]float64 // min corner of the grid
}

// gridCell stores a contiguous range into the SoA arrays plus a bounding sphere.
type gridCell struct {
	start, count int
	centerX      float32
	centerY      float32
	centerZ      float32
	radius       float32
}

// buildGrid assigns points to cells and reorders SoA arrays so each cell's
// points are contiguous. Returns the grid and reordered arrays.
func buildGrid(xs, ys, zs []float32, rgba []uint32, indices []int) (*spatialGrid, []float32, []float32, []float32, []uint32, []int) {
	n := len(xs)
	if n == 0 {
		return nil, xs, ys, zs, rgba, indices
	}

	// Find bounding box with a small margin.
	minX, minY, minZ := float64(xs[0]), float64(ys[0]), float64(zs[0])
	maxX, maxY, maxZ := minX, minY, minZ
	for i := 1; i < n; i++ {
		x, y, z := float64(xs[i]), float64(ys[i]), float64(zs[i])
		if x < minX {
			minX = x
		}
		if x > maxX {
			maxX = x
		}
		if y < minY {
			minY = y
		}
		if y > maxY {
			maxY = y
		}
		if z < minZ {
			minZ = z
		}
		if z > maxZ {
			maxZ = z
		}
	}

	// Add small epsilon to avoid edge points falling outside.
	const eps = 0.001
	minX -= eps
	minY -= eps
	minZ -= eps
	maxX += eps
	maxY += eps
	maxZ += eps

	g := &spatialGrid{
		origin:   [3]float64{minX, minY, minZ},
		cellSize: [3]float64{(maxX - minX) / gridSize, (maxY - minY) / gridSize, (maxZ - minZ) / gridSize},
	}

	// Ensure no zero-size cells.
	for i := range g.cellSize {
		if g.cellSize[i] < eps {
			g.cellSize[i] = eps
		}
	}

	// Count points per cell.
	cellIdx := make([]int, n)
	counts := [gridSize * gridSize * gridSize]int{}
	invCellX := 1.0 / g.cellSize[0]
	invCellY := 1.0 / g.cellSize[1]
	invCellZ := 1.0 / g.cellSize[2]

	for i := 0; i < n; i++ {
		cx := int((float64(xs[i]) - minX) * invCellX)
		cy := int((float64(ys[i]) - minY) * invCellY)
		cz := int((float64(zs[i]) - minZ) * invCellZ)
		if cx >= gridSize {
			cx = gridSize - 1
		}
		if cy >= gridSize {
			cy = gridSize - 1
		}
		if cz >= gridSize {
			cz = gridSize - 1
		}
		idx := cx*gridSize*gridSize + cy*gridSize + cz
		cellIdx[i] = idx
		counts[idx]++
	}

	// Compute start offsets (prefix sum).
	offset := 0
	for i := range g.cells {
		g.cells[i].start = offset
		g.cells[i].count = counts[i]
		offset += counts[i]
	}

	// Reorder arrays by cell.
	newXs := make([]float32, n)
	newYs := make([]float32, n)
	newZs := make([]float32, n)
	newRGBA := make([]uint32, n)
	newIndices := make([]int, n)
	writePos := [gridSize * gridSize * gridSize]int{}
	for i := range writePos {
		writePos[i] = g.cells[i].start
	}

	for i := 0; i < n; i++ {
		ci := cellIdx[i]
		wp := writePos[ci]
		newXs[wp] = xs[i]
		newYs[wp] = ys[i]
		newZs[wp] = zs[i]
		newRGBA[wp] = rgba[i]
		newIndices[wp] = indices[i]
		writePos[ci]++
	}

	// Compute bounding spheres for each cell.
	for i := range g.cells {
		c := &g.cells[i]
		if c.count == 0 {
			continue
		}
		// Compute center as average.
		var sx, sy, sz float64
		end := c.start + c.count
		for j := c.start; j < end; j++ {
			sx += float64(newXs[j])
			sy += float64(newYs[j])
			sz += float64(newZs[j])
		}
		fn := float64(c.count)
		c.centerX = float32(sx / fn)
		c.centerY = float32(sy / fn)
		c.centerZ = float32(sz / fn)

		// Compute radius as max distance from center.
		var maxR2 float64
		cx, cy, cz := float64(c.centerX), float64(c.centerY), float64(c.centerZ)
		for j := c.start; j < end; j++ {
			dx := float64(newXs[j]) - cx
			dy := float64(newYs[j]) - cy
			dz := float64(newZs[j]) - cz
			r2 := dx*dx + dy*dy + dz*dz
			if r2 > maxR2 {
				maxR2 = r2
			}
		}
		c.radius = float32(math.Sqrt(maxR2))
	}

	return g, newXs, newYs, newZs, newRGBA, newIndices
}

// visibleCells returns indices of cells that intersect the frustum defined
// by 6 planes. Each plane is [nx, ny, nz, d] where nx*x + ny*y + nz*z + d >= 0
// means the point is inside.
func (g *spatialGrid) visibleCells(planes [6][4]float32) []gridCell {
	var result []gridCell
	for i := range g.cells {
		c := &g.cells[i]
		if c.count == 0 {
			continue
		}
		// Sphere-frustum test: if sphere is fully outside any plane, skip.
		outside := false
		for _, p := range planes {
			dist := p[0]*c.centerX + p[1]*c.centerY + p[2]*c.centerZ + p[3]
			if dist < -c.radius {
				outside = true
				break
			}
		}
		if !outside {
			result = append(result, *c)
		}
	}
	return result
}

// extractFrustumPlanes derives 6 frustum planes from view-projection parameters.
// The planes are in the form [nx, ny, nz, d] with inward-pointing normals.
func extractFrustumPlanes(m [9]float64, tx, ty, tz, zoom, aspect float64) [6][4]float32 {
	var planes [6][4]float32

	// Build a combined view-projection matrix rows for plane extraction.
	// The perspective projection for our renderer is:
	//   screenX = (rx / dist) * zoom  where dist = tz - rz
	//   screenY = (ry / dist) * zoom
	// This is equivalent to a projection matrix where:
	//   clip_x = rx * zoom
	//   clip_y = ry * zoom
	//   clip_w = dist = tz - rz
	// Frustum planes from clip space: left/right/top/bottom/near/far.

	// Row vectors of the combined transform:
	// row0 = [m0, m1, m2, tx] * zoom  (for x)
	// row1 = [m3, m4, m5, ty] * zoom  (for y)
	// row3 = [tz - m6z... ]           (for w = tz - rz)
	// Actually row3: w = tz - (m6*x + m7*y + m8*z) = -m6*x - m7*y - m8*z + tz

	r0 := [4]float64{m[0] * zoom, m[1] * zoom, m[2] * zoom, tx * zoom}
	r1 := [4]float64{m[3] * zoom, m[4] * zoom, m[5] * zoom, ty * zoom}
	r3 := [4]float64{-m[6], -m[7], -m[8], tz}

	// Left plane: row3 + row0  (clip_x + clip_w >= 0)
	normalizePlane := func(a, b, c, d float64) [4]float32 {
		l := math.Sqrt(a*a + b*b + c*c)
		if l < 1e-10 {
			return [4]float32{}
		}
		return [4]float32{float32(a / l), float32(b / l), float32(c / l), float32(d / l)}
	}

	// Use aspect ratio to scale horizontal planes.
	_ = aspect
	planes[0] = normalizePlane(r3[0]+r0[0], r3[1]+r0[1], r3[2]+r0[2], r3[3]+r0[3]) // left
	planes[1] = normalizePlane(r3[0]-r0[0], r3[1]-r0[1], r3[2]-r0[2], r3[3]-r0[3]) // right
	planes[2] = normalizePlane(r3[0]+r1[0], r3[1]+r1[1], r3[2]+r1[2], r3[3]+r1[3]) // bottom
	planes[3] = normalizePlane(r3[0]-r1[0], r3[1]-r1[1], r3[2]-r1[2], r3[3]-r1[3]) // top

	// Near plane: dist >= 0.1 → -m6*x - m7*y - m8*z + tz >= 0.1
	planes[4] = normalizePlane(-m[6], -m[7], -m[8], tz-0.1)

	// Far plane: a generous far distance to avoid clipping visible points.
	planes[5] = normalizePlane(m[6], m[7], m[8], -(tz - 100.0))

	return planes
}
