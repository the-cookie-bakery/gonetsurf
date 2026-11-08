//go:build vulkan && cgo

package vulkan

import (
	"strconv"
	"strings"

	"gonetsurf/parsers/css/parser"
	"gonetsurf/parsers/css/tokenizer"
	cssvalues "gonetsurf/parsers/css/values"
)

type trackDescriptor struct {
	kind     int
	value    float64
	flex     float64
	minSize  float64
	maxSize  float64
	hasRange bool
}

func parseTrackList(text string) []trackDescriptor {
	values := parser.ParseComponentValueList(text)
	tracks := []trackDescriptor{}
	for _, value := range values {
		if parser.IsWhitespace(value) {
			continue
		}
		function, ok := value.(*parser.Function)
		if ok && strings.EqualFold(function.Name, "minmax") {
			if track, ok := parseMinMaxTrack(function.Value); ok {
				tracks = append(tracks, track)
			}
			continue
		}
		if ok && strings.EqualFold(function.Name, "repeat") {
			groups := splitCommaValues(function.Value)
			if len(groups) != 2 {
				continue
			}
			countToken, ok := parser.AsToken(groups[0][0])
			if !ok || countToken.Type != tokenizer.TokenNumber {
				continue
			}
			count := int(countToken.Number)
			inner := parseTrackList(parser.SerializeValues(groups[1]))
			for index := 0; index < count; index++ {
				tracks = append(tracks, inner...)
			}
			continue
		}
		if track, ok := parseTrackValue(value); ok {
			tracks = append(tracks, track)
		}
	}
	return tracks
}

func parseTrackValue(value parser.ComponentValue) (trackDescriptor, bool) {
	token, ok := parser.AsToken(value)
	if !ok {
		return trackDescriptor{}, false
	}
	if token.Type == tokenizer.TokenIdent {
		switch strings.ToLower(token.Value) {
		case "auto", "min-content", "max-content", "fit-content":
			return trackDescriptor{kind: 3}, true
		}
	}
	numeric, ok := cssvalues.ParseNumeric(value)
	if !ok {
		return trackDescriptor{}, false
	}
	switch numeric.Kind {
	case cssvalues.Flex:
		return trackDescriptor{kind: 2, flex: numeric.Number}, true
	case cssvalues.Length:
		return trackDescriptor{kind: 0, value: numeric.Number}, true
	case cssvalues.Other:
		if numeric.Unit == "%" {
			return trackDescriptor{kind: 1, value: numeric.Number * 100}, true
		}
		return trackDescriptor{kind: 0, value: numeric.Number}, true
	}
	return trackDescriptor{}, false
}

func parseMinMaxTrack(values []parser.ComponentValue) (trackDescriptor, bool) {
	groups := splitCommaValues(values)
	if len(groups) != 2 {
		return trackDescriptor{}, false
	}
	minimum, ok := parseTrackArgument(groups[0])
	if !ok {
		return trackDescriptor{}, false
	}
	maximum, ok := parseTrackArgument(groups[1])
	if !ok {
		return trackDescriptor{}, false
	}
	track := trackDescriptor{kind: 0, minSize: minimum.value, hasRange: true}
	if minimum.kind == 3 {
		track.kind = 3
	}
	switch {
	case maximum.kind == 2:
		track.kind = 2
		track.flex = maximum.flex
	case maximum.kind == 3:
		track.kind = 3
	case maximum.kind == 1:
		track.kind = 1
		track.value = maximum.value
		track.maxSize = maximum.value
	default:
		track.kind = 0
		track.value = maximum.value
		track.maxSize = maximum.value
	}
	return track, true
}

func parseTrackArgument(values []parser.ComponentValue) (trackDescriptor, bool) {
	for _, value := range values {
		if parser.IsWhitespace(value) {
			continue
		}
		return parseTrackValue(value)
	}
	return trackDescriptor{}, false
}

func splitCommaValues(values []parser.ComponentValue) [][]parser.ComponentValue {
	groups := [][]parser.ComponentValue{}
	current := []parser.ComponentValue{}
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

func parseGridLine(text string) (start int, span int, explicit bool) {
	values := parser.ParseComponentValueList(strings.TrimSpace(text))
	if len(values) == 0 {
		return 0, 1, false
	}
	token, ok := parser.AsToken(values[0])
	if !ok {
		return 0, 1, false
	}
	if token.Type == tokenizer.TokenIdent && strings.EqualFold(token.Value, "auto") {
		return 0, 1, false
	}
	if token.Type == tokenizer.TokenIdent && strings.EqualFold(token.Value, "span") {
		for _, value := range values[1:] {
			if parser.IsWhitespace(value) {
				continue
			}
			if next, ok := parser.AsToken(value); ok && next.Type == tokenizer.TokenNumber {
				return 0, int(next.Number), true
			}
			break
		}
		return 0, 1, false
	}
	if token.Type == tokenizer.TokenNumber && token.NumericKind == tokenizer.Integer {
		return int(token.Number), 1, true
	}
	return 0, 1, false
}

func parseGridSpan(startText, endText string) (start, span int, explicit bool) {
	startValue, startSpan, startExplicit := parseGridLine(startText)
	endValue, endSpan, endExplicit := parseGridLine(endText)
	explicit = startExplicit || endExplicit
	if startValue > 0 && endValue > 0 && endValue > startValue {
		return startValue, endValue - startValue, explicit
	}
	if startValue > 0 && endSpan > 0 {
		return startValue, endSpan, explicit
	}
	if startSpan > 0 {
		return 0, startSpan, explicit
	}
	if endSpan > 0 {
		return 0, endSpan, explicit
	}
	if startValue > 0 {
		return startValue, 1, explicit
	}
	return 0, 1, explicit
}

func parseOrder(text string) int {
	value, err := strconv.Atoi(strings.TrimSpace(text))
	if err != nil {
		return 0
	}
	return value
}

func gridAlignmentValue(text string) float64 {
	switch strings.ToLower(strings.TrimSpace(text)) {
	case "center":
		return 1
	case "end", "flex-end", "right", "last baseline":
		return 2
	case "stretch":
		return 3
	}
	return 0
}
