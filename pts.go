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

	acc := newPointAccumulator(count)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		p, ok := parsePTSLine(line)
		if !ok {
			continue
		}
		acc.appendPoint(p)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading PTS: %w", err)
	}
	return acc.finish(fmt.Errorf("no points found in PTS file"))
}
