// Package scene records painter drawing operations into a backend-neutral
// display list that any renderer can execute.
package scene

import (
	"image"

	"gonetsurf/rendering/geometry"
	"gonetsurf/rendering/interfaces"
)

// Recorder implements interfaces.Canvas by appending commands.
type Recorder struct {
	width    int
	height   int
	clip     geometry.Rect
	commands []interfaces.Command
}

// New returns a recorder for a frame of the given size.
func New(width, height int) *Recorder {
	if width < 0 {
		width = 0
	}
	if height < 0 {
		height = 0
	}
	return &Recorder{
		width:  width,
		height: height,
		clip:   geometry.Rect{Width: float64(width), Height: float64(height)},
	}
}

// Width returns the frame width.
func (recorder *Recorder) CanvasWidth() int { return recorder.width }

// Height returns the frame height.
func (recorder *Recorder) CanvasHeight() int { return recorder.height }

// Clip returns the current clip rectangle.
func (recorder *Recorder) Clip() geometry.Rect { return recorder.clip }

// SetClip replaces the clip rectangle.
func (recorder *Recorder) SetClip(rect geometry.Rect) {
	recorder.clip = rect.Intersect(geometry.Rect{Width: float64(recorder.width), Height: float64(recorder.height)})
	recorder.commands = append(recorder.commands, interfaces.Command{Kind: interfaces.CommandSetClip, Rect: recorder.clip})
}

// ResetClip restores the full frame clip.
func (recorder *Recorder) ResetClip() {
	recorder.clip = geometry.Rect{Width: float64(recorder.width), Height: float64(recorder.height)}
	recorder.commands = append(recorder.commands, interfaces.Command{Kind: interfaces.CommandResetClip})
}

// Clear appends a clear command.
func (recorder *Recorder) Clear(color geometry.Color) {
	recorder.commands = append(recorder.commands, interfaces.Command{Kind: interfaces.CommandClear, Color: color})
}

// FillRect appends a rectangle fill.
func (recorder *Recorder) FillRect(rect geometry.Rect, color geometry.Color) {
	recorder.commands = append(recorder.commands, interfaces.Command{Kind: interfaces.CommandFillRect, Rect: rect, Color: color})
}

// FillPath appends a path fill.
func (recorder *Recorder) FillPath(path *geometry.Path, rule geometry.FillRule, color geometry.Color) {
	recorder.commands = append(recorder.commands, interfaces.Command{Kind: interfaces.CommandFillPath, Path: path, FillRule: rule, Color: color})
}

// StrokePath appends a path stroke.
func (recorder *Recorder) StrokePath(path *geometry.Path, style geometry.StrokeStyle, color geometry.Color) {
	recorder.commands = append(recorder.commands, interfaces.Command{Kind: interfaces.CommandStrokePath, Path: path, Stroke: style, Color: color})
}

// FillMask appends a masked color fill.
func (recorder *Recorder) FillMask(mask *geometry.Mask, color geometry.Color) {
	recorder.commands = append(recorder.commands, interfaces.Command{Kind: interfaces.CommandFillMask, Mask: mask, Color: color})
}

// FillGradient appends a gradient fill.
func (recorder *Recorder) FillGradient(gradient geometry.Gradient, area, clip geometry.Rect, mask *geometry.Mask) {
	recorder.commands = append(recorder.commands, interfaces.Command{
		Kind:     interfaces.CommandFillGradient,
		Gradient: gradient,
		Rect:     area,
		Clip:     clip,
		Mask:     mask,
	})
}

// DrawImage appends an image draw.
func (recorder *Recorder) DrawImage(source image.Image, destination, clip geometry.Rect, mask *geometry.Mask, opacity float64) {
	recorder.commands = append(recorder.commands, interfaces.Command{
		Kind:    interfaces.CommandDrawImage,
		Image:   source,
		Rect:    destination,
		Clip:    clip,
		Mask:    mask,
		Opacity: opacity,
	})
}

// DrawGlyphMask appends a glyph draw.
func (recorder *Recorder) DrawGlyphMask(mask *image.Alpha, xPos, yPos int, color geometry.Color) {
	recorder.commands = append(recorder.commands, interfaces.Command{
		Kind:  interfaces.CommandDrawGlyph,
		Glyph: interfaces.Glyph{Mask: mask, X: xPos, Y: yPos, Color: color},
	})
}

// BlendPixel appends a single coverage blend.
func (recorder *Recorder) BlendPixel(xPos, yPos int, color geometry.Color, coverage float64) {
	recorder.commands = append(recorder.commands, interfaces.Command{
		Kind:     interfaces.CommandBlendPixel,
		X:        xPos,
		Y:        yPos,
		Color:    color,
		Coverage: coverage,
	})
}

// Blit appends a surface blit.
func (recorder *Recorder) Blit(offsetX, offsetY int, source interfaces.Surface, op geometry.CompositeOp) {
	recorder.commands = append(recorder.commands, interfaces.Command{Kind: interfaces.CommandBlit, X: offsetX, Y: offsetY, Source: source, Composite: op})
}

// BlitOpacity appends an opacity blit.
func (recorder *Recorder) BlitOpacity(offsetX, offsetY int, source interfaces.Surface, opacity float64) {
	recorder.commands = append(recorder.commands, interfaces.Command{Kind: interfaces.CommandBlitOpacity, X: offsetX, Y: offsetY, Source: source, Opacity: opacity})
}

// NewLayer creates an independent command list.
func (recorder *Recorder) NewLayer() interfaces.Canvas {
	return New(recorder.width, recorder.height)
}

// CompositeLayer appends a transformed layer composite.
func (recorder *Recorder) CompositeLayer(layer interfaces.Canvas, transform geometry.Matrix4, opacity float64) {
	recorded, ok := layer.(*Recorder)
	if !ok {
		return
	}
	recorder.commands = append(recorder.commands, interfaces.Command{
		Kind:      interfaces.CommandCompositeLayer,
		Layer:     &interfaces.Layer{Commands: recorded.commands},
		Transform: transform,
		Opacity:   opacity,
	})
}

// CompositeLayerMasked appends a masked layer composite.
func (recorder *Recorder) CompositeLayerMasked(layer interfaces.Canvas, clip geometry.Rect, mask *geometry.Mask, opacity float64) {
	recorded, ok := layer.(*Recorder)
	if !ok {
		return
	}
	recorder.commands = append(recorder.commands, interfaces.Command{
		Kind:    interfaces.CommandCompositeLayerMasked,
		Layer:   &interfaces.Layer{Commands: recorded.commands},
		Rect:    clip,
		Mask:    mask,
		Opacity: opacity,
	})
}

// Scene returns the recorded scene.
func (recorder *Recorder) Scene() *interfaces.Scene {
	return &interfaces.Scene{Width: recorder.width, Height: recorder.height, Commands: recorder.commands}
}
