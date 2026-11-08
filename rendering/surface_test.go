package rendering

import (
	"image/color"
	"testing"
)

func TestFillRectOpaque(t *testing.T) {
	surface := NewSurface(4, 4)
	red := NewColor(255, 0, 0)
	surface.FillRect(Rect{X: 1, Y: 1, Width: 2, Height: 2}, red)

	for yPos := 0; yPos < 4; yPos++ {
		for xPos := 0; xPos < 4; xPos++ {
			inside := xPos >= 1 && xPos < 3 && yPos >= 1 && yPos < 3
			offset := yPos*surface.Stride + xPos*4
			alpha := surface.Pix[offset+3]
			if inside && alpha != 255 {
				t.Errorf("inside pixel (%d,%d) alpha = %d", xPos, yPos, alpha)
			}
			if !inside && alpha != 0 {
				t.Errorf("outside pixel (%d,%d) alpha = %d", xPos, yPos, alpha)
			}
		}
	}
}

func TestFillRectFractionalCoverage(t *testing.T) {
	surface := NewSurface(3, 1)
	surface.FillRect(Rect{X: 0.5, Y: 0, Width: 1, Height: 1}, NewColor(0, 0, 0))
	first := surface.Pix[3]
	second := surface.Pix[7]
	third := surface.Pix[11]
	if first < 127 || first > 128 {
		t.Errorf("first pixel alpha = %d, want ~128", first)
	}
	if second < 127 || second > 128 {
		t.Errorf("second pixel alpha = %d, want ~128", second)
	}
	if third != 0 {
		t.Errorf("third pixel alpha = %d, want 0", third)
	}
}

func TestClearAndClip(t *testing.T) {
	surface := NewSurface(4, 4)
	surface.Clear(NewColor(10, 20, 30))
	surface.SetClip(Rect{X: 2, Y: 2, Width: 2, Height: 2})
	surface.FillRect(Rect{Width: 4, Height: 4}, NewColor(255, 0, 0))

	clipped := surface.Pix[2*surface.Stride+2*4]
	if clipped != 255 {
		t.Errorf("clipped pixel red = %d, want 255", clipped)
	}
	outside := surface.Pix[0]
	if outside != 10 {
		t.Errorf("unclipped pixel red = %d, want 10", outside)
	}
	surface.ResetClip()
	if surface.Clip().Width != 4 || surface.Clip().Height != 4 {
		t.Errorf("reset clip = %#v", surface.Clip())
	}
}

func TestImageUnpremultiplies(t *testing.T) {
	surface := NewSurface(1, 1)
	surface.BlendPixel(0, 0, Color{Red: 255, Green: 0, Blue: 0, Alpha: 128}, 1)
	converted := surface.Image()
	got := converted.NRGBAAt(0, 0)
	want := color.NRGBA{R: 255, A: 128}
	if got != want {
		t.Errorf("image pixel = %#v, want %#v", got, want)
	}
}
