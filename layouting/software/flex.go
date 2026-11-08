package software

import (
	"gonetsurf/layouting/model"
	"math"
	"sort"
	"strings"

	"gonetsurf/parsers/css/parser"
	"gonetsurf/parsers/css/tokenizer"
)

// flexItem holds the resolved flex values for one child.
type flexItem struct {
	box      *model.Box
	order    int
	grow     float64
	shrink   float64
	basis    model.Size
	baseSize float64
	mainSize float64
	margin   float64
}

// isFlexContainer reports whether display establishes a flex formatting
// context.
func isFlexContainer(box *model.Box) bool {
	display := displayOf(box)
	return display == "flex" || display == "inline-flex"
}

func (layout *Layout) layoutFlex(box *model.Box) {
	direction := strings.ToLower(strings.TrimSpace(box.Style.GetString("flex-direction")))
	if direction == "" {
		direction = "row"
	}
	row := direction == "row" || direction == "row-reverse"
	reverse := strings.HasSuffix(direction, "reverse")
	items := []flexItem{}
	for _, child := range box.Children {
		if isOutOfFlow(child) {
			layout.positioned = append(layout.positioned, child)
			continue
		}
		item := flexItem{
			box:    child,
			order:  parseFlexInteger(child.Style.GetString("order")),
			grow:   parseFlexNumber(child.Style.GetString("flex-grow"), 0),
			shrink: parseFlexNumber(child.Style.GetString("flex-shrink"), 1),
			basis:  model.ParseSizeWithBase(child.Style.GetString("flex-basis"), 0, layout.ViewportWidth, layout.ViewportHeight),
		}
		item.margin = childMarginMain(child, row, box.ContentWidth)
		items = append(items, item)
	}
	sort.SliceStable(items, func(left, right int) bool { return items[left].order < items[right].order })
	if reverse {
		for left, right := 0, len(items)-1; left < right; left, right = left+1, right-1 {
			items[left], items[right] = items[right], items[left]
		}
	}
	if len(items) == 0 {
		box.ContentHeight = 0
		return
	}
	heightSpecified := !model.ParseSize(box.Style.GetString("height")).Auto
	mainGap := layout.flexGap(box, row)
	mainSize := box.ContentWidth
	if !row {
		mainSize = box.ContentHeight
	}
	for index := range items {
		items[index].baseSize = layout.flexBaseSize(&items[index], row, mainSize)
	}
	if !row && mainSize <= 0 {
		totalBase := 0.0
		for _, item := range items {
			totalBase += item.baseSize + item.margin
		}
		totalBase += mainGap * float64(len(items)-1)
		mainSize = totalBase
		box.ContentHeight = mainSize
	}
	wrap := strings.HasPrefix(strings.ToLower(strings.TrimSpace(box.Style.GetString("flex-wrap"))), "wrap")
	if row && wrap {
		lines := groupFlexLines(items, mainSize, mainGap)
		lineY := box.ContentY
		for _, line := range lines {
			resolveFlexLineSizes(line, mainSize, mainGap)
			used := mainGap * float64(len(line)-1)
			for _, item := range line {
				used += item.mainSize + item.margin
			}
			offset, spacing := justifyOffsets(strings.ToLower(strings.TrimSpace(box.Style.GetString("justify-content"))), mainSize-used, len(line))
			lineHeight := layout.layoutFlexRowLine(box, line, offset, spacing, mainGap, lineY, 0)
			lineY += lineHeight
		}
		contentHeight := lineY - box.ContentY
		if heightSpecified {
			contentHeight = math.Max(contentHeight, box.ContentHeight)
		}
		box.ContentHeight = contentHeight
		return
	}
	if !row && wrap {
		lines := groupFlexLines(items, mainSize, mainGap)
		lineX := box.ContentX
		maxLineHeight := 0.0
		for _, line := range lines {
			resolveFlexLineSizes(line, mainSize, mainGap)
			used := mainGap * float64(len(line)-1)
			for _, item := range line {
				used += item.mainSize + item.margin
			}
			offset, spacing := justifyOffsets(strings.ToLower(strings.TrimSpace(box.Style.GetString("justify-content"))), mainSize-used, len(line))
			lineWidth := layout.layoutFlexColumnLine(box, line, offset, spacing, mainGap, lineX)
			for _, item := range line {
				if item.mainSize > maxLineHeight {
					maxLineHeight = item.mainSize
				}
			}
			lineX += lineWidth
		}
		if heightSpecified {
			box.ContentHeight = mainSize
		} else {
			box.ContentHeight = maxLineHeight
		}
		return
	}
	resolveFlexLineSizes(items, mainSize, mainGap)
	used := mainGap * float64(len(items)-1)
	for _, item := range items {
		used += item.mainSize + item.margin
	}
	remaining := mainSize - used
	offset, spacing := justifyOffsets(strings.ToLower(strings.TrimSpace(box.Style.GetString("justify-content"))), remaining, len(items))
	if row {
		crossSize := 0.0
		if heightSpecified {
			crossSize = box.ContentHeight
		}
		box.ContentHeight = layout.layoutFlexRowLine(box, items, offset, spacing, mainGap, box.ContentY, crossSize)
	} else {
		layout.layoutFlexColumnLine(box, items, offset, spacing, mainGap, box.ContentX)
	}
}

