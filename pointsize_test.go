package pointcloud

import "testing"

// TestProjectChunkSized verifies the enlarged-point splat covers the expected
// number of pixels for each size, shape and mode. A single point at the origin
// projects to the exact center pixel under identity rotation at camera distance
// 4.0, so the splat is fully inside the framebuffer and its pixel count is
// deterministic.
func TestProjectChunkSized(t *testing.T) {
	const w, h = 64, 64
	stride := w * 4
	cx, cy := float32(w)/2, float32(h)/2

	xs := []float32{0}
	ys := []float32{0}
	zs := []float32{0}
	rgba := []uint32{0} // no color bit: renders with the default color

	countLit := func(pix []byte) int {
		n := 0
		for i := 0; i < len(pix); i += 4 {
			if pix[i] != 0 || pix[i+1] != 0 || pix[i+2] != 0 {
				n++
			}
		}
		return n
	}

	run := func(size int, shape PointShape, mode PointSizeMode) int {
		pix := make([]byte, w*h*4)
		projectChunkSized(xs, ys, zs, rgba, pix, stride, w, h,
			1, 0, 0, 0, 1, 0, 0, 0, 1,
			0, 0, 4.0,
			1.0, cx, cy, 255, 255, 255,
			size, shape, mode, nil)
		return countLit(pix)
	}

	cases := []struct {
		name  string
		size  int
		shape PointShape
		mode  PointSizeMode
		want  int
	}{
		{"size-1-single-pixel", 1, PointSquare, PointSizeFixed, 1},
		{"square-3", 3, PointSquare, PointSizeFixed, 9},
		{"square-5", 5, PointSquare, PointSizeFixed, 25},
		{"round-5-disc", 5, PointRound, PointSizeFixed, 13},
		{"depth-scaled-5-at-center", 5, PointSquare, PointSizeDepthScaled, 25},
	}
	for _, tc := range cases {
		got := run(tc.size, tc.shape, tc.mode)
		if got != tc.want {
			t.Errorf("%s: got %d lit pixels, want %d", tc.name, got, tc.want)
		}
	}
}

// TestBlendStampAntialiased verifies that compositing a coverage stamp yields
// both fully covered interior pixels and partially covered (antialiased) edge
// pixels — the hard-disc path would produce only 0 or 255.
func TestBlendStampAntialiased(t *testing.T) {
	const w, h = 64, 64
	stride := w * 4
	pix := make([]byte, w*h*4)

	stamps := buildStamps(6)
	if stamps[4] == nil {
		t.Fatal("buildStamps did not produce a radius-4 stamp")
	}

	// White opaque point at the center, radius 4, over a black framebuffer.
	blendStamp(pix, stride, w, h, 32, 32, 4, stamps[4], 0xFFFFFFFF)

	full, partial := 0, 0
	for i := 0; i < len(pix); i += 4 {
		switch v := pix[i]; {
		case v == 255:
			full++
		case v > 0:
			partial++
		}
	}
	if full == 0 {
		t.Error("expected fully covered interior pixels")
	}
	if partial == 0 {
		t.Error("expected partially covered (antialiased) edge pixels")
	}
}
