//go:build vulkan && cgo

package vulkan

import (
	"math"
	"testing"

	"gonetsurf/rendering/geometry"
	"gonetsurf/rendering/interfaces"
	"gonetsurf/rendering/software"
)

func TestRendererMaskedGradientParity(t *testing.T) {
	mask := geometry.NewMask(48, 48)
	for y := 0; y < 48; y++ {
		for x := 0; x < 48; x++ {
			dx := float64(x) - 24
			dy := float64(y) - 24
			coverage := 1 - math.Sqrt(dx*dx+dy*dy)/24
			if coverage < 0 {
				coverage = 0
			}
			if coverage > 1 {
				coverage = 1
			}
			mask.Coverage[y*48+x] = coverage
		}
	}
	gradient := geometry.Gradient{
		Kind:    geometry.RadialGradient,
		CenterX: 24,
		CenterY: 24,
		RadiusX: 22,
		RadiusY: 22,
		Stops: []geometry.GradientStop{
			{Position: 0, Color: geometry.NewColor(255, 80, 0)},
			{Position: 1, Color: geometry.NewColor(0, 40, 200)},
		},
	}
	scene := &interfaces.Scene{Width: 64, Height: 64}
	scene.Commands = []interfaces.Command{
		{Kind: interfaces.CommandClear, Color: geometry.NewColor(255, 255, 255)},
		{Kind: interfaces.CommandFillGradient, Gradient: gradient, Rect: geometry.Rect{X: 8, Y: 8, Width: 48, Height: 48}, Clip: geometry.Rect{Width: 64, Height: 64}, Mask: mask},
	}
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
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
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
