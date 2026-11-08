package software

import (
	"math"

	"gonetsurf/rendering/geometry"
)

func compositeTransformed(destination, source *Surface, matrix geometry.Affine, opacity float64) {
	if destination == nil || source == nil || opacity <= 0 {
		return
	}
	if opacity > 1 {
		opacity = 1
	}
	inverse, ok := matrix.Inverse()
	if !ok {
		return
	}
	corners := []Point{
		matrix.Apply(Point{X: 0, Y: 0}),
		matrix.Apply(Point{X: float64(source.Width), Y: 0}),
		matrix.Apply(Point{X: 0, Y: float64(source.Height)}),
		matrix.Apply(Point{X: float64(source.Width), Y: float64(source.Height)}),
	}
	minX := math.Inf(1)
	minY := math.Inf(1)
	maxX := math.Inf(-1)
	maxY := math.Inf(-1)
	for _, corner := range corners {
		minX = math.Min(minX, corner.X)
		minY = math.Min(minY, corner.Y)
		maxX = math.Max(maxX, corner.X)
		maxY = math.Max(maxY, corner.Y)
	}
	firstX := int(math.Max(0, math.Floor(minX)))
	firstY := int(math.Max(0, math.Floor(minY)))
	lastX := int(math.Min(float64(destination.Width), math.Ceil(maxX)))
	lastY := int(math.Min(float64(destination.Height), math.Ceil(maxY)))
	for yPos := firstY; yPos < lastY; yPos++ {
		for xPos := firstX; xPos < lastX; xPos++ {
			sourcePoint := inverse.Apply(Point{X: float64(xPos) + 0.5, Y: float64(yPos) + 0.5})
			red, green, blue, alpha := samplePremultiplied(source, sourcePoint.X-0.5, sourcePoint.Y-0.5)
			if alpha <= 0 {
				continue
			}
			color := Color{
				Red:   toByte(red / alpha),
				Green: toByte(green / alpha),
				Blue:  toByte(blue / alpha),
				Alpha: 255,
			}
			destination.blendPixel(xPos, yPos, color, alpha*opacity)
		}
	}
}

func compositeProjective(destination, source *Surface, matrix geometry.Matrix4, opacity float64) {
	if destination == nil || source == nil || opacity <= 0 {
		return
	}
	if opacity > 1 {
		opacity = 1
	}
	inverse, ok := matrix.Inverse()
	if !ok {
		return
	}
	minX := math.Inf(1)
	minY := math.Inf(1)
	maxX := math.Inf(-1)
	maxY := math.Inf(-1)
	for _, corner := range [][2]float64{{0, 0}, {float64(source.Width), 0}, {0, float64(source.Height)}, {float64(source.Width), float64(source.Height)}} {
		xPos, yPos, valid := matrix.Project(corner[0], corner[1])
		if !valid {
			continue
		}
		minX = math.Min(minX, xPos)
		minY = math.Min(minY, yPos)
		maxX = math.Max(maxX, xPos)
		maxY = math.Max(maxY, yPos)
	}
	if math.IsInf(minX, 1) {
		return
	}
	firstX := int(math.Max(0, math.Floor(minX)))
	firstY := int(math.Max(0, math.Floor(minY)))
	lastX := int(math.Min(float64(destination.Width), math.Ceil(maxX)))
	lastY := int(math.Min(float64(destination.Height), math.Ceil(maxY)))
	for yPos := firstY; yPos < lastY; yPos++ {
		for xPos := firstX; xPos < lastX; xPos++ {
			sourceX, sourceY, valid := inverse.Project(float64(xPos)+0.5, float64(yPos)+0.5)
			if !valid {
				continue
			}
			red, green, blue, alpha := samplePremultiplied(source, sourceX-0.5, sourceY-0.5)
			if alpha <= 0 {
				continue
			}
			color := Color{Red: toByte(red / alpha), Green: toByte(green / alpha), Blue: toByte(blue / alpha), Alpha: 255}
			destination.blendPixel(xPos, yPos, color, alpha*opacity)
		}
	}
}

func samplePremultiplied(source *Surface, u, v float64) (red, green, blue, alpha float64) {
	x0 := int(math.Floor(u))
	y0 := int(math.Floor(v))
	fractionX := u - float64(x0)
	fractionY := v - float64(y0)
	for _, offset := range [4][2]int{{0, 0}, {1, 0}, {0, 1}, {1, 1}} {
		xPos := x0 + offset[0]
		yPos := y0 + offset[1]
		if xPos < 0 || yPos < 0 || xPos >= source.Width || yPos >= source.Height {
			continue
		}
		weightX := fractionX
		if offset[0] == 0 {
			weightX = 1 - fractionX
		}
		weightY := fractionY
		if offset[1] == 0 {
			weightY = 1 - fractionY
		}
		weight := weightX * weightY
		offsetBytes := yPos*source.Stride + xPos*4
		red += float64(source.Pix[offsetBytes]) / 255 * weight
		green += float64(source.Pix[offsetBytes+1]) / 255 * weight
		blue += float64(source.Pix[offsetBytes+2]) / 255 * weight
		alpha += float64(source.Pix[offsetBytes+3]) / 255 * weight
	}
	return red, green, blue, alpha
}
