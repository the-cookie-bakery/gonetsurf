package software

import (
	"image"
	"image/draw"
	"math"
)

// DrawImage draws source into destination, clipped by clip and masked by mask.
// Sampling is bilinear and alpha-aware.
func (surface *Surface) DrawImage(source image.Image, destination Rect, clip Rect, mask *Mask, opacity float64) {
	if source == nil || destination.Width <= 0 || destination.Height <= 0 || opacity <= 0 {
		return
	}
	nrgba := toNRGBA(source)
	sourceWidth := nrgba.Bounds().Dx()
	sourceHeight := nrgba.Bounds().Dy()
	if sourceWidth == 0 || sourceHeight == 0 {
		return
	}
	minX := int(math.Floor(destination.X))
	minY := int(math.Floor(destination.Y))
	maxX := int(math.Ceil(destination.X + destination.Width))
	maxY := int(math.Ceil(destination.Y + destination.Height))
	if int(math.Ceil(clip.X)) > minX {
		minX = int(math.Ceil(clip.X))
	}
	if int(math.Floor(clip.X+clip.Width)) < maxX {
		maxX = int(math.Floor(clip.X + clip.Width))
	}
	if int(math.Ceil(clip.Y)) > minY {
		minY = int(math.Ceil(clip.Y))
	}
	if int(math.Floor(clip.Y+clip.Height)) < maxY {
		maxY = int(math.Floor(clip.Y + clip.Height))
	}
	if minX < 0 {
		minX = 0
	}
	if minY < 0 {
		minY = 0
	}
	if maxX > surface.Width {
		maxX = surface.Width
	}
	if maxY > surface.Height {
		maxY = surface.Height
	}
	for yPos := minY; yPos < maxY; yPos++ {
		for xPos := minX; xPos < maxX; xPos++ {
			if !pointInRect(float64(xPos)+0.5, float64(yPos)+0.5, clip) {
				continue
			}
			coverage := opacity
			if mask != nil {
				coverage *= mask.At(xPos, yPos)
			}
			if coverage <= 0 {
				continue
			}
			u := (float64(xPos)+0.5-destination.X)/destination.Width*float64(sourceWidth) - 0.5
			v := (float64(yPos)+0.5-destination.Y)/destination.Height*float64(sourceHeight) - 0.5
			red, green, blue, alpha := sampleBilinear(nrgba, u, v)
			if alpha <= 0 {
				continue
			}
			surface.blendPixel(xPos, yPos, Color{Red: toByte(red), Green: toByte(green), Blue: toByte(blue), Alpha: 255}, alpha*coverage)
		}
	}
}

func sampleBilinear(source *image.NRGBA, u, v float64) (red, green, blue, alpha float64) {
	bounds := source.Bounds()
	x0 := int(math.Floor(u))
	y0 := int(math.Floor(v))
	fractionX := u - float64(x0)
	fractionY := v - float64(y0)
	for _, offset := range [4][2]int{{0, 0}, {1, 0}, {0, 1}, {1, 1}} {
		xPos := x0 + offset[0]
		yPos := y0 + offset[1]
		if xPos < bounds.Min.X || xPos >= bounds.Max.X || yPos < bounds.Min.Y || yPos >= bounds.Max.Y {
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
		pixelOffset := yPos*source.Stride + xPos*4
		sampleAlpha := float64(source.Pix[pixelOffset+3]) / 255
		alpha += sampleAlpha * weight
		red += float64(source.Pix[pixelOffset]) / 255 * sampleAlpha * weight
		green += float64(source.Pix[pixelOffset+1]) / 255 * sampleAlpha * weight
		blue += float64(source.Pix[pixelOffset+2]) / 255 * sampleAlpha * weight
	}
	if alpha > 0 {
		red /= alpha
		green /= alpha
		blue /= alpha
	}
	return red, green, blue, alpha
}

func toNRGBA(source image.Image) *image.NRGBA {
	if converted, ok := source.(*image.NRGBA); ok {
		return converted
	}
	bounds := source.Bounds()
	converted := image.NewNRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	draw.Draw(converted, converted.Bounds(), source, bounds.Min, draw.Src)
	return converted
}