func resolveFlexLineSizes(items []flexItem, mainSize, gap float64) {
	totalBase := 0.0
	for _, item := range items {
		totalBase += item.baseSize + item.margin
	}
	totalBase += gap * float64(len(items)-1)
	free := mainSize - totalBase
	totalGrow := 0.0
	totalShrink := 0.0
	for _, item := range items {
		totalGrow += item.grow
		totalShrink += item.shrink * item.baseSize
	}
	for index := range items {
		items[index].mainSize = items[index].baseSize
		if free > 0 && totalGrow > 0 {
			items[index].mainSize += free * items[index].grow / totalGrow
		} else if free < 0 && totalShrink > 0 {
			items[index].mainSize += free * (items[index].shrink * items[index].baseSize) / totalShrink
			if items[index].mainSize < 0 {
				items[index].mainSize = 0
			}
		}
	}
}

func groupFlexLines(items []flexItem, mainSize, gap float64) [][]flexItem {
	lines := [][]flexItem{}
	current := []flexItem{}
	used := 0.0
	for _, item := range items {
		outer := item.baseSize + item.margin
		if len(current) > 0 {
			outer += gap
		}
		if len(current) > 0 && used+outer > mainSize {
			lines = append(lines, current)
			current = nil
			used = 0
			outer = item.baseSize + item.margin
		}
		current = append(current, item)
		used += outer
	}
	if len(current) > 0 {
		lines = append(lines, current)
	}
	return lines
}

