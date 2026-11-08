package geometry

import (
	"math"
	"sort"
)

const subSamples = 4

// RasterizeFill rasterizes path into a coverage mask.
func RasterizeFill(path *Path, width, height int, rule FillRule) *Mask {
	mask := NewMask(width, height)
	if path == nil {
		return mask
	}
	edges := buildEdges(path.Flatten(0.25))
	accumulateEdges(edges, mask, rule, false)
	return mask
}

type rasterEdge struct {
	x0      float64
	y0      float64
	x1      float64
	y1      float64
	winding int
}

type crossing struct {
	x       float64
	winding int
}

type crossingList []crossing

func (list crossingList) Len() int {
	return len(list)
}

func (list crossingList) Less(first, second int) bool {
	return list[first].x < list[second].x
}

func (list crossingList) Swap(first, second int) {
	list[first], list[second] = list[second], list[first]
}

func buildEdges(subpaths []Subpath) []rasterEdge {
	var edges []rasterEdge
	for _, subpath := range subpaths {
		points := subpath.Points
		if len(points) < 2 {
			continue
		}
		for index := 0; index+1 < len(points); index++ {
			edges = append(edges, newEdge(points[index], points[index+1]))
		}
		last := points[len(points)-1]
		first := points[0]
		if last.X != first.X || last.Y != first.Y {
			edges = append(edges, newEdge(last, first))
		}
	}
	return edges
}

func newEdge(start, end Point) rasterEdge {
	winding := 1
	if end.Y < start.Y {
		winding = -1
	}
	return rasterEdge{x0: start.X, y0: start.Y, x1: end.X, y1: end.Y, winding: winding}
}

func accumulateEdges(edges []rasterEdge, mask *Mask, rule FillRule, union bool) {
	if len(edges) == 0 || mask.Width == 0 || mask.Height == 0 {
		return
	}
	minY := math.Inf(1)
	maxY := math.Inf(-1)
	for _, edge := range edges {
		minY = math.Min(minY, math.Min(edge.y0, edge.y1))
		maxY = math.Max(maxY, math.Max(edge.y0, edge.y1))
	}
	firstRow := int(math.Floor(minY))
	lastRow := int(math.Ceil(maxY)) - 1
	if firstRow < 0 {
		firstRow = 0
	}
	if lastRow >= mask.Height {
		lastRow = mask.Height - 1
	}
	if firstRow > lastRow {
		return
	}

	crossings := make(crossingList, 0, len(edges))
	for yPos := firstRow; yPos <= lastRow; yPos++ {
		for subIndex := 0; subIndex < subSamples; subIndex++ {
			sampleY := float64(yPos) + (float64(subIndex)+0.5)/float64(subSamples)
			crossings = crossings[:0]
			for _, edge := range edges {
				if !edgeCrossesRow(edge, sampleY) {
					continue
				}
				deltaY := edge.y1 - edge.y0
				progress := (sampleY - edge.y0) / deltaY
				xPos := edge.x0 + progress*(edge.x1-edge.x0)
				crossings = append(crossings, crossing{x: xPos, winding: edge.winding})
			}
			if len(crossings) < 2 {
				continue
			}
			sort.Stable(crossings)
			if rule == EvenOddFill {
				for index := 0; index+1 < len(crossings); index += 2 {
					fillSpan(mask, yPos, crossings[index].x, crossings[index+1].x, union)
				}
				continue
			}
			winding := 0
			startX := 0.0
			for _, item := range crossings {
				previous := winding
				winding += item.winding
				switch {
				case previous == 0 && winding != 0:
					startX = item.x
				case previous != 0 && winding == 0:
					fillSpan(mask, yPos, startX, item.x, union)
				}
			}
		}
	}
}

func edgeCrossesRow(edge rasterEdge, sampleY float64) bool {
	if edge.y0 == edge.y1 {
		return false
	}
	if edge.y0 < edge.y1 {
		return sampleY >= edge.y0 && sampleY < edge.y1
	}
	return sampleY >= edge.y1 && sampleY < edge.y0
}

func fillSpan(mask *Mask, yPos int, startX, endX float64, union bool) {
	if endX <= startX {
		return
	}
	if startX < 0 {
		startX = 0
	}
	if endX > float64(mask.Width) {
		endX = float64(mask.Width)
	}
	if endX <= startX {
		return
	}
	firstPixel := int(math.Floor(startX))
	lastPixel := int(math.Ceil(endX)) - 1
	rowOffset := yPos * mask.Width
	delta := 1 / float64(subSamples)
	for xPos := firstPixel; xPos <= lastPixel; xPos++ {
		left := math.Max(startX, float64(xPos))
		right := math.Min(endX, float64(xPos+1))
		overlap := right - left
		if overlap <= 0 {
			continue
		}
		contribution := overlap * delta
		index := rowOffset + xPos
		if union {
			current := mask.Coverage[index]
			mask.Coverage[index] = 1 - (1-current)*(1-contribution)
		} else {
			mask.Coverage[index] += contribution
		}
	}
}

// BlurMask returns a blurred copy of mask. The radius is the CSS blur radius
// in pixels; three box-blur passes approximate a Gaussian.
func BlurMask(mask *Mask, radius float64) *Mask {
	if mask == nil || radius <= 0 {
		return mask
	}
	boxRadius := int(math.Round(radius / 3))
	if boxRadius < 1 {
		boxRadius = 1
	}
	current := mask
	for pass := 0; pass < 3; pass++ {
		current = boxBlur(current, boxRadius)
	}
	return current
}

func boxBlur(source *Mask, radius int) *Mask {
	horizontal := NewMask(source.Width, source.Height)
	window := 2*radius + 1
	for yPos := 0; yPos < source.Height; yPos++ {
		sum := 0.0
		for offset := -radius; offset <= radius; offset++ {
			xPos := offset
			if xPos >= 0 && xPos < source.Width {
				sum += source.Coverage[yPos*source.Width+xPos]
			}
		}
		for xPos := 0; xPos < source.Width; xPos++ {
			horizontal.Coverage[yPos*source.Width+xPos] = sum / float64(window)
			removeX := xPos - radius
			addX := xPos + radius + 1
			if removeX >= 0 && removeX < source.Width {
				sum -= source.Coverage[yPos*source.Width+removeX]
			}
			if addX >= 0 && addX < source.Width {
				sum += source.Coverage[yPos*source.Width+addX]
			}
		}
	}
	result := NewMask(source.Width, source.Height)
	for xPos := 0; xPos < source.Width; xPos++ {
		sum := 0.0
		for offset := -radius; offset <= radius; offset++ {
			yPos := offset
			if yPos >= 0 && yPos < source.Height {
				sum += horizontal.Coverage[yPos*source.Width+xPos]
			}
		}
		for yPos := 0; yPos < source.Height; yPos++ {
			result.Coverage[yPos*source.Width+xPos] = sum / float64(window)
			removeY := yPos - radius
			addY := yPos + radius + 1
			if removeY >= 0 && removeY < source.Height {
				sum -= horizontal.Coverage[removeY*source.Width+xPos]
			}
			if addY >= 0 && addY < source.Height {
				sum += horizontal.Coverage[addY*source.Width+xPos]
			}
		}
	}
	return result
}
