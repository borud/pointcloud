package pointcloud

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
)

// ReadPCD reads a point cloud from a PCD (Point Cloud Library) format file.
// Supports ASCII and binary formats.
func ReadPCD(r io.Reader) (*PointCloud, error) {
	br := bufio.NewReader(r)

	var pointCount int
	var dataFormat string
	var fieldNames []string
	var fieldSizes []int
	var fieldTypes []byte

	// Parse header.
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			return nil, fmt.Errorf("unexpected end of PCD header")
		}
		line = strings.TrimSpace(line)

		if strings.HasPrefix(line, "#") {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}

		switch fields[0] {
		case "FIELDS":
			fieldNames = fields[1:]
		case "SIZE":
			for _, s := range fields[1:] {
				var sz int
				fmt.Sscanf(s, "%d", &sz)
				fieldSizes = append(fieldSizes, sz)
			}
		case "TYPE":
			for _, t := range fields[1:] {
				if len(t) > 0 {
					fieldTypes = append(fieldTypes, t[0])
				}
			}
		case "POINTS":
			fmt.Sscanf(fields[1], "%d", &pointCount)
		case "DATA":
			dataFormat = fields[1]
			goto readData
		}
	}

readData:
	if pointCount == 0 {
		return nil, fmt.Errorf("PCD file has no points")
	}

	// Find x, y, z and color field indices.
	xIdx, yIdx, zIdx := -1, -1, -1
	rgbIdx := -1
	rIdx, gIdx, bIdx := -1, -1, -1
	for i, name := range fieldNames {
		switch name {
		case "x":
			xIdx = i
		case "y":
			yIdx = i
		case "z":
			zIdx = i
		case "rgb", "rgba":
			rgbIdx = i
		case "red":
			rIdx = i
		case "green":
			gIdx = i
		case "blue":
			bIdx = i
		}
	}
	if xIdx < 0 || yIdx < 0 || zIdx < 0 {
		return nil, fmt.Errorf("PCD file missing x, y, or z fields")
	}
	hasRGB := rgbIdx >= 0
	hasSeparateColor := rIdx >= 0 && gIdx >= 0 && bIdx >= 0

	acc := newPointAccumulator(pointCount)

	switch dataFormat {
	case "ascii":
		err := readPCDASCII(br, &acc, pointCount, fieldNames, xIdx, yIdx, zIdx, rgbIdx, rIdx, gIdx, bIdx, hasRGB, hasSeparateColor)
		if err != nil {
			return nil, err
		}
	case "binary":
		err := readPCDBinary(br, &acc, pointCount, fieldSizes, fieldTypes, xIdx, yIdx, zIdx, rgbIdx, rIdx, gIdx, bIdx, hasRGB, hasSeparateColor)
		if err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("unsupported PCD data format: %s", dataFormat)
	}

	return acc.finish(fmt.Errorf("PCD file has no points"))
}

