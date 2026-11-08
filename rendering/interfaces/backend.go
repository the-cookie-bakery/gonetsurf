package interfaces

import (
	"image"

	layoutinginterfaces "gonetsurf/layouting/interfaces"
)

// Backend creates renderers for one rendering pipeline.
type Backend interface {
	Name() string
	NewRenderer() (Renderer, error)
}

// Renderer executes recorded scenes and returns a frame surface.
type Renderer interface {
	Name() string
	Render(scene *Scene) (Surface, error)
	Close() error
}

// GeometryRenderer consumes backend-resident layout geometry.
type GeometryRenderer interface {
	SetGeometry(handle layoutinginterfaces.GeometryHandle) error
}

// SwapchainPresenter attaches to a native window and presents surfaces through
// a Vulkan swapchain.
type SwapchainPresenter interface {
	AttachPresenter(display string, window uintptr, width, height int) error
	PresentSurface(surface Surface, damage image.Rectangle) error
}

// Presenter presents scenes directly into a native window.
type Presenter interface {
	Resize(width, height int) error
	Present(scene *Scene, damage image.Rectangle) error
	Close() error
}
