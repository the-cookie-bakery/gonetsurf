//go:build vulkan && cgo

package vulkan

import (
	"math"
	"testing"

	"gonetsurf/rendering/geometry"
	"gonetsurf/rendering/interfaces"
	"gonetsurf/rendering/software"
)

func compareSceneParity(t *testing.T, scene *interfaces.Scene) {
	t.Helper()
	softwareSurface, err := software.NewRenderer().Render(scene)
	if err != nil {
		t.Fatal(err)
	}
	backend, err := NewBackend()
	if err != nil {
		t.Fatal(err)
	}
	renderer, err := backend.NewRenderer()
	if err != nil {
		t.Fatal(err)
	}
	defer renderer.Close()
	vulkanSurface, err := renderer.Render(scene)
	if err != nil {
		t.Fatal(err)
	}
	for y := 0; y < scene.Height; y++ {
		for x := 0; x < scene.Width; x++ {
			first := softwareSurface.Image().NRGBAAt(x, y)
			second := vulkanSurface.Image().NRGBAAt(x, y)
			delta := func(a, b uint8) int {
				if a > b {
					return int(a - b)
				}
				return int(b - a)
			}
			if delta(first.R, second.R) > 8 || delta(first.G, second.G) > 8 || delta(first.B, second.B) > 8 || delta(first.A, second.A) > 8 {
				t.Fatalf("pixel (%d,%d) = %#v, want %#v", x, y, second, first)
			}
		}
	}
}

func TestRendererMaskedLayerParity(t *testing.T) {
	mask := geometry.NewMask(64, 64)
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			dx := float64(x) - 32
			dy := float64(y) - 32
			coverage := 1 - math.Sqrt(dx*dx+dy*dy)/30
			if coverage < 0 {
				coverage = 0
			}
			if coverage > 1 {
				coverage = 1
			}
			mask.Coverage[y*64+x] = coverage
		}
	}
	layer := &interfaces.Layer{Commands: []interfaces.Command{
		{Kind: interfaces.CommandFillRect, Rect: geometry.Rect{X: 8, Y: 8, Width: 40, Height: 40}, Color: geometry.NewColor(255, 0, 0)},
		{Kind: interfaces.CommandFillRect, Rect: geometry.Rect{X: 24, Y: 24, Width: 32, Height: 32}, Color: geometry.NewColor(0, 0, 255)},
	}}
	scene := &interfaces.Scene{Width: 64, Height: 64}
	scene.Commands = []interfaces.Command{
		{Kind: interfaces.CommandClear, Color: geometry.NewColor(255, 255, 255)},
		{Kind: interfaces.CommandCompositeLayerMasked, Layer: layer, Mask: mask, Rect: geometry.Rect{Width: 64, Height: 64}, Opacity: 0.6},
	}
	compareSceneParity(t, scene)
}

func TestRendererTransformedLayerParity(t *testing.T) {
	layer := &interfaces.Layer{Commands: []interfaces.Command{
		{Kind: interfaces.CommandFillRect, Rect: geometry.Rect{X: 10, Y: 10, Width: 30, Height: 20}, Color: geometry.NewColor(0, 200, 100)},
	}}
	transform := geometry.IdentityMatrix4()
	transform[3] = 6
	transform[7] = 4
	scene := &interfaces.Scene{Width: 64, Height: 64}
	scene.Commands = []interfaces.Command{
		{Kind: interfaces.CommandClear, Color: geometry.NewColor(255, 255, 255)},
		{Kind: interfaces.CommandCompositeLayer, Layer: layer, Transform: transform, Opacity: 1},
	}
	compareSceneParity(t, scene)
}
