package rendering

import css "gonetsurf/rendering/utils/css"

// ParseColor parses a CSS color into a non-premultiplied 8-bit sRGB color.
// Out-of-gamut display-p3 and Lab-family colors are clipped to sRGB.
func ParseColor(input string) (Color, bool) {
	return css.ParseColor(input)
}
