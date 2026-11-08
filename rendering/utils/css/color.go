// Package css contains shared CSS value parsing helpers used by the painter
// and by software image rasterizers such as SVG.
package css

import (
	"math"
	"strings"

	"gonetsurf/parsers/css/colors"
	"gonetsurf/rendering/geometry"
)

// ParseColor parses a CSS color into a straight sRGB color. Out-of-gamut
// display-p3 and Lab-family colors are gamut-mapped to sRGB.
func ParseColor(input string) (geometry.Color, bool) {
	parsed, ok := colors.Parse(strings.TrimSpace(input))
	if !ok || parsed == nil {
		return geometry.Transparent, false
	}
	converted, ok := parsed.ToSRGBGamutMapped()
	if !ok {
		return geometry.Transparent, false
	}
	return geometry.Color{
		Red:   channelToByte(converted.Red),
		Green: channelToByte(converted.Green),
		Blue:  channelToByte(converted.Blue),
		Alpha: channelToByte(converted.Alpha),
	}, true
}

func channelToByte(value float64) uint8 {
	if value <= 0 {
		return 0
	}
	if value >= 1 {
		return 255
	}
	return uint8(math.Round(value * 255))
}
