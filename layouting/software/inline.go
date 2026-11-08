package software

import (
	"gonetsurf/layouting/model"
	"math"
	"strings"
	"unicode"

	"gonetsurf/dom"
	"gonetsurf/styles"

	"golang.org/x/text/unicode/bidi"
)

// inlineRun is one unbreakable item in an inline formatting context.
type inlineRun struct {
	box          *model.Box
	style        *styles.Style
	text         string
	visual       string
	width        float64
	height       float64
	ascent       float64
	lineHeight   float64
	spaceBefore  bool
	forceBreak   bool
	atomic       bool
	logical      int
	marginLeft   float64
	marginRight  float64
	marginTop    float64
	marginBottom float64
}

type inlineLine struct {
	runs   []inlineRun
	width  float64
	forced bool
}

const inlineBreakTolerance = 6.0

func (layout *Layout) layoutInlineFormatting(box *model.Box) {
	runs := []inlineRun{}
	floatBottom := 0.0
	floats := []*model.Box{}
	layout.collectInlineRuns(box, box.Style, &runs, &floatBottom, &floats)
	if len(runs) == 0 {
		if floatBottom > box.ContentY {
			box.ContentHeight = floatBottom - box.ContentY
		} else {
			box.ContentHeight = 0
		}
		return
	}
	wrap := allowsInlineWrapping(box.Style)
	lines := layout.breakInlineLines(runs, box.ContentWidth, wrap)
	lineY := box.ContentY
	if floatBottom > lineY {
		lineY = floatBottom
	}
	result := make([]*model.Box, 0, len(runs))
	lineCount := len(lines)
	for lineIndex, line := range lines {
		ordered := orderInlineLine(line.runs, box.Style)
		lineHeight, baseline := layout.inlineLineMetrics(box.Style, ordered)
		used := inlineLineWidth(ordered)
		offset := inlineTextOffset(box.Style, box.ContentWidth-used, lineIndex == lineCount-1)
		if isRTLStyle(box.Style) && isAlignStart(box.Style) {
			offset = box.ContentWidth - used
		}
		cursor := box.ContentX + offset
		justifyExtra := 0.0
		if isJustify(box.Style) && !line.forced && lineIndex != lineCount-1 {
			spaces := 0
			for _, run := range ordered {
				if run.spaceBefore && !run.atomic {
					spaces++
				}
			}
			if spaces > 0 {
				justifyExtra = math.Max(0, box.ContentWidth-used) / float64(spaces)
			}
		}
		for runIndex, run := range ordered {
			if runIndex > 0 && run.spaceBefore && !run.atomic {
				cursor += inlineSpaceWidth(run.style)
				cursor += justifyExtra
			}
			itemY := lineY + baseline - run.ascent
			switch inlineVerticalAlign(run.style) {
			case "top", "text-top":
				itemY = lineY
			case "bottom", "text-bottom":
				itemY = lineY + lineHeight - run.height
			case "middle":
				itemY = lineY + (lineHeight-run.height)/2
			case "sub":
				itemY = lineY + baseline - run.ascent + run.height*0.2
			case "super":
				itemY = lineY + baseline - run.ascent - run.height*0.4
			}
			if run.atomic {
				targetX := cursor + run.marginLeft
				targetY := itemY + run.marginTop
				shiftBox(run.box, targetX-run.box.X, targetY-run.box.Y)
				run.box.Width = run.width - run.marginLeft - run.marginRight
				run.box.Height = run.height - run.marginTop - run.marginBottom
				cursor += run.width
				result = append(result, run.box)
				continue
			}
			word := &model.Box{
				Node:          run.box.Node,
				Style:         run.style,
				Kind:          model.TextBox,
				Text:          run.text,
				VisualText:    run.visual,
				FontSize:      run.box.FontSize,
				LineHeight:    run.lineHeight,
				Ascent:        run.ascent,
				Parent:        box,
				X:             cursor,
				Y:             itemY,
				Width:         run.width,
				Height:        run.height,
				ContentX:      cursor,
				ContentY:      itemY,
				ContentWidth:  run.width,
				ContentHeight: run.height,
			}
			if word.VisualText == "" {
				word.VisualText = word.Text
			}
			result = append(result, word)
			cursor += run.width
		}
		lineY += lineHeight
	}
	box.Children = result
	for _, child := range result {
		child.Parent = box
	}
	if floatBottom > lineY {
		lineY = floatBottom
	}
	box.ContentHeight = math.Max(0, lineY-box.ContentY)
}

