// Package pointcloud provides point cloud data types and format readers.
package pointcloud

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
)

// Point3D represents a point in 3D space with optional color.
type Point3D struct {
	X, Y, Z  float64
	R, G, B  uint8
	HasColor bool
}

// PointCloud holds a collection of 3D points with bounding box metadata.
type PointCloud struct {
	Points           []Point3D
	MinX, MinY, MinZ float64
	MaxX, MaxY, MaxZ float64
	boundsComputed   bool

	// NormScale is the scale factor applied by Normalize (2.0/maxDim).
	// Zero if Normalize has not been called.
	NormScale float64
	// NormCenter is the center point subtracted during Normalize.
	NormCenter [3]float64
}

// ComputeBounds calculates the bounding box of the point cloud.
func (pc *PointCloud) ComputeBounds() {
	if len(pc.Points) == 0 {
		return
	}
	pc.MinX, pc.MinY, pc.MinZ = math.MaxFloat64, math.MaxFloat64, math.MaxFloat64
	pc.MaxX, pc.MaxY, pc.MaxZ = -math.MaxFloat64, -math.MaxFloat64, -math.MaxFloat64
	for _, p := range pc.Points {
		if p.X < pc.MinX {
			pc.MinX = p.X
		}
		if p.Y < pc.MinY {
			pc.MinY = p.Y
		}
		if p.Z < pc.MinZ {
			pc.MinZ = p.Z
		}
		if p.X > pc.MaxX {
			pc.MaxX = p.X
		}
		if p.Y > pc.MaxY {
			pc.MaxY = p.Y
		}
		if p.Z > pc.MaxZ {
			pc.MaxZ = p.Z
		}
	}
	pc.boundsComputed = true
}

// Normalize centers the point cloud at the origin and scales it so the
// largest dimension spans [-1, 1].
func (pc *PointCloud) Normalize() {
	if len(pc.Points) == 0 {
		return
	}
	if !pc.boundsComputed {
		pc.ComputeBounds()
	}

	cx := (pc.MinX + pc.MaxX) / 2
	cy := (pc.MinY + pc.MaxY) / 2
	cz := (pc.MinZ + pc.MaxZ) / 2

	dx := pc.MaxX - pc.MinX
	dy := pc.MaxY - pc.MinY
	dz := pc.MaxZ - pc.MinZ
	maxDim := math.Max(dx, math.Max(dy, dz))
	if maxDim == 0 {
		maxDim = 1
	}
	scale := 2.0 / maxDim

	pc.NormScale = scale
	pc.NormCenter = [3]float64{cx, cy, cz}

	for i := range pc.Points {
		pc.Points[i].X = (pc.Points[i].X - cx) * scale
		pc.Points[i].Y = (pc.Points[i].Y - cy) * scale
		pc.Points[i].Z = (pc.Points[i].Z - cz) * scale
	}

	pc.ComputeBounds()
}

// maxFieldIndex returns the maximum of the given field indices.
func maxFieldIndex(indices ...int) int {
	m := indices[0]
	for _, idx := range indices[1:] {
		if idx > m {
			m = idx
		}
	}
	return m
}

// SupportedExtensions returns all file extensions the readers handle.
func SupportedExtensions() []string {
	return []string{".xyz", ".ply", ".pts", ".pcd"}
}

// ReadFile detects the format by file extension and reads the point cloud.
func ReadFile(path string) (*PointCloud, error) {
	ext := strings.ToLower(filepath.Ext(path))

	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	switch ext {
	case ".xyz":
		return ReadXYZ(f)
	case ".ply":
		return ReadPLY(f)
	case ".pts":
		return ReadPTS(f)
	case ".pcd":
		return ReadPCD(f)
	default:
		return nil, fmt.Errorf("unsupported format: %s", ext)
	}
}
