package software

import (
	"image"

	"gonetsurf/rendering/geometry"
	"gonetsurf/rendering/interfaces"
)

// Size returns the surface dimensions.
func (surface *Surface) Size() (width, height int) {
	if surface == nil {
		return 0, 0
	}
	return surface.Width, surface.Height
}

// CanvasWidth returns the surface width for interfaces.Canvas.
func (surface *Surface) CanvasWidth() int {
	if surface == nil {
		return 0
	}
	return surface.Width
}

// CanvasHeight returns the surface height for interfaces.Canvas.
func (surface *Surface) CanvasHeight() int {
	if surface == nil {
		return 0
	}
	return surface.Height
}

// Premultiplied returns the raw premultiplied RGBA8 pixels.
func (surface *Surface) Premultiplied() (pixels []uint8, stride int) {
	if surface == nil {
		return nil, 0
	}
	return surface.Pix, surface.Stride
}

// FillGradient fills an area with a resolved gradient.
func (surface *Surface) FillGradient(gradient geometry.Gradient, area, clip geometry.Rect, mask *geometry.Mask) {
	if surface == nil || area.Width <= 0 || area.Height <= 0 || clip.Width <= 0 || clip.Height <= 0 {
		return
	}
	minX := maxIntValue(int(clip.X), int(area.X))
	minY := maxIntValue(int(clip.Y), int(area.Y))
	maxX := minIntValue(int(clip.X+clip.Width), int(area.X+area.Width), surface.Width)
	maxY := minIntValue(int(clip.Y+clip.Height), int(area.Y+area.Height), surface.Height)
	if minX < 0 {
		minX = 0
	}
	if minY < 0 {
		minY = 0
	}
	for yPos := minY; yPos < maxY; yPos++ {
		for xPos := minX; xPos < maxX; xPos++ {
			coverage := 1.0
			if mask != nil {
				coverage = mask.At(xPos, yPos)
			}
			if coverage <= 0 || !pointInRect(float64(xPos)+0.5, float64(yPos)+0.5, clip) {
				continue
			}
			surface.blendPixel(xPos, yPos, gradient.ColorAt(float64(xPos)+0.5, float64(yPos)+0.5), coverage)
		}
	}
}

// DrawGlyphMask blends a glyph alpha mask at the given destination origin.
func (surface *Surface) DrawGlyphMask(mask *image.Alpha, xPos, yPos int, color geometry.Color) {
	if surface == nil || mask == nil {
		return
	}
	maskBounds := mask.Bounds()
	for maskY := maskBounds.Min.Y; maskY < maskBounds.Max.Y; maskY++ {
		for maskX := maskBounds.Min.X; maskX < maskBounds.Max.X; maskX++ {
			alpha := mask.AlphaAt(maskX, maskY).A
			if alpha == 0 {
				continue
			}
			destinationX := xPos + (maskX - maskBounds.Min.X)
			destinationY := yPos + (maskY - maskBounds.Min.Y)
			surface.blendPixel(destinationX, destinationY, color, float64(alpha)/255)
		}
	}
}

// BlendPixel blends one coverage pixel.
func (surface *Surface) BlendPixel(xPos, yPos int, color geometry.Color, coverage float64) {
	surface.blendPixel(xPos, yPos, color, coverage)
}

// NewLayer creates an independent surface with the same dimensions.
func (surface *Surface) NewLayer() interfaces.Canvas {
	return NewSurface(surface.Width, surface.Height)
}

// CompositeLayer composites a layer with a transform and opacity.
func (surface *Surface) CompositeLayer(layer interfaces.Canvas, transform geometry.Matrix4, opacity float64) {
	layerSurface, ok := layer.(*Surface)
	if !ok || layerSurface == nil {
		return
	}
	if transform.IsAffine2D() {
		compositeTransformed(surface, layerSurface, transform.Affine2D(), opacity)
		return
	}
	compositeProjective(surface, layerSurface, transform, opacity)
}

// CompositeLayerMasked composites a layer through a coverage mask.
func (surface *Surface) CompositeLayerMasked(layer interfaces.Canvas, clip geometry.Rect, mask *geometry.Mask, opacity float64) {
	layerSurface, ok := layer.(*Surface)
	if !ok || layerSurface == nil {
		return
	}
	destination := Rect{Width: float64(surface.Width), Height: float64(surface.Height)}
	surface.DrawImage(layerSurface.Image(), destination, clip, mask, opacity)
}

var _ interfaces.Surface = (*Surface)(nil)
var _ interfaces.Canvas = (*Surface)(nil)