func (layout *Layout) collectInlineRuns(box *model.Box, inherited *styles.Style, result *[]inlineRun, floatBottom *float64, floats *[]*model.Box) {
	for _, child := range box.Children {
		if isOutOfFlow(child) {
			child.StaticX = box.ContentX
			child.StaticY = box.ContentY
			child.HasStaticPosition = true
			layout.positioned = append(layout.positioned, child)
			continue
		}
		if isFloat(child) {
			layout.layoutFloat(child, box.ContentX, box.ContentY, box.ContentWidth)
			positionFloat(child, *floats, box.ContentX, box.ContentWidth)
			*floats = append(*floats, child)
			if bottom := floatOuterBottom(child); bottom > *floatBottom {
				*floatBottom = bottom
			}
			continue
		}
		switch child.Kind {
		case model.TextBox:
			style := child.Style
			if style == nil {
				style = inherited
			}
			appendTextRuns(layout, child, style, result)
		case model.InlineBox:
			if isAtomicInline(child) {
				run := layout.atomicInlineRun(child, box.ContentWidth)
				*result = append(*result, run)
				continue
			}
			if isLineBreak(child) {
				*result = append(*result, inlineRun{forceBreak: true})
				continue
			}
			layout.collectInlineRuns(child, child.Style, result, floatBottom, floats)
		default:
			if child.IsBlockLevel() {
				continue
			}
			layout.collectInlineRuns(child, child.Style, result, floatBottom, floats)
		}
	}
}

func appendTextRuns(layout *Layout, box *model.Box, style *styles.Style, result *[]inlineRun) {
	whiteSpace := strings.ToLower(strings.TrimSpace(style.GetString("white-space")))
	preserve := whiteSpace == "pre" || whiteSpace == "pre-wrap" || whiteSpace == "pre-line" || whiteSpace == "break-spaces"
	preserveBreaks := preserve
	collapse := whiteSpace != "pre" && whiteSpace != "pre-wrap" && whiteSpace != "break-spaces"
	if !preserve {
		text := strings.ReplaceAll(box.Text, "\n", " ")
		if collapse {
			text = strings.Join(strings.Fields(text), " ")
		}
		words := strings.Fields(text)
		for index, word := range words {
			width, _ := layout.measureText(word, style)
			metrics := layout.metricsForStyle(style)
			lineHeight := box.LineHeight
			if lineHeight <= 0 {
				_, lineHeight = model.FontMetricsForStyle(style)
			}
			*result = append(*result, inlineRun{
				box:         box,
				style:       style,
				text:        word,
				visual:      word,
				width:       width,
				height:      lineHeight,
				ascent:      metrics.Ascent,
				lineHeight:  lineHeight,
				spaceBefore: index > 0,
			})
		}
		return
	}
	lines := strings.Split(box.Text, "\n")
	for lineIndex, line := range lines {
		if lineIndex > 0 && preserveBreaks {
			*result = append(*result, inlineRun{forceBreak: true})
		}
		if line == "" {
			continue
		}
		if whiteSpace == "pre" {
			width, _ := layout.measureText(line, style)
			metrics := layout.metricsForStyle(style)
			lineHeight := box.LineHeight
			if lineHeight <= 0 {
				_, lineHeight = model.FontMetricsForStyle(style)
			}
			*result = append(*result, inlineRun{
				box:        box,
				style:      style,
				text:       line,
				visual:     line,
				width:      width,
				height:     lineHeight,
				ascent:     metrics.Ascent,
				lineHeight: lineHeight,
			})
			continue
		}
		words := splitPreservedWords(line)
		if whiteSpace == "pre-line" {
			words = strings.Fields(line)
		}
		for index, word := range words {
			width, _ := layout.measureText(word, style)
			metrics := layout.metricsForStyle(style)
			lineHeight := box.LineHeight
			if lineHeight <= 0 {
				_, lineHeight = model.FontMetricsForStyle(style)
			}
			*result = append(*result, inlineRun{
				box:         box,
				style:       style,
				text:        word,
				visual:      word,
				width:       width,
				height:      lineHeight,
				ascent:      metrics.Ascent,
				lineHeight:  lineHeight,
				spaceBefore: index > 0,
			})
		}
	}
}

