package rendering

import (
	"gonetsurf/rendering/geometry"
	"gonetsurf/rendering/interfaces"
	"math"
	"strings"

	"gonetsurf/parsers/css/parser"
	"gonetsurf/parsers/css/tokenizer"
)

type gradientKind uint8

const (
	linearGradient gradientKind = iota
	radialGradient
	conicGradient
)

type gradientShape uint8

const (
	ellipseShape gradientShape = iota
	circleShape
)

type gradientExtent uint8

const (
	farthestCornerExtent gradientExtent = iota
	closestSideExtent
	closestCornerExtent
	farthestSideExtent
)

type positionKind uint8

const (
	percentPosition positionKind = iota
	lengthPosition
	anglePosition
)

type gradientPosition struct {
	value float64
	kind  positionKind
	has   bool
}

type gradientStopSpec struct {
	color    Color
	position gradientPosition
}

type gradientSpec struct {
	kind      gradientKind
	repeating bool
	angle     float64
	corner    string
	shape     gradientShape
	extent    gradientExtent
	sizeX     lengthValue
	sizeY     lengthValue
	hasSize   bool
	positionX lengthValue
	positionY lengthValue
	fromAngle float64
	stops     []gradientStopSpec
}

type resolvedGradientStop struct {
	color    Color
	position float64
}

func parseGradientFunction(function *parser.Function, currentColor Color) (*gradientSpec, bool) {
	name := strings.ToLower(function.Name)
	spec := &gradientSpec{
		shape:     ellipseShape,
		extent:    farthestCornerExtent,
		sizeX:     lengthValue{auto: true},
		sizeY:     lengthValue{auto: true},
		positionX: lengthValue{value: 50, percent: true},
		positionY: lengthValue{value: 50, percent: true},
	}
	switch name {
	case "linear-gradient":
		spec.kind = linearGradient
		spec.angle = 180
	case "repeating-linear-gradient":
		spec.kind = linearGradient
		spec.repeating = true
		spec.angle = 180
	case "radial-gradient":
		spec.kind = radialGradient
	case "repeating-radial-gradient":
		spec.kind = radialGradient
		spec.repeating = true
	case "conic-gradient":
		spec.kind = conicGradient
	case "repeating-conic-gradient":
		spec.kind = conicGradient
		spec.repeating = true
	default:
		return nil, false
	}

	groups := nonEmptyGroups(splitCommaGroups(function.Value))
	if len(groups) == 0 {
		return nil, false
	}
	index := 0
	switch spec.kind {
	case linearGradient:
		if angle, corner, ok := parseLinearDirection(groups[0]); ok {
			spec.angle = angle
			spec.corner = corner
			index = 1
		}
	case radialGradient:
		if parseRadialConfig(spec, groups[0]) {
			index = 1
		}
	case conicGradient:
		if parseConicConfig(spec, groups[0]) {
			index = 1
		}
	}
	if index >= len(groups) {
		return nil, false
	}
	for _, group := range groups[index:] {
		color, ok := parseGradientColor(group, currentColor)
		if !ok {
			return nil, false
		}
		var positions []gradientPosition
		for _, value := range group {
			if parser.IsWhitespace(value) {
				continue
			}
			if isColorComponent(value, currentColor) {
				continue
			}
			position, ok := parseGradientPosition(value)
			if ok {
				positions = append(positions, position)
			}
		}
		if len(positions) == 0 {
			spec.stops = append(spec.stops, gradientStopSpec{color: color})
			continue
		}
		for _, position := range positions {
			spec.stops = append(spec.stops, gradientStopSpec{color: color, position: position})
		}
	}
	if len(spec.stops) == 0 {
		return nil, false
	}
	return spec, true
}

func nonEmptyGroups(groups [][]parser.ComponentValue) [][]parser.ComponentValue {
	result := make([][]parser.ComponentValue, 0, len(groups))
	for _, group := range groups {
		if len(nonWhitespaceValues(group)) > 0 {
			result = append(result, group)
		}
	}
	return result
}

func parseLinearDirection(values []parser.ComponentValue) (float64, string, bool) {
	tokens := nonWhitespaceValues(values)
	if len(tokens) == 0 {
		return 0, "", false
	}
	if token, ok := parser.AsToken(tokens[0]); ok && token.Type == tokenizer.TokenIdent && strings.EqualFold(token.Value, "to") {
		var sides []string
		for _, value := range tokens[1:] {
			sideToken, ok := parser.AsToken(value)
			if !ok || sideToken.Type != tokenizer.TokenIdent {
				return 0, "", false
			}
			sides = append(sides, strings.ToLower(sideToken.Value))
		}
		if len(sides) == 0 || len(sides) > 2 {
			return 0, "", false
		}
		switch strings.Join(sides, " ") {
		case "top":
			return 0, "", true
		case "right":
			return 90, "", true
		case "bottom":
			return 180, "", true
		case "left":
			return 270, "", true
		case "top right", "right top":
			return 0, "top right", true
		case "bottom right", "right bottom":
			return 0, "bottom right", true
		case "bottom left", "left bottom":
			return 0, "bottom left", true
		case "top left", "left top":
			return 0, "top left", true
		}
		return 0, "", false
	}
	if angle, ok := parseAngleValue(tokens[0]); ok {
		return angle, "", true
	}
	return 0, "", false
}

