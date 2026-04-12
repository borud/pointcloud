# streaming

Simulated LIDAR sweep of two objects (a sphere and a box) with point decay.

    go run .

A beam sweeps 360 degrees from the origin. Points appear where the beam
hits a surface and age out over time. The decay slider (0.1s-10s) controls
how quickly old points disappear -- turn it down and you'll see one object
fade as the beam moves to the other.

This demonstrates streaming data into the viewer. It keeps a ring buffer of timestamped batches, flatten on each tick, and calls `SetPointsPreserveView()`.

## files

- `main.go` -- app setup, update loop, decay slider
- `buffer.go` -- sliding window buffer with time and count eviction
- `generator.go` -- LIDAR sim with ray-sphere/ray-box intersection