func splitPreservedWords(line string) []string {
	words := []string{}
	current := strings.Builder{}
	for _, character := range line {
		if unicode.IsSpace(character) {
			if current.Len() > 0 {
				words = append(words, current.String())
				current.Reset()
			}
			if character == ' ' {
				words = append(words, " ")
			}
			continue
		}
		current.WriteRune(character)
	}
	if current.Len() > 0 {
		words = append(words, current.String())
	}
	return words
}

func (layout *Layout) atomicInlineRun(box *model.Box, availableWidth float64) inlineRun {
	box.IntrinsicsComputed = false
	box.Provisional = false
	marginLeft := box.Margin.Left.Resolve(availableWidth)
	marginRight := box.Margin.Right.Resolve(availableWidth)
	marginTop := box.Margin.Top.Resolve(availableWidth)
	marginBottom := box.Margin.Bottom.Resolve(availableWidth)
	paddingLeft := box.Padding.Left.Resolve(availableWidth)
	paddingRight := box.Padding.Right.Resolve(availableWidth)
	paddingTop := box.Padding.Top.Resolve(availableWidth)
	paddingBottom := box.Padding.Bottom.Resolve(availableWidth)
	borderLeft := box.Border.Left.Resolve(availableWidth)
	borderRight := box.Border.Right.Resolve(availableWidth)
	borderTop := box.Border.Top.Resolve(availableWidth)
	borderBottom := box.Border.Bottom.Resolve(availableWidth)
	horizontalExtra := marginLeft + marginRight + paddingLeft + paddingRight + borderLeft + borderRight
	verticalExtra := marginTop + marginBottom + paddingTop + paddingBottom + borderTop + borderBottom
	size := model.ParseSize(box.Style.GetString("width"))
	contentWidth := 0.0
	if size.Auto || size.Keyword != "" {
		layout.computeIntrinsicWidths(box)
		contentWidth = math.Max(box.IntrinsicMinWidth, math.Min(availableWidth-horizontalExtra, box.IntrinsicMaxWidth))
	} else {
		contentWidth = layout.resolveWidth(box, availableWidth, marginLeft+marginRight, horizontalExtra-marginLeft-marginRight)
	}
	box.ContentWidth = math.Max(0, contentWidth)
	box.Width = box.ContentWidth + horizontalExtra - marginLeft - marginRight
	box.X = 0
	box.Y = 0
	box.ContentX = paddingLeft + borderLeft
	box.ContentY = paddingTop + borderTop
	layout.layoutAtomicInlineContent(box)
	if isReplacedElement(box) && layout.IntrinsicSize != nil {
		heightSize := model.ParseSize(box.Style.GetString("height"))
		if heightSize.Auto || heightSize.Keyword != "" {
			intrinsicWidth, intrinsicHeight, ok, pending := layout.IntrinsicSize(box)
			if ok && intrinsicWidth > 0 && box.ContentWidth > 0 {
				box.ContentHeight = box.ContentWidth * intrinsicHeight / intrinsicWidth
			} else if pending && box.ContentWidth > 0 {
				box.ContentHeight = box.ContentWidth * 150 / 300
				box.Provisional = true
			}
		}
	}
	height := layout.resolveContentHeight(box, box.ContentHeight, 0, verticalExtra)
	box.ContentHeight = height
	box.Height = height + verticalExtra - marginTop - marginBottom
	ascent := box.Height
	if hasInlineLineContent(box) {
		if baseline := boxBaseline(box); baseline > 0 {
			ascent = baseline
		}
	}
	return inlineRun{
		box:          box,
		style:        box.Style,
		width:        box.Width + marginLeft + marginRight,
		height:       box.Height + marginTop + marginBottom,
		ascent:       ascent + marginTop,
		lineHeight:   box.Height + marginTop + marginBottom,
		atomic:       true,
		marginLeft:   marginLeft,
		marginRight:  marginRight,
		marginTop:    marginTop,
		marginBottom: marginBottom,
	}
}

