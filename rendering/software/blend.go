package software

import "gonetsurf/rendering/interfaces"

// Blit composites source onto the surface at the given offset.
func (surface *Surface) Blit(offsetX, offsetY int, source interfaces.Surface, op CompositeOp) {
	sourceSurface := asSoftwareSurface(source)
	if sourceSurface == nil {
		return
	}
	if op == Copy {
		blitCopy(surface, offsetX, offsetY, sourceSurface)
		return
	}
	for sourceY := 0; sourceY < sourceSurface.Height; sourceY++ {
		destinationY := offsetY + sourceY
		if destinationY < 0 || destinationY >= surface.Height {
			continue
		}
		for sourceX := 0; sourceX < sourceSurface.Width; sourceX++ {
			destinationX := offsetX + sourceX
			if destinationX < 0 || destinationX >= surface.Width {
				continue
			}
			if !pointInRect(float64(destinationX)+0.5, float64(destinationY)+0.5, surface.clip) {
				continue
			}
			sourceOffset := sourceY*sourceSurface.Stride + sourceX*4
			destinationOffset := destinationY*surface.Stride + destinationX*4
			compositePixel(surface.Pix, destinationOffset, sourceSurface.Pix, sourceOffset, op)
		}
	}
}

func blitCopy(surface *Surface, offsetX, offsetY int, source *Surface) {
	minY := maxIntValue(offsetY, int(surface.clip.Y))
	maxY := minIntValue(offsetY+source.Height, int(surface.clip.Y+surface.clip.Height), surface.Height)
	minX := maxIntValue(offsetX, int(surface.clip.X))
	maxX := minIntValue(offsetX+source.Width, int(surface.clip.X+surface.clip.Width), surface.Width)
	if minX >= maxX || minY >= maxY {
		return
	}
	rowBytes := (maxX - minX) * 4
	for destinationY := minY; destinationY < maxY; destinationY++ {
		sourceY := destinationY - offsetY
		sourceOffset := sourceY*source.Stride + (minX-offsetX)*4
		destinationOffset := destinationY*surface.Stride + minX*4
		copy(surface.Pix[destinationOffset:destinationOffset+rowBytes], source.Pix[sourceOffset:sourceOffset+rowBytes])
	}
}

func minIntValue(values ...int) int {
	result := values[0]
	for _, value := range values[1:] {
		if value < result {
			result = value
		}
	}
	return result
}

func maxIntValue(values ...int) int {
	result := values[0]
	for _, value := range values[1:] {
		if value > result {
			result = value
		}
	}
	return result
}

func compositePixel(destination []uint8, destinationOffset int, source []uint8, sourceOffset int, op CompositeOp) {
	sourceAlpha := float64(source[sourceOffset+3]) / 255
	destinationAlpha := float64(destination[destinationOffset+3]) / 255
	sourceFactor, destinationFactor := compositeFactors(op, sourceAlpha, destinationAlpha)

	sourceRed := float64(source[sourceOffset]) / 255
	sourceGreen := float64(source[sourceOffset+1]) / 255
	sourceBlue := float64(source[sourceOffset+2]) / 255
	destinationRed := float64(destination[destinationOffset]) / 255
	destinationGreen := float64(destination[destinationOffset+1]) / 255
	destinationBlue := float64(destination[destinationOffset+2]) / 255

	destination[destinationOffset] = toByte(sourceRed*sourceFactor + destinationRed*destinationFactor)
	destination[destinationOffset+1] = toByte(sourceGreen*sourceFactor + destinationGreen*destinationFactor)
	destination[destinationOffset+2] = toByte(sourceBlue*sourceFactor + destinationBlue*destinationFactor)
	destination[destinationOffset+3] = toByte(sourceAlpha*sourceFactor + destinationAlpha*destinationFactor)
}

