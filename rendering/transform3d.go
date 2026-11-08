package rendering

import (
	"gonetsurf/layouting/model"
	"math"
	"strings"

	"gonetsurf/parsers/css/parser"
)

// parseBoxTransformMatrix returns the full 3D transform for a box, including
// the individual translate/rotate/scale properties, transform, perspective,
// and transform-origin.
func parseBoxTransformMatrix(box *model.Box) (matrix4, bool) {
	if box == nil || box.Style == nil {
		return identityMatrix4(), false
	}
	found := false
	matrix := identityMatrix4()

	if text := strings.TrimSpace(box.Style.GetString("translate")); text != "" && !strings.EqualFold(text, "none") {
		if next, ok := parseTranslateProperty(text, box); ok {
			matrix = matrix.multiply(next)
			found = true
		}
	}
	if text := strings.TrimSpace(box.Style.GetString("rotate")); text != "" && !strings.EqualFold(text, "none") {
		if next, ok := parseRotateProperty(text, box); ok {
			matrix = matrix.multiply(next)
			found = true
		}
	}
	if text := strings.TrimSpace(box.Style.GetString("scale")); text != "" && !strings.EqualFold(text, "none") {
		if next, ok := parseScaleProperty(text, box); ok {
			matrix = matrix.multiply(next)
			found = true
		}
	}
	if text := strings.TrimSpace(box.Style.GetString("transform")); text != "" && !strings.EqualFold(text, "none") {
		values := parser.ParseComponentValueList(text)
		for _, value := range values {
			if parser.IsWhitespace(value) {
				continue
			}
			function, ok := value.(*parser.Function)
			if !ok {
				return identityMatrix4(), false
			}
			next, ok := transformFunctionMatrix4(function, box)
			if !ok {
				return identityMatrix4(), false
			}
			matrix = matrix.multiply(next)
			found = true
		}
	}
	if !found {
		return identityMatrix4(), false
	}
	if text := strings.TrimSpace(box.Style.GetString("perspective")); text != "" && !strings.EqualFold(text, "none") {
		values := nonWhitespaceValues(parser.ParseComponentValueList(text))
		if len(values) == 1 {
			if length, ok := parseLengthValue(values[0], 16); ok && length.value > 0 {
				originX, originY := perspectiveOrigin(box)
				matrix = translationMatrix4(originX, originY, 0).
					multiply(perspectiveMatrix4(length.value)).
					multiply(translationMatrix4(-originX, -originY, 0)).
					multiply(matrix)
			}
		}
	}
	originX, originY, originZ := transformOrigin3D(box)
	return translationMatrix4(originX, originY, originZ).
		multiply(matrix).
		multiply(translationMatrix4(-originX, -originY, -originZ)), true
}

