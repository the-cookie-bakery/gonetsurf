package software

import "gonetsurf/rendering/interfaces"

// Renderer is the CPU rendering backend.
type Renderer struct{}

// NewRenderer returns a software renderer.
func NewRenderer() *Renderer { return &Renderer{} }

// Name returns the backend name.
func (renderer *Renderer) Name() string { return "software" }

// Render executes a scene into a CPU surface.
func (renderer *Renderer) Render(scene *interfaces.Scene) (interfaces.Surface, error) {
	target := NewSurface(scene.Width, scene.Height)
	Execute(scene, target)
	return target, nil
}

// Close releases renderer resources.
func (renderer *Renderer) Close() error { return nil }

// Backend constructs software renderers.
type Backend struct{}

// NewBackend returns a software backend.
func NewBackend() *Backend { return &Backend{} }

// Name returns the backend name.
func (backend *Backend) Name() string { return "software" }

// NewRenderer returns a software renderer.
func (backend *Backend) NewRenderer() (interfaces.Renderer, error) {
	return NewRenderer(), nil
}

var _ interfaces.Renderer = (*Renderer)(nil)
var _ interfaces.Backend = (*Backend)(nil)
