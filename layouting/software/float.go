package software

import (
	"gonetsurf/layouting/model"
	"math"
	"strings"
)

func floatOf(box *model.Box) string {
	if box == nil || box.Style == nil {
		return ""
	}
	value := strings.ToLower(strings.TrimSpace(box.Style.GetString("float")))
	switch value {
	case "left", "right", "inline-start", "inline-end":
		return value
	default:
		return ""
	}
}

func isFloat(box *model.Box) bool {
	return floatOf(box) != ""
}

func (layout *Layout) layoutFloat(box *model.Box, containingX, containingY, containingWidth float64) float64 {
	marginLeft := box.Margin.Left.Resolve(containingWidth)
	marginRight := box.Margin.Right.Resolve(containingWidth)
	marginTop := box.Margin.Top.Resolve(containingWidth)
	marginBottom := box.Margin.Bottom.Resolve(containingWidth)
	paddingLeft := box.Padding.Left.Resolve(containingWidth)
	paddingRight := box.Padding.Right.Resolve(containingWidth)
	paddingTop := box.Padding.Top.Resolve(containingWidth)
	paddingBottom := box.Padding.Bottom.Resolve(containingWidth)
	borderLeft := box.Border.Left.Resolve(containingWidth)
	borderRight := box.Border.Right.Resolve(containingWidth)
	borderTop := box.Border.Top.Resolve(containingWidth)
	borderBottom := box.Border.Bottom.Resolve(containingWidth)
	horizontalExtra := paddingLeft + paddingRight + borderLeft + borderRight
	verticalExtra := paddingTop + paddingBottom + borderTop + borderBottom
	widthSize := model.ParseSize(box.Style.GetString("width"))
	if widthSize.Auto || widthSize.Keyword != "" {
		layout.computeIntrinsicWidths(box)
		available := containingWidth - marginLeft - marginRight - horizontalExtra
		box.ContentWidth = math.Max(box.IntrinsicMinWidth, math.Min(available, box.IntrinsicMaxWidth))
	} else {
		box.ContentWidth = layout.resolveWidth(box, containingWidth, marginLeft+marginRight, horizontalExtra)
	}
	box.Width = box.ContentWidth + horizontalExtra
	side := floatOf(box)
	if side == "right" || side == "inline-end" {
		box.X = containingX + containingWidth - marginRight - box.Width
	} else {
		box.X = containingX + marginLeft
	}
	box.Y = containingY + marginTop
	box.ContentX = box.X + borderLeft + paddingLeft
	box.ContentY = box.Y + borderTop + paddingTop
	layout.layoutFlexItemContent(box)
	box.ContentHeight = layout.resolveContentHeight(box, box.ContentHeight, 0, verticalExtra)
	box.Height = box.ContentHeight + verticalExtra
	return box.Y + box.Height + marginBottom
}

// positionFloat moves a laid-out float around earlier floats in the same
// formatting context. Existing floats are examined in document order.
func positionFloat(box *model.Box, existing []*model.Box, containingX, containingWidth float64) {
	if box == nil || len(existing) == 0 {
		return
	}
	baseX := box.X
	baseY := box.Y
	targetX := baseX
	targetY := baseY
	for attempts := 0; attempts <= len(existing)+1; attempts++ {
		x := targetX
		for _, other := range existing {
			if !verticalOverlap(targetY, targetY+box.Height, other.Y, other.Y+other.Height) {
				continue
			}
			horizontalOverlap := x < other.X+other.Width && x+box.Width > other.X
			if !horizontalOverlap {
				continue
			}
			if floatOf(box) == "right" || floatOf(box) == "inline-end" {
				candidate := other.X - box.Width
				if candidate < x {
					x = candidate
				}
			} else {
				candidate := other.X + other.Width
				if candidate > x {
					x = candidate
				}
			}
		}
		if x >= containingX && x+box.Width <= containingX+containingWidth {
			targetX = x
			break
		}
		lowest := targetY
		for _, other := range existing {
			if !verticalOverlap(targetY, targetY+box.Height, other.Y, other.Y+other.Height) {
				continue
			}
			if bottom := other.Y + other.Height; bottom > lowest {
				lowest = bottom
			}
		}
		if lowest <= targetY {
			break
		}
		targetY = lowest
		targetX = baseX
	}
	if targetX != box.X || targetY != box.Y {
		shiftBox(box, targetX-box.X, targetY-box.Y)
	}
}

func verticalOverlap(firstTop, firstBottom, secondTop, secondBottom float64) bool {
	return firstTop < secondBottom && secondTop < firstBottom
}

func floatOuterBottom(box *model.Box) float64 {
	if box == nil {
		return 0
	}
	return box.Y + box.Height + box.Margin.Bottom.Resolve(box.Width)
}

func clearOf(box *model.Box) string {
	if box == nil || box.Style == nil {
		return ""
	}
	value := strings.ToLower(strings.TrimSpace(box.Style.GetString("clear")))
	switch value {
	case "left", "right", "both", "inline-start", "inline-end":
		return value
	default:
		return ""
	}
}

func clearY(floats []*model.Box, clear string) float64 {
	required := 0.0
	for _, box := range floats {
		side := floatOf(box)
		switch {
		case clear == "both":
		case clear == "left" && side != "left" && side != "inline-start":
			continue
		case clear == "right" && side != "right" && side != "inline-end":
			continue
		case clear == "inline-start" && side != "left" && side != "inline-start":
			continue
		case clear == "inline-end" && side != "right" && side != "inline-end":
			continue
		}
		if bottom := floatOuterBottom(box); bottom > required {
			required = bottom
		}
	}
	return required
}