func parseRadialConfig(spec *gradientSpec, values []parser.ComponentValue) bool {
	tokens := nonWhitespaceValues(values)
	if len(tokens) == 0 {
		return false
	}
	recognized := false
	var sizeLengths []lengthValue
	index := 0
	for index < len(tokens) {
		token, ok := parser.AsToken(tokens[index])
		if !ok {
			break
		}
		if token.Type == tokenizer.TokenIdent {
			lower := strings.ToLower(token.Value)
			switch lower {
			case "circle":
				spec.shape = circleShape
				recognized = true
				index++
				continue
			case "ellipse":
				spec.shape = ellipseShape
				recognized = true
				index++
				continue
			case "closest-side":
				spec.extent = closestSideExtent
				recognized = true
				index++
				continue
			case "closest-corner":
				spec.extent = closestCornerExtent
				recognized = true
				index++
				continue
			case "farthest-side":
				spec.extent = farthestSideExtent
				recognized = true
				index++
				continue
			case "farthest-corner":
				spec.extent = farthestCornerExtent
				recognized = true
				index++
				continue
			case "at":
				positionTokens := tokens[index+1:]
				if len(positionTokens) > 0 {
					spec.positionX, spec.positionY = parseBackgroundPosition(positionTokens, 16)
					recognized = true
				}
				return recognized
			}
		}
		if length, ok := parseLengthValue(tokens[index], 16); ok {
			sizeLengths = append(sizeLengths, length)
			recognized = true
			index++
			continue
		}
		return recognized
	}
	if len(sizeLengths) > 0 {
		spec.hasSize = true
		spec.sizeX = sizeLengths[0]
		spec.sizeY = sizeLengths[0]
		if len(sizeLengths) > 1 {
			spec.sizeY = sizeLengths[1]
		}
		if spec.shape == ellipseShape && len(sizeLengths) == 1 {
			spec.sizeY = lengthValue{auto: true}
		}
	}
	return recognized
}

func parseConicConfig(spec *gradientSpec, values []parser.ComponentValue) bool {
	tokens := nonWhitespaceValues(values)
	if len(tokens) == 0 {
		return false
	}
	recognized := false
	index := 0
	for index < len(tokens) {
		token, ok := parser.AsToken(tokens[index])
		if !ok || token.Type != tokenizer.TokenIdent {
			return recognized
		}
		switch strings.ToLower(token.Value) {
		case "from":
			if index+1 >= len(tokens) {
				return recognized
			}
			angle, ok := parseAngleValue(tokens[index+1])
			if !ok {
				return recognized
			}
			spec.fromAngle = angle
			recognized = true
			index += 2
		case "at":
			if index+1 >= len(tokens) {
				return recognized
			}
			spec.positionX, spec.positionY = parseBackgroundPosition(tokens[index+1:], 16)
			recognized = true
			return recognized
		default:
			return recognized
		}
	}
	return recognized
}

