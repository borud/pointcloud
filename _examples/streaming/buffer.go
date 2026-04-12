package main

import (
	"sync"
	"time"

	"github.com/borud/pointcloud"
)

type batch struct {
	points  []pointcloud.Point3D
	arrived time.Time
}

// StreamBuffer maintains a sliding window of point batches with
// time-based and count-based eviction.
type StreamBuffer struct {
	mu        sync.Mutex
	batches   []batch
	maxPoints int
	maxAge    time.Duration
}

// NewStreamBuffer creates a buffer that evicts batches older than maxAge
// and trims oldest batches when total points exceed maxPoints.
func NewStreamBuffer(maxPoints int, maxAge time.Duration) *StreamBuffer {
	return &StreamBuffer{
		maxPoints: maxPoints,
		maxAge:    maxAge,
	}
}

// SetMaxAge updates the maximum batch age. Safe for concurrent use.
func (b *StreamBuffer) SetMaxAge(d time.Duration) {
	b.mu.Lock()
	b.maxAge = d
	b.evict()
	b.mu.Unlock()
}

// Add appends a new batch of points and evicts stale data.
func (b *StreamBuffer) Add(pts []pointcloud.Point3D) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.batches = append(b.batches, batch{
		points:  pts,
		arrived: time.Now(),
	})
	b.evict()
}

func (b *StreamBuffer) evict() {
	now := time.Now()

	// Phase 1: remove batches older than maxAge.
	cutoff := 0
	for cutoff < len(b.batches) && now.Sub(b.batches[cutoff].arrived) > b.maxAge {
		cutoff++
	}
	if cutoff > 0 {
		b.batches = b.batches[cutoff:]
	}

	// Phase 2: remove oldest batches until total points <= maxPoints.
	total := 0
	for _, batch := range b.batches {
		total += len(batch.points)
	}
	for len(b.batches) > 0 && total > b.maxPoints {
		total -= len(b.batches[0].points)
		b.batches = b.batches[1:]
	}
}

// Flatten concatenates all batch points into a single pre-allocated slice.
func (b *StreamBuffer) Flatten() []pointcloud.Point3D {
	b.mu.Lock()
	defer b.mu.Unlock()

	total := 0
	for _, batch := range b.batches {
		total += len(batch.points)
	}

	pts := make([]pointcloud.Point3D, 0, total)
	for _, batch := range b.batches {
		pts = append(pts, batch.points...)
	}
	return pts
}

// Stats returns the current buffer state for display.
func (b *StreamBuffer) Stats() (batches, points int, oldestAge time.Duration) {
	b.mu.Lock()
	defer b.mu.Unlock()

	for _, batch := range b.batches {
		points += len(batch.points)
	}
	batches = len(b.batches)
	if batches > 0 {
		oldestAge = time.Since(b.batches[0].arrived)
	}
	return
}
