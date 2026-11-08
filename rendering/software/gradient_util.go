package software

import "math"

type resolvedGradientStop struct {
	color    Color
	position float64
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
	return Color{Red: toByte(red), Green: toByte(green), Blue: toByte(blue), Alpha: toByte(alpha)}
}
