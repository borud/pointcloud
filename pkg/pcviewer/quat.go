package pcviewer

import "math"

// Quat represents a unit quaternion for 3D rotation.
type Quat struct {
	X, Y, Z, W float64
}

// QuatIdentity returns the identity quaternion (no rotation).
func QuatIdentity() Quat {
	return Quat{W: 1}
}

// QuatFromAxisAngle creates a quaternion from an axis and angle (radians).
func QuatFromAxisAngle(ax, ay, az, angle float64) Quat {
	s := math.Sin(angle / 2)
	return Quat{
		X: ax * s,
		Y: ay * s,
		Z: az * s,
		W: math.Cos(angle / 2),
	}
}

// QuatFromEulerXY creates a quaternion matching the old Euler convention:
// rotate by ay around Y, then by ax around X.
func QuatFromEulerXY(ax, ay float64) Quat {
	qx := QuatFromAxisAngle(1, 0, 0, ax)
	qy := QuatFromAxisAngle(0, 1, 0, ay)
	return qx.Mul(qy)
}

// Mul returns the Hamilton product a*b (apply b first, then a).
func (a Quat) Mul(b Quat) Quat {
	return Quat{
		W: a.W*b.W - a.X*b.X - a.Y*b.Y - a.Z*b.Z,
		X: a.W*b.X + a.X*b.W + a.Y*b.Z - a.Z*b.Y,
		Y: a.W*b.Y - a.X*b.Z + a.Y*b.W + a.Z*b.X,
		Z: a.W*b.Z + a.X*b.Y - a.Y*b.X + a.Z*b.W,
	}
}

// Normalize returns the quaternion scaled to unit length.
func (q Quat) Normalize() Quat {
	l := math.Sqrt(q.X*q.X + q.Y*q.Y + q.Z*q.Z + q.W*q.W)
	if l < 1e-10 {
		return QuatIdentity()
	}
	return Quat{q.X / l, q.Y / l, q.Z / l, q.W / l}
}

// ToMatrix returns a row-major 3x3 rotation matrix.
func (q Quat) ToMatrix() [9]float64 {
	xx, yy, zz := q.X*q.X, q.Y*q.Y, q.Z*q.Z
	xy, xz, yz := q.X*q.Y, q.X*q.Z, q.Y*q.Z
	wx, wy, wz := q.W*q.X, q.W*q.Y, q.W*q.Z
	return [9]float64{
		1 - 2*(yy+zz), 2 * (xy - wz), 2 * (xz + wy),
		2 * (xy + wz), 1 - 2*(xx+zz), 2 * (yz - wx),
		2 * (xz - wy), 2 * (yz + wx), 1 - 2*(xx+yy),
	}
}
