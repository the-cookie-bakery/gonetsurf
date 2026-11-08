//go:build vulkan && cgo

package vulkan

import (
	"testing"

	"gonetsurf/rendering/geometry"
	"gonetsurf/rendering/interfaces"
	"gonetsurf/rendering/software"
)

func benchmarkScene() *interfaces.Scene {
	scene := &interfaces.Scene{Width: 256, Height: 256}
	scene.Commands = []interfaces.Command{{Kind: interfaces.CommandClear, Color: geometry.NewColor(255, 255, 255)}}
	for index := 0; index < 64; index++ {
		scene.Commands = append(scene.Commands, interfaces.Command{
			Kind:  interfaces.CommandFillRect,
			Rect:  geometry.Rect{X: float64(index % 8 * 30), Y: float64(index / 8 * 30), Width: 24, Height: 24},
			Color: geometry.NewColor(uint8(index*3), 80, 200),
		})
	}
	path := geometry.NewPath()
	path.AddCircle(128, 128, 80)
	scene.Commands = append(scene.Commands, interfaces.Command{
		Kind:     interfaces.CommandFillPath,
		Path:     path,
		FillRule: geometry.NonZeroFill,
		Color:    geometry.NewColor(20, 120, 220),
		Clip:     geometry.Rect{Width: 256, Height: 256},
	})
	scene.Commands = append(scene.Commands, interfaces.Command{
		Kind: interfaces.CommandFillGradient,
		Gradient: geometry.Gradient{
			Kind:       geometry.LinearGradient,
			CenterX:    128,
			CenterY:    128,
			DirectionX: 1,
			LineLength: 200,
			Stops: []geometry.GradientStop{
				{Position: 0, Color: geometry.NewColor(0, 0, 0)},
				{Position: 1, Color: geometry.NewColor(255, 255, 255)},
			},
		},
		Rect: geometry.Rect{X: 20, Y: 20, Width: 216, Height: 60},
		Clip: geometry.Rect{Width: 256, Height: 256},
	})
	layer := &interfaces.Layer{Commands: []interfaces.Command{{
		Kind:  interfaces.CommandFillRect,
		Rect:  geometry.Rect{X: 40, Y: 180, Width: 80, Height: 40},
		Color: geometry.NewColor(200, 40, 40),
	}}}
	scene.Commands = append(scene.Commands, interfaces.Command{
		Kind:      interfaces.CommandCompositeLayer,
		Layer:     layer,
		Transform: geometry.IdentityMatrix4(),
		Opacity:   0.7,
	})
	return scene
}

func BenchmarkSoftwareSceneRender(b *testing.B) {
	scene := benchmarkScene()
	renderer := software.NewRenderer()
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		if _, err := renderer.Render(scene); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkVulkanSceneRender(b *testing.B) {
	scene := benchmarkScene()
	backend, err := NewBackend()
	if err != nil {
		b.Fatal(err)
	}
	renderer, err := backend.NewRenderer()
	if err != nil {
		b.Fatal(err)
	}
	defer renderer.Close()
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		if _, err := renderer.Render(scene); err != nil {
			b.Fatal(err)
		}
	}
}
