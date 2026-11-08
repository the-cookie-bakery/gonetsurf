package interfaces

import "image"

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

// Presenter presents scenes directly into a native window.
type Presenter interface {
	Resize(width, height int) error
	Present(scene *Scene, damage image.Rectangle) error
	Close() error
}
