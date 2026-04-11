package pointcloud

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

// Mul returns the Hamilton product q*b (apply b first, then q).
func (q Quat) Mul(b Quat) Quat {
	return Quat{
		W: q.W*b.W - q.X*b.X - q.Y*b.Y - q.Z*b.Z,
		X: q.W*b.X + q.X*b.W + q.Y*b.Z - q.Z*b.Y,
		Y: q.W*b.Y - q.X*b.Z + q.Y*b.W + q.Z*b.X,
		Z: q.W*b.Z + q.X*b.Y - q.Y*b.X + q.Z*b.W,
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

// Conjugate returns the conjugate of the quaternion, which for unit
// quaternions is also the inverse rotation.
func (q Quat) Conjugate() Quat {
	return Quat{X: -q.X, Y: -q.Y, Z: -q.Z, W: q.W}
}

// RotateVec3 rotates a 3D vector by the quaternion using the formula q*v*q^-1.
func (q Quat) RotateVec3(v [3]float64) [3]float64 {
	// Optimized quaternion-vector rotation (avoids full quaternion multiply).
	// t = 2 * cross(q.xyz, v)
	tx := 2 * (q.Y*v[2] - q.Z*v[1])
	ty := 2 * (q.Z*v[0] - q.X*v[2])
	tz := 2 * (q.X*v[1] - q.Y*v[0])
	// result = v + q.w*t + cross(q.xyz, t)
	return [3]float64{
		v[0] + q.W*tx + (q.Y*tz - q.Z*ty),
		v[1] + q.W*ty + (q.Z*tx - q.X*tz),
		v[2] + q.W*tz + (q.X*ty - q.Y*tx),
	}
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
