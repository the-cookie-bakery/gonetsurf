//go:build vulkan && cgo

package vulkan

import (
	"image"
	"testing"

	"gonetsurf/rendering/geometry"
	"gonetsurf/rendering/interfaces"
	"gonetsurf/rendering/software"
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
	if typed, ok := renderer.(*Renderer); ok && typed.gpuErr != nil {
		t.Fatalf("GPU path failed: %v", typed.gpuErr)
	}
	if width, height := surface.Size(); width != 8 || height != 8 {
		t.Fatalf("surface = %dx%d", width, height)
	}
	pixel := surface.Image().NRGBAAt(0, 0)
	if pixel.R != 10 || pixel.G != 20 || pixel.B != 30 {
		t.Fatalf("pixel = %#v", pixel)
	}
}

func TestRendererDrawsSolidRect(t *testing.T) {
	backend, _ := NewBackend()
	renderer, _ := backend.NewRenderer()
	scene := &interfaces.Scene{Width: 16, Height: 16}
	scene.Commands = []interfaces.Command{
		{Kind: interfaces.CommandClear, Color: geometry.Color{}},
		{Kind: interfaces.CommandFillRect, Rect: geometry.Rect{X: 4, Y: 4, Width: 8, Height: 8}, Color: geometry.NewColor(255, 0, 0)},
	}
	surface, err := renderer.Render(scene)
	if err != nil {
		t.Fatal(err)
	}
	if typed, ok := renderer.(*Renderer); ok && typed.gpuErr != nil {
		t.Fatalf("GPU path failed: %v", typed.gpuErr)
	}
	pixel := surface.Image().NRGBAAt(8, 8)
	if pixel.R < 250 || pixel.G > 5 || pixel.B > 5 {
		t.Fatalf("rect pixel = %#v", pixel)
	}
	outside := surface.Image().NRGBAAt(1, 1)
	if outside.R > 5 || outside.G > 5 || outside.B > 5 {
		t.Fatalf("outside pixel = %#v", outside)
	}
}

func TestRendererDrawsImage(t *testing.T) {
	backend, _ := NewBackend()
	renderer, _ := backend.NewRenderer()
	source := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	for offset := 0; offset < len(source.Pix); offset += 4 {
		source.Pix[offset] = 0
		source.Pix[offset+1] = 0
		source.Pix[offset+2] = 255
		source.Pix[offset+3] = 255
	}
	scene := &interfaces.Scene{Width: 16, Height: 16}
	scene.Commands = []interfaces.Command{
		{Kind: interfaces.CommandClear, Color: geometry.NewColor(0, 0, 0)},
		{Kind: interfaces.CommandDrawImage, Image: source, Rect: geometry.Rect{X: 0, Y: 0, Width: 16, Height: 16}, Clip: geometry.Rect{Width: 16, Height: 16}, Opacity: 1},
	}
	surface, err := renderer.Render(scene)
	if err != nil {
		t.Fatal(err)
	}
	if typed, ok := renderer.(*Renderer); ok && typed.gpuErr != nil {
		t.Fatalf("GPU path failed: %v", typed.gpuErr)
	}
	pixel := surface.Image().NRGBAAt(8, 8)
	if pixel.B < 250 || pixel.R > 5 || pixel.G > 5 {
		t.Fatalf("image pixel = %#v", pixel)
	}
}

func TestRendererDrawsPath(t *testing.T) {
	backend, _ := NewBackend()
	renderer, _ := backend.NewRenderer()
	path := geometry.NewPath()
	path.AddRect(geometry.Rect{X: 4, Y: 4, Width: 8, Height: 8})
	scene := &interfaces.Scene{Width: 16, Height: 16}
	scene.Commands = []interfaces.Command{
		{Kind: interfaces.CommandClear, Color: geometry.Color{}},
		{Kind: interfaces.CommandFillPath, Path: path, FillRule: geometry.NonZeroFill, Color: geometry.NewColor(0, 255, 0)},
	}
	surface, err := renderer.Render(scene)
	if err != nil {
		t.Fatal(err)
	}
	pixel := surface.Image().NRGBAAt(8, 8)
	if pixel.G < 250 || pixel.R > 5 || pixel.B > 5 {
		t.Fatalf("path pixel = %#v", pixel)
	}
	if outside := surface.Image().NRGBAAt(1, 1); outside.A != 0 {
		t.Fatalf("outside pixel = %#v", outside)
	}
}

func TestRendererDrawsGlyph(t *testing.T) {
	backend, _ := NewBackend()
	renderer, _ := backend.NewRenderer()
	mask := image.NewAlpha(image.Rect(0, 0, 2, 2))
	for index := range mask.Pix {
		mask.Pix[index] = 255
	}
	scene := &interfaces.Scene{Width: 16, Height: 16}
	scene.Commands = []interfaces.Command{
		{Kind: interfaces.CommandClear, Color: geometry.Color{}},
		{Kind: interfaces.CommandDrawGlyph, Glyph: interfaces.Glyph{Mask: mask, X: 4, Y: 4, Color: geometry.NewColor(0, 0, 255)}},
	}
	surface, err := renderer.Render(scene)
	if err != nil {
		t.Fatal(err)
	}
	pixel := surface.Image().NRGBAAt(4, 4)
	if pixel.B < 250 || pixel.R > 5 || pixel.G > 5 {
		t.Fatalf("glyph pixel = %#v", pixel)
	}
}

