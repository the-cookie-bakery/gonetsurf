//go:build vulkan && cgo

package vulkan

import (
	"strconv"
	"strings"

	"gonetsurf/dom"
	"gonetsurf/layouting/model"
	"gonetsurf/styles"
)

type encodedNode struct {
	box        *model.Box
	depth      int
	parent     int
	first      int
	next       int
	childCount int
	subtree    int
}

type encodedTree struct {
	nodes    []encodedNode
	values   []float32
	aux      []float32
	maxDepth int
	pending  []pendingResource
}

func (tree *encodedTree) allocAux(count int) int {
	start := len(tree.aux)
	tree.aux = append(tree.aux, make([]float32, count)...)
	return start
}

type pendingResource struct {
	kind int
	url  string
	node *dom.Node
}

func splitTextNodes(box *model.Box, options backendOptions) {
	if box == nil {
		return
	}
	children := make([]*model.Box, 0, len(box.Children))
	changed := false
	for _, child := range box.Children {
		if child.Kind == model.TextBox && !strings.HasPrefix(child.Pseudo, "gpu-word") {
			changed = true
			text := strings.ReplaceAll(child.Text, "\n", " ")
			words := strings.Fields(text)
			for index, word := range words {
				ascent := child.Ascent
				if options.Metrics != nil {
					ascent = options.Metrics(child.Style).Ascent
				}
				if ascent <= 0 {
					ascent = model.MetricsForStyle(child.Style).Ascent
				}
				pseudo := "gpu-word"
				if index > 0 {
					pseudo = "gpu-word-space"
				}
				wordBox := &model.Box{
					Node:       child.Node,
					Style:      child.Style,
					Kind:       model.TextBox,
					Pseudo:     pseudo,
					Text:       word,
					VisualText: word,
					Parent:     box,
					FontSize:   child.FontSize,
					LineHeight: child.LineHeight,
					Ascent:     ascent,
				}
				children = append(children, wordBox)
			}
			continue
		}
		if child.Kind != model.TextBox {
			splitTextNodes(child, options)
		}
		children = append(children, child)
	}
	if changed {
		box.Children = children
	}
}

func encodeTree(root *model.Box, options backendOptions) *encodedTree {
	if root == nil {
		return &encodedTree{}
	}
	tree := &encodedTree{}
	tree.walk(root, -1, 0, options)
	tree.values = make([]float32, len(tree.nodes)*nodeFieldCount)
	for index, encoded := range tree.nodes {
		tree.encode(index, encoded.box, options)
	}
	return tree
}

func (tree *encodedTree) walk(box *model.Box, parent, depth int, options backendOptions) int {
	index := len(tree.nodes)
	tree.nodes = append(tree.nodes, encodedNode{box: box, depth: depth, parent: parent, first: -1, next: -1})
	if depth > tree.maxDepth {
		tree.maxDepth = depth
	}
	first := -1
	previous := -1
	for _, child := range box.Children {
		childIndex := tree.walk(child, index, depth+1, options)
		if first < 0 {
			first = childIndex
		}
		if previous >= 0 {
			tree.nodes[previous].next = childIndex
		}
		previous = childIndex
	}
	tree.nodes[index].first = first
	tree.nodes[index].childCount = len(box.Children)
	tree.nodes[index].subtree = len(tree.nodes) - index
	return index
}