func transformFunctionMatrix4(function *parser.Function, box *model.Box) (matrix4, bool) {
	name := strings.ToLower(function.Name)
	values := nonWhitespaceValues(function.Value)
	switch name {
	case "translate":
		xPos, okX := transformLength(values, 0, box.Width)
		yPos, okY := transformLength(values, 1, box.Height)
		zPos, _ := transformLength(values, 2, box.Width)
		if !okX || !okY {
			return identityMatrix4(), false
		}
		return translationMatrix4(xPos, yPos, zPos), true
	case "translate3d":
		xPos, okX := transformLength(values, 0, box.Width)
		yPos, okY := transformLength(values, 1, box.Height)
		zPos, okZ := transformLength(values, 2, box.Width)
		if !okX || !okY || !okZ {
			return identityMatrix4(), false
		}
		return translationMatrix4(xPos, yPos, zPos), true
	case "translatex":
		xPos, ok := transformLength(values, 0, box.Width)
		return translationMatrix4(xPos, 0, 0), ok
	case "translatey":
		yPos, ok := transformLength(values, 0, box.Height)
		return translationMatrix4(0, yPos, 0), ok
	case "translatez":
		zPos, ok := transformLength(values, 0, box.Width)
		return translationMatrix4(0, 0, zPos), ok
	case "scale":
		xScale, ok := transformNumber(values, 0)
		if !ok {
			return identityMatrix4(), false
		}
		yScale := xScale
		zScale := 1.0
		if len(values) > 1 {
			yScale, ok = transformNumber(values, 1)
			if !ok {
				return identityMatrix4(), false
			}
		}
		if len(values) > 2 {
			zScale, ok = transformNumber(values, 2)
			if !ok {
				return identityMatrix4(), false
			}
		}
		return scaleMatrix4(xScale, yScale, zScale), true
	case "scale3d":
		if len(values) < 3 {
			return identityMatrix4(), false
		}
		xScale, okX := transformNumber(values, 0)
		yScale, okY := transformNumber(values, 1)
		zScale, okZ := transformNumber(values, 2)
		if !okX || !okY || !okZ {
			return identityMatrix4(), false
		}
		return scaleMatrix4(xScale, yScale, zScale), true
	case "scalex":
		xScale, ok := transformNumber(values, 0)
		return scaleMatrix4(xScale, 1, 1), ok
	case "scaley":
		yScale, ok := transformNumber(values, 0)
		return scaleMatrix4(1, yScale, 1), ok
	case "scalez":
		zScale, ok := transformNumber(values, 0)
		return scaleMatrix4(1, 1, zScale), ok
	case "rotate":
		angle, ok := transformAngle(values, 0)
		if !ok {
			return identityMatrix4(), false
		}
		return rotationZMatrix4(angle), true
	case "rotatex":
		angle, ok := transformAngle(values, 0)
		return rotationXMatrix4(angle), ok
	case "rotatey":
		angle, ok := transformAngle(values, 0)
		return rotationYMatrix4(angle), ok
	case "rotatez":
		angle, ok := transformAngle(values, 0)
		return rotationZMatrix4(angle), ok
	case "rotate3d":
		if len(values) < 4 {
			return identityMatrix4(), false
		}
		xAxis, okX := transformNumber(values, 0)
		yAxis, okY := transformNumber(values, 1)
		zAxis, okZ := transformNumber(values, 2)
		angle, okAngle := transformAngle(values, 3)
		if !okX || !okY || !okZ || !okAngle {
			return identityMatrix4(), false
		}
		return rotationAxisMatrix4(xAxis, yAxis, zAxis, angle), true
	case "skew":
		xAngle, okX := transformAngle(values, 0)
		yAngle, okY := transformAngle(values, 1)
		if !okX || !okY {
			return identityMatrix4(), false
		}
		return affineToMatrix4(skewAffine(xAngle, yAngle)), true
	case "skewx":
		angle, ok := transformAngle(values, 0)
		return affineToMatrix4(skewAffine(angle, 0)), ok
	case "skewy":
		angle, ok := transformAngle(values, 0)
		return affineToMatrix4(skewAffine(0, angle)), ok
	case "matrix":
		if len(values) != 6 {
			return identityMatrix4(), false
		}
		numbers := make([]float64, 6)
		for index := range numbers {
			value, ok := transformNumber(values, index)
			if !ok {
				return identityMatrix4(), false
			}
			numbers[index] = value
		}
		return affineToMatrix4(affine{A: numbers[0], B: numbers[1], C: numbers[2], D: numbers[3], E: numbers[4], F: numbers[5]}), true
	case "matrix3d":
		if len(values) != 16 {
			return identityMatrix4(), false
		}
		var result matrix4
		for index := 0; index < 16; index++ {
			value, ok := transformNumber(values, index)
			if !ok {
				return identityMatrix4(), false
			}
			result[(index%4)*4+index/4] = value
		}
		return result, true
	case "perspective":
		distance, ok := transformLength(values, 0, box.Width)
		if !ok || distance <= 0 {
			return identityMatrix4(), false
		}
		return perspectiveMatrix4(distance), true
	default:
		return identityMatrix4(), false
	}
}

func parseTranslateProperty(text string, box *model.Box) (matrix4, bool) {
	values := nonWhitespaceValues(parser.ParseComponentValueList(text))
	if len(values) == 0 || len(values) > 3 {
		return identityMatrix4(), false
	}
	xPos, okX := transformLength(values, 0, box.Width)
	yPos, okY := transformLength(values, 1, box.Height)
	zPos, _ := transformLength(values, 2, box.Width)
	if !okX || !okY {
		return identityMatrix4(), false
	}
	return translationMatrix4(xPos, yPos, zPos), true
}

