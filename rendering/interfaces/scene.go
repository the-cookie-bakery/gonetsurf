package interfaces

import (
	"image"

	"gonetsurf/rendering/geometry"
)

// CommandKind identifies a recorded canvas operation.
type CommandKind uint8

const (
	// CommandClear clears the target.
	CommandClear CommandKind = iota
	// CommandFillRect fills a rectangle.
	CommandFillRect
	// CommandFillPath fills a path.
	CommandFillPath
	// CommandStrokePath strokes a path.
	CommandStrokePath
	// CommandFillMask composites color through a coverage mask.
	CommandFillMask
	// CommandFillGradient fills an area with a gradient.
	CommandFillGradient
	// CommandDrawImage draws an image.
	CommandDrawImage
	// CommandDrawGlyph draws a glyph mask.
	CommandDrawGlyph
	// CommandBlendPixel blends one coverage pixel.
	CommandBlendPixel
	// CommandSetClip replaces the clip rectangle.
	CommandSetClip
	// CommandResetClip restores the full clip.
	CommandResetClip
	// CommandBlit composites a source surface.
	CommandBlit
	// CommandBlitOpacity composites a source surface with opacity.
	CommandBlitOpacity
	// CommandCompositeLayer composites a recorded layer.
	CommandCompositeLayer
	// CommandCompositeLayerMasked composites a recorded layer through a mask.
	CommandCompositeLayerMasked
)

// Glyph is a positioned alpha mask.
type Glyph struct {
	Mask  *image.Alpha
	X     int
	Y     int
	Color geometry.Color
}

// Layer is a nested command list.
type Layer struct {
	Commands []Command
}

// Command is one recorded canvas operation.
type Command struct {
	Kind CommandKind

	Color     geometry.Color
	Rect      geometry.Rect
	Clip      geometry.Rect
	Path      *geometry.Path
	FillRule  geometry.FillRule
	Stroke    geometry.StrokeStyle
	Mask      *geometry.Mask
	Gradient  geometry.Gradient
	Image     image.Image
	Glyph     Glyph
	Opacity   float64
	Coverage  float64
	X         int
	Y         int
	Transform geometry.Matrix4
	Composite geometry.CompositeOp
	Source    Surface
	Layer     *Layer
	UseNode   bool
	NodeIndex int
}

// Scene is a backend-neutral recorded frame.
type Scene struct {
	Width    int
	Height   int
	Commands []Command
}
