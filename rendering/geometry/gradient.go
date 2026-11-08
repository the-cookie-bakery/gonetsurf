package geometry

import "math"

// GradientKind selects the gradient geometry.
type GradientKind uint8

const (
	// LinearGradient interpolates along a line.
	LinearGradient GradientKind = iota
	// RadialGradient interpolates from a center point.
	RadialGradient
	// ConicGradient interpolates around a center point.
	ConicGradient
)

// GradientStop is a resolved color stop.
type GradientStop struct {
	Position float64
	Color    Color
}

// Gradient describes a resolved gradient in surface coordinates.
type Gradient struct {
	Kind      GradientKind
	Repeating bool
	Stops     []GradientStop

	CenterX    float64
	CenterY    float64
	DirectionX float64
	DirectionY float64
	LineLength float64
	RadiusX    float64
	RadiusY    float64
	FromAngle  float64
}

// ColorAt samples the gradient at a surface position.
func (gradient Gradient) ColorAt(xPos, yPos float64) Color {
	position := 0.0
	switch gradient.Kind {
	case LinearGradient:
		if gradient.LineLength <= 0 {
			return Transparent
		}
		offset := (xPos-gradient.CenterX)*gradient.DirectionX + (yPos-gradient.CenterY)*gradient.DirectionY
		position = 0.5 + offset/gradient.LineLength
	case RadialGradient:
		if gradient.RadiusX <= 0 || gradient.RadiusY <= 0 {
			return Transparent
		}
		normalizedX := (xPos - gradient.CenterX) / gradient.RadiusX
		normalizedY := (yPos - gradient.CenterY) / gradient.RadiusY
		position = math.Hypot(normalizedX, normalizedY)
	case ConicGradient:
		angle := math.Atan2(xPos-gradient.CenterX, -(yPos - gradient.CenterY))
		if angle < 0 {
			angle += 2 * math.Pi
		}
		position = (angle - gradient.FromAngle) / (2 * math.Pi)
		position = math.Mod(position, 1)
		if position < 0 {
			position += 1
		}
	}
	return SampleGradient(gradient.Stops, position, gradient.Repeating)
}

// SampleGradient samples color stops at a normalized position.
func SampleGradient(stops []GradientStop, position float64, repeating bool) Color {
	if len(stops) == 0 {
		return Transparent
	}
	if len(stops) == 1 {
		return stops[0].Color
	}
	first := stops[0].Position
	last := stops[len(stops)-1].Position
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
			return stops[0].Color
		}
		if position >= last {
			return stops[len(stops)-1].Color
		}
	}
	for index := 0; index+1 < len(stops); index++ {
		start := stops[index]
		end := stops[index+1]
		if position < start.Position || position > end.Position {
			continue
		}
		span := end.Position - start.Position
		if span <= 0 {
			return end.Color
		}
		return InterpolateColor(start.Color, end.Color, (position-start.Position)/span)
	}
	if position < first {
		return stops[0].Color
	}
	return stops[len(stops)-1].Color
}

// InterpolateColor blends two colors in premultiplied space.
func InterpolateColor(first, second Color, fraction float64) Color {
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
	return Color{Red: ToByte(red), Green: ToByte(green), Blue: ToByte(blue), Alpha: ToByte(alpha)}
}