func (tree *encodedTree) encode(index int, box *model.Box, options backendOptions) {
	encoded := tree.nodes[index]
	setValue(tree, index, nodeParent, float64(encoded.parent))
	setValue(tree, index, nodeDepth, float64(encoded.depth))
	setValue(tree, index, nodeFirst, float64(encoded.first))
	setValue(tree, index, nodeNext, float64(encoded.next))
	setValue(tree, index, nodeCount, float64(encoded.childCount))
	setValue(tree, index, nodeSubtreeSize, float64(encoded.subtree))
	setValue(tree, index, nodeScrollLeft, box.ScrollLeft)
	setValue(tree, index, nodeScrollTop, box.ScrollTop)
	setValue(tree, index, nodeKind, float64(box.Kind))
	flags := 0
	if box.Kind == model.TextBox {
		// Text leaves are positioned by their container; they have no children.
	} else {
		inlineChildren := false
		for _, child := range box.Children {
			if !child.IsBlockLevel() {
				inlineChildren = true
				break
			}
		}
		if inlineChildren {
			flags |= flagInlineContainer
		} else if len(box.Children) > 0 {
			flags |= flagBlockContainer
		}
	}
	style := box.Style
	if style != nil {
		if strings.EqualFold(strings.TrimSpace(style.GetString("box-sizing")), "border-box") {
			flags |= flagBorderBox
		}
		position := strings.ToLower(strings.TrimSpace(style.GetString("position")))
		if position == "absolute" || position == "fixed" {
			flags |= flagOutOfFlow
		}
		floating := strings.ToLower(strings.TrimSpace(style.GetString("float")))
		switch floating {
		case "left", "inline-start":
			flags |= flagFloat | flagFloatLeft
		case "right", "inline-end":
			flags |= flagFloat | flagFloatRight
		}
		switch strings.ToLower(strings.TrimSpace(style.GetString("text-align"))) {
		case "center":
			setValue(tree, index, nodeTextAlign, 1)
		case "right", "end":
			setValue(tree, index, nodeTextAlign, 2)
		}
		switch strings.ToLower(strings.TrimSpace(style.GetString("vertical-align"))) {
		case "middle", "center":
			setValue(tree, index, nodeVerticalAlign, 1)
		case "top", "text-top":
			setValue(tree, index, nodeVerticalAlign, 2)
		case "bottom", "text-bottom":
			setValue(tree, index, nodeVerticalAlign, 3)
		}
		clearValue := strings.ToLower(strings.TrimSpace(style.GetString("clear")))
		clear := 0.0
		switch clearValue {
		case "left", "inline-start":
			clear = 1
		case "right", "inline-end":
			clear = 2
		case "both":
			clear = 3
		}
		setValue(tree, index, nodeClear, clear)
		positionValue := strings.ToLower(strings.TrimSpace(style.GetString("position")))
		positionKind := 0.0
		switch positionValue {
		case "relative":
			flags |= flagRelative
			positionKind = 1
		case "absolute":
			flags |= flagAbsolute | flagOutOfFlow
			positionKind = 2
		case "fixed":
			flags |= flagFixed | flagOutOfFlow
			positionKind = 3
		}
		setValue(tree, index, nodePositionKind, positionKind)
		encodeOffset(tree, index, nodeLeft, nodeLeftMode, nodeLeftOffset, style.GetString("left"), options)
		encodeOffset(tree, index, nodeRight, nodeRightMode, nodeRightOffset, style.GetString("right"), options)
		encodeOffset(tree, index, nodeTop, nodeTopMode, nodeTopOffset, style.GetString("top"), options)
		encodeOffset(tree, index, nodeBottom, nodeBottomMode, nodeBottomOffset, style.GetString("bottom"), options)
		writingMode := strings.ToLower(strings.TrimSpace(style.GetString("writing-mode")))
		if strings.HasPrefix(writingMode, "vertical") || strings.HasPrefix(writingMode, "sideways") {
			flags |= flagVertical
			if strings.HasSuffix(writingMode, "rl") {
				flags |= flagVerticalRL
			} else {
				flags |= flagVerticalLR
			}
			setValue(tree, index, nodeWritingMode, 1)
		}
		display := model.NormalizeDisplay(style.GetString("display"))
		if strings.HasPrefix(display, "flow-root") || strings.HasPrefix(display, "flex") ||
			strings.HasPrefix(display, "inline-flex") || strings.HasPrefix(display, "grid") ||
			strings.HasPrefix(display, "inline-grid") || strings.HasPrefix(display, "table") ||
			strings.HasPrefix(display, "inline-table") || position == "absolute" || position == "fixed" ||
			floating == "left" || floating == "right" || floating == "inline-start" || floating == "inline-end" {
			flags |= flagBlockFormatting
		}
		for _, property := range []string{"overflow-x", "overflow-y"} {
			value := strings.ToLower(strings.TrimSpace(style.GetString(property)))
			if value != "" && value != "visible" {
				flags |= flagBlockFormatting
			}
			if value == "scroll" || value == "auto" {
				if property == "overflow-x" {
					flags |= flagOverflowX
				} else {
					flags |= flagOverflowY
				}
			}
		}
		if box.ClipX {
			flags |= flagClipX
		}
		if box.ClipY {
			flags |= flagClipY
		}
		if strings.EqualFold(strings.TrimSpace(style.GetString("white-space")), "nowrap") {
			flags |= flagNoWrap
		}
		if box.Kind == model.InlineBox && (display == "inline-block" || display == "inline-flex" ||
			display == "inline-grid" || display == "inline-table" || isReplaced(box.Node)) {
			setValue(tree, index, nodeAtomicInline, 1)
		}
		if strings.HasPrefix(display, "flex") || strings.HasPrefix(display, "inline-flex") {
			flags |= flagFlexContainer
			direction := strings.ToLower(strings.TrimSpace(style.GetString("flex-direction")))
			if strings.HasPrefix(direction, "column") {
				flags |= flagFlexColumn
			}
			if strings.HasSuffix(direction, "reverse") {
				flags |= flagFlexReverse
			}
			if strings.HasPrefix(strings.ToLower(strings.TrimSpace(style.GetString("flex-wrap"))), "wrap") {
				flags |= flagFlexWrap
			}
			justify := strings.ToLower(strings.TrimSpace(style.GetString("justify-content")))
			justifyValue := 0.0
			switch justify {
			case "space-between":
				justifyValue = 1
			case "center":
				justifyValue = 2
			case "flex-end", "end":
				justifyValue = 3
			case "space-around":
				justifyValue = 4
			}
			setValue(tree, index, nodeFlexJustify, justifyValue)
			align := strings.ToLower(strings.TrimSpace(style.GetString("align-items")))
			alignValue := 0.0
			switch align {
			case "center":
				alignValue = 1
			case "flex-end", "end":
				alignValue = 2
			case "stretch":
				alignValue = 3
			}
			setValue(tree, index, nodeFlexAlign, alignValue)
			gapText := style.GetString("column-gap")
			if strings.TrimSpace(gapText) == "" || strings.EqualFold(strings.TrimSpace(gapText), "normal") {
				gapText = style.GetString("gap")
			}
			gap := model.ParseLength(gapText)
			if !gap.Auto {
				gapValue := gap.Value
				if gap.Percent {
					gapValue = 0
				}
				setValue(tree, index, nodeFlexGap, gapValue)
			}
		}
		grow, _ := strconv.ParseFloat(strings.TrimSpace(style.GetString("flex-grow")), 64)
		setValue(tree, index, nodeFlexGrow, grow)
		shrink, _ := strconv.ParseFloat(strings.TrimSpace(style.GetString("flex-shrink")), 64)
		if strings.TrimSpace(style.GetString("flex-shrink")) == "" {
			shrink = 1
		}
		setValue(tree, index, nodeFlexShrink, shrink)
		basisMode, basisValue, basisOffset := encodeSize(style.GetString("flex-basis"), options)
		setValue(tree, index, nodeFlexBasisMode, float64(basisMode))
		setValue(tree, index, nodeFlexBasis, float64(basisValue))
		setValue(tree, index, nodeFlexBasisOffset, float64(basisOffset))
		order, _ := strconv.Atoi(strings.TrimSpace(style.GetString("order")))
		setValue(tree, index, nodeOrder, float64(order))
		if strings.HasPrefix(display, "grid") || strings.HasPrefix(display, "inline-grid") {
			flags |= flagGridContainer
			encodeGrid(tree, index, box, style)
		}
		if strings.HasPrefix(display, "table") || strings.HasPrefix(display, "inline-table") {
			if !strings.HasPrefix(display, "table-row") && !strings.HasPrefix(display, "table-cell") && !strings.HasPrefix(display, "table-column") {
				flags |= flagTableContainer
				encodeTable(tree, index, box, style)
			}
		}
		if strings.HasPrefix(display, "table-row") && !strings.HasPrefix(display, "table-row-group") {
			flags |= flagTableRow
		}
		if strings.HasPrefix(display, "table-cell") {
			flags |= flagTableCell
		}
		if display == "table-row-group" || display == "table-header-group" || display == "table-footer-group" {
			flags |= flagTableRowGroup
		}
	}
	if strings.HasPrefix(box.Pseudo, "gpu-word-space") {
		setValue(tree, index, nodeSpaceBefore, 1)
	}
	if box.Node != nil && box.Node.Type == dom.ElementNode && strings.EqualFold(box.Node.Data, "br") {
		setValue(tree, index, nodeForceBreak, 1)
	}
	setValue(tree, index, nodeFlags2, float64(flags&0xffff))
	setValue(tree, index, nodeFlags3, float64(flags>>16))

	widthMode, widthValue, widthOffset := encodeSize(box.Style.GetString("width"), options)
	setValue(tree, index, nodeModeW, float64(widthMode))
	setValue(tree, index, nodeSpecW, float64(widthValue))
	setValue(tree, index, nodeSpecWOffset, float64(widthOffset))
	heightMode, heightValue, heightOffset := encodeSize(box.Style.GetString("height"), options)
	setValue(tree, index, nodeModeH, float64(heightMode))
	setValue(tree, index, nodeSpecH, float64(heightValue))
	setValue(tree, index, nodeSpecHOffset, float64(heightOffset))
	minWidthMode, minWidth, minWidthOffset := encodeMinMax(box.Style.GetString("min-width"), options)
	setValue(tree, index, nodeMinwMode, float64(minWidthMode))
	setValue(tree, index, nodeMinW, float64(minWidth))
	setValue(tree, index, nodeMinWOffset, float64(minWidthOffset))
	maxWidthMode, maxWidth, maxWidthOffset := encodeMinMax(box.Style.GetString("max-width"), options)
	setValue(tree, index, nodeMaxwMode, float64(maxWidthMode))
	setValue(tree, index, nodeMaxW, float64(maxWidth))
	setValue(tree, index, nodeMaxWOffset, float64(maxWidthOffset))
	minHeightMode, minHeight, minHeightOffset := encodeMinMax(box.Style.GetString("min-height"), options)
	setValue(tree, index, nodeMinhMode, float64(minHeightMode))
	setValue(tree, index, nodeMinH, float64(minHeight))
	setValue(tree, index, nodeMinHOffset, float64(minHeightOffset))
	maxHeightMode, maxHeight, maxHeightOffset := encodeMinMax(box.Style.GetString("max-height"), options)
	setValue(tree, index, nodeMaxhMode, float64(maxHeightMode))
	setValue(tree, index, nodeMaxH, float64(maxHeight))
	setValue(tree, index, nodeMaxHOffset, float64(maxHeightOffset))

	encodeEdge(tree, index, nodeMt, nodeMtMode, box.Margin.Top)
	encodeEdge(tree, index, nodeMr, nodeMrMode, box.Margin.Right)
	encodeEdge(tree, index, nodeMb, nodeMbMode, box.Margin.Bottom)
	setValue(tree, index, nodeOrigMb, box.Margin.Bottom.Value)
	encodeEdge(tree, index, nodeMl, nodeMlMode, box.Margin.Left)
	encodeEdge(tree, index, nodePt, nodePtMode, box.Padding.Top)
	encodeEdge(tree, index, nodePr, nodePrMode, box.Padding.Right)
	encodeEdge(tree, index, nodePb, nodePbMode, box.Padding.Bottom)
	encodeEdge(tree, index, nodePl, nodePlMode, box.Padding.Left)
	setValue(tree, index, nodeBt, box.Border.Top.Value)
	setValue(tree, index, nodeBr, box.Border.Right.Value)
	setValue(tree, index, nodeBb, box.Border.Bottom.Value)
	setValue(tree, index, nodeBl, box.Border.Left.Value)
	if ratio, ok := model.ParseAspectRatio(style.GetString("aspect-ratio")); ok {
		setValue(tree, index, nodeAspect, ratio)
	}
	setValue(tree, index, nodeFont, box.FontSize)
	setValue(tree, index, nodeLine, box.LineHeight)
	ascent := box.Ascent
	if options.Metrics != nil {
		ascent = options.Metrics(box.Style).Ascent
	}
	if box.Kind == model.TextBox && ascent <= 0 {
		ascent = model.MetricsForStyle(box.Style).Ascent
	}
	setValue(tree, index, nodeAscent, ascent)

	if box.Kind == model.TextBox {
		text := box.Text
		glyphs := model.ShapeText(options.Shaper, text, box.Style)
		total := 0.0
		for _, glyph := range glyphs {
			total += glyph.Advance
		}
		longest := 0.0
		for _, word := range strings.Fields(text) {
			width := 0.0
			for _, glyph := range model.ShapeText(options.Shaper, word, box.Style) {
				width += glyph.Advance
			}
			if width > longest {
				longest = width
			}
		}
		setValue(tree, index, nodeTextW, total)
		setValue(tree, index, nodeWordW, longest)
	}

	if isReplaced(box.Node) {
		flags = int(tree.values[index*nodeFieldCount+nodeFlags2]) | int(tree.values[index*nodeFieldCount+nodeFlags3])<<16
		tree.values[index*nodeFieldCount+nodeFlags2] = float32((flags | flagReplaced) & 0xffff)
		tree.values[index*nodeFieldCount+nodeFlags3] = float32((flags | flagReplaced) >> 16)
		width, height, ok, pending := float64(0), float64(0), false, false
		if options.IntrinsicSize != nil {
			width, height, ok, pending = options.IntrinsicSize(box.Node, box.Style)
		}
		if ok && width > 0 && height > 0 {
			setValue(tree, index, nodeReplW, width)
			setValue(tree, index, nodeReplH, height)
		} else {
			setValue(tree, index, nodeReplW, 300)
			setValue(tree, index, nodeReplH, 150)
		}
		if pending {
			setValue(tree, index, nodeReplFlags, 1)
			source := ""
			if box.Node != nil {
				switch strings.ToLower(box.Node.Data) {
				case "img":
					source, _ = box.Node.GetAttribute("src")
				case "video":
					source, _ = box.Node.GetAttribute("poster")
				}
			}
			tree.pending = append(tree.pending, pendingResource{kind: 1, url: source, node: box.Node})
		}
	}
}

