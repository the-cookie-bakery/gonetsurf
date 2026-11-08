package rendering

import (
	"gonetsurf/layouting/model"
	"gonetsurf/rendering/interfaces"
	"math"
)

func (painter *Painter) paintScrollbars(box *model.Box, surface interfaces.Canvas, xPos, yPos float64) {
	if box == nil || (!box.HasVerticalScrollbar && !box.HasHorizontalScrollbar) {
		return
	}
	const thickness = model.ScrollbarThickness
	trackColor := NewColor(238, 238, 240)
	thumbColor := NewColor(148, 148, 154)
	contentX := xPos + (box.ContentX - box.X)
	contentY := yPos + (box.ContentY - box.Y)
	if box.HasVerticalScrollbar {
		trackX := contentX + box.ScrollportWidth()
		trackY := contentY
		trackHeight := box.ContentHeight
		surface.FillRect(Rect{X: trackX, Y: trackY, Width: thickness, Height: trackHeight}, trackColor)
		maximum := box.MaxScrollTop()
		if maximum > 0 && box.ScrollHeight > 0 {
			thumbHeight := math.Max(20, trackHeight*box.ContentHeight/box.ScrollHeight)
			if thumbHeight > trackHeight {
				thumbHeight = trackHeight
			}
			progress := math.Max(0, math.Min(1, box.ScrollTop/maximum))
			thumbY := trackY + (trackHeight-thumbHeight)*progress
			surface.FillRect(Rect{X: trackX, Y: thumbY, Width: thickness, Height: thumbHeight}, thumbColor)
		}
	}
	if box.HasHorizontalScrollbar {
		trackX := contentX
		trackY := contentY + box.ScrollportHeight()
		trackWidth := box.ContentWidth
		surface.FillRect(Rect{X: trackX, Y: trackY, Width: trackWidth, Height: thickness}, trackColor)
		maximum := box.MaxScrollLeft()
		if maximum > 0 && box.ScrollWidth > 0 {
			thumbWidth := math.Max(20, trackWidth*box.ContentWidth/box.ScrollWidth)
			if thumbWidth > trackWidth {
				thumbWidth = trackWidth
			}
			progress := math.Max(0, math.Min(1, box.ScrollLeft/maximum))
			thumbX := trackX + (trackWidth-thumbWidth)*progress
			surface.FillRect(Rect{X: thumbX, Y: trackY, Width: thumbWidth, Height: thickness}, thumbColor)
		}
	}
}
