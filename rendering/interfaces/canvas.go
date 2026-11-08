package interfaces

import (
	"image"

	"gonetsurf/rendering/geometry"
)

// Canvas is the drawing interface implemented by every rendering pipeline.
// The painter emits drawing operations to a Canvas; the software pipeline
// rasterizes them directly and the scene recorder captures them for another
// backend.
type Canvas interface {
	CanvasWidth() int
	CanvasHeight() int
	Clip() geometry.Rect
	SetClip(geometry.Rect)
	ResetClip()

	Clear(geometry.Color)
	FillRect(geometry.Rect, geometry.Color)
	FillPath(*geometry.Path, geometry.FillRule, geometry.Color)
	StrokePath(*geometry.Path, geometry.StrokeStyle, geometry.Color)
	FillMask(*geometry.Mask, geometry.Color)
	FillGradient(geometry.Gradient, geometry.Rect, geometry.Rect, *geometry.Mask)
	DrawImage(source image.Image, destination, clip geometry.Rect, mask *geometry.Mask, opacity float64)
	DrawGlyphMask(mask *image.Alpha, xPos, yPos int, color geometry.Color)
	BlendPixel(xPos, yPos int, color geometry.Color, coverage float64)

	Blit(offsetX, offsetY int, source Surface, op geometry.CompositeOp)
	BlitOpacity(offsetX, offsetY int, source Surface, opacity float64)

	NewLayer() Canvas
	CompositeLayer(layer Canvas, transform geometry.Matrix4, opacity float64)
	CompositeLayerMasked(layer Canvas, clip geometry.Rect, mask *geometry.Mask, opacity float64)
}
