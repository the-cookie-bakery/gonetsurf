package geometry

import "math"

// Rect is an axis-aligned rectangle in surface coordinates.
type Rect struct {
	X      float64
	Y      float64
	Width  float64
	Height float64
}

// IsEmpty reports whether the rectangle has no area.
func (rect Rect) IsEmpty() bool {
	return rect.Width <= 0 || rect.Height <= 0
}

// Intersect returns the overlapping part of two rectangles.
func (rect Rect) Intersect(other Rect) Rect {
	minX := math.Max(rect.X, other.X)
	minY := math.Max(rect.Y, other.Y)
	maxX := math.Min(rect.X+rect.Width, other.X+other.Width)
	maxY := math.Min(rect.Y+rect.Height, other.Y+other.Height)
	if maxX <= minX || maxY <= minY {
		return Rect{}
	}
	return Rect{X: minX, Y: minY, Width: maxX - minX, Height: maxY - minY}
}

// ContainsPoint reports whether a point is inside the rectangle.
func (rect Rect) ContainsPoint(xPos, yPos float64) bool {
	return xPos >= rect.X && xPos < rect.X+rect.Width &&
		yPos >= rect.Y && yPos < rect.Y+rect.Height
}

// Point is a position in surface coordinates.
type Point struct {
	X float64
	Y float64
}

// Add returns the vector sum.
func (point Point) Add(other Point) Point {
	return Point{X: point.X + other.X, Y: point.Y + other.Y}
}

// Subtract returns the vector difference.
func (point Point) Subtract(other Point) Point {
	return Point{X: point.X - other.X, Y: point.Y - other.Y}
}

// Scaled multiplies the vector by a factor.
func (point Point) Scaled(factor float64) Point {
	return Point{X: point.X * factor, Y: point.Y * factor}
}

// Length returns the vector magnitude.
func (point Point) Length() float64 {
	return math.Hypot(point.X, point.Y)
}

// Normalized returns a unit vector, or the zero vector when empty.
func (point Point) Normalized() Point {
	length := point.Length()
	if length == 0 {
		return Point{}
	}
	return Point{X: point.X / length, Y: point.Y / length}
}

// LeftNormal returns the left-hand normal of the vector.
func (point Point) LeftNormal() Point {
	return Point{X: -point.Y, Y: point.X}
}

// CrossProduct returns the scalar cross product of two vectors.
func CrossProduct(first, second Point) float64 {
	return first.X*second.Y - first.Y*second.X
}