func TestRendererDrawsGradient(t *testing.T) {
	backend, _ := NewBackend()
	renderer, _ := backend.NewRenderer()
	gradient := geometry.Gradient{
		Kind:       geometry.LinearGradient,
		CenterX:    8,
		CenterY:    8,
		DirectionX: 1,
		LineLength: 16,
		Stops: []geometry.GradientStop{
			{Position: 0, Color: geometry.NewColor(255, 0, 0)},
			{Position: 1, Color: geometry.NewColor(0, 0, 255)},
		},
	}
	scene := &interfaces.Scene{Width: 16, Height: 16}
	scene.Commands = []interfaces.Command{
		{Kind: interfaces.CommandClear, Color: geometry.Color{}},
		{Kind: interfaces.CommandFillGradient, Gradient: gradient, Rect: geometry.Rect{Width: 16, Height: 16}, Clip: geometry.Rect{Width: 16, Height: 16}},
	}
	surface, err := renderer.Render(scene)
	if err != nil {
		t.Fatal(err)
	}
	left := surface.Image().NRGBAAt(0, 8)
	right := surface.Image().NRGBAAt(15, 8)
	if left.R < 200 || left.B > 50 {
		t.Fatalf("left gradient pixel = %#v", left)
	}
	if right.B < 200 || right.R > 50 {
		t.Fatalf("right gradient pixel = %#v", right)
	}
}

func TestRendererParityWithSoftware(t *testing.T) {
	scene := &interfaces.Scene{Width: 64, Height: 64}
	path := geometry.NewPath()
	path.AddCircle(20, 20, 12)
	imageSource := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	for offset := 0; offset < len(imageSource.Pix); offset += 4 {
		imageSource.Pix[offset] = 0
		imageSource.Pix[offset+1] = 128
		imageSource.Pix[offset+2] = 255
		imageSource.Pix[offset+3] = 200
	}
	glyph := image.NewAlpha(image.Rect(0, 0, 4, 4))
	for index := range glyph.Pix {
		glyph.Pix[index] = 180
	}
	layer := &interfaces.Layer{Commands: []interfaces.Command{
		{Kind: interfaces.CommandFillRect, Rect: geometry.Rect{X: 4, Y: 40, Width: 20, Height: 20}, Color: geometry.NewColor(0, 255, 0)},
	}}
	scene.Commands = []interfaces.Command{
		{Kind: interfaces.CommandClear, Color: geometry.NewColor(250, 250, 250)},
		{Kind: interfaces.CommandFillRect, Rect: geometry.Rect{X: 2, Y: 2, Width: 30, Height: 20}, Color: geometry.NewColor(255, 0, 0)},
		{Kind: interfaces.CommandFillPath, Path: path, FillRule: geometry.NonZeroFill, Color: geometry.NewColor(0, 0, 255)},
		{Kind: interfaces.CommandFillGradient, Gradient: geometry.Gradient{
			Kind:       geometry.LinearGradient,
			CenterX:    32,
			CenterY:    32,
			DirectionX: 1,
			LineLength: 40,
			Stops: []geometry.GradientStop{
				{Position: 0, Color: geometry.NewColor(0, 0, 0)},
				{Position: 1, Color: geometry.NewColor(255, 255, 255)},
			},
		}, Rect: geometry.Rect{X: 30, Y: 2, Width: 32, Height: 20}, Clip: geometry.Rect{Width: 64, Height: 64}},
		{Kind: interfaces.CommandDrawImage, Image: imageSource, Rect: geometry.Rect{X: 2, Y: 30, Width: 12, Height: 12}, Opacity: 0.8},
		{Kind: interfaces.CommandDrawGlyph, Glyph: interfaces.Glyph{Mask: glyph, X: 20, Y: 30, Color: geometry.NewColor(20, 20, 20)}},
		{Kind: interfaces.CommandBlendPixel, X: 40, Y: 40, Color: geometry.NewColor(200, 0, 200), Coverage: 0.5},
		{Kind: interfaces.CommandCompositeLayer, Layer: layer, Opacity: 0.6},
	}
	softwareRenderer := software.NewRenderer()
	softwareSurface, err := softwareRenderer.Render(scene)
	if err != nil {
		t.Fatal(err)
	}
	backend, _ := NewBackend()
	vulkanRenderer, _ := backend.NewRenderer()
	vulkanSurface, err := vulkanRenderer.Render(scene)
	if err != nil {
		t.Fatal(err)
	}
	if typed, ok := vulkanRenderer.(*Renderer); ok && typed.gpuErr != nil {
		t.Fatalf("GPU path failed: %v", typed.gpuErr)
	}
	expected := softwareSurface.Image()
	actual := vulkanSurface.Image()
	for yPos := 0; yPos < 64; yPos++ {
		for xPos := 0; xPos < 64; xPos++ {
			first := expected.NRGBAAt(xPos, yPos)
			second := actual.NRGBAAt(xPos, yPos)
			delta := func(first, second uint8) int {
				if first > second {
					return int(first - second)
				}
				return int(second - first)
			}
			if delta(first.R, second.R) > 8 || delta(first.G, second.G) > 8 || delta(first.B, second.B) > 8 || delta(first.A, second.A) > 8 {
				t.Fatalf("pixel (%d,%d) = %#v, want %#v", xPos, yPos, second, first)
			}
		}
	}
}