func parseRotateProperty(text string, box *model.Box) (matrix4, bool) {
	values := nonWhitespaceValues(parser.ParseComponentValueList(text))
	if len(values) == 0 {
		return identityMatrix4(), false
	}
	angle, ok := transformAngle(values, len(values)-1)
	if !ok {
		return identityMatrix4(), false
	}
	if len(values) == 1 {
		return rotationZMatrix4(angle), true
	}
	if len(values) == 2 {
		token, _ := parser.AsToken(values[0])
		switch strings.ToLower(token.Value) {
		case "x":
			return rotationXMatrix4(angle), true
		case "y":
			return rotationYMatrix4(angle), true
		case "z":
			return rotationZMatrix4(angle), true
		}
	}
	if len(values) == 4 {
		xAxis, okX := transformNumber(values, 0)
		yAxis, okY := transformNumber(values, 1)
		zAxis, okZ := transformNumber(values, 2)
		if !okX || !okY || !okZ {
			return identityMatrix4(), false
		}
		return rotationAxisMatrix4(xAxis, yAxis, zAxis, angle), true
	}
	return identityMatrix4(), false
}

func parseScaleProperty(text string, box *model.Box) (matrix4, bool) {
	values := nonWhitespaceValues(parser.ParseComponentValueList(text))
	if len(values) == 0 || len(values) > 3 {
		return identityMatrix4(), false
	}
	xScale, ok := transformNumber(values, 0)
	if !ok {
		return identityMatrix4(), false
	}
	yScale := xScale
	zScale := 1.0
	if len(values) > 1 {
		yScale, ok = transformNumber(values, 1)
		if !ok {
			return identityMatrix4(), false
		}
	}
	if len(values) > 2 {
		zScale, ok = transformNumber(values, 2)
		if !ok {
			return identityMatrix4(), false
		}
	}
	return scaleMatrix4(xScale, yScale, zScale), true
}

func affineToMatrix4(matrix affine) matrix4 {
	return matrix4{
		matrix.A, matrix.C, 0, matrix.E,
		matrix.B, matrix.D, 0, matrix.F,
		0, 0, 1, 0,
		0, 0, 0, 1,
	}
}

func transformOrigin3D(box *model.Box) (float64, float64, float64) {
	originX := box.X + box.Width/2
	originY := box.Y + box.Height/2
	originZ := 0.0
	if box.Style == nil {
		return originX, originY, originZ
	}
	values := nonWhitespaceValues(parser.ParseComponentValueList(box.Style.GetString("transform-origin")))
	if len(values) == 0 {
		return originX, originY, originZ
	}
	xLength, yLength := parseBackgroundPosition(values, 16)
	originX = box.X + xLength.resolve(box.Width)
	originY = box.Y + yLength.resolve(box.Height)
	if len(values) > 2 {
		if length, ok := parseLengthValue(values[2], 16); ok {
			originZ = length.value
		}
	}
	return originX, originY, originZ
}

func perspectiveOrigin(box *model.Box) (float64, float64) {
	originX := box.X + box.Width/2
	originY := box.Y + box.Height/2
	if box.Style == nil {
		return originX, originY
	}
	values := nonWhitespaceValues(parser.ParseComponentValueList(box.Style.GetString("perspective-origin")))
	if len(values) == 0 {
		return originX, originY
	}
	xLength, yLength := parseBackgroundPosition(values, 16)
	return box.X + xLength.resolve(box.Width), box.Y + yLength.resolve(box.Height)
}

func rotationAxisMatrix4(xAxis, yAxis, zAxis, radians float64) matrix4 {
	length := math.Sqrt(xAxis*xAxis + yAxis*yAxis + zAxis*zAxis)
	if length == 0 {
		return identityMatrix4()
	}
	xAxis /= length
	yAxis /= length
	zAxis /= length
	cosine := math.Cos(radians)
	sine := math.Sin(radians)
	oneMinus := 1 - cosine
	return matrix4{
		cosine + xAxis*xAxis*oneMinus, xAxis*yAxis*oneMinus - zAxis*sine, xAxis*zAxis*oneMinus + yAxis*sine, 0,
		yAxis*xAxis*oneMinus + zAxis*sine, cosine + yAxis*yAxis*oneMinus, yAxis*zAxis*oneMinus - xAxis*sine, 0,
		zAxis*xAxis*oneMinus - yAxis*sine, zAxis*yAxis*oneMinus + xAxis*sine, cosine + zAxis*zAxis*oneMinus, 0,
		0, 0, 0, 1,
	}
}

func hasBoxTransform(box *model.Box) bool {
	_, ok := parseBoxTransformMatrix(box)
	return ok
}