func encodeSize(text string, options backendOptions) (float32, float32, float32) {
	lower := strings.ToLower(strings.TrimSpace(text))
	if strings.Contains(lower, "calc(") && strings.Contains(lower, "%") {
		atZero := model.ParseSizeWithBase(text, 0, options.ViewportWidth, options.ViewportHeight)
		atHundred := model.ParseSizeWithBase(text, 100, options.ViewportWidth, options.ViewportHeight)
		coefficient := (atHundred.Value - atZero.Value) / 100
		offset := atZero.Value
		return 2, float32(coefficient * 100), float32(offset)
	}
	size := model.ParseSizeWithBase(text, 0, options.ViewportWidth, options.ViewportHeight)
	if size.Keyword != "" {
		switch size.Keyword {
		case "min-content":
			return 3, 0, 0
		case "max-content":
			return 4, 0, 0
		case "fit-content":
			return 5, 0, 0
		case "stretch":
			return 6, 0, 0
		}
	}
	if size.Percent {
		return 2, float32(size.Value), 0
	}
	if size.Auto {
		return 0, 0, 0
	}
	return 1, float32(size.Value), 0
}

func encodeMinMax(text string, options backendOptions) (float32, float32, float32) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" || strings.EqualFold(trimmed, "none") || strings.EqualFold(trimmed, "auto") {
		return 2, 0, 0
	}
	if coefficient, offset, ok := encodeCalc(text, options); ok {
		return 1, coefficient * 100, offset
	}
	size := model.ParseSizeWithBase(trimmed, 0, options.ViewportWidth, options.ViewportHeight)
	if size.Percent {
		return 1, float32(size.Value), 0
	}
	if size.Auto || size.Keyword != "" {
		return 2, 0, 0
	}
	return 0, float32(size.Value), 0
}

