package pointcloud

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

// ReadPTS reads a point cloud from a PTS (Leica) format file.
// The first line is the point count. Subsequent lines contain:
// x y z intensity [r g b] (space-separated).
func ReadPTS(r io.Reader) (*PointCloud, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	// First line: point count.
	if !scanner.Scan() {
		return nil, fmt.Errorf("PTS file is empty")
	}
	var count int
	if _, err := fmt.Sscanf(strings.TrimSpace(scanner.Text()), "%d", &count); err != nil {
		return nil, fmt.Errorf("PTS: invalid point count on first line: %w", err)
	}

	pc := &PointCloud{Points: make([]Point3D, 0, count)}

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var x, y, z float64
		var intensity int
		var r, g, b int
		n, _ := fmt.Sscanf(line, "%f %f %f %d %d %d %d", &x, &y, &z, &intensity, &r, &g, &b)
		if n < 3 {
			continue
		}
		p := Point3D{X: x, Y: y, Z: z}
		if n >= 7 {
			p.R, p.G, p.B = uint8(r), uint8(g), uint8(b)
			p.HasColor = true
		}
		pc.Points = append(pc.Points, p)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading PTS: %w", err)
	}
	if len(pc.Points) == 0 {
		return nil, fmt.Errorf("no points found in PTS file")
	}

	pc.ComputeBounds()
	return pc, nil
}
