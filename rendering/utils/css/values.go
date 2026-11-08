package css

import (
	"math"
	"strings"

	"gonetsurf/parsers/css/parser"
	"gonetsurf/parsers/css/tokenizer"
)

// LengthValue is a CSS length or percentage.
type LengthValue struct {
	Value   float64
	Percent bool
	Auto    bool
}

// Resolve converts the length against a reference size.
func (length LengthValue) Resolve(base float64) float64 {
	if length.Auto {
		return 0
	}
	if length.Percent {
		return base * length.Value / 100
	}
	return length.Value
}

// NonWhitespaceValues removes whitespace component values.
func NonWhitespaceValues(values []parser.ComponentValue) []parser.ComponentValue {
	result := make([]parser.ComponentValue, 0, len(values))
	for _, value := range values {
		if parser.IsWhitespace(value) {
			continue
		}
		result = append(result, value)
	}
	return result
}

// SplitCommaGroups splits component values on top-level commas.
func SplitCommaGroups(values []parser.ComponentValue) [][]parser.ComponentValue {
	var groups [][]parser.ComponentValue
	var current []parser.ComponentValue
	for _, value := range values {
		if token, ok := parser.AsToken(value); ok && token.Type == tokenizer.TokenComma {
			groups = append(groups, current)
			current = nil
			continue
		}
		current = append(current, value)
	}
	groups = append(groups, current)
	return groups
}

// ParseLengthValue parses a CSS length or percentage.
func ParseLengthValue(value parser.ComponentValue, fontSize float64) (LengthValue, bool) {
	token, ok := parser.AsToken(value)
	if !ok {
		return LengthValue{}, false
	}
	switch token.Type {
	case tokenizer.TokenPercentage:
		return LengthValue{Value: token.Number, Percent: true}, true
	case tokenizer.TokenNumber:
		return LengthValue{Value: token.Number}, true
	case tokenizer.TokenDimension:
		unit := strings.ToLower(token.Value)
		factor := 1.0
		switch unit {
		case "px":
			factor = 1
		case "pt":
			factor = 96.0 / 72.0
		case "pc":
			factor = 16
		case "in":
			factor = 96
		case "cm":
			factor = 96.0 / 2.54
		case "mm":
			factor = 96.0 / 25.4
		case "em":
			factor = fontSize
		case "rem":
			factor = 16
		case "ex", "ch":
			factor = fontSize / 2
		default:
			return LengthValue{}, false
		}
		return LengthValue{Value: token.Number * factor}, true
	}
	return LengthValue{}, false
}

// ParseCSSPixelValue parses a pixel-valued CSS length.
func ParseCSSPixelValue(text string) (float64, bool) {
	values := NonWhitespaceValues(parser.ParseComponentValueList(strings.TrimSpace(text)))
	if len(values) != 1 {
		return 0, false
	}
	length, ok := ParseLengthValue(values[0], 16)
	if !ok || length.Percent || length.Auto {
		return 0, false
	}
	return length.Value, true
}

// TransformLength parses a transform argument length.
func TransformLength(values []parser.ComponentValue, index int, base float64) (float64, bool) {
	if index >= len(values) {
		if index == 0 {
			return 0, false
		}
		return 0, true
	}
	length, ok := ParseLengthValue(values[index], 16)
	if !ok {
		return 0, false
	}
	return length.Resolve(base), true
}

// TransformNumber parses a transform argument number or percentage.
func TransformNumber(values []parser.ComponentValue, index int) (float64, bool) {
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

// ParseAngle parses a CSS angle in degrees.
func ParseAngle(value parser.ComponentValue) (float64, bool) {
	token, ok := parser.AsToken(value)
	if !ok {
		return 0, false
	}
	switch token.Type {
	case tokenizer.TokenNumber:
		return token.Number, true
	case tokenizer.TokenDimension:
		unit := strings.ToLower(token.Value)
		switch unit {
		case "deg":
			return token.Number, true
		case "grad":
			return token.Number * 0.9, true
		case "rad":
			return token.Number * 180 / math.Pi, true
		case "turn":
			return token.Number * 360, true
		}
	}
	return 0, false
}

// TransformAngle parses a transform argument angle into radians.
func TransformAngle(values []parser.ComponentValue, index int) (float64, bool) {
	if index >= len(values) {
		if index == 0 {
			return 0, false
		}
		return 0, true
	}
	angle, ok := ParseAngle(values[index])
	if !ok {
		return 0, false
	}
	return angle * math.Pi / 180, true
}