func encodeCalc(text string, options backendOptions) (float32, float32, bool) {
	lower := strings.ToLower(strings.TrimSpace(text))
	if !strings.Contains(lower, "calc(") || !strings.Contains(lower, "%") {
		return 0, 0, false
	}
	atZero := model.ParseSizeWithBase(text, 0, options.ViewportWidth, options.ViewportHeight)
	atHundred := model.ParseSizeWithBase(text, 100, options.ViewportWidth, options.ViewportHeight)
	coefficient := (atHundred.Value - atZero.Value) / 100
	return float32(coefficient), float32(atZero.Value), true
}

func encodeEdge(tree *encodedTree, index, valueField, modeField int, length model.Length) {
	mode := float32(0)
	if length.Percent {
		mode = 1
	} else if length.Auto {
		mode = 2
	}
	setValue(tree, index, valueField, length.Value)
	setValue(tree, index, modeField, float64(mode))
}

func setValue(tree *encodedTree, index, field int, value float64) {
	if tree.values == nil {
		return
	}
	tree.values[index*nodeFieldCount+field] = float32(value)
}

func isReplaced(node *dom.Node) bool {
	if node == nil || node.Type != dom.ElementNode {
		return false
	}
	switch strings.ToLower(node.Data) {
	case "img", "video", "canvas", "iframe", "object", "embed", "svg":
		return true
	}
	return false
}

