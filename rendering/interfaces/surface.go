// Package interfaces defines the backend-neutral rendering contracts shared by
// the software and Vulkan pipelines. The geometry package owns all value types.
package interfaces

import (
	"image"

	"gonetsurf/rendering/geometry"
)

// Surface is a backend-neutral frame buffer. Implementations may be backed by
// CPU pixels or an offscreen GPU image that has been read back.
type Surface interface {
	Size() (width, height int)
	Bounds() geometry.Rect
	Clear(geometry.Color)
	Blit(offsetX, offsetY int, source Surface, op geometry.CompositeOp)
	BlitOpacity(offsetX, offsetY int, source Surface, opacity float64)
	Image() *image.NRGBA
}

// PixelSurface is implemented by CPU-backed surfaces that expose raw
// premultiplied RGBA8 storage.
type PixelSurface interface {
	Surface
	Premultiplied() (pixels []uint8, stride int)
}
