package rendering

import (
	"gonetsurf/layouting/model"
	"math"
	"strconv"
	"strings"

	"gonetsurf/parsers/css/parser"
	"gonetsurf/parsers/css/tokenizer"
)

type affine struct {
	A float64
	B float64
	C float64
	D float64
	E float64
	F float64
}

var identityAffine = affine{A: 1, D: 1}

func (matrix affine) multiply(other affine) affine {
	return affine{
		A: matrix.A*other.A + matrix.C*other.B,
		B: matrix.B*other.A + matrix.D*other.B,
		C: matrix.A*other.C + matrix.C*other.D,
		D: matrix.B*other.C + matrix.D*other.D,
		E: matrix.A*other.E + matrix.C*other.F + matrix.E,
		F: matrix.B*other.E + matrix.D*other.F + matrix.F,
	}
}

func (matrix affine) inverse() (affine, bool) {
	determinant := matrix.A*matrix.D - matrix.B*matrix.C
	if math.Abs(determinant) < 1e-12 {
		return identityAffine, false
	}
	return affine{
		A: matrix.D / determinant,
		B: -matrix.B / determinant,
		C: -matrix.C / determinant,
		D: matrix.A / determinant,
		E: (matrix.C*matrix.F - matrix.D*matrix.E) / determinant,
		F: (matrix.B*matrix.E - matrix.A*matrix.F) / determinant,
	}, true
}

func (matrix affine) apply(point Point) Point {
	return Point{
		X: matrix.A*point.X + matrix.C*point.Y + matrix.E,
		Y: matrix.B*point.X + matrix.D*point.Y + matrix.F,
	}
}

func translationAffine(xPos, yPos float64) affine {
	return affine{A: 1, D: 1, E: xPos, F: yPos}
}

func scaleAffine(xScale, yScale float64) affine {
	return affine{A: xScale, D: yScale}
}

func rotationAffine(radians float64) affine {
	cosine := math.Cos(radians)
	sine := math.Sin(radians)
	return affine{A: cosine, B: sine, C: -sine, D: cosine}
}

func skewAffine(xAngle, yAngle float64) affine {
	return affine{A: 1, B: math.Tan(yAngle), C: math.Tan(xAngle), D: 1}
}

// parseBoxTransform returns the absolute transform matrix for box and whether
// the box declares a transform.
func parseBoxTransform(box *model.Box) (affine, bool) {
	if box == nil || box.Style == nil {
		return identityAffine, false
	}
	text := strings.TrimSpace(box.Style.GetString("transform"))
	if text == "" || strings.EqualFold(text, "none") {
		return identityAffine, false
	}
	values := parser.ParseComponentValueList(text)
	matrix := identityAffine
	found := false
	for _, value := range values {
		if parser.IsWhitespace(value) {
			continue
		}
		function, ok := value.(*parser.Function)
		if !ok {
			return identityAffine, false
		}
		next, ok := transformFunction(function, box)
		if !ok {
			return identityAffine, false
		}
		matrix = matrix.multiply(next)
		found = true
	}
	if !found {
		return identityAffine, false
	}
	originX, originY := transformOrigin(box)
	return translationAffine(originX, originY).
		multiply(matrix).
		multiply(translationAffine(-originX, -originY)), true
}