func encodeTracks(tree *encodedTree, tracks []trackDescriptor) int {
	start := tree.allocAux(len(tracks) * 8)
	for index, track := range tracks {
		base := start + index*8
		tree.aux[base+0] = float32(track.kind)
		tree.aux[base+1] = float32(track.value)
		tree.aux[base+2] = float32(track.flex)
		tree.aux[base+3] = float32(track.minSize)
		tree.aux[base+4] = float32(track.maxSize)
		if track.hasRange {
			tree.aux[base+5] = 1
		}
	}
	return start
}

func encodeGrid(tree *encodedTree, index int, box *model.Box, style *styles.Style) {
	columns := parseTrackList(style.GetString("grid-template-columns"))
	rows := parseTrackList(style.GetString("grid-template-rows"))
	columnStart := encodeTracks(tree, columns)
	rowStart := encodeTracks(tree, rows)
	setValue(tree, index, nodeColTrackStart, float64(columnStart))
	setValue(tree, index, nodeColTrackCount, float64(len(columns)))
	setValue(tree, index, nodeRowTrackStart, float64(rowStart))
	setValue(tree, index, nodeRowTrackCount, float64(len(rows)))
	columnGap := model.ParseLength(style.GetString("column-gap"))
	rowGap := model.ParseLength(style.GetString("row-gap"))
	if columnGap.Auto {
		columnGap = model.ParseLength(style.GetString("gap"))
	}
	if rowGap.Auto {
		rowGap = model.ParseLength(style.GetString("gap"))
	}
	setValue(tree, index, nodeColGap, columnGap.Value)
	setValue(tree, index, nodeRowGap, rowGap.Value)
	setValue(tree, index, nodeGridJustify, gridAlignmentValue(style.GetString("justify-items")))
	setValue(tree, index, nodeGridAlign, gridAlignmentValue(style.GetString("align-items")))

	maxColumns := len(columns)
	if maxColumns < 1 {
		maxColumns = 1
	}
	maxRows := len(rows)
	if maxRows < 1 {
		maxRows = 1
	}
	if len(box.Children) > maxColumns {
		maxColumns = len(box.Children)
	}
	if len(box.Children) > maxRows {
		maxRows = len(box.Children)
	}
	for _, child := range box.Children {
		if child.Style == nil {
			continue
		}
		column, columnSpan, _ := parseGridSpan(child.Style.GetString("grid-column-start"), child.Style.GetString("grid-column-end"))
		row, rowSpan, _ := parseGridSpan(child.Style.GetString("grid-row-start"), child.Style.GetString("grid-row-end"))
		if column > 0 && column-1+columnSpan > maxColumns {
			maxColumns = column - 1 + columnSpan
		}
		if row > 0 && row-1+rowSpan > maxRows {
			maxRows = row - 1 + rowSpan
		}
	}
	scratch := tree.allocAux(maxColumns + maxRows + maxColumns*maxRows)
	setValue(tree, index, nodeGridScratchStart, float64(scratch))
	setValue(tree, index, nodeGridMaxCols, float64(maxColumns))
	setValue(tree, index, nodeGridMaxRows, float64(maxRows))

	for _, child := range box.Children {
		childIndex := tree.indexOfChild(index, child)
		if childIndex < 0 || child.Style == nil {
			continue
		}
		column, columnSpan, explicitColumn := parseGridSpan(child.Style.GetString("grid-column-start"), child.Style.GetString("grid-column-end"))
		row, rowSpan, explicitRow := parseGridSpan(child.Style.GetString("grid-row-start"), child.Style.GetString("grid-row-end"))
		explicit := 0.0
		if explicitColumn || explicitRow {
			explicit = 1
		}
		setValue(tree, childIndex, nodeGridColStart, float64(column))
		setValue(tree, childIndex, nodeGridColSpan, float64(columnSpan))
		setValue(tree, childIndex, nodeGridRowStart, float64(row))
		setValue(tree, childIndex, nodeGridRowSpan, float64(rowSpan))
		setValue(tree, childIndex, nodeGridExplicit, explicit)
		setValue(tree, childIndex, nodeGridOrder, float64(parseOrder(child.Style.GetString("order"))))
		setValue(tree, childIndex, nodeGridJustifySelf, gridAlignmentValue(child.Style.GetString("justify-self")))
		setValue(tree, childIndex, nodeGridAlignSelf, gridAlignmentValue(child.Style.GetString("align-self")))
	}
}

