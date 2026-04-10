package pointcloud

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

// ReadXYZ reads a point cloud from an XYZ format file.
// Each line should contain at least 3 whitespace or comma-separated floats (x, y, z).
// Lines starting with '#' or '//' are treated as comments.
// Extra columns beyond x, y, z are ignored.
func ReadXYZ(r io.Reader) (*PointCloud, error) {
	pc := &PointCloud{}
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
			continue
		}

		normalized := line
		if strings.Contains(normalized, ",") {
			normalized = strings.ReplaceAll(normalized, ",", " ")
		}

		var x, y, z float64
		var r, g, b int
		n, _ := fmt.Sscanf(normalized, "%f %f %f %d %d %d", &x, &y, &z, &r, &g, &b)
		if n < 3 {
			continue
		}
		p := Point3D{X: x, Y: y, Z: z}
		if n >= 6 {
			p.R, p.G, p.B = uint8(r), uint8(g), uint8(b)
			p.HasColor = true
		}
		pc.Points = append(pc.Points, p)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading XYZ: %w", err)
	}
	if len(pc.Points) == 0 {
		return nil, fmt.Errorf("no points found in XYZ file")
	}

	pc.ComputeBounds()
	return pc, nil
}