func readPCDASCII(br *bufio.Reader, acc *pointAccumulator, count int, _ []string, xi, yi, zi, rgbi, ri, gi, bi int, hasRGB, hasSeparateColor bool) error {
	maxIdx := maxFieldIndex(xi, yi, zi)
	if hasRGB {
		maxIdx = maxFieldIndex(maxIdx, rgbi)
	}
	if hasSeparateColor {
		maxIdx = maxFieldIndex(maxIdx, ri, gi, bi)
	}

	for i := 0; i < count; i++ {
		line, err := br.ReadString('\n')
		if err != nil && err != io.EOF {
			return fmt.Errorf("reading PCD point %d: %w", i, err)
		}
		parts := strings.Fields(strings.TrimSpace(line))
		if len(parts) <= maxIdx {
			continue
		}

		x, err := strconv.ParseFloat(parts[xi], 64)
		if err != nil {
			return fmt.Errorf("PCD point %d: invalid x value %q: %w", i, parts[xi], err)
		}
		y, err := strconv.ParseFloat(parts[yi], 64)
		if err != nil {
			return fmt.Errorf("PCD point %d: invalid y value %q: %w", i, parts[yi], err)
		}
		z, err := strconv.ParseFloat(parts[zi], 64)
		if err != nil {
			return fmt.Errorf("PCD point %d: invalid z value %q: %w", i, parts[zi], err)
		}
		p := Point3D{X: x, Y: y, Z: z}
		if hasRGB {
			rgbf, err := strconv.ParseFloat(parts[rgbi], 64)
			if err != nil {
				return fmt.Errorf("PCD point %d: invalid rgb value %q: %w", i, parts[rgbi], err)
			}
			rgb := math.Float32bits(float32(rgbf))
			p.R = uint8((rgb >> 16) & 0xFF)
			p.G = uint8((rgb >> 8) & 0xFF)
			p.B = uint8(rgb & 0xFF)
			p.HasColor = true
		} else if hasSeparateColor {
			r, err := strconv.ParseUint(parts[ri], 10, 8)
			if err != nil {
				return fmt.Errorf("PCD point %d: invalid red value %q: %w", i, parts[ri], err)
			}
			g, err := strconv.ParseUint(parts[gi], 10, 8)
			if err != nil {
				return fmt.Errorf("PCD point %d: invalid green value %q: %w", i, parts[gi], err)
			}
			b, err := strconv.ParseUint(parts[bi], 10, 8)
			if err != nil {
				return fmt.Errorf("PCD point %d: invalid blue value %q: %w", i, parts[bi], err)
			}
			p.R, p.G, p.B = uint8(r), uint8(g), uint8(b)
			p.HasColor = true
		}
		acc.appendPoint(p)
	}
	return nil
}

func readPCDBinary(br *bufio.Reader, acc *pointAccumulator, count int, sizes []int, types []byte, xi, yi, zi, rgbi, ri, gi, bi int, hasRGB, hasSeparateColor bool) error {
	// Compute stride and offsets.
	offsets := make([]int, len(sizes))
	stride := 0
	for i, s := range sizes {
		offsets[i] = stride
		stride += s
	}

	buf := make([]byte, stride)
	for i := 0; i < count; i++ {
		if _, err := io.ReadFull(br, buf); err != nil {
			return fmt.Errorf("reading PCD binary point %d: %w", i, err)
		}

		x := readPCDFloat(buf[offsets[xi]:], sizes[xi], types[xi])
		y := readPCDFloat(buf[offsets[yi]:], sizes[yi], types[yi])
		z := readPCDFloat(buf[offsets[zi]:], sizes[zi], types[zi])
		p := Point3D{X: x, Y: y, Z: z}
		if hasRGB && sizes[rgbi] == 4 {
			rgb := binary.LittleEndian.Uint32(buf[offsets[rgbi]:])
			p.R = uint8((rgb >> 16) & 0xFF)
			p.G = uint8((rgb >> 8) & 0xFF)
			p.B = uint8(rgb & 0xFF)
			p.HasColor = true
		} else if hasSeparateColor {
			p.R = uint8(readPCDFloat(buf[offsets[ri]:], sizes[ri], types[ri]))
			p.G = uint8(readPCDFloat(buf[offsets[gi]:], sizes[gi], types[gi]))
			p.B = uint8(readPCDFloat(buf[offsets[bi]:], sizes[bi], types[bi]))
			p.HasColor = true
		}
		acc.appendPoint(p)
	}
	return nil
}

func readPCDFloat(data []byte, size int, typ byte) float64 {
	switch {
	case typ == 'F' && size == 4:
		return float64(math.Float32frombits(binary.LittleEndian.Uint32(data)))
	case typ == 'F' && size == 8:
		return math.Float64frombits(binary.LittleEndian.Uint64(data))
	case typ == 'U' && size == 1:
		return float64(data[0])
	case typ == 'U' && size == 2:
		return float64(binary.LittleEndian.Uint16(data))
	case typ == 'U' && size == 4:
		return float64(binary.LittleEndian.Uint32(data))
	case typ == 'I' && size == 1:
		return float64(int8(data[0]))
	case typ == 'I' && size == 2:
		return float64(int16(binary.LittleEndian.Uint16(data)))
	case typ == 'I' && size == 4:
		return float64(int32(binary.LittleEndian.Uint32(data)))
	default:
		return 0
	}
}
