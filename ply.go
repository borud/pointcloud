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

// plyProperty describes a single property in the PLY header.
type plyProperty struct {
	name     string
	dataType string // float, double, uchar, int, etc.
}

// ReadPLY reads a point cloud from a PLY (Stanford Polygon Format) file.
// Supports ASCII, binary_little_endian, and binary_big_endian formats.
func ReadPLY(r io.Reader) (*PointCloud, error) {
	br := bufio.NewReader(r)

	// Parse header.
	magic, err := br.ReadString('\n')
	if err != nil || strings.TrimSpace(magic) != "ply" {
		return nil, fmt.Errorf("not a PLY file")
	}

	var format string
	var vertexCount int
	var properties []plyProperty
	inVertexElement := false

	for {
		line, err := br.ReadString('\n')
		if err != nil {
			return nil, fmt.Errorf("unexpected end of PLY header")
		}
		line = strings.TrimSpace(line)

		if line == "end_header" {
			break
		}

		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}

		switch fields[0] {
		case "format":
			if len(fields) >= 2 {
				format = fields[1]
			}
		case "element":
			if len(fields) >= 3 && fields[1] == "vertex" {
				fmt.Sscanf(fields[2], "%d", &vertexCount)
				inVertexElement = true
			} else {
				inVertexElement = false
			}
		case "property":
			if inVertexElement && len(fields) >= 3 {
				// Skip list properties.
				if fields[1] == "list" {
					continue
				}
				properties = append(properties, plyProperty{
					name:     fields[len(fields)-1],
					dataType: fields[1],
				})
			}
		}
	}

	if vertexCount == 0 {
		return nil, fmt.Errorf("PLY file has no vertices")
	}

	// Find x, y, z and color property indices.
	xIdx, yIdx, zIdx := -1, -1, -1
	rIdx, gIdx, bIdx := -1, -1, -1
	for i, p := range properties {
		switch p.name {
		case "x":
			xIdx = i
		case "y":
			yIdx = i
		case "z":
			zIdx = i
		case "red":
			rIdx = i
		case "green":
			gIdx = i
		case "blue":
			bIdx = i
		}
	}
	if xIdx < 0 || yIdx < 0 || zIdx < 0 {
		return nil, fmt.Errorf("PLY file missing x, y, or z properties")
	}
	hasColor := rIdx >= 0 && gIdx >= 0 && bIdx >= 0

	acc := newPointAccumulator(vertexCount)

	switch format {
	case "ascii":
		err = readPLYASCII(br, &acc, vertexCount, properties, xIdx, yIdx, zIdx, rIdx, gIdx, bIdx, hasColor)
	case "binary_little_endian":
		err = readPLYBinary(br, &acc, vertexCount, properties, xIdx, yIdx, zIdx, rIdx, gIdx, bIdx, hasColor, binary.LittleEndian)
	case "binary_big_endian":
		err = readPLYBinary(br, &acc, vertexCount, properties, xIdx, yIdx, zIdx, rIdx, gIdx, bIdx, hasColor, binary.BigEndian)
	default:
		return nil, fmt.Errorf("unsupported PLY format: %s", format)
	}

	if err != nil {
		return nil, err
	}

	return acc.finish(fmt.Errorf("PLY file has no vertices"))
}

func readPLYASCII(br *bufio.Reader, acc *pointAccumulator, count int, _ []plyProperty, xi, yi, zi, ri, gi, bi int, hasColor bool) error {
	maxIdx := maxFieldIndex(xi, yi, zi)
	if hasColor {
		maxIdx = maxFieldIndex(maxIdx, ri, gi, bi)
	}

	for i := 0; i < count; i++ {
		line, err := br.ReadString('\n')
		if err != nil && err != io.EOF {
			return fmt.Errorf("reading PLY vertex %d: %w", i, err)
		}
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) <= maxIdx {
			return fmt.Errorf("PLY vertex %d: expected at least %d fields, got %d", i, maxIdx+1, len(fields))
		}

		x, err := strconv.ParseFloat(fields[xi], 64)
		if err != nil {
			return fmt.Errorf("PLY vertex %d: invalid x value %q: %w", i, fields[xi], err)
		}
		y, err := strconv.ParseFloat(fields[yi], 64)
		if err != nil {
			return fmt.Errorf("PLY vertex %d: invalid y value %q: %w", i, fields[yi], err)
		}
		z, err := strconv.ParseFloat(fields[zi], 64)
		if err != nil {
			return fmt.Errorf("PLY vertex %d: invalid z value %q: %w", i, fields[zi], err)
		}
		p := Point3D{X: x, Y: y, Z: z}
		if hasColor {
			r, err := strconv.ParseUint(fields[ri], 10, 8)
			if err != nil {
				return fmt.Errorf("PLY vertex %d: invalid red value %q: %w", i, fields[ri], err)
			}
			g, err := strconv.ParseUint(fields[gi], 10, 8)
			if err != nil {
				return fmt.Errorf("PLY vertex %d: invalid green value %q: %w", i, fields[gi], err)
			}
			b, err := strconv.ParseUint(fields[bi], 10, 8)
			if err != nil {
				return fmt.Errorf("PLY vertex %d: invalid blue value %q: %w", i, fields[bi], err)
			}
			p.R, p.G, p.B = uint8(r), uint8(g), uint8(b)
			p.HasColor = true
		}
		acc.appendPoint(p)
	}
	return nil
}

