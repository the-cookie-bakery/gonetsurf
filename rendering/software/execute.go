package software

import "gonetsurf/rendering/interfaces"

// Execute replays a recorded scene into a software surface.
func Execute(scene *interfaces.Scene, target *Surface) {
	if scene == nil || target == nil {
		return
	}
	executeCommands(scene.Commands, target)
}

func executeCommands(commands []interfaces.Command, target *Surface) {
	for _, command := range commands {
		switch command.Kind {
		case interfaces.CommandClear:
			target.Clear(command.Color)
		case interfaces.CommandFillRect:
			target.FillRect(command.Rect, command.Color)
		case interfaces.CommandFillPath:
			target.FillPath(command.Path, command.FillRule, command.Color)
		case interfaces.CommandStrokePath:
			target.StrokePath(command.Path, command.Stroke, command.Color)
		case interfaces.CommandFillMask:
			target.FillMask(command.Mask, command.Color)
		case interfaces.CommandFillGradient:
			target.FillGradient(command.Gradient, command.Rect, command.Clip, command.Mask)
		case interfaces.CommandDrawImage:
			target.DrawImage(command.Image, command.Rect, command.Clip, command.Mask, command.Opacity)
		case interfaces.CommandDrawGlyph:
			target.DrawGlyphMask(command.Glyph.Mask, command.Glyph.X, command.Glyph.Y, command.Glyph.Color)
		case interfaces.CommandBlendPixel:
			target.BlendPixel(command.X, command.Y, command.Color, command.Coverage)
		case interfaces.CommandSetClip:
			target.SetClip(command.Rect)
		case interfaces.CommandResetClip:
			target.ResetClip()
		case interfaces.CommandBlit:
			target.Blit(command.X, command.Y, command.Source, command.Composite)
		case interfaces.CommandBlitOpacity:
			target.BlitOpacity(command.X, command.Y, command.Source, command.Opacity)
		case interfaces.CommandCompositeLayer:
			if command.Layer == nil {
				continue
			}
			layer := NewSurface(target.Width, target.Height)
			executeCommands(command.Layer.Commands, layer)
			target.CompositeLayer(layer, command.Transform, command.Opacity)
		case interfaces.CommandCompositeLayerMasked:
			if command.Layer == nil {
				continue
			}
			layer := NewSurface(target.Width, target.Height)
			executeCommands(command.Layer.Commands, layer)
			target.CompositeLayerMasked(layer, command.Rect, command.Mask, command.Opacity)
		}
	}
}