func (layout *Layout) layoutFlexRowLine(box *model.Box, items []flexItem, offset, spacing, gap, startY, crossSize float64) float64 {
	cursor := box.ContentX + offset
	maxHeight := 0.0
	for index := range items {
		item := &items[index]
		marginLeft := item.box.Margin.Left.Resolve(box.ContentWidth)
		marginTop := item.box.Margin.Top.Resolve(box.ContentWidth)
		marginBottom := item.box.Margin.Bottom.Resolve(box.ContentWidth)
		paddingLeft := item.box.Padding.Left.Resolve(box.ContentWidth)
		paddingRight := item.box.Padding.Right.Resolve(box.ContentWidth)
		paddingTop := item.box.Padding.Top.Resolve(box.ContentWidth)
		paddingBottom := item.box.Padding.Bottom.Resolve(box.ContentWidth)
		borderLeft := item.box.Border.Left.Resolve(box.ContentWidth)
		borderRight := item.box.Border.Right.Resolve(box.ContentWidth)
		borderTop := item.box.Border.Top.Resolve(box.ContentWidth)
		borderBottom := item.box.Border.Bottom.Resolve(box.ContentWidth)
		horizontalExtra := paddingLeft + paddingRight + borderLeft + borderRight
		verticalExtra := paddingTop + paddingBottom + borderTop + borderBottom
		item.mainSize = layout.clampFlexWidth(item.box, item.mainSize, item.mainSize, box.ContentWidth, horizontalExtra)
		item.box.ContentWidth = math.Max(0, item.mainSize)
		item.box.Width = item.box.ContentWidth + horizontalExtra
		item.box.X = cursor + marginLeft
		item.box.ContentX = item.box.X + borderLeft + paddingLeft
		item.box.Y = startY + marginTop
		item.box.ContentY = item.box.Y + borderTop + paddingTop
		layout.layoutFlexItemContent(item.box)
		item.box.ContentHeight = layout.flexContentHeight(item.box)
		item.box.ContentHeight = layout.resolveContentHeight(item.box, item.box.ContentHeight, 0, verticalExtra)
		item.box.Height = item.box.ContentHeight + verticalExtra
		outerHeight := item.box.Height + marginTop + marginBottom
		if outerHeight > maxHeight {
			maxHeight = outerHeight
		}
		cursor += item.margin + item.mainSize + gap + spacing
	}
	if crossSize > maxHeight {
		maxHeight = crossSize
	}
	align := strings.ToLower(strings.TrimSpace(box.Style.GetString("align-items")))
	if align == "" {
		align = "stretch"
	}
	for index := range items {
		item := &items[index]
		alignSelf := strings.ToLower(strings.TrimSpace(item.box.Style.GetString("align-self")))
		if alignSelf == "" || alignSelf == "auto" {
			alignSelf = align
		}
		marginTop := item.box.Margin.Top.Resolve(box.ContentWidth)
		marginBottom := item.box.Margin.Bottom.Resolve(box.ContentWidth)
		previousY := item.box.Y
		switch alignSelf {
		case "flex-end", "end":
			item.box.Y = startY + maxHeight - marginBottom - item.box.Height
		case "center":
			item.box.Y = startY + (maxHeight-item.box.Height-marginTop-marginBottom)/2 + marginTop
		case "baseline":
			maxAscent := 0.0
			for _, other := range items {
				if ascent := boxBaseline(other.box); ascent > maxAscent {
					maxAscent = ascent
				}
			}
			item.box.Y = startY + maxAscent - boxBaseline(item.box)
		case "stretch":
			if model.ParseSize(item.box.Style.GetString("height")).Auto {
				item.box.Height = maxHeight - marginTop - marginBottom
			}
		}
		if deltaY := item.box.Y - previousY; deltaY != 0 {
			for _, child := range item.box.Children {
				shiftBox(child, 0, deltaY)
			}
		}
		item.box.ContentY = item.box.Y + item.box.Border.Top.Resolve(box.ContentWidth) + item.box.Padding.Top.Resolve(box.ContentWidth)
	}
	return maxHeight
}