func readPLYBinary(br *bufio.Reader, acc *pointAccumulator, count int, props []plyProperty, xi, yi, zi, ri, gi, bi int, hasColor bool, order binary.ByteOrder) error {
	// Compute byte offsets for each property.
	offsets := make([]int, len(props))
	sizes := make([]int, len(props))
	stride := 0
	for i, p := range props {
		offsets[i] = stride
		s := plyTypeSize(p.dataType)
		sizes[i] = s
		stride += s
	}

	buf := make([]byte, stride)
	for i := 0; i < count; i++ {
		if _, err := io.ReadFull(br, buf); err != nil {
			return fmt.Errorf("reading PLY binary vertex %d: %w", i, err)
		}

		x := readPLYFloat(buf[offsets[xi]:], props[xi].dataType, order)
		y := readPLYFloat(buf[offsets[yi]:], props[yi].dataType, order)
		z := readPLYFloat(buf[offsets[zi]:], props[zi].dataType, order)
		p := Point3D{X: x, Y: y, Z: z}
		if hasColor {
			p.R = uint8(readPLYFloat(buf[offsets[ri]:], props[ri].dataType, order))
			p.G = uint8(readPLYFloat(buf[offsets[gi]:], props[gi].dataType, order))
			p.B = uint8(readPLYFloat(buf[offsets[bi]:], props[bi].dataType, order))
			p.HasColor = true
		}
		acc.appendPoint(p)
	}
	return nil
}

func readPLYFloat(data []byte, dtype string, order binary.ByteOrder) float64 {
	switch dtype {
	case "float", "float32":
		return float64(math.Float32frombits(order.Uint32(data)))
	case "double", "float64":
		return math.Float64frombits(order.Uint64(data))
	case "uchar", "uint8":
		return float64(data[0])
	case "char", "int8":
		return float64(int8(data[0]))
	case "ushort", "uint16":
		return float64(order.Uint16(data))
	case "short", "int16":
		return float64(int16(order.Uint16(data)))
	case "uint", "uint32":
		return float64(order.Uint32(data))
	case "int", "int32":
		return float64(int32(order.Uint32(data)))
	default:
		return 0
	}
}

func plyTypeSize(dtype string) int {
	switch dtype {
	case "char", "uchar", "int8", "uint8":
		return 1
	case "short", "ushort", "int16", "uint16":
		return 2
	case "int", "uint", "int32", "uint32", "float", "float32":
		return 4
	case "double", "float64":
		return 8
	default:
		return 4
	}
}

// WritePLY writes a point cloud in PLY ASCII format.
func WritePLY(w io.Writer, pc *PointCloud) error {
	hasColor := len(pc.Points) > 0 && pc.Points[0].HasColor

	fmt.Fprintf(w, "ply\n")
	fmt.Fprintf(w, "format ascii 1.0\n")
	fmt.Fprintf(w, "element vertex %d\n", len(pc.Points))
	fmt.Fprintf(w, "property float x\n")
	fmt.Fprintf(w, "property float y\n")
	fmt.Fprintf(w, "property float z\n")
	if hasColor {
		fmt.Fprintf(w, "property uchar red\n")
		fmt.Fprintf(w, "property uchar green\n")
		fmt.Fprintf(w, "property uchar blue\n")
	}
	fmt.Fprintf(w, "end_header\n")

	bw := bufio.NewWriter(w)
	for _, p := range pc.Points {
		if hasColor {
			fmt.Fprintf(bw, "%f %f %f %d %d %d\n", p.X, p.Y, p.Z, p.R, p.G, p.B)
		} else {
			fmt.Fprintf(bw, "%f %f %f\n", p.X, p.Y, p.Z)
		}
	}
	return bw.Flush()
}
