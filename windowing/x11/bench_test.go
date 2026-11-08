package x11

import (
	"image"
	"os"
	"testing"

	"gonetsurf/rendering/geometry"
	"gonetsurf/rendering/software"
)

func BenchmarkPresentSHM(b *testing.B) {
	if os.Getenv("DISPLAY") == "" {
		b.Skip("DISPLAY is not set")
	}
	display := New(Options{Width: 256, Height: 256, Title: "gonetsurf shm benchmark"})
	opened, err := display.Open()
	if err != nil {
		b.Fatal(err)
	}
	defer display.Close()
	window := opened.(*Window)
	window.Surface().Clear(geometry.NewColor(20, 40, 60))
	_ = window.Present(image.Rect(0, 0, 256, 256))
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		if err := window.Present(image.Rect(0, 0, 256, 256)); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEncodeNRGBABand(b *testing.B) {
	surface := software.NewSurface(256, 256)
	surface.Clear(geometry.NewColor(20, 40, 60))
	converted := surface.Image()
	data := make([]byte, 256*256*4)
	masks := [3]uint32{0xff0000, 0xff00, 0xff}
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		encodeNRGBABand(converted, 256, 0, 256, 0, masks, data)
	}
}
