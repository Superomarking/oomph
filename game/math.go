package game

import (
	"math"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/go-gl/mathgl/mgl64"
)

// Round will round a number to a given precision.
func Round(val float64, precision int) float64 {
	p := math.Pow10(precision)
	value := float64(int(val*p)) / p
	return value
}

// Vec32To64 converts a 32 bit vector to a 64 bit one.
func Vec32To64(vec3 mgl32.Vec3) mgl64.Vec3 {
	return mgl64.Vec3{float64(vec3[0]), float64(vec3[1]), float64(vec3[2])}
}

// DirectionVector returns a direction vector from the given yaw and pitch values.
func DirectionVector(yaw, pitch float64) mgl64.Vec3 {
	yawRad, pitchRad := mgl64.DegToRad(yaw), mgl64.DegToRad(pitch)
	m := math.Cos(pitchRad)

	return mgl64.Vec3{
		-m * math.Sin(yawRad),
		-math.Sin(pitchRad),
		m * math.Cos(yawRad),
	}
}

// AbsVec64 will return the given vector, but all the values of it are switched to their absolute values.
func AbsVec64(vec mgl64.Vec3) mgl64.Vec3 {
	return mgl64.Vec3{math.Abs(vec.X()), math.Abs(vec.Y()), math.Abs(vec.Z())}
}
