package geometry

import "math"

type pathVerb uint8

const (
	moveToVerb pathVerb = iota
	lineToVerb
	quadToVerb
	cubicToVerb
	closeVerb
)

// Path is a sequence of move, line, quadratic, cubic, and close commands.
type Path struct {
	verbs  []pathVerb
	points []Point
}

// NewPath returns an empty path.
func NewPath() *Path {
	return &Path{}
}

// MoveTo starts a new subpath at (xPos, yPos).
func (path *Path) MoveTo(xPos, yPos float64) {
	path.verbs = append(path.verbs, moveToVerb)
	path.points = append(path.points, Point{X: xPos, Y: yPos})
}

// LineTo appends a line to the current point.
func (path *Path) LineTo(xPos, yPos float64) {
	path.verbs = append(path.verbs, lineToVerb)
	path.points = append(path.points, Point{X: xPos, Y: yPos})
}

// QuadTo appends a quadratic curve with the given control point.
func (path *Path) QuadTo(controlX, controlY, xPos, yPos float64) {
	path.verbs = append(path.verbs, quadToVerb)
	path.points = append(path.points, Point{X: controlX, Y: controlY}, Point{X: xPos, Y: yPos})
}

// CubicTo appends a cubic curve with the given control points.
func (path *Path) CubicTo(control1X, control1Y, control2X, control2Y, xPos, yPos float64) {
	path.verbs = append(path.verbs, cubicToVerb)
	path.points = append(path.points,
		Point{X: control1X, Y: control1Y},
		Point{X: control2X, Y: control2Y},
		Point{X: xPos, Y: yPos},
	)
}

// Close closes the current subpath.
func (path *Path) Close() {
	path.verbs = append(path.verbs, closeVerb)
}

// AddRect appends an axis-aligned rectangle as a closed subpath.
func (path *Path) AddRect(rect Rect) {
	path.MoveTo(rect.X, rect.Y)
	path.LineTo(rect.X+rect.Width, rect.Y)
	path.LineTo(rect.X+rect.Width, rect.Y+rect.Height)
	path.LineTo(rect.X, rect.Y+rect.Height)
	path.Close()
}

// AddPolygon appends a closed polygon.
func (path *Path) AddPolygon(points []Point) {
	if len(points) == 0 {
		return
	}
	path.MoveTo(points[0].X, points[0].Y)
	for _, point := range points[1:] {
		path.LineTo(point.X, point.Y)
	}
	path.Close()
}

// AddCircle appends a closed approximation of a circle.
func (path *Path) AddCircle(centerX, centerY, radius float64) {
	const circleSegments = 32
	if radius <= 0 {
		return
	}
	for index := 0; index < circleSegments; index++ {
		angle := 2 * math.Pi * float64(index) / circleSegments
		xPos := centerX + radius*math.Cos(angle)
		yPos := centerY + radius*math.Sin(angle)
		if index == 0 {
			path.MoveTo(xPos, yPos)
		} else {
			path.LineTo(xPos, yPos)
		}
	}
	path.Close()
}

// CornerRadii holds the horizontal and vertical radii of each corner.
type CornerRadii struct {
	TopLeft     Point
	TopRight    Point
	BottomRight Point
	BottomLeft  Point
}