func hasInlineLineContent(box *model.Box) bool {
	for _, child := range box.Children {
		if child.Kind == model.TextBox || child.Kind == model.InlineBox || child.Kind == model.AnonymousBlockBox {
			return true
		}
	}
	return false
}

func (layout *Layout) layoutAtomicInlineContent(box *model.Box) {
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
		layout.layoutInlineFormatting(box)
	}
}

func isAtomicInline(box *model.Box) bool {
	if box == nil || box.Style == nil {
		return false
	}
	if box.Pseudo != "" {
		return true
	}
	switch displayOf(box) {
	case "inline-block", "inline-flex", "inline-grid", "inline-table":
		return true
	}
	return isReplacedElement(box)
}

func isReplacedElement(box *model.Box) bool {
	if box == nil || box.Node == nil || box.Node.Type != dom.ElementNode {
		return false
	}
	switch strings.ToLower(box.Node.Data) {
	case "img", "video", "canvas", "iframe", "object", "embed", "svg":
		return true
	}
	return false
}

func isLineBreak(box *model.Box) bool {
	return box != nil && box.Node != nil && box.Node.Type == dom.ElementNode && strings.EqualFold(box.Node.Data, "br")
}

func allowsInlineWrapping(style *styles.Style) bool {
	whiteSpace := strings.ToLower(strings.TrimSpace(style.GetString("white-space")))
	switch whiteSpace {
	case "nowrap", "pre":
		return false
	}
	return true
}

func (layout *Layout) breakInlineLines(runs []inlineRun, available float64, wrap bool) []inlineLine {
	lines := []inlineLine{}
	current := inlineLine{}
	width := 0.0
	flush := func() {
		lines = append(lines, current)
		current = inlineLine{}
		width = 0
	}
	for index := range runs {
		run := runs[index]
		if run.forceBreak {
			current.forced = true
			flush()
			continue
		}
		space := 0.0
		if len(current.runs) > 0 && run.spaceBefore && !run.atomic {
			space = inlineSpaceWidth(run.style)
		}
		if wrap && len(current.runs) > 0 && width+space+run.width > available+inlineBreakTolerance {
			flush()
			space = 0
		}
		run.spaceBefore = space > 0
		current.runs = append(current.runs, run)
		width += space + run.width
	}
	if len(current.runs) > 0 {
		flush()
	}
	return lines
}

func inlineLineWidth(runs []inlineRun) float64 {
	width := 0.0
	for index, run := range runs {
		if index > 0 && run.spaceBefore && !run.atomic {
			width += inlineSpaceWidth(run.style)
		}
		width += run.width
	}
	return width
}

func inlineSpaceWidth(style *styles.Style) float64 {
	fontSize := 16.0
	if style != nil {
		if value, ok := model.ParseComputedPixels(style.GetString("font-size")); ok && value > 0 {
			fontSize = value
		}
	}
	return fontSize * 0.25
}

func (layout *Layout) inlineLineMetrics(style *styles.Style, runs []inlineRun) (float64, float64) {
	_, strutHeight := model.FontMetricsForStyle(style)
	strutAscent := layout.metricsForStyle(style).Ascent
	lineHeight := strutHeight
	ascent := strutAscent
	for _, run := range runs {
		if run.lineHeight > lineHeight {
			lineHeight = run.lineHeight
		}
		runAscent := run.ascent
		switch inlineVerticalAlign(run.style) {
		case "middle":
			fontSize := 16.0
			if run.style != nil {
				if value, ok := model.ParseComputedPixels(run.style.GetString("font-size")); ok && value > 0 {
					fontSize = value
				}
			}
			runAscent = run.height/2 + fontSize*0.25
		case "top", "bottom", "text-top", "text-bottom":
			continue
		}
		if runAscent > ascent {
			ascent = runAscent
		}
	}
	if lineHeight <= 0 {
		lineHeight = 19.2
	}
	return lineHeight, ascent
}