func (tree *encodedTree) indexOfChild(parent int, child *model.Box) int {
	encoded := tree.nodes[parent]
	if childIndex := encoded.first; childIndex >= 0 {
		for current := childIndex; current >= 0; current = tree.nodes[current].next {
			if tree.nodes[current].box == child {
				return current
			}
		}
	}
	return -1
}

func encodeTable(tree *encodedTree, index int, box *model.Box, style *styles.Style) {
	rows := tableRows(box)
	maxColumns := 1
	maxRows := len(rows)
	for _, row := range rows {
		columns := 0
		for _, cell := range row.Children {
			if cell.Style == nil || !strings.HasPrefix(model.NormalizeDisplay(cell.Style.GetString("display")), "table-cell") {
				continue
			}
			columnSpan, rowSpan := tableCellSpans(cell)
			columns += columnSpan
			if rowSpan > 1 && maxRows < len(rows)+rowSpan {
				maxRows = len(rows) + rowSpan
			}
		}
		if columns > maxColumns {
			maxColumns = columns
		}
	}
	if maxRows < 1 {
		maxRows = 1
	}
	setValue(tree, index, nodeTableMaxCols, float64(maxColumns))
	setValue(tree, index, nodeTableMaxRows, float64(maxRows))
	scratch := tree.allocAux(2*maxColumns + maxRows + maxColumns*maxRows)
	setValue(tree, index, nodeTableScratchStart, float64(scratch))
	collapse := 0.0
	if strings.EqualFold(strings.TrimSpace(style.GetString("border-collapse")), "collapse") {
		collapse = 1
	}
	setValue(tree, index, nodeTableCollapse, collapse)
	for rowIndex, row := range rows {
		rowIndexValue := tree.indexOfBox(row)
		if rowIndexValue >= 0 {
			setValue(tree, rowIndexValue, nodeTableRow, float64(rowIndex))
		}
		for _, cell := range row.Children {
			cellIndex := tree.indexOfBox(cell)
			if cellIndex < 0 || cell.Style == nil {
				continue
			}
			columnSpan, rowSpan := tableCellSpans(cell)
			setValue(tree, cellIndex, nodeTableColSpan, float64(columnSpan))
			setValue(tree, cellIndex, nodeTableRowSpan, float64(rowSpan))
			setValue(tree, cellIndex, nodeTableRow, float64(rowIndex))
			setValue(tree, cellIndex, nodeTableCollapse, collapse)
		}
	}
}

