package software

import (
	"gonetsurf/layouting/model"
	"math"
	"sort"
	"strings"

	"gonetsurf/parsers/css/parser"
	"gonetsurf/parsers/css/tokenizer"
	cssvalues "gonetsurf/parsers/css/values"
)

type trackSize struct {
	size     float64
	flex     float64
	auto     bool
	minSize  float64
	maxSize  float64
	hasRange bool
}

type gridItem struct {
	box        *model.Box
	order      int
	column     int
	row        int
	columnSpan int
	rowSpan    int
	explicit   bool
}

func isGridContainer(box *model.Box) bool {
	display := displayOf(box)
	return display == "grid" || display == "inline-grid"
}

func (layout *Layout) layoutGrid(box *model.Box) {
	items := []gridItem{}
	for _, child := range box.Children {
		if isOutOfFlow(child) {
			layout.positioned = append(layout.positioned, child)
			continue
		}
		item := gridItem{box: child, order: parseFlexInteger(child.Style.GetString("order"))}
		column, columnSpan, explicitColumn := gridLine(child.Style.GetString("grid-column-start"), child.Style.GetString("grid-column-end"))
		row, rowSpan, explicitRow := gridLine(child.Style.GetString("grid-row-start"), child.Style.GetString("grid-row-end"))
		item.column = column
		item.columnSpan = columnSpan
		item.row = row
		item.rowSpan = rowSpan
		item.explicit = explicitColumn || explicitRow
		items = append(items, item)
	}
	if len(items) == 0 {
		box.ContentHeight = 0
		return
	}
	sort.SliceStable(items, func(left, right int) bool {
		if items[left].explicit != items[right].explicit {
			return items[left].explicit
		}
		return items[left].order < items[right].order
	})
	columnTracks := layout.parseTracks(box.Style.GetString("grid-template-columns"), box.ContentWidth)
	rowTracks := layout.parseTracks(box.Style.GetString("grid-template-rows"), box.ContentHeight)
	columnCount := len(columnTracks)
	if columnCount == 0 {
		columnCount = 1
	}
	columnAuto := make([]float64, columnCount)
	for index := range items {
		layout.computeIntrinsicWidths(items[index].box)
		column := items[index].column
		if column <= 0 {
			column = 1
		}
		if column-1 < len(columnAuto) {
			width := items[index].box.IntrinsicMaxWidth + cellHorizontalExtra(items[index].box, box.ContentWidth)
			if width > columnAuto[column-1] {
				columnAuto[column-1] = width
			}
		}
	}
	columnGap := model.ParseLength(box.Style.GetString("column-gap")).Resolve(box.ContentWidth)
	rowGap := model.ParseLength(box.Style.GetString("row-gap")).Resolve(box.ContentWidth)
	columnWidths := resolveTracks(columnTracks, box.ContentWidth, columnGap, columnAuto)
	if len(columnWidths) == 0 {
		columnWidths = columnAuto
	}
	if len(columnWidths) == 0 {
		columnWidths = []float64{box.ContentWidth}
	}
	for _, item := range items {
		baseColumn := item.column
		if item.explicit && baseColumn > 0 {
			baseColumn--
		}
		required := baseColumn + item.columnSpan
		for len(columnWidths) < required {
			width := box.ContentWidth / float64(len(columnWidths)+1)
			for len(columnAuto) < len(columnWidths)+1 {
				columnAuto = append(columnAuto, 0)
			}
			if len(columnAuto) > len(columnWidths) && columnAuto[len(columnWidths)] > 0 {
				width = columnAuto[len(columnWidths)]
			}
			columnWidths = append(columnWidths, width)
		}
	}
	columnCount = len(columnWidths)
	occupied := map[[2]int]bool{}
	column := 0
	row := 0
	for index := range items {
		item := &items[index]
		if item.explicit {
			if item.column > 0 {
				item.column--
			} else {
				item.column = 0
			}
			if item.row > 0 {
				item.row--
			} else {
				item.row = 0
			}
			if item.columnSpan < 1 {
				item.columnSpan = 1
			}
			if item.rowSpan < 1 {
				item.rowSpan = 1
			}
			for currentRow := item.row; currentRow < item.row+item.rowSpan; currentRow++ {
				for currentColumn := item.column; currentColumn < item.column+item.columnSpan; currentColumn++ {
					occupied[[2]int{currentRow, currentColumn}] = true
				}
			}
			continue
		}
		if item.columnSpan < 1 {
			item.columnSpan = 1
		}
		if item.rowSpan < 1 {
			item.rowSpan = 1
		}
		for {
			if column+item.columnSpan > columnCount {
				row++
				column = 0
				continue
			}
			free := true
			for currentRow := row; currentRow < row+item.rowSpan; currentRow++ {
				for currentColumn := column; currentColumn < column+item.columnSpan; currentColumn++ {
					if occupied[[2]int{currentRow, currentColumn}] {
						free = false
					}
				}
			}
			if free {
				item.column = column
				item.row = row
				for currentRow := row; currentRow < row+item.rowSpan; currentRow++ {
					for currentColumn := column; currentColumn < column+item.columnSpan; currentColumn++ {
						occupied[[2]int{currentRow, currentColumn}] = true
					}
				}
				column += item.columnSpan
				break
			}
			column++
		}
	}
	temporaryY := box.ContentY
	for index := range items {
		item := &items[index]
		width := trackSpanWidth(columnWidths, item.column, item.columnSpan, columnGap)
		x := box.ContentX + trackOffset(columnWidths, item.column, columnGap)
		layout.layoutGridItem(box, item.box, x, temporaryY, width)
	}
	rowCount := 0
	for _, item := range items {
		if item.row+item.rowSpan > rowCount {
			rowCount = item.row + item.rowSpan
		}
	}
	rowHeights := make([]float64, rowCount)
	for _, item := range items {
		if item.rowSpan == 1 && item.box.Height > rowHeights[item.row] {
			rowHeights[item.row] = item.box.Height
		}
	}
	resolvedRows := resolveTracks(rowTracks, box.ContentHeight, rowGap, rowHeights)
	if len(resolvedRows) < rowCount {
		resolvedRows = append(resolvedRows, make([]float64, rowCount-len(resolvedRows))...)
	}
	for index := range items {
		item := &items[index]
		targetY := box.ContentY + trackOffset(resolvedRows, item.row, rowGap)
		if item.rowSpan <= 1 && item.row < len(resolvedRows) {
			rowHeight := resolvedRows[item.row]
			switch gridAlignment(box, item.box, "align") {
			case "center":
				targetY += (rowHeight - item.box.Height) / 2
			case "end", "flex-end", "last baseline":
				targetY += rowHeight - item.box.Height
			case "stretch":
				if model.ParseSize(item.box.Style.GetString("height")).Auto {
					extra := rowHeight - item.box.Height
					item.box.Height = rowHeight
					item.box.ContentHeight += extra
				}
			}
		}
		deltaY := targetY - temporaryY
		if deltaY != 0 {
			shiftBox(item.box, 0, deltaY)
		}
	}
	totalHeight := 0.0
	for index := 0; index < rowCount; index++ {
		totalHeight += resolvedRows[index]
		if index < rowCount-1 {
			totalHeight += rowGap
		}
	}
	box.ContentHeight = totalHeight
}

