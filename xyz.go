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
	acc := newPointAccumulator(0)
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
			continue
		}
		p, ok := parseXYZLine(line)
		if !ok {
			continue
		}
		acc.appendPoint(p)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading XYZ: %w", err)
	}
	return acc.finish(fmt.Errorf("no points found in XYZ file"))
}
