// pcbench is an end-to-end benchmark for the point cloud viewer.
//
// It opens a Fyne window, loads or generates a point cloud, drives an
// automated rotation for 30 seconds, and reports frame time statistics.
package main

import (
	"flag"
	"fmt"
	"math"
	"math/rand/v2"
	"os"
	"sort"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"

	"github.com/borud/pointcloud/pkg/pcviewer"
	"github.com/borud/pointcloud/pkg/pointcloud"
)

func main() {
	filePath := flag.String("file", "", "PLY file to load")
	nPoints := flag.Int("points", 500_000, "number of synthetic points (if no --file)")
	duration := flag.Duration("duration", 30*time.Second, "benchmark duration")
	lod := flag.Bool("lod", false, "enable LOD decimation during interaction")
	flag.Parse()

	var pts []pointcloud.Point3D
	if *filePath != "" {
		pc, err := pointcloud.ReadFile(*filePath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		pc.Normalize()
		pts = pc.Points
		fmt.Printf("loaded %d points from %s\n", len(pts), *filePath)
	} else {
		pts = generateSyntheticPoints(*nPoints)
		fmt.Printf("generated %d synthetic points\n", len(pts))
	}

	a := app.New()
	w := a.NewWindow("pcbench")
	w.Resize(fyne.NewSize(1024, 768))

	v := pcviewer.New()
	v.SetUpAxis(pcviewer.ZUp)
	v.SetLODEnabled(*lod)
	w.SetContent(v)

	// Collect frame render times from the draw callback.
	var mu sync.Mutex
	var frameTimes []time.Duration

	v.SetOnFrameDrawn(func(d time.Duration) {
		mu.Lock()
		frameTimes = append(frameTimes, d)
		mu.Unlock()
	})

	// Set points after the window is shown so sizing is available.
	go func() {
		time.Sleep(500 * time.Millisecond)
		v.SetPoints(pts)

		runBenchmark(v, &mu, &frameTimes, *duration)
		fyne.Do(func() {
			a.Quit()
		})
	}()

	w.ShowAndRun()

	// Print final stats after window closes.
	mu.Lock()
	printStats(frameTimes)
	mu.Unlock()
}

func runBenchmark(v *pcviewer.Viewer, mu *sync.Mutex, allFrameTimes *[]time.Duration, dur time.Duration) {
	// Small rotation per frame to simulate interaction.
	dq := pcviewer.QuatFromAxisAngle(0.2, 0.7, 0.1, 0.02)

	// Interval buffer; drained each second. All times are also
	// appended to allFrameTimes for the final summary.
	var intervalBuf []time.Duration

	deadline := time.Now().Add(dur)
	lastPrint := time.Now()

	for time.Now().Before(deadline) {
		q := v.Orientation()
		v.SetOrientation(dq.Mul(q).Normalize())

		// Print rolling stats once per second.
		elapsed := time.Since(lastPrint)
		if elapsed >= time.Second {
			mu.Lock()
			intervalBuf = append(intervalBuf, *allFrameTimes...)
			*allFrameTimes = (*allFrameTimes)[:0]
			mu.Unlock()

			n := len(intervalBuf)
			if n > 0 {
				var sum time.Duration
				for _, t := range intervalBuf {
					sum += t
				}
				avg := sum / time.Duration(n)
				fps := float64(n) / elapsed.Seconds()
				fmt.Printf("frames=%d  avg_render=%v  fps=%.1f\n", n, avg, fps)
			}

			// Keep for final summary, then reset interval buffer.
			mu.Lock()
			*allFrameTimes = append(*allFrameTimes, intervalBuf...)
			mu.Unlock()
			intervalBuf = intervalBuf[:0]
			lastPrint = time.Now()
		}

		// Yield to let Fyne render.
		time.Sleep(1 * time.Millisecond)
	}

}

func printStats(times []time.Duration) {
	if len(times) == 0 {
		fmt.Println("no frames recorded")
		return
	}

	sort.Slice(times, func(i, j int) bool { return times[i] < times[j] })

	var sum time.Duration
	for _, t := range times {
		sum += t
	}

	mean := sum / time.Duration(len(times))
	p50 := times[len(times)/2]
	p95 := times[int(math.Ceil(float64(len(times))*0.95))-1]
	p99 := times[int(math.Ceil(float64(len(times))*0.99))-1]

	meanFPS := 1.0 / mean.Seconds()

	fmt.Println("\n=== Summary ===")
	fmt.Printf("frames: %d\n", len(times))
	fmt.Printf("mean:   %v  (%.1f FPS)\n", mean, meanFPS)
	fmt.Printf("p50:    %v  (%.1f FPS)\n", p50, 1.0/p50.Seconds())
	fmt.Printf("p95:    %v  (%.1f FPS)\n", p95, 1.0/p95.Seconds())
	fmt.Printf("p99:    %v  (%.1f FPS)\n", p99, 1.0/p99.Seconds())
}

func generateSyntheticPoints(n int) []pointcloud.Point3D {
	rng := rand.New(rand.NewPCG(42, 0))

	pts := make([]pointcloud.Point3D, n)
	for i := range pts {
		pts[i] = pointcloud.Point3D{
			X: rng.Float64()*2 - 1,
			Y: rng.Float64()*2 - 1,
			Z: rng.Float64()*2 - 1,
		}
		if i%2 == 0 {
			pts[i].HasColor = true
			pts[i].R = uint8(rng.IntN(256))
			pts[i].G = uint8(rng.IntN(256))
			pts[i].B = uint8(rng.IntN(256))
		}
	}

	// Normalize to [-1, 1].
	pc := &pointcloud.PointCloud{Points: pts}
	pc.Normalize()
	return pc.Points
}