func TestRendererCoversFullQuad(t *testing.T) {
	backend, _ := NewBackend()
	renderer, _ := backend.NewRenderer()
	source := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	for offset := 0; offset < len(source.Pix); offset += 4 {
		source.Pix[offset] = 0
		source.Pix[offset+1] = 0
		source.Pix[offset+2] = 255
		source.Pix[offset+3] = 255
	}
	scene := &interfaces.Scene{Width: 16, Height: 16}
	scene.Commands = []interfaces.Command{
		{Kind: interfaces.CommandClear, Color: geometry.Color{}},
		{Kind: interfaces.CommandDrawImage, Image: source, Rect: geometry.Rect{Width: 16, Height: 16}, Clip: geometry.Rect{Width: 16, Height: 16}, Opacity: 1},
	}
	surface, err := renderer.Render(scene)
	if err != nil {
		t.Fatal(err)
	}
	picture := surface.Image()
	for _, point := range [][2]int{{0, 0}, {15, 0}, {0, 15}, {15, 15}, {8, 15}, {15, 8}} {
		pixel := picture.NRGBAAt(point[0], point[1])
		if pixel.B < 250 || pixel.R > 5 || pixel.G > 5 || pixel.A < 250 {
			t.Fatalf("pixel %v = %#v", point, pixel)
		}
	}
}

func TestRendererDoesNotReuseStaleCoverageOverlays(t *testing.T) {
	backend, _ := NewBackend()
	renderer, _ := backend.NewRenderer()
	buildScene := func(left, top int) *interfaces.Scene {
		scene := &interfaces.Scene{Width: 64, Height: 64}
		scene.Commands = append(scene.Commands, interfaces.Command{Kind: interfaces.CommandClear, Color: geometry.Color{}})
		for yPos := 0; yPos < 20; yPos++ {
			for xPos := 0; xPos < 20; xPos++ {
				scene.Commands = append(scene.Commands, interfaces.Command{
					Kind:     interfaces.CommandBlendPixel,
					X:        left + xPos,
					Y:        top + yPos,
					Color:    geometry.NewColor(255, 0, 0),
					Coverage: 1,
				})
			}
		}
		return scene
	}
	first, err := renderer.Render(buildScene(10, 10))
	if err != nil {
		t.Fatal(err)
	}
	second, err := renderer.Render(buildScene(40, 40))
	if err != nil {
		t.Fatal(err)
	}
	if pixel := first.Image().NRGBAAt(15, 15); pixel.R < 250 || pixel.A < 250 {
		t.Fatalf("first overlay pixel = %#v", pixel)
	}
	if pixel := second.Image().NRGBAAt(15, 15); pixel.A != 0 {
		t.Fatalf("stale overlay pixel at old position = %#v", pixel)
	}
	if pixel := second.Image().NRGBAAt(45, 45); pixel.R < 250 || pixel.A < 250 {
		t.Fatalf("second overlay pixel = %#v", pixel)
	}
}

func TestRendererDoesNotReuseStaleLayers(t *testing.T) {
	backend, _ := NewBackend()
	renderer, _ := backend.NewRenderer()
	buildScene := func(left int) *interfaces.Scene {
		layer := &interfaces.Layer{Commands: []interfaces.Command{
			{Kind: interfaces.CommandFillRect, Rect: geometry.Rect{X: float64(left), Y: 10, Width: 10, Height: 10}, Color: geometry.NewColor(0, 255, 0)},
		}}
		return &interfaces.Scene{Width: 64, Height: 64, Commands: []interfaces.Command{
			{Kind: interfaces.CommandClear, Color: geometry.Color{}},
			{Kind: interfaces.CommandCompositeLayer, Layer: layer, Opacity: 1, Transform: geometry.IdentityMatrix4()},
		}}
	}
	first, err := renderer.Render(buildScene(10))
	if err != nil {
		t.Fatal(err)
	}
	second, err := renderer.Render(buildScene(40))
	if err != nil {
		t.Fatal(err)
	}
	if pixel := first.Image().NRGBAAt(15, 15); pixel.G < 250 || pixel.A < 250 {
		t.Fatalf("first layer pixel = %#v", pixel)
	}
	if pixel := second.Image().NRGBAAt(15, 15); pixel.A != 0 {
		t.Fatalf("stale layer pixel at old position = %#v", pixel)
	}
	if pixel := second.Image().NRGBAAt(45, 15); pixel.G < 250 || pixel.A < 250 {
		t.Fatalf("second layer pixel = %#v", pixel)
	}
}
