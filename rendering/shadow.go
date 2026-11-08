package rendering

import (
	"gonetsurf/layouting/model"
	"gonetsurf/rendering/interfaces"
	"math"
	"strings"

	"gonetsurf/parsers/css/parser"
	"gonetsurf/parsers/css/tokenizer"
	"gonetsurf/styles"
)

type boxShadow struct {
	inset   bool
	offsetX float64
	offsetY float64
	blur    float64
	spread  float64
	color   Color
}

func parseBoxShadows(style *styles.Style) []boxShadow {
	if style == nil {
		return nil
	}
	groups := splitCommaGroups(style.Get("box-shadow"))
	if len(groups) == 0 {
		return nil
	}
	defaultColor, ok := ParseColor(style.GetString("color"))
	if !ok {
		defaultColor = NewColor(0, 0, 0)
	}
	fontSize := 16.0
	if value, ok := parseCSSPixelValue(style.GetString("font-size")); ok && value > 0 {
		fontSize = value
	}

	var shadows []boxShadow
	for _, group := range groups {
		shadow := boxShadow{color: defaultColor}
		var lengths []float64
		var color Color
		hasColor := false
		for _, value := range group {
			if parser.IsWhitespace(value) {
				continue
			}
			if token, ok := parser.AsToken(value); ok {
				if token.Type == tokenizer.TokenIdent {
					lower := strings.ToLower(token.Value)
					if lower == "inset" {
						shadow.inset = true
						continue
					}
					if lower == "none" {
						return nil
					}
				}
				if length, ok := parseLengthValue(value, fontSize); ok && !length.percent && !length.auto {
					lengths = append(lengths, length.value)
					continue
				}
				if token.Type == tokenizer.TokenHash {
					if parsed, ok := ParseColor(token.Representation); ok {
						color = parsed
						hasColor = true
					}
					continue
				}
				if token.Type == tokenizer.TokenIdent {
					if parsed, ok := ParseColor(token.Value); ok {
						color = parsed
						hasColor = true
					}
				}
				continue
			}
			if parsed, ok := ParseColor(parser.SerializeValues([]parser.ComponentValue{value})); ok {
				color = parsed
				hasColor = true
			}
		}
		if len(lengths) < 2 || len(lengths) > 4 {
			continue
		}
		shadow.offsetX = lengths[0]
		shadow.offsetY = lengths[1]
		if len(lengths) > 2 {
			shadow.blur = math.Max(0, lengths[2])
		}
		if len(lengths) > 3 {
			shadow.spread = lengths[3]
		}
		if hasColor {
			shadow.color = color
		}
		if shadow.color.Alpha == 0 {
			continue
		}
		shadows = append(shadows, shadow)
	}
	return shadows
}

func (painter *Painter) paintOuterShadows(box *model.Box, surface interfaces.Canvas, xPos, yPos float64) {
	shadows := parseBoxShadows(box.Style)
	if len(shadows) == 0 {
		return
	}
	base := box.ContainingWidth
	if base <= 0 {
		base = box.Width
	}
	radii := borderRadii(box.Style, box.Width, box.Height, base)
	for index := len(shadows) - 1; index >= 0; index-- {
		shadow := shadows[index]
		if shadow.inset {
			continue
		}
		rect := Rect{
			X:      xPos + shadow.offsetX - shadow.spread,
			Y:      yPos + shadow.offsetY - shadow.spread,
			Width:  box.Width + shadow.spread*2,
			Height: box.Height + shadow.spread*2,
		}
		if rect.Width <= 0 || rect.Height <= 0 {
			continue
		}
		path := NewPath()
		path.AddRoundedRect(rect, expandRadii(radii, shadow.spread))
		mask := RasterizeFill(path, surface.CanvasWidth(), surface.CanvasHeight(), NonZeroFill)
		if shadow.blur > 0 {
			mask = BlurMask(mask, shadow.blur)
		}
		surface.FillMask(mask, shadow.color)
	}
}

func (painter *Painter) paintInnerShadows(box *model.Box, surface interfaces.Canvas, xPos, yPos float64) {
	shadows := parseBoxShadows(box.Style)
	if len(shadows) == 0 {
		return
	}
	base := box.ContainingWidth
	if base <= 0 {
		base = box.Width
	}
	radii := borderRadii(box.Style, box.Width, box.Height, base)
	outerRect := Rect{X: xPos, Y: yPos, Width: box.Width, Height: box.Height}
	outerPath := NewPath()
	outerPath.AddRoundedRect(outerRect, radii)
	outerMask := RasterizeFill(outerPath, surface.CanvasWidth(), surface.CanvasHeight(), NonZeroFill)

	for index := len(shadows) - 1; index >= 0; index-- {
		shadow := shadows[index]
		if !shadow.inset {
			continue
		}
		rect := Rect{
			X:      xPos + shadow.offsetX + shadow.spread,
			Y:      yPos + shadow.offsetY + shadow.spread,
			Width:  math.Max(0, box.Width-shadow.spread*2),
			Height: math.Max(0, box.Height-shadow.spread*2),
		}
		if rect.Width <= 0 || rect.Height <= 0 {
			continue
		}
		path := NewPath()
		path.AddRoundedRect(rect, expandRadii(radii, -shadow.spread))
		shadowMask := RasterizeFill(path, surface.CanvasWidth(), surface.CanvasHeight(), NonZeroFill)
		ring := NewMask(surface.CanvasWidth(), surface.CanvasHeight())
		for yPixel := 0; yPixel < surface.CanvasHeight(); yPixel++ {
			for xPixel := 0; xPixel < surface.CanvasWidth(); xPixel++ {
				index := yPixel*surface.CanvasWidth() + xPixel
				ring.Coverage[index] = outerMask.Coverage[index] * (1 - shadowMask.Coverage[index])
			}
		}
		if shadow.blur > 0 {
			ring = BlurMask(ring, shadow.blur)
		}
		surface.FillMask(ring, shadow.color)
	}
}

func expandRadii(radii CornerRadii, amount float64) CornerRadii {
	adjust := func(radius Point) Point {
		return Point{X: math.Max(0, radius.X+amount), Y: math.Max(0, radius.Y+amount)}
	}
	return CornerRadii{
		TopLeft:     adjust(radii.TopLeft),
		TopRight:    adjust(radii.TopRight),
		BottomRight: adjust(radii.BottomRight),
		BottomLeft:  adjust(radii.BottomLeft),
	}
}
