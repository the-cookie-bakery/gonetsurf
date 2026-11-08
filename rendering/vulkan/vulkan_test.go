//go:build vulkan && cgo

package vulkan

import (
	"testing"

	"gonetsurf/rendering/geometry"
	"gonetsurf/rendering/interfaces"
)

func TestRendererProducesSurface(t *testing.T) {
	backend, err := NewBackend()
	if err != nil {
		t.Fatal(err)
	}
	renderer, err := backend.NewRenderer()
	if err != nil {
		t.Fatal(err)
	}
	scene := &interfaces.Scene{Width: 8, Height: 8}
	scene.Commands = []interfaces.Command{{Kind: interfaces.CommandClear, Color: geometry.NewColor(10, 20, 30)}}
	surface, err := renderer.Render(scene)
	if err != nil {
		t.Fatal(err)
	}
	if width, height := surface.Size(); width != 8 || height != 8 {
		t.Fatalf("surface = %dx%d", width, height)
	}
	pixel := surface.Image().NRGBAAt(0, 0)
	if pixel.R != 10 || pixel.G != 20 || pixel.B != 30 {
		t.Fatalf("pixel = %#v", pixel)
	}
}