func (layout *Layout) layoutGridItem(container, item *model.Box, x, y, width float64) {
	justify := gridAlignment(container, item, "justify")
	itemWidth := width
	if justify != "" && justify != "stretch" && justify != "normal" {
		size := model.ParseSize(item.Style.GetString("width"))
		if size.Auto || size.Keyword != "" {
			layout.computeIntrinsicWidths(item)
			itemWidth = math.Max(item.IntrinsicMinWidth, math.Min(width, item.IntrinsicMaxWidth))
		} else {
			itemWidth = math.Max(0, math.Min(width, layout.resolveWidth(item, width, 0, 0)))
		}
		remaining := width - itemWidth
		switch justify {
		case "center":
			x += remaining / 2
		case "end", "right":
			x += remaining
		}
	}
	width = itemWidth
	paddingLeft := item.Padding.Left.Resolve(width)
	paddingRight := item.Padding.Right.Resolve(width)
	paddingTop := item.Padding.Top.Resolve(width)
	paddingBottom := item.Padding.Bottom.Resolve(width)
	borderLeft := item.Border.Left.Resolve(width)
	borderRight := item.Border.Right.Resolve(width)
	borderTop := item.Border.Top.Resolve(width)
	borderBottom := item.Border.Bottom.Resolve(width)
	horizontalExtra := paddingLeft + paddingRight + borderLeft + borderRight
	verticalExtra := paddingTop + paddingBottom + borderTop + borderBottom
	item.ContentWidth = math.Max(0, width-horizontalExtra)
	item.Width = width
	item.X = x
	item.ContentX = x + borderLeft + paddingLeft
	item.Y = y
	item.ContentY = y + borderTop + paddingTop
	if !model.ParseSize(item.Style.GetString("height")).Auto {
		item.ContentHeight = layout.resolveContentHeight(item, 0, 0, verticalExtra)
		item.Height = item.ContentHeight + verticalExtra
		return
	}
	layout.layoutFlexItemContent(item)
	item.Height = item.ContentHeight + verticalExtra
}

