//go:build vulkan && cgo

package vulkan

import (
	"image"
	"os"
	"testing"

	"gonetsurf/rendering/geometry"
	"gonetsurf/rendering/software"
	"gonetsurf/windowing"
	"gonetsurf/windowing/x11"
)

func TestRendererPresentsThroughSwapchain(t *testing.T) {
	if os.Getenv("DISPLAY") == "" {
		t.Skip("DISPLAY is not set")
	}
	display := x11.New(x11.Options{Width: 64, Height: 64, Title: "gonetsurf vulkan test"})
	window, err := display.Open()
	if err != nil {
		t.Skipf("cannot open X11 window: %v", err)
	}
	defer display.Close()
	native, ok := window.(windowing.NativeWindow)
	if !ok {
		t.Fatal("window does not expose a native handle")
	}
	renderer := &Renderer{}
	surface := software.NewSurface(64, 64)
	surface.Clear(geometry.NewColor(30, 40, 50))
	if err := renderer.AttachPresenter(native.DisplayName(), native.NativeWindowID(), 64, 64); err != nil {
		t.Fatalf("attach presenter: %v", err)
	}
	if err := renderer.PresentSurface(surface, image.Rect(0, 0, 64, 64)); err != nil {
		t.Fatalf("present surface: %v", err)
	}
	renderer.Close()
}