func parseAngleValue(value parser.ComponentValue) (float64, bool) {
	token, ok := parser.AsToken(value)
	if !ok {
		return 0, false
	}
	switch token.Type {
	case tokenizer.TokenNumber:
		return token.Number, true
	case tokenizer.TokenDimension:
		switch strings.ToLower(token.Value) {
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

func parseGradientColor(values []parser.ComponentValue, currentColor Color) (Color, bool) {
	for _, value := range values {
		if parser.IsWhitespace(value) {
			continue
		}
		if token, ok := parser.AsToken(value); ok && token.Type == tokenizer.TokenIdent && strings.EqualFold(token.Value, "currentcolor") {
			return currentColor, true
		}
		if parsed, ok := ParseColor(parser.SerializeValues([]parser.ComponentValue{value})); ok {
			return parsed, true
		}
	}
	return Transparent, false
}

func isColorComponent(value parser.ComponentValue, currentColor Color) bool {
	if parser.IsWhitespace(value) {
		return false
	}
	if token, ok := parser.AsToken(value); ok && token.Type == tokenizer.TokenIdent && strings.EqualFold(token.Value, "currentcolor") {
		return true
	}
	_, ok := ParseColor(parser.SerializeValues([]parser.ComponentValue{value}))
	return ok
}

func parseGradientPosition(value parser.ComponentValue) (gradientPosition, bool) {
	token, ok := parser.AsToken(value)
	if !ok {
		return gradientPosition{}, false
	}
	switch token.Type {
	case tokenizer.TokenPercentage:
		return gradientPosition{value: token.Number / 100, kind: percentPosition, has: true}, true
	case tokenizer.TokenNumber:
		return gradientPosition{value: token.Number, kind: lengthPosition, has: true}, true
	case tokenizer.TokenDimension:
		unit := strings.ToLower(token.Value)
		switch unit {
		case "deg", "grad", "rad", "turn":
			angle, ok := parseAngleValue(value)
			if !ok {
				return gradientPosition{}, false
			}
			return gradientPosition{value: angle / 360, kind: anglePosition, has: true}, true
		default:
			length, ok := parseLengthValue(value, 16)
			if !ok {
				return gradientPosition{}, false
			}
			return gradientPosition{value: length.value, kind: lengthPosition, has: true}, true
		}
	}
	return gradientPosition{}, false
}

func resolveGradientStops(stops []gradientStopSpec, reference float64) []resolvedGradientStop {
	resolved := make([]resolvedGradientStop, len(stops))
	for index, stop := range stops {
		resolved[index] = resolvedGradientStop{color: stop.color}
		if !stop.position.has {
			continue
		}
		position := stop.position.value
		switch stop.position.kind {
		case lengthPosition:
			if reference > 0 {
				position /= reference
			}
		case anglePosition:
			// Already normalized to turns.
		}
		resolved[index].position = position
	}
	if len(resolved) == 0 {
		return nil
	}
	if !stops[0].position.has {
		resolved[0].position = 0
	}
	if !stops[len(stops)-1].position.has {
		resolved[len(resolved)-1].position = 1
	}
	index := 0
	for index < len(resolved) {
		if stops[index].position.has {
			index++
			continue
		}
		start := index - 1
		end := index
		for end < len(resolved) && !stops[end].position.has {
			end++
		}
		if start < 0 || end >= len(resolved) {
			break
		}
		startPosition := resolved[start].position
		endPosition := resolved[end].position
		count := end - start
		for offset := 1; offset < count; offset++ {
			resolved[start+offset].position = startPosition + (endPosition-startPosition)*float64(offset)/float64(count)
		}
		index = end
	}
	for index := 1; index < len(resolved); index++ {
		if resolved[index].position < resolved[index-1].position {
			resolved[index].position = resolved[index-1].position
		}
	}
	return resolved
}

func gradientColorAt(stops []resolvedGradientStop, position float64, repeating bool) Color {
	if len(stops) == 0 {
		return Transparent
	}
	if len(stops) == 1 {
		return stops[0].color
	}
	first := stops[0].position
	last := stops[len(stops)-1].position
	if repeating {
		period := last - first
		if period <= 0 {
			period = 1
		}
		wrapped := math.Mod(position-first, period)
		if wrapped < 0 {
			wrapped += period
		}
		position = first + wrapped
	} else {
		if position <= first {
			return stops[0].color
		}
		if position >= last {
			return stops[len(stops)-1].color
		}
	}
	for index := 0; index+1 < len(stops); index++ {
		start := stops[index]
		end := stops[index+1]
		if position < start.position || position > end.position {
			continue
		}
		span := end.position - start.position
		if span <= 0 {
			return end.color
		}
		return interpolateColor(start.color, end.color, (position-start.position)/span)
	}
	if position < first {
		return stops[0].color
	}
	return stops[len(stops)-1].color
}

func interpolateColor(first, second Color, fraction float64) Color {
	if fraction <= 0 {
		return first
	}
	if fraction >= 1 {
		return second
	}
	firstAlpha := float64(first.Alpha) / 255
	secondAlpha := float64(second.Alpha) / 255
	alpha := firstAlpha*(1-fraction) + secondAlpha*fraction
	if alpha <= 0 {
		return Transparent
	}
	red := (float64(first.Red)/255*firstAlpha*(1-fraction) + float64(second.Red)/255*secondAlpha*fraction) / alpha
	green := (float64(first.Green)/255*firstAlpha*(1-fraction) + float64(second.Green)/255*secondAlpha*fraction) / alpha
	blue := (float64(first.Blue)/255*firstAlpha*(1-fraction) + float64(second.Blue)/255*secondAlpha*fraction) / alpha
	return Color{Red: geometry.ToByte(red), Green: geometry.ToByte(green), Blue: geometry.ToByte(blue), Alpha: geometry.ToByte(alpha)}
}

func (painter *Painter) paintGradient(spec *gradientSpec, surface interfaces.Canvas, area, clip Rect, mask *Mask) {
	if spec == nil || area.Width <= 0 || area.Height <= 0 || clip.Width <= 0 || clip.Height <= 0 {
		return
	}
	var gradient geometry.Gradient
	gradient.Repeating = spec.repeating
	gradient.Kind = geometry.LinearGradient
	switch spec.kind {
	case linearGradient:
		angle := spec.angle * math.Pi / 180
		if spec.corner != "" {
			cornerAngle := math.Atan2(area.Width, area.Height)
			switch spec.corner {
			case "bottom right":
				cornerAngle = math.Pi - cornerAngle
			case "bottom left":
				cornerAngle = math.Pi + cornerAngle
			case "top left":
				cornerAngle = 2*math.Pi - cornerAngle
			}
			angle = cornerAngle
		}
		directionX := math.Sin(angle)
		directionY := -math.Cos(angle)
		lineLength := math.Abs(area.Width*directionX) + math.Abs(area.Height*directionY)
		if lineLength <= 0 {
			return
		}
		gradient.Kind = geometry.LinearGradient
		gradient.CenterX = area.X + area.Width/2
		gradient.CenterY = area.Y + area.Height/2
		gradient.DirectionX = directionX
		gradient.DirectionY = directionY
		gradient.LineLength = lineLength
		gradient.Stops = toGeometryStops(resolveGradientStops(spec.stops, lineLength))
	case radialGradient:
		centerX := area.X + spec.positionX.resolve(area.Width)
		centerY := area.Y + spec.positionY.resolve(area.Height)
		radiusX, radiusY := radialRadii(spec, area, centerX, centerY)
		if radiusX <= 0 || radiusY <= 0 {
			return
		}
		reference := math.Max(radiusX, radiusY)
		gradient.Kind = geometry.RadialGradient
		gradient.CenterX = centerX
		gradient.CenterY = centerY
		gradient.RadiusX = radiusX
		gradient.RadiusY = radiusY
		gradient.Stops = toGeometryStops(resolveGradientStops(spec.stops, reference))
	case conicGradient:
		centerX := area.X + spec.positionX.resolve(area.Width)
		centerY := area.Y + spec.positionY.resolve(area.Height)
		gradient.Kind = geometry.ConicGradient
		gradient.CenterX = centerX
		gradient.CenterY = centerY
		gradient.FromAngle = spec.fromAngle * math.Pi / 180
		gradient.Stops = toGeometryStops(resolveGradientStops(spec.stops, 1))
	default:
		return
	}
	surface.FillGradient(gradient, area, clip, mask)
}

func toGeometryStops(stops []resolvedGradientStop) []geometry.GradientStop {
	result := make([]geometry.GradientStop, len(stops))
	for index, stop := range stops {
		result[index] = geometry.GradientStop{Position: stop.position, Color: stop.color}
	}
	return result
}

func radialRadii(spec *gradientSpec, area Rect, centerX, centerY float64) (float64, float64) {
	if spec.hasSize {
		radiusX := spec.sizeX.resolve(area.Width)
		radiusY := spec.sizeY.resolve(area.Height)
		if spec.shape == circleShape {
			return radiusX, radiusX
		}
		if spec.sizeY.auto {
			radiusY = radiusX
		}
		if spec.sizeX.auto {
			radiusX = radiusY
		}
		return radiusX, radiusY
	}
	left := centerX - area.X
	right := area.X + area.Width - centerX
	top := centerY - area.Y
	bottom := area.Y + area.Height - centerY
	var radiusX, radiusY float64
	switch spec.extent {
	case closestSideExtent:
		radiusX = math.Min(left, right)
		radiusY = math.Min(top, bottom)
	case farthestSideExtent:
		radiusX = math.Max(left, right)
		radiusY = math.Max(top, bottom)
	case closestCornerExtent:
		radiusX, radiusY = cornerRadii(left, right, top, bottom, false)
	case farthestCornerExtent:
		radiusX, radiusY = cornerRadii(left, right, top, bottom, true)
	}
	if spec.shape == circleShape {
		radius := math.Hypot(radiusX, radiusY)
		if spec.extent == closestSideExtent || spec.extent == farthestSideExtent {
			radius = math.Max(radiusX, radiusY)
			if spec.extent == closestSideExtent {
				radius = math.Min(radiusX, radiusY)
			}
		}
		return radius, radius
	}
	return radiusX, radiusY
}

func cornerRadii(left, right, top, bottom float64, farthest bool) (float64, float64) {
	corners := [4][2]float64{{left, top}, {right, top}, {right, bottom}, {left, bottom}}
	best := corners[0]
	bestDistance := math.Hypot(best[0], best[1])
	for _, corner := range corners[1:] {
		distance := math.Hypot(corner[0], corner[1])
		if (farthest && distance > bestDistance) || (!farthest && distance < bestDistance) {
			best = corner
			bestDistance = distance
		}
	}
	return math.Abs(best[0]), math.Abs(best[1])
}