func gridAlignment(container, item *model.Box, axis string) string {
	selfProperty := "align-self"
	itemsProperty := "align-items"
	if axis == "justify" {
		selfProperty = "justify-self"
		itemsProperty = "justify-items"
	}
	value := strings.ToLower(strings.TrimSpace(item.Style.GetString(selfProperty)))
	if value == "" || value == "auto" {
		value = strings.ToLower(strings.TrimSpace(container.Style.GetString(itemsProperty)))
	}
	switch value {
	case "center", "end", "start", "stretch", "flex-start", "flex-end", "normal", "right", "left":
		return value
	}
	return "stretch"
}

func (layout *Layout) parseTracks(text string, available float64) []trackSize {
	values := parser.ParseComponentValueList(text)
	tracks := []trackSize{}
	for _, value := range values {
		if parser.IsWhitespace(value) {
			continue
		}
		if function, ok := value.(*parser.Function); ok && strings.EqualFold(function.Name, "minmax") {
			if track, ok := parseMinMaxTrack(function.Value, available); ok {
				tracks = append(tracks, track)
			}
			continue
		}
		if function, ok := value.(*parser.Function); ok && strings.EqualFold(function.Name, "repeat") {
			groups := splitCommas(function.Value)
			if len(groups) != 2 {
				continue
			}
			countToken, ok := parser.AsToken(groups[0][0])
			if !ok || countToken.Type != tokenizer.TokenNumber {
				continue
			}
			count := int(countToken.Number)
			inner := layout.parseTracks(parser.SerializeValues(groups[1]), available)
			for index := 0; index < count; index++ {
				tracks = append(tracks, inner...)
			}
			continue
		}
		if track, ok := parseTrack(value, available); ok {
			tracks = append(tracks, track)
		}
	}
	return tracks
}

func parseTrack(value parser.ComponentValue, available float64) (trackSize, bool) {
	token, ok := parser.AsToken(value)
	if !ok {
		return trackSize{}, false
	}
	if token.Type == tokenizer.TokenIdent {
		switch strings.ToLower(token.Value) {
		case "auto", "min-content", "max-content", "fit-content":
			return trackSize{auto: true}, true
		}
	}
	numeric, ok := cssvalues.ParseNumeric(value)
	if !ok {
		return trackSize{}, false
	}
	switch numeric.Kind {
	case cssvalues.Flex:
		return trackSize{flex: numeric.Number}, true
	case cssvalues.Length:
		return trackSize{size: numeric.Number}, true
	case cssvalues.Other:
		if numeric.Unit == "%" {
			return trackSize{size: available * numeric.Number}, true
		}
		return trackSize{size: numeric.Number}, true
	}
	return trackSize{}, false
}