func (layout *Layout) layoutFlexColumnLine(box *model.Box, items []flexItem, offset, spacing, gap, startX float64) float64 {
	cursor := box.ContentY + offset
	maxWidth := 0.0
	align := strings.ToLower(strings.TrimSpace(box.Style.GetString("align-items")))
	if align == "" {
		align = "stretch"
	}
	for index := range items {
		item := &items[index]
		marginLeft := item.box.Margin.Left.Resolve(box.ContentWidth)
		marginRight := item.box.Margin.Right.Resolve(box.ContentWidth)
		marginTop := item.box.Margin.Top.Resolve(box.ContentWidth)
		paddingLeft := item.box.Padding.Left.Resolve(box.ContentWidth)
		paddingRight := item.box.Padding.Right.Resolve(box.ContentWidth)
		paddingTop := item.box.Padding.Top.Resolve(box.ContentWidth)
		paddingBottom := item.box.Padding.Bottom.Resolve(box.ContentWidth)
		borderLeft := item.box.Border.Left.Resolve(box.ContentWidth)
		borderRight := item.box.Border.Right.Resolve(box.ContentWidth)
		borderTop := item.box.Border.Top.Resolve(box.ContentWidth)
		borderBottom := item.box.Border.Bottom.Resolve(box.ContentWidth)
		horizontalExtra := paddingLeft + paddingRight + borderLeft + borderRight
		verticalExtra := paddingTop + paddingBottom + borderTop + borderBottom
		alignSelf := strings.ToLower(strings.TrimSpace(item.box.Style.GetString("align-self")))
		if alignSelf == "" || alignSelf == "auto" {
			alignSelf = align
		}
		width := 0.0
		if alignSelf == "stretch" {
			width = math.Max(0, box.ContentWidth-marginLeft-marginRight-horizontalExtra)
		} else {
			width = layout.resolveWidth(item.box, box.ContentWidth, marginLeft+marginRight, horizontalExtra)
		}
		item.box.ContentWidth = width
		item.box.Width = width + horizontalExtra
		item.box.X = startX + marginLeft
		item.box.ContentX = item.box.X + borderLeft + paddingLeft
		item.box.Y = cursor + marginTop
		item.box.ContentY = item.box.Y + borderTop + paddingTop
		layout.layoutFlexItemContent(item.box)
		item.mainSize = layout.clampHeight(item.box, item.mainSize, box.ContentHeight, verticalExtra)
		item.box.ContentHeight = math.Max(0, item.mainSize)
		item.box.Height = item.box.ContentHeight + verticalExtra
		outerWidth := item.box.Width + marginLeft + marginRight
		if outerWidth > maxWidth {
			maxWidth = outerWidth
		}
		cursor += item.margin + item.mainSize + gap + spacing
	}
	for index := range items {
		item := &items[index]
		alignSelf := strings.ToLower(strings.TrimSpace(item.box.Style.GetString("align-self")))
		if alignSelf == "" || alignSelf == "auto" {
			alignSelf = align
		}
		marginLeft := item.box.Margin.Left.Resolve(box.ContentWidth)
		marginRight := item.box.Margin.Right.Resolve(box.ContentWidth)
		previousX := item.box.X
		switch alignSelf {
		case "flex-end", "end":
			item.box.X = startX + box.ContentWidth - marginRight - item.box.Width
		case "center":
			item.box.X = startX + (box.ContentWidth-item.box.Width-marginLeft-marginRight)/2 + marginLeft
		}
		if deltaX := item.box.X - previousX; deltaX != 0 {
			for _, child := range item.box.Children {
				shiftBox(child, deltaX, 0)
			}
		}
		item.box.ContentX = item.box.X + item.box.Border.Left.Resolve(box.ContentWidth) + item.box.Padding.Left.Resolve(box.ContentWidth)
	}
	if maxWidth <= 0 {
		return 0
	}
	return maxWidth
}

func (layout *Layout) layoutFlexItemContent(box *model.Box) {
	if box.Kind == model.TextBox {
		width, height := layout.measureText(box.Text, box.Style)
		box.ContentWidth = width
		box.ContentHeight = height
		return
	}
	hasBlock := false
	for _, child := range box.Children {
		if child.IsBlockLevel() {
			hasBlock = true
			break
		}
	}
	if hasBlock {
		layout.layoutBlockChildren(box, false, false, 0)
	} else {
		layout.layoutInlineChildren(box)
	}
}

func (layout *Layout) flexContentHeight(box *model.Box) float64 {
	if box.Kind == model.TextBox {
		_, height := layout.measureText(box.Text, box.Style)
		return height
	}
	return box.ContentHeight
}