// AddRoundedRect appends a closed rounded rectangle. Radii are clamped and
// scaled down when adjacent corners would overlap, following CSS Backgrounds
// and Borders 3 §5.5.
func (path *Path) AddRoundedRect(rect Rect, radii CornerRadii) {
	if rect.Width <= 0 || rect.Height <= 0 {
		return
	}
	radii = NormalizeRadii(rect, radii)
	if radii.TopLeft.X == 0 && radii.TopRight.X == 0 && radii.BottomRight.X == 0 && radii.BottomLeft.X == 0 &&
		radii.TopLeft.Y == 0 && radii.TopRight.Y == 0 && radii.BottomRight.Y == 0 && radii.BottomLeft.Y == 0 {
		path.AddRect(rect)
		return
	}
	const arc = 0.5522847498307936
	right := rect.X + rect.Width
	bottom := rect.Y + rect.Height

	path.MoveTo(rect.X+radii.TopLeft.X, rect.Y)
	path.LineTo(right-radii.TopRight.X, rect.Y)
	path.CubicTo(
		right-radii.TopRight.X*(1-arc), rect.Y,
		right, rect.Y+radii.TopRight.Y*(1-arc),
		right, rect.Y+radii.TopRight.Y,
	)
	path.LineTo(right, bottom-radii.BottomRight.Y)
	path.CubicTo(
		right, bottom-radii.BottomRight.Y*(1-arc),
		right-radii.BottomRight.X*(1-arc), bottom,
		right-radii.BottomRight.X, bottom,
	)
	path.LineTo(rect.X+radii.BottomLeft.X, bottom)
	path.CubicTo(
		rect.X+radii.BottomLeft.X*(1-arc), bottom,
		rect.X, bottom-radii.BottomLeft.Y*(1-arc),
		rect.X, bottom-radii.BottomLeft.Y,
	)
	path.LineTo(rect.X, rect.Y+radii.TopLeft.Y)
	path.CubicTo(
		rect.X, rect.Y+radii.TopLeft.Y*(1-arc),
		rect.X+radii.TopLeft.X*(1-arc), rect.Y,
		rect.X+radii.TopLeft.X, rect.Y,
	)
	path.Close()
}

// NormalizeRadii clamps and scales corner radii to fit a rectangle.
func NormalizeRadii(rect Rect, radii CornerRadii) CornerRadii {
	clamp := func(radius Point) Point {
		return Point{X: math.Max(0, radius.X), Y: math.Max(0, radius.Y)}
	}
	radii.TopLeft = clamp(radii.TopLeft)
	radii.TopRight = clamp(radii.TopRight)
	radii.BottomRight = clamp(radii.BottomRight)
	radii.BottomLeft = clamp(radii.BottomLeft)
	scale := 1.0
	scale = math.Min(scale, rect.Width/(radii.TopLeft.X+radii.TopRight.X))
	scale = math.Min(scale, rect.Width/(radii.BottomLeft.X+radii.BottomRight.X))
	scale = math.Min(scale, rect.Height/(radii.TopLeft.Y+radii.BottomLeft.Y))
	scale = math.Min(scale, rect.Height/(radii.TopRight.Y+radii.BottomRight.Y))
	if scale < 1 {
		radii.TopLeft = radii.TopLeft.Scaled(scale)
		radii.TopRight = radii.TopRight.Scaled(scale)
		radii.BottomRight = radii.BottomRight.Scaled(scale)
		radii.BottomLeft = radii.BottomLeft.Scaled(scale)
	}
	return radii
}

// Subpath is a flattened path subpath.
type Subpath struct {
	Points []Point
	Closed bool
}

// Flatten converts the path into line segments.
func (path *Path) Flatten(tolerance float64) []Subpath {
	if tolerance <= 0 {
		tolerance = 0.25
	}
	var subpaths []Subpath
	var current []Point
	closed := false
	flush := func() {
		if len(current) > 0 {
			subpaths = append(subpaths, Subpath{Points: current, Closed: closed})
		}
		current = nil
		closed = false
	}

	pointIndex := 0
	currentPoint := Point{}
	startPoint := Point{}
	for _, verb := range path.verbs {
		switch verb {
		case moveToVerb:
			flush()
			currentPoint = path.points[pointIndex]
			startPoint = currentPoint
			current = append(current, currentPoint)
			pointIndex++
		case lineToVerb:
			currentPoint = path.points[pointIndex]
			current = append(current, currentPoint)
			pointIndex++
		case quadToVerb:
			control := path.points[pointIndex]
			end := path.points[pointIndex+1]
			current = flattenQuad(current, currentPoint, control, end, tolerance, 0)
			currentPoint = end
			pointIndex += 2
		case cubicToVerb:
			control1 := path.points[pointIndex]
			control2 := path.points[pointIndex+1]
			end := path.points[pointIndex+2]
			current = flattenCubic(current, currentPoint, control1, control2, end, tolerance, 0)
			currentPoint = end
			pointIndex += 3
		case closeVerb:
			if len(current) > 0 {
				closed = true
				currentPoint = startPoint
				flush()
			}
		}
	}
	flush()
	return subpaths
}

