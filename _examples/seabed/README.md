# seabed

Procedurally generated seabed terrain with bathymetric depth coloring.

    go run .

Layered noise produces hills, a trench, scattered rocks, and sand ripples.
Color goes from dark blue in the deep parts to sandy tan at the peaks.
Every regeneration uses a new random seed so you get a different landscape
each time.

## controls

- Height -- adjusts terrain relief without regenerating. Crank it down
  for a nearly flat ocean floor or up for exaggerated peaks.
- Points -- log-scale slider from 100K to 50M. Set the value and hit
  Regenerate to rebuild at the new resolution.
- Regenerate -- new random terrain at the current point count.
