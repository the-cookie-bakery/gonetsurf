package software

import (
	"gonetsurf/layouting/model"
	"gonetsurf/styles"
)

func (layout *Layout) shapeText(text string, style *styles.Style) []model.Glyph {
	return model.ShapeText(layout.Shaper, text, style)
}