func (layout *Layout) flexBaseSize(item *flexItem, row bool, containerMain float64) float64 {
	if item.basis.Auto || item.basis.Keyword == "content" {
		// fall through to width/height resolution
	} else {
		size := model.ParseSizeWithBase(item.box.Style.GetString("flex-basis"), containerMain, layout.ViewportWidth, layout.ViewportHeight)
		if size.Auto || size.Keyword != "" {
			// fall through to width/height resolution
		} else if size.Percent {
			return containerMain * size.Value / 100
		} else {
			return size.Value
		}
	}
	if row {
		size := model.ParseSize(item.box.Style.GetString("width"))
		if !size.Auto && size.Keyword == "" {
			if size.Percent {
				return containerMain * size.Value / 100
			}
			return size.Value
		}
		layout.computeIntrinsicWidths(item.box)
		return item.box.IntrinsicMaxWidth
	}
	size := model.ParseSize(item.box.Style.GetString("height"))
	if !size.Auto && size.Keyword == "" {
		if size.Percent {
			return containerMain * size.Value / 100
		}
		return size.Value
	}
	return 0
}

func (layout *Layout) flexGap(box *model.Box, row bool) float64 {
	property := "row-gap"
	if row {
		property = "column-gap"
	}
	length := model.ParseLength(box.Style.GetString(property))
	if length.Auto {
		return 0
	}
	return length.Resolve(box.ContentWidth)
}

func childMarginMain(box *model.Box, row bool, containingWidth float64) float64 {
	if row {
		return box.Margin.Left.Resolve(containingWidth) + box.Margin.Right.Resolve(containingWidth)
	}
	return box.Margin.Top.Resolve(containingWidth) + box.Margin.Bottom.Resolve(containingWidth)
}

func justifyOffsets(justify string, remaining float64, count int) (float64, float64) {
	if remaining < 0 {
		remaining = 0
	}
	switch justify {
	case "flex-end", "end":
		return remaining, 0
	case "center":
		return remaining / 2, 0
	case "space-between":
		if count > 1 {
			return 0, remaining / float64(count-1)
		}
	case "space-around":
		spacing := remaining / float64(count)
		return spacing / 2, spacing
	case "space-evenly":
		spacing := remaining / float64(count+1)
		return spacing, spacing
	}
	return 0, 0
}

func (layout *Layout) clampFlexWidth(box *model.Box, mainSize, available, containingWidth, horizontalExtra float64) float64 {
	borderBox := strings.EqualFold(strings.TrimSpace(box.Style.GetString("box-sizing")), "border-box")
	mainSize = layout.clampSize(box, box.Style.GetString("min-width"), mainSize, containingWidth, horizontalExtra, borderBox, true)
	mainSize = layout.clampSize(box, box.Style.GetString("max-width"), mainSize, containingWidth, horizontalExtra, borderBox, false)
	if mainSize > available && model.ParseSize(box.Style.GetString("max-width")).Auto {
		mainSize = available
	}
	return math.Max(0, mainSize)
}

func boxBaseline(box *model.Box) float64 {
	if box == nil {
		return 0
	}
	for index := len(box.Children) - 1; index >= 0; index-- {
		child := box.Children[index]
		if child.Kind == model.TextBox || child.Kind == model.InlineBox || child.Kind == model.AnonymousBlockBox {
			if baseline, ok := childBaseline(child); ok {
				return baseline - box.Y
			}
		}
	}
	if box.Ascent > 0 {
		return box.Ascent
	}
	return box.FontSize * 0.8
}

func childBaseline(box *model.Box) (float64, bool) {
	if box == nil {
		return 0, false
	}
	if box.Kind == model.TextBox {
		return box.Y + box.Ascent, true
	}
	for index := len(box.Children) - 1; index >= 0; index-- {
		if baseline, ok := childBaseline(box.Children[index]); ok {
			return baseline, true
		}
	}
	return 0, false
}

func parseFlexNumber(text string, fallback float64) float64 {
	values := parser.ParseComponentValueList(text)
	if len(values) == 1 {
		if token, ok := parser.AsToken(values[0]); ok && token.Type == tokenizer.TokenNumber {
			return token.Number
		}
	}
	return fallback
}

func parseFlexInteger(text string) int {
	values := parser.ParseComponentValueList(text)
	if len(values) == 1 {
		if token, ok := parser.AsToken(values[0]); ok && token.Type == tokenizer.TokenNumber && token.NumericKind == tokenizer.Integer {
			return int(token.Number)
		}
	}
	return 0
}
