package software

import (
	"gonetsurf/layouting/model"
	"strings"

	"gonetsurf/parsers/css/parser"
	"gonetsurf/rendering/geometry"
	css "gonetsurf/rendering/utils/css"
)

func nonWhitespaceValues(values []parser.ComponentValue) []parser.ComponentValue {
	return css.NonWhitespaceValues(values)
}

func transformLength(values []parser.ComponentValue, index int, base float64) (float64, bool) {
	return css.TransformLength(values, index, base)
}

func transformNumber(values []parser.ComponentValue, index int) (float64, bool) {
	return css.TransformNumber(values, index)
}

func transformAngle(values []parser.ComponentValue, index int) (float64, bool) {
	return css.TransformAngle(values, index)
}

func ParseColor(input string) (Color, bool) { return css.ParseColor(input) }

func transformFunction(function *parser.Function, box *model.Box) (geometry.Affine, bool) {
	name := strings.ToLower(function.Name)
	values := nonWhitespaceValues(function.Value)
	switch name {
	case "translate":
		xPos, okX := transformLength(values, 0, box.Width)
		yPos, okY := transformLength(values, 1, box.Height)
		if !okX || !okY {
			return geometry.IdentityAffine(), false
		}
		return geometry.TranslationAffine(xPos, yPos), true
	case "translatex":
		xPos, ok := transformLength(values, 0, box.Width)
		if !ok {
			return geometry.IdentityAffine(), false
		}
		return geometry.TranslationAffine(xPos, 0), true
	case "translatey":
		yPos, ok := transformLength(values, 0, box.Height)
		if !ok {
			return geometry.IdentityAffine(), false
		}
		return geometry.TranslationAffine(0, yPos), true
	case "scale":
		xScale, ok := transformNumber(values, 0)
		if !ok {
			return geometry.IdentityAffine(), false
		}
		yScale := xScale
		if len(values) > 1 {
			yScale, ok = transformNumber(values, 1)
			if !ok {
				return geometry.IdentityAffine(), false
			}
		}
		return geometry.ScaleAffine(xScale, yScale), true
	case "scalex":
		xScale, ok := transformNumber(values, 0)
		if !ok {
			return geometry.IdentityAffine(), false
		}
		return geometry.ScaleAffine(xScale, 1), true
	case "scaley":
		yScale, ok := transformNumber(values, 0)
		if !ok {
			return geometry.IdentityAffine(), false
		}
		return geometry.ScaleAffine(1, yScale), true
	case "rotate":
		angle, ok := transformAngle(values, 0)
		if !ok {
			return geometry.IdentityAffine(), false
		}
		return geometry.RotationAffine(angle), true
	case "skew":
		xAngle, okX := transformAngle(values, 0)
		yAngle, okY := transformAngle(values, 1)
		if !okX || !okY {
			return geometry.IdentityAffine(), false
		}
		return geometry.SkewAffine(xAngle, yAngle), true
	case "skewx":
		angle, ok := transformAngle(values, 0)
		if !ok {
			return geometry.IdentityAffine(), false
		}
		return geometry.SkewAffine(angle, 0), true
	case "skewy":
		angle, ok := transformAngle(values, 0)
		if !ok {
			return geometry.IdentityAffine(), false
		}
		return geometry.SkewAffine(0, angle), true
	case "matrix":
		if len(values) != 6 {
			return geometry.IdentityAffine(), false
		}
		numbers := make([]float64, 6)
		for index := range numbers {
			value, ok := transformNumber(values, index)
			if !ok {
				return geometry.IdentityAffine(), false
			}
			numbers[index] = value
		}
		return geometry.Affine{A: numbers[0], B: numbers[1], C: numbers[2], D: numbers[3], E: numbers[4], F: numbers[5]}, true
	default:
		return geometry.IdentityAffine(), false
	}
}