func inlineTextOffset(style *styles.Style, remaining float64, lastLine bool) float64 {
	if remaining <= 0 {
		return 0
	}
	switch strings.ToLower(strings.TrimSpace(style.GetString("text-align"))) {
	case "center":
		return remaining / 2
	case "right", "end":
		return remaining
	case "justify", "justify-all":
		return 0
	}
	return 0
}

func isJustify(style *styles.Style) bool {
	align := strings.ToLower(strings.TrimSpace(style.GetString("text-align")))
	return align == "justify" || align == "justify-all"
}

func isAlignStart(style *styles.Style) bool {
	align := strings.ToLower(strings.TrimSpace(style.GetString("text-align")))
	return align == "" || align == "start" || align == "justify"
}

func isRTLStyle(style *styles.Style) bool {
	return strings.EqualFold(strings.TrimSpace(style.GetString("direction")), "rtl")
}

func inlineVerticalAlign(style *styles.Style) string {
	if style == nil {
		return "baseline"
	}
	value := strings.ToLower(strings.TrimSpace(style.GetString("vertical-align")))
	if value == "" || value == "baseline" {
		return "baseline"
	}
	return value
}

// orderInlineLine applies the Unicode bidi algorithm to the assembled line.
// Text words keep logical Text while VisualText and visual placement follow
// the bidi ordering.
func orderInlineLine(runs []inlineRun, style *styles.Style) []inlineRun {
	sequence := make([]int, 0, len(runs))
	for index, run := range runs {
		if run.atomic || run.text != "" {
			sequence = append(sequence, index)
		}
	}
	if len(sequence) < 2 {
		return runs
	}
	joined := make([]rune, 0, len(sequence)*4)
	offsets := make([][2]int, len(sequence))
	for position, index := range sequence {
		if position > 0 {
			joined = append(joined, ' ')
		}
		start := len(joined)
		if runs[index].atomic {
			joined = append(joined, '\uFFFC')
		} else {
			joined = append(joined, []rune(runs[index].text)...)
		}
		offsets[position] = [2]int{start, len(joined)}
	}
	defaultDirection := bidi.LeftToRight
	if isRTLStyle(style) {
		defaultDirection = bidi.RightToLeft
	}
	bidiRuns := model.SegmentBidi(string(joined), defaultDirection)
	order := make([]int, 0, len(sequence))
	visual := make(map[int]string, len(sequence))
	assigned := make(map[int]bool, len(sequence))
	for _, run := range bidiRuns {
		indices := []int{}
		for position, span := range offsets {
			if assigned[position] {
				continue
			}
			if span[0] <= run.End && span[1] > run.Start {
				indices = append(indices, position)
			}
		}
		if run.Direction == bidi.RightToLeft {
			for left, right := 0, len(indices)-1; left < right; left, right = left+1, right-1 {
				indices[left], indices[right] = indices[right], indices[left]
			}
		}
		for _, position := range indices {
			if assigned[position] {
				continue
			}
			assigned[position] = true
			index := sequence[position]
			order = append(order, index)
			if run.Direction == bidi.RightToLeft && !runs[index].atomic {
				visual[index] = bidi.ReverseString(runs[index].text)
			} else {
				visual[index] = runs[index].text
			}
		}
	}
	seen := make([]bool, len(runs))
	reordered := make([]inlineRun, 0, len(runs))
	for _, index := range order {
		if seen[index] {
			continue
		}
		seen[index] = true
		run := runs[index]
		run.visual = visual[index]
		run.logical = index
		reordered = append(reordered, run)
	}
	for index, run := range runs {
		if seen[index] {
			continue
		}
		reordered = append(reordered, run)
	}
	for index := 1; index < len(reordered); index++ {
		if !reordered[index].atomic && !reordered[index-1].atomic {
			reordered[index].spaceBefore = true
		}
	}
	return reordered
}
