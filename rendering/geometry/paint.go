package geometry

// FillRule selects how overlapping subpaths are filled.
type FillRule uint8

const (
	// NonZeroFill fills regions with a nonzero winding number.
	NonZeroFill FillRule = iota
	// EvenOddFill fills regions with an odd crossing count.
	EvenOddFill
)

// LineCap selects how open subpath ends are drawn.
type LineCap uint8

const (
	// ButtCap ends the stroke exactly at the endpoint.
	ButtCap LineCap = iota
	// RoundCap draws a semicircle at the endpoint.
	RoundCap
	// SquareCap extends the stroke by half its width.
	SquareCap
)

// LineJoin selects how segments meet at a vertex.
type LineJoin uint8

const (
	// MiterJoin extends the outer edges to a point.
	MiterJoin LineJoin = iota
	// BevelJoin connects the outer edges with a straight line.
	BevelJoin
	// RoundJoin draws a circular arc at the vertex.
	RoundJoin
)

// StrokeStyle configures path stroking.
type StrokeStyle struct {
	Width      float64
	Cap        LineCap
	Join       LineJoin
	MiterLimit float64
}

// DefaultStrokeStyle returns a butt/round stroke of the given width.
func DefaultStrokeStyle(width float64) StrokeStyle {
	return StrokeStyle{Width: width, Cap: ButtCap, Join: RoundJoin, MiterLimit: 10}
}

// CompositeOp is a Porter-Duff compositing operator.
type CompositeOp uint8

const (
	// SourceOver places the source over the destination.
	SourceOver CompositeOp = iota
	// SourceIn keeps the source inside the destination alpha.
	SourceIn
	// SourceOut keeps the source outside the destination alpha.
	SourceOut
	// SourceAtop places the source over the destination, clipped to it.
	SourceAtop
	// DestinationOver places the destination over the source.
	DestinationOver
	// DestinationIn keeps the destination inside the source alpha.
	DestinationIn
	// DestinationOut keeps the destination outside the source alpha.
	DestinationOut
	// DestinationAtop places the destination over the source, clipped to it.
	DestinationAtop
	// Xor keeps the non-overlapping parts of both images.
	Xor
	// Clear clears the destination.
	Clear
	// Copy replaces the destination with the source.
	Copy
)
