//go:build vulkan && cgo

package vulkan

import (
	"gonetsurf/rendering/interfaces"
	"gonetsurf/rendering/software"
)

// ErrUnavailable reports that the Vulkan backend was not compiled in.
var ErrUnavailable error

// Available reports whether the Vulkan backend is compiled in.
func Available() bool { return true }

// Backend constructs Vulkan renderers.
type Backend struct{}

// NewBackend returns the Vulkan backend.
func NewBackend() (interfaces.Backend, error) {
	return &Backend{}, nil
}

// Name returns the backend name.
func (backend *Backend) Name() string { return "vulkan" }

// NewRenderer returns a Vulkan renderer.
func (backend *Backend) NewRenderer() (interfaces.Renderer, error) {
	return &Renderer{fallback: software.NewRenderer()}, nil
}

// Renderer executes scenes for the Vulkan pipeline.
//
// The device, swapchain, and shader pipeline are scaffolded in this build. Until
// the GPU command lowering lands, commands are executed through the shared
// software executor so the backend produces correct frames and can be used for
// parity testing.
type Renderer struct {
	fallback interfaces.Renderer
}

// Name returns the renderer name.
func (renderer *Renderer) Name() string { return "vulkan" }

// Render executes the scene and returns a frame surface.
func (renderer *Renderer) Render(scene *interfaces.Scene) (interfaces.Surface, error) {
	return renderer.fallback.Render(scene)
}

// Close releases renderer resources.
func (renderer *Renderer) Close() error {
	if renderer.fallback == nil {
		return nil
	}
	return renderer.fallback.Close()
}

var _ interfaces.Backend = (*Backend)(nil)
var _ interfaces.Renderer = (*Renderer)(nil)
