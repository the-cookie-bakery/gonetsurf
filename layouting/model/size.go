package model

import (
	"strconv"
	"strings"

	"gonetsurf/parsers/css/parser"
	cssvalues "gonetsurf/parsers/css/values"
)

// Size is a computed width or height before containing-block resolution.
type Size struct {
	Value   float64
	Percent bool
	Auto    bool
	Keyword string
	// FitArgument is the optional argument of fit-content(<length-percentage>).
	FitArgument    float64
	FitPercent     bool
	FitHasArgument bool
}

func ParseOffset(text string) Length {
	if strings.TrimSpace(text) == "" {
		return Length{Auto: true}
	}
	return ParseLength(text)
}

func ParseAspectRatio(text string) (float64, bool) {
	text = strings.TrimSpace(text)
	if text == "" {
		return 0, false
	}
	for _, prefix := range []string{"auto ", "auto/"} {
		text = strings.TrimSpace(strings.TrimPrefix(text, prefix))
	}
	parts := strings.Split(text, "/")
	if len(parts) != 2 {
		return 0, false
	}
	width, ok := ParseRatioNumber(parts[0])
	if !ok {
		return 0, false
	}
	height, ok := ParseRatioNumber(parts[1])
	if !ok || width <= 0 || height <= 0 {
		return 0, false
	}
	return width / height, true
}

func ParseRatioNumber(text string) (float64, bool) {
	text = strings.TrimSpace(text)
	if text == "" {
		return 0, false
	}
	value := 0.0
	seenDigit := false
	for _, character := range text {
		if character == '.' {
			if !seenDigit {
				return 0, false
			}
			continue
		}
		if character < '0' || character > '9' {
			return 0, false
		}
		seenDigit = true
		value = value*10 + float64(character-'0')
	}
	if !seenDigit {
		return 0, false
	}
	// Re-parse with strconv semantics when a decimal point is present.
	if strings.Contains(text, ".") {
		parsed, err := strconv.ParseFloat(text, 64)
		if err != nil {
			return 0, false
		}
		return parsed, true
	}
	return value, true
}

func ParseSize(text string) Size {
	return ParseSizeWithBase(text, 0, 0, 0)
}

func ParseSizeWithBase(text string, base, viewportWidth, viewportHeight float64) Size {
	text = strings.TrimSpace(text)
	if text == "" || strings.EqualFold(text, "auto") || strings.EqualFold(text, "none") {
		return Size{Auto: true}
	}
	switch strings.ToLower(text) {
	case "min-content", "max-content", "fit-content", "stretch":
		return Size{Keyword: strings.ToLower(text)}
	}
	values := parser.ParseComponentValueList(text)
	if len(values) != 1 {
		return Size{Auto: true}
	}
	if function, ok := values[0].(*parser.Function); ok {
		if strings.EqualFold(function.Name, "fit-content") {
			argument, ok := parseFitContentArgument(function.Value)
			if !ok {
				return Size{Auto: true}
			}
			argument.Keyword = "fit-content"
			argument.FitHasArgument = true
			return argument
		}
		if expression, ok := cssvalues.ParseMath(function); ok {
			if value, ok := evaluateMathSize(expression, base, viewportWidth, viewportHeight); ok {
				return Size{Value: value}
			}
		}
	}
	numeric, ok := cssvalues.ParseNumeric(values[0])
	if !ok {
		return Size{Auto: true}
	}
	if numeric.Unit == "%" {
		return Size{Value: numeric.Number * 100, Percent: true}
	}
	if numeric.Kind == cssvalues.Length || (numeric.Kind == cssvalues.Other && numeric.Unit == "") {
		return Size{Value: numeric.Number}
	}
	return Size{Auto: true}
}

func evaluateMathSize(expression *cssvalues.MathExpression, base, viewportWidth, viewportHeight float64) (float64, bool) {
	if expression == nil {
		return 0, false
	}
	switch expression.Kind {
	case cssvalues.MathValue:
		if expression.Value.Unit == "%" {
			if base <= 0 && viewportWidth <= 0 {
				return 0, false
			}
			return expression.Value.Number * base, true
		}
		if expression.Value.Kind == cssvalues.Length {
			return cssvalues.ToPixels(expression.Value, viewportWidth, viewportHeight)
		}
		if expression.Value.Kind == cssvalues.Other && expression.Value.Unit == "" {
			return expression.Value.Number, true
		}
		return 0, false
	case cssvalues.MathSum:
		left, ok := evaluateMathSize(expression.Left, base, viewportWidth, viewportHeight)
		if !ok {
			return 0, false
		}
		right, ok := evaluateMathSize(expression.Right, base, viewportWidth, viewportHeight)
		if !ok {
			return 0, false
		}
		if expression.Subtract {
			return left - right, true
		}
		return left + right, true
	case cssvalues.MathProduct:
		left, ok := evaluateMathSize(expression.Left, base, viewportWidth, viewportHeight)
		if !ok {
			return 0, false
		}
		right, ok := evaluateMathSize(expression.Right, base, viewportWidth, viewportHeight)
		if !ok {
			return 0, false
		}
		if expression.Divide {
			if right == 0 {
				return 0, false
			}
			return left / right, true
		}
		return left * right, true
	case cssvalues.MathFunction:
		arguments := make([]float64, len(expression.Args))
		for index, argument := range expression.Args {
			value, ok := evaluateMathSize(argument, base, viewportWidth, viewportHeight)
			if !ok {
				return 0, false
			}
			arguments[index] = value
		}
		switch strings.ToLower(expression.Name) {
		case "min":
			if len(arguments) == 0 {
				return 0, false
			}
			result := arguments[0]
			for _, value := range arguments[1:] {
				if value < result {
					result = value
				}
			}
			return result, true
		case "max":
			if len(arguments) == 0 {
				return 0, false
			}
			result := arguments[0]
			for _, value := range arguments[1:] {
				if value > result {
					result = value
				}
			}
			return result, true
		case "clamp":
			if len(arguments) != 3 {
				return 0, false
			}
			result := arguments[2]
			if result < arguments[0] {
				result = arguments[0]
			}
			if result > arguments[1] {
				result = arguments[1]
			}
			return result, true
		}
	}
	return 0, false
}

func parseFitContentArgument(values []parser.ComponentValue) (Size, bool) {
	for _, value := range values {
		if parser.IsWhitespace(value) {
			continue
		}
		numeric, ok := cssvalues.ParseNumeric(value)
		if !ok {
			return Size{}, false
		}
		if numeric.Unit == "%" {
			return Size{Value: numeric.Number * 100, Percent: true}, true
		}
		if numeric.Kind == cssvalues.Length || (numeric.Kind == cssvalues.Other && numeric.Unit == "") {
			return Size{Value: numeric.Number}, true
		}
		return Size{}, false
	}
	return Size{}, false
}

func ParseComputedPixels(text string) (float64, bool) {
	values := parser.ParseComponentValueList(text)
	if len(values) != 1 {
		return 0, false
	}
	numeric, ok := cssvalues.ParseNumeric(values[0])
	if !ok || numeric.Kind != cssvalues.Length || !strings.EqualFold(numeric.Unit, "px") {
		return 0, false
	}
	return numeric.Number, true
}

func ParseComputedNumber(text string) (float64, bool) {
	values := parser.ParseComponentValueList(text)
	if len(values) != 1 {
		return 0, false
	}
	numeric, ok := cssvalues.ParseNumeric(values[0])
	if !ok || numeric.Kind != cssvalues.Other || numeric.Unit != "" {
		return 0, false
	}
	return numeric.Number, true
}
