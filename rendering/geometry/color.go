// Package geometry contains backend-neutral rendering value types and pure
// geometry, color, mask, and transform math shared by every rendering
// pipeline. It has no dependencies outside the standard library.
package geometry

import "math"

// Color is a straight (non-premultiplied) sRGB color with 8-bit channels.
type Color struct {
	Red   uint8
	Green uint8
	Blue  uint8
	Alpha uint8
}

// NewColor returns an opaque color from its channels.
func NewColor(red, green, blue uint8) Color {
	return Color{Red: red, Green: green, Blue: blue, Alpha: 255}
}

// Transparent is the fully transparent color.
var Transparent = Color{}

// Premultiplied scales the color by its alpha and coverage.
func (color Color) Premultiplied(coverage float64) (red, green, blue, alpha uint8) {
	if coverage <= 0 || color.Alpha == 0 {
		return 0, 0, 0, 0
	}
	scale := float64(color.Alpha) / 255 * coverage
	return ToByte(float64(color.Red) / 255 * scale),
		ToByte(float64(color.Green) / 255 * scale),
		ToByte(float64(color.Blue) / 255 * scale),
		ToByte(scale)
}

// ToByte clamps a normalized value and converts it to an 8-bit channel.
func ToByte(value float64) uint8 {
	if value <= 0 {
		return 0
	}
	if value >= 1 {
		return 255
	}
	return uint8(math.Round(value * 255))
}