func transformFunction(function *parser.Function, box *model.Box) (affine, bool) {
	name := strings.ToLower(function.Name)
	values := nonWhitespaceValues(function.Value)
	switch name {
	case "translate":
		xPos, okX := transformLength(values, 0, box.Width)
		yPos, okY := transformLength(values, 1, box.Height)
		if !okX || !okY {
			return identityAffine, false
		}
		return translationAffine(xPos, yPos), true
	case "translatex":
		xPos, ok := transformLength(values, 0, box.Width)
		if !ok {
			return identityAffine, false
		}
		return translationAffine(xPos, 0), true
	case "translatey":
		yPos, ok := transformLength(values, 0, box.Height)
		if !ok {
			return identityAffine, false
		}
		return translationAffine(0, yPos), true
	case "scale":
		xScale, ok := transformNumber(values, 0)
		if !ok {
			return identityAffine, false
		}
		yScale := xScale
		if len(values) > 1 {
			yScale, ok = transformNumber(values, 1)
			if !ok {
				return identityAffine, false
			}
		}
		return scaleAffine(xScale, yScale), true
	case "scalex":
		xScale, ok := transformNumber(values, 0)
		if !ok {
			return identityAffine, false
		}
		return scaleAffine(xScale, 1), true
	case "scaley":
		yScale, ok := transformNumber(values, 0)
		if !ok {
			return identityAffine, false
		}
		return scaleAffine(1, yScale), true
	case "rotate":
		angle, ok := transformAngle(values, 0)
		if !ok {
			return identityAffine, false
		}
		return rotationAffine(angle), true
	case "skew":
		xAngle, okX := transformAngle(values, 0)
		yAngle, okY := transformAngle(values, 1)
		if !okX || !okY {
			return identityAffine, false
		}
		return skewAffine(xAngle, yAngle), true
	case "skewx":
		angle, ok := transformAngle(values, 0)
		if !ok {
			return identityAffine, false
		}
		return skewAffine(angle, 0), true
	case "skewy":
		angle, ok := transformAngle(values, 0)
		if !ok {
			return identityAffine, false
		}
		return skewAffine(0, angle), true
	case "matrix":
		if len(values) != 6 {
			return identityAffine, false
		}
		numbers := make([]float64, 6)
		for index := range numbers {
			value, ok := transformNumber(values, index)
			if !ok {
				return identityAffine, false
			}
			numbers[index] = value
		}
		return affine{A: numbers[0], B: numbers[1], C: numbers[2], D: numbers[3], E: numbers[4], F: numbers[5]}, true
	default:
		return identityAffine, false
	}
}

func transformLength(values []parser.ComponentValue, index int, base float64) (float64, bool) {
	if index >= len(values) {
		if index == 0 {
			return 0, false
		}
		return 0, true
	}
	length, ok := parseLengthValue(values[index], 16)
	if !ok {
		return 0, false
	}
	return length.resolve(base), true
}

func transformNumber(values []parser.ComponentValue, index int) (float64, bool) {
	if index >= len(values) {
		if index == 0 {
			return 1, false
		}
		return 1, true
	}
	token, ok := parser.AsToken(values[index])
	if !ok {
		return 0, false
	}
	switch token.Type {
	case tokenizer.TokenNumber:
		return token.Number, true
	case tokenizer.TokenPercentage:
		return token.Number / 100, true
	}
	return 0, false
}

func transformAngle(values []parser.ComponentValue, index int) (float64, bool) {
	if index >= len(values) {
		if index == 0 {
			return 0, false
		}
		return 0, true
	}
	angle, ok := parseAngleValue(values[index])
	if !ok {
		return 0, false
	}
	return angle * math.Pi / 180, true
}

func transformOrigin(box *model.Box) (float64, float64) {
	originX := box.X + box.Width/2
	originY := box.Y + box.Height/2
	if box.Style == nil {
		return originX, originY
	}
	text := strings.TrimSpace(box.Style.GetString("transform-origin"))
	if text == "" || strings.EqualFold(text, "center") {
		return originX, originY
	}
	values := nonWhitespaceValues(parser.ParseComponentValueList(text))
	xLength, yLength := parseBackgroundPosition(values, 16)
	return box.X + xLength.resolve(box.Width), box.Y + yLength.resolve(box.Height)
}

// compositeTransformed draws source onto destination through matrix and opacity.
func parseOpacityValue(text string) (float64, bool) {
	text = strings.TrimSpace(text)
	if text == "" {
		return 1, false
	}
	value, err := strconv.ParseFloat(text, 64)
	if err != nil {
		return 1, false
	}
	return math.Max(0, math.Min(1, value)), true
}
