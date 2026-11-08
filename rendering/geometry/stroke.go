package geometry

import "math"

// RasterizeStroke rasterizes a stroked path into a coverage mask.
func RasterizeStroke(path *Path, width, height int, style StrokeStyle) *Mask {
	mask := NewMask(width, height)
	if path == nil || style.Width <= 0 {
		return mask
	}
	if style.MiterLimit <= 0 {
		style.MiterLimit = 10
	}
	halfWidth := style.Width / 2
	for _, subpath := range path.Flatten(0.25) {
		points := deduplicatePoints(subpath.Points)
		if len(points) == 0 {
			continue
		}
		if len(points) == 1 {
			addCap(mask, points[0], Point{X: 1}, halfWidth, style.Cap)
			continue
		}
		segmentCount := len(points) - 1
		if subpath.Closed {
			segmentCount = len(points)
		}
		for index := 0; index < segmentCount; index++ {
			start := points[index]
			end := points[(index+1)%len(points)]
			addSegment(mask, start, end, halfWidth)
		}
		if subpath.Closed {
			for index := range points {
				previous := points[(index-1+len(points))%len(points)]
				next := points[(index+1)%len(points)]
				addJoin(mask, points[index], previous, next, halfWidth, style)
			}
			continue
		}
		for index := 1; index+1 < len(points); index++ {
			addJoin(mask, points[index], points[index-1], points[index+1], halfWidth, style)
		}
		addCap(mask, points[0], points[0].Subtract(points[1]), halfWidth, style.Cap)
		last := len(points) - 1
		addCap(mask, points[last], points[last].Subtract(points[last-1]), halfWidth, style.Cap)
	}
	return mask
}

func addSegment(mask *Mask, start, end Point, halfWidth float64) {
	direction := end.Subtract(start)
	if direction.Length() == 0 {
		return
	}
	normal := direction.Normalized().LeftNormal().Scaled(halfWidth)
	polygon := []Point{
		start.Add(normal),
		end.Add(normal),
		end.Subtract(normal),
		start.Subtract(normal),
	}
	addPolygon(mask, polygon)
}

func addCap(mask *Mask, endpoint, outward Point, halfWidth float64, cap LineCap) {
	switch cap {
	case RoundCap:
		circle := NewPath()
		circle.AddCircle(endpoint.X, endpoint.Y, halfWidth)
		for _, subpath := range circle.Flatten(0.25) {
			addPolygon(mask, subpath.Points)
		}
	case SquareCap:
		direction := outward.Normalized()
		if direction.Length() == 0 {
			return
		}
		normal := direction.LeftNormal().Scaled(halfWidth)
		extension := direction.Scaled(halfWidth)
		polygon := []Point{
			endpoint.Add(normal),
			endpoint.Add(normal).Add(extension),
			endpoint.Subtract(normal).Add(extension),
			endpoint.Subtract(normal),
		}
		addPolygon(mask, polygon)
	}
}

func addJoin(mask *Mask, vertex, previous, next Point, halfWidth float64, style StrokeStyle) {
	previousDirection := vertex.Subtract(previous)
	nextDirection := next.Subtract(vertex)
	if previousDirection.Length() == 0 || nextDirection.Length() == 0 {
		return
	}
	previousDirection = previousDirection.Normalized()
	nextDirection = nextDirection.Normalized()
	cross := CrossProduct(previousDirection, nextDirection)
	if math.Abs(cross) < 1e-9 {
		return
	}
	side := 1.0
	if cross > 0 {
		side = -1
	}
	previousOuter := vertex.Add(previousDirection.LeftNormal().Scaled(halfWidth * side))
	nextOuter := vertex.Add(nextDirection.LeftNormal().Scaled(halfWidth * side))

	switch style.Join {
	case RoundJoin:
		circle := NewPath()
		circle.AddCircle(vertex.X, vertex.Y, halfWidth)
		for _, subpath := range circle.Flatten(0.25) {
			addPolygon(mask, subpath.Points)
		}
	case BevelJoin:
		addPolygon(mask, []Point{vertex, previousOuter, nextOuter})
	default:
		miterPoint, ok := lineIntersection(previousOuter, previousDirection, nextOuter, nextDirection)
		if !ok {
			addPolygon(mask, []Point{vertex, previousOuter, nextOuter})
			return
		}
		miterLength := miterPoint.Subtract(vertex).Length() / halfWidth
		if miterLength > style.MiterLimit {
			addPolygon(mask, []Point{vertex, previousOuter, nextOuter})
			return
		}
		addPolygon(mask, []Point{vertex, previousOuter, miterPoint, nextOuter})
	}
}

func addPolygon(mask *Mask, points []Point) {
	if len(points) < 3 {
		return
	}
	edges := make([]rasterEdge, 0, len(points))
	for index := range points {
		edges = append(edges, newEdge(points[index], points[(index+1)%len(points)]))
	}
	primitive := NewMask(mask.Width, mask.Height)
	accumulateEdges(edges, primitive, NonZeroFill, false)
	mask.Union(primitive)
}

func deduplicatePoints(points []Point) []Point {
	result := make([]Point, 0, len(points))
	for _, point := range points {
		if len(result) > 0 && result[len(result)-1].Subtract(point).Length() < 1e-9 {
			continue
		}
		result = append(result, point)
	}
	return result
}

func lineIntersection(first Point, firstDirection Point, second Point, secondDirection Point) (Point, bool) {
	cross := CrossProduct(firstDirection, secondDirection)
	if math.Abs(cross) < 1e-9 {
		return Point{}, false
	}
	delta := second.Subtract(first)
	progress := CrossProduct(delta, secondDirection) / cross
	return first.Add(firstDirection.Scaled(progress)), true
}