func compositeFactors(op CompositeOp, sourceAlpha, destinationAlpha float64) (sourceFactor, destinationFactor float64) {
	switch op {
	case SourceOver:
		return 1, 1 - sourceAlpha
	case SourceIn:
		return destinationAlpha, 0
	case SourceOut:
		return 1 - destinationAlpha, 0
	case SourceAtop:
		return destinationAlpha, 1 - sourceAlpha
	case DestinationOver:
		return 1 - destinationAlpha, 1
	case DestinationIn:
		return 0, sourceAlpha
	case DestinationOut:
		return 0, 1 - sourceAlpha
	case DestinationAtop:
		return 1 - destinationAlpha, sourceAlpha
	case Xor:
		return 1 - destinationAlpha, 1 - sourceAlpha
	case Clear:
		return 0, 0
	default:
		return 1, 0
	}
}

func pointInRect(xPos, yPos float64, rect Rect) bool {
	return xPos >= rect.X && yPos >= rect.Y && xPos < rect.X+rect.Width && yPos < rect.Y+rect.Height
}

// BlitOpacity composites source onto the surface with a global opacity.
func (surface *Surface) BlitOpacity(offsetX, offsetY int, source interfaces.Surface, opacity float64) {
	sourceSurface := asSoftwareSurface(source)
	if sourceSurface == nil || opacity <= 0 {
		return
	}
	if opacity > 1 {
		opacity = 1
	}
	for sourceY := 0; sourceY < sourceSurface.Height; sourceY++ {
		destinationY := offsetY + sourceY
		if destinationY < 0 || destinationY >= surface.Height {
			continue
		}
		for sourceX := 0; sourceX < sourceSurface.Width; sourceX++ {
			destinationX := offsetX + sourceX
			if destinationX < 0 || destinationX >= surface.Width {
				continue
			}
			if !pointInRect(float64(destinationX)+0.5, float64(destinationY)+0.5, surface.clip) {
				continue
			}
			sourceOffset := sourceY*sourceSurface.Stride + sourceX*4
			destinationOffset := destinationY*surface.Stride + destinationX*4
			sourceAlpha := float64(sourceSurface.Pix[sourceOffset+3]) / 255 * opacity
			if sourceAlpha <= 0 {
				continue
			}
			destinationAlpha := float64(surface.Pix[destinationOffset+3]) / 255
			surface.Pix[destinationOffset] = toByte(float64(sourceSurface.Pix[sourceOffset])/255*opacity + float64(surface.Pix[destinationOffset])/255*(1-sourceAlpha))
			surface.Pix[destinationOffset+1] = toByte(float64(sourceSurface.Pix[sourceOffset+1])/255*opacity + float64(surface.Pix[destinationOffset+1])/255*(1-sourceAlpha))
			surface.Pix[destinationOffset+2] = toByte(float64(sourceSurface.Pix[sourceOffset+2])/255*opacity + float64(surface.Pix[destinationOffset+2])/255*(1-sourceAlpha))
			surface.Pix[destinationOffset+3] = toByte(sourceAlpha + destinationAlpha*(1-sourceAlpha))
		}
	}
}

func asSoftwareSurface(source interfaces.Surface) *Surface {
	if source == nil {
		return nil
	}
	if softwareSurface, ok := source.(*Surface); ok {
		return softwareSurface
	}
	converted := source.Image()
	if converted == nil {
		return nil
	}
	bounds := converted.Bounds()
	surface := NewSurface(bounds.Dx(), bounds.Dy())
	for yPos := 0; yPos < bounds.Dy(); yPos++ {
		for xPos := 0; xPos < bounds.Dx(); xPos++ {
			sourceOffset := (bounds.Min.Y+yPos)*converted.Stride + (bounds.Min.X+xPos)*4
			color := Color{
				Red:   converted.Pix[sourceOffset],
				Green: converted.Pix[sourceOffset+1],
				Blue:  converted.Pix[sourceOffset+2],
				Alpha: converted.Pix[sourceOffset+3],
			}
			red, green, blue, alpha := color.Premultiplied(1)
			destinationOffset := yPos*surface.Stride + xPos*4
			surface.Pix[destinationOffset] = red
			surface.Pix[destinationOffset+1] = green
			surface.Pix[destinationOffset+2] = blue
			surface.Pix[destinationOffset+3] = alpha
		}
	}
	return surface
}
