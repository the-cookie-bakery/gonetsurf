package software

import "gonetsurf/rendering/geometry"

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
	RoundCap = geometry.RoundCap
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

func NewColor(red, green, blue uint8) Color { return geometry.NewColor(red, green, blue) }

func NewPath() *Path { return geometry.NewPath() }

func NewMask(width, height int) *Mask { return geometry.NewMask(width, height) }

func DefaultStrokeStyle(width float64) StrokeStyle { return geometry.DefaultStrokeStyle(width) }

func RasterizeFill(path *Path, width, height int, rule FillRule) *Mask {
	return geometry.RasterizeFill(path, width, height, rule)
}

func RasterizeStroke(path *Path, width, height int, style StrokeStyle) *Mask {
	return geometry.RasterizeStroke(path, width, height, style)
}

func BlurMask(mask *Mask, radius float64) *Mask { return geometry.BlurMask(mask, radius) }