func resolveTracks(tracks []trackSize, available, gap float64, autoSizes []float64) []float64 {
	if len(tracks) == 0 {
		return append([]float64(nil), autoSizes...)
	}
	widths := make([]float64, len(tracks))
	fixed := 0.0
	flexTotal := 0.0
	autoCount := 0
	for index, track := range tracks {
		switch {
		case track.flex > 0:
			flexTotal += track.flex
		case track.auto:
			autoCount++
		default:
			widths[index] = math.Max(track.minSize, track.size)
			fixed += widths[index]
		}
	}
	free := available - fixed
	if len(tracks) > 1 {
		free -= gap * float64(len(tracks)-1)
	}
	if free < 0 {
		free = 0
	}
	if flexTotal > 0 {
		for index, track := range tracks {
			if track.flex > 0 {
				widths[index] = math.Max(track.minSize, free*track.flex/flexTotal)
				if track.hasRange && track.maxSize > 0 && widths[index] > track.maxSize {
					widths[index] = track.maxSize
				}
			}
		}
		return widths
	}
	if autoCount > 0 {
		for index, track := range tracks {
			if !track.auto {
				continue
			}
			if index < len(autoSizes) && autoSizes[index] > 0 {
				widths[index] = autoSizes[index]
			} else if autoCount > 0 {
				widths[index] = free / float64(autoCount)
			}
			if widths[index] < track.minSize {
				widths[index] = track.minSize
			}
			if track.hasRange && track.maxSize > 0 && widths[index] > track.maxSize {
				widths[index] = track.maxSize
			}
		}
	}
	return widths
}

func parseMinMaxTrack(values []parser.ComponentValue, available float64) (trackSize, bool) {
	groups := splitCommas(values)
	if len(groups) != 2 {
		return trackSize{}, false
	}
	minimum, ok := parseTrackArgument(groups[0], available)
	if !ok {
		return trackSize{}, false
	}
	maximum, ok := parseTrackArgument(groups[1], available)
	if !ok {
		return trackSize{}, false
	}
	track := trackSize{minSize: minimum.size, hasRange: true}
	if minimum.auto {
		track.auto = true
	}
	switch {
	case maximum.flex > 0:
		track.flex = maximum.flex
	case maximum.auto:
		track.auto = true
	default:
		track.size = maximum.size
		track.maxSize = maximum.size
	}
	return track, true
}

func parseTrackArgument(values []parser.ComponentValue, available float64) (trackSize, bool) {
	for _, value := range values {
		if parser.IsWhitespace(value) {
			continue
		}
		return parseTrack(value, available)
	}
	return trackSize{}, false
}

func gridLine(startText, endText string) (start int, span int, explicit bool) {
	start, startSpan := parseGridLine(startText)
	end, endSpan := parseGridLine(endText)
	explicit = start > 0 || end > 0
	if start > 0 && end > 0 && end > start {
		return start, end - start, explicit
	}
	if start > 0 && endSpan > 0 {
		return start, endSpan, explicit
	}
	if startSpan > 0 {
		return start, startSpan, explicit
	}
	if endSpan > 0 {
		return 0, endSpan, explicit
	}
	if start > 0 {
		return start, 1, explicit
	}
	return 0, 1, explicit
}

func parseGridLine(text string) (int, int) {
	values := parser.ParseComponentValueList(strings.TrimSpace(text))
	if len(values) == 0 {
		return 0, 0
	}
	if token, ok := parser.AsToken(values[0]); ok {
		if token.Type == tokenizer.TokenIdent && strings.EqualFold(token.Value, "auto") {
			return 0, 1
		}
		if token.Type == tokenizer.TokenIdent && strings.EqualFold(token.Value, "span") {
			for _, value := range values[1:] {
				if parser.IsWhitespace(value) {
					continue
				}
				if next, ok := parser.AsToken(value); ok && next.Type == tokenizer.TokenNumber {
					return 0, int(next.Number)
				}
				break
			}
		}
		if token.Type == tokenizer.TokenNumber && token.NumericKind == tokenizer.Integer {
			return int(token.Number), 1
		}
	}
	return 0, 1
}

func trackOffset(widths []float64, index int, gap float64) float64 {
	offset := 0.0
	for current := 0; current < index && current < len(widths); current++ {
		offset += widths[current] + gap
	}
	return offset
}

func trackSpanWidth(widths []float64, index, span int, gap float64) float64 {
	width := 0.0
	for current := index; current < index+span && current < len(widths); current++ {
		width += widths[current]
		if current > index {
			width += gap
		}
	}
	return width
}

func splitCommas(values []parser.ComponentValue) [][]parser.ComponentValue {
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
