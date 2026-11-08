package rendering

import "gonetsurf/rendering/geometry"

// Geometry aliases keep the frontend painter source readable while the
// canonical definitions live in rendering/geometry.
type Color = geometry.Color
type Rect = geometry.Rect
type Point = geometry.Point
type Path = geometry.Path
type CornerRadii = geometry.CornerRadii
type FillRule = geometry.FillRule
type StrokeStyle = geometry.StrokeStyle
type LineCap = geometry.LineCap
type LineJoin = geometry.LineJoin
type CompositeOp = geometry.CompositeOp
type Mask = geometry.Mask

const (
	NonZeroFill = geometry.NonZeroFill
	EvenOddFill = geometry.EvenOddFill

	ButtCap   = geometry.ButtCap
	RoundCap  = geometry.RoundCap
	SquareCap = geometry.SquareCap

	MiterJoin = geometry.MiterJoin
	BevelJoin = geometry.BevelJoin
	RoundJoin = geometry.RoundJoin

	SourceOver      = geometry.SourceOver
	SourceIn        = geometry.SourceIn
	SourceOut       = geometry.SourceOut
	SourceAtop      = geometry.SourceAtop
	DestinationOver = geometry.DestinationOver
	DestinationIn   = geometry.DestinationIn
	DestinationOut  = geometry.DestinationOut
	DestinationAtop = geometry.DestinationAtop
	Xor             = geometry.Xor
	Clear           = geometry.Clear
	Copy            = geometry.Copy
)

var Transparent = geometry.Transparent

// NewColor returns an opaque color.
func NewColor(red, green, blue uint8) Color {
	return geometry.NewColor(red, green, blue)
}

// NewPath returns an empty path.
func NewPath() *Path {
	return geometry.NewPath()
}

// NewMask returns an empty coverage mask.
func NewMask(width, height int) *Mask {
	return geometry.NewMask(width, height)
}

// DefaultStrokeStyle returns a butt/round stroke.
func DefaultStrokeStyle(width float64) StrokeStyle {
	return geometry.DefaultStrokeStyle(width)
}

// RasterizeFill rasterizes a path into a coverage mask.
func RasterizeFill(path *Path, width, height int, rule FillRule) *Mask {
	return geometry.RasterizeFill(path, width, height, rule)
}

// RasterizeStroke rasterizes a stroked path into a coverage mask.
func RasterizeStroke(path *Path, width, height int, style StrokeStyle) *Mask {
	return geometry.RasterizeStroke(path, width, height, style)
}

// BlurMask blurs a coverage mask.
func BlurMask(mask *Mask, radius float64) *Mask {
	return geometry.BlurMask(mask, radius)
}

func toGeometryAffine(matrix affine) geometry.Affine {
	return geometry.Affine{A: matrix.A, B: matrix.B, C: matrix.C, D: matrix.D, E: matrix.E, F: matrix.F}
}

func toGeometryMatrix4(matrix matrix4) geometry.Matrix4 {
	var result geometry.Matrix4
	copy(result[:], matrix[:])
	return result
}

func fromGeometryAffine(matrix geometry.Affine) affine {
	return affine{A: matrix.A, B: matrix.B, C: matrix.C, D: matrix.D, E: matrix.E, F: matrix.F}
}

func fromGeometryMatrix4(matrix geometry.Matrix4) matrix4 {
	var result matrix4
	copy(result[:], matrix[:])
	return result
}