// Bounds returns the tight bounding box of the path's control points.
func (path *Path) Bounds() Rect {
	if path == nil || len(path.points) == 0 {
		return Rect{}
	}
	minX := path.points[0].X
	minY := path.points[0].Y
	maxX := minX
	maxY := minY
	for _, point := range path.points[1:] {
		minX = math.Min(minX, point.X)
		minY = math.Min(minY, point.Y)
		maxX = math.Max(maxX, point.X)
		maxY = math.Max(maxY, point.Y)
	}
	return Rect{X: minX, Y: minY, Width: maxX - minX, Height: maxY - minY}
}

// Transform returns a copy of the path with the affine transformation applied.
func (path *Path) Transform(matrix Affine) *Path {
	transformed := &Path{}
	pointIndex := 0
	for _, verb := range path.verbs {
		switch verb {
		case moveToVerb:
			point := matrix.Apply(path.points[pointIndex])
			transformed.MoveTo(point.X, point.Y)
			pointIndex++
		case lineToVerb:
			point := matrix.Apply(path.points[pointIndex])
			transformed.LineTo(point.X, point.Y)
			pointIndex++
		case quadToVerb:
			control := matrix.Apply(path.points[pointIndex])
			end := matrix.Apply(path.points[pointIndex+1])
			transformed.QuadTo(control.X, control.Y, end.X, end.Y)
			pointIndex += 2
		case cubicToVerb:
			control1 := matrix.Apply(path.points[pointIndex])
			control2 := matrix.Apply(path.points[pointIndex+1])
			end := matrix.Apply(path.points[pointIndex+2])
			transformed.CubicTo(control1.X, control1.Y, control2.X, control2.Y, end.X, end.Y)
			pointIndex += 3
		case closeVerb:
			transformed.Close()
		}
	}
	return transformed
}

func flattenQuad(points []Point, start, control, end Point, tolerance float64, depth int) []Point {
	if depth >= 20 || distanceToLine(control, start, end) <= tolerance {
		return append(points, end)
	}
	left := midpoint(start, control)
	right := midpoint(control, end)
	center := midpoint(left, right)
	points = flattenQuad(points, start, left, center, tolerance, depth+1)
	return flattenQuad(points, center, right, end, tolerance, depth+1)
}

func flattenCubic(points []Point, start, control1, control2, end Point, tolerance float64, depth int) []Point {
	if depth >= 20 {
		return append(points, end)
	}
	flatness := math.Max(distanceToLine(control1, start, end), distanceToLine(control2, start, end))
	if flatness <= tolerance {
		return append(points, end)
	}
	startControl1 := midpoint(start, control1)
	control1Control2 := midpoint(control1, control2)
	control2End := midpoint(control2, end)
	startMiddle := midpoint(startControl1, control1Control2)
	middleEnd := midpoint(control1Control2, control2End)
	middle := midpoint(startMiddle, middleEnd)
	points = flattenCubic(points, start, startControl1, startMiddle, middle, tolerance, depth+1)
	return flattenCubic(points, middle, middleEnd, control2End, end, tolerance, depth+1)
}

func midpoint(first, second Point) Point {
	return Point{X: (first.X + second.X) / 2, Y: (first.Y + second.Y) / 2}
}

func distanceToLine(point, start, end Point) float64 {
	direction := end.Subtract(start)
	length := direction.Length()
	if length == 0 {
		return point.Subtract(start).Length()
	}
	return math.Abs(CrossProduct(direction, point.Subtract(start))) / length
}