func (tree *encodedTree) indexOfBox(box *model.Box) int {
	for index := range tree.nodes {
		if tree.nodes[index].box == box {
			return index
		}
	}
	return -1
}

func tableRows(box *model.Box) []*model.Box {
	rows := []*model.Box{}
	for _, child := range box.Children {
		if child.Style == nil {
			continue
		}
		display := model.NormalizeDisplay(child.Style.GetString("display"))
		switch display {
		case "table-row":
			rows = append(rows, child)
		case "table-row-group", "table-header-group", "table-footer-group":
			for _, row := range child.Children {
				if row.Style != nil && model.NormalizeDisplay(row.Style.GetString("display")) == "table-row" {
					rows = append(rows, row)
				}
			}
		}
	}
	return rows
}

func tableCellSpans(cell *model.Box) (int, int) {
	columnSpan := 1
	rowSpan := 1
	if cell.Node != nil {
		if value, ok := cell.Node.GetAttribute("colspan"); ok {
			if parsed, err := strconv.Atoi(strings.TrimSpace(value)); err == nil && parsed > 0 {
				columnSpan = parsed
			}
		}
		if value, ok := cell.Node.GetAttribute("rowspan"); ok {
			if parsed, err := strconv.Atoi(strings.TrimSpace(value)); err == nil && parsed > 0 {
				rowSpan = parsed
			}
		}
	}
	return columnSpan, rowSpan
}

func encodeOffset(tree *encodedTree, index, valueField, modeField, offsetField int, text string, options backendOptions) {
	if coefficient, offset, ok := encodeCalc(text, options); ok {
		setValue(tree, index, valueField, float64(coefficient*100))
		setValue(tree, index, modeField, 1)
		setValue(tree, index, offsetField, float64(offset))
		return
	}
	offset := model.ParseOffset(text)
	mode := 0.0
	if offset.Percent {
		mode = 1
	} else if offset.Auto {
		mode = 2
	}
	setValue(tree, index, valueField, offset.Value)
	setValue(tree, index, modeField, mode)
}
