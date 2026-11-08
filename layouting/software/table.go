package software

import (
	"gonetsurf/layouting/model"
	"math"
	"strconv"
	"strings"
)

type tableCellInfo struct {
	box     *model.Box
	row     int
	column  int
	colSpan int
	rowSpan int
}

func isTableContainer(box *model.Box) bool {
	display := displayOf(box)
	return display == "table" || display == "inline-table"
}

func displayOf(box *model.Box) string {
	if box == nil || box.Style == nil {
		return ""
	}
	return model.NormalizeDisplay(box.Style.GetString("display"))
}

func (layout *Layout) layoutTable(box *model.Box) {
	rows := collectTableRows(box)
	if len(rows) == 0 {
		box.ContentHeight = 0
		return
	}
	cells, columnCount := buildTableGrid(rows)
	if columnCount == 0 {
		box.ContentHeight = 0
		return
	}
	collapse := isCollapsedTable(box)
	minWidths := make([]float64, columnCount)
	maxWidths := make([]float64, columnCount)
	fixed := make([]bool, columnCount)
	applyColumnWidths(box, minWidths, maxWidths, fixed, box.ContentWidth)
	for _, cell := range cells {
		layout.computeIntrinsicWidths(cell.box)
		horizontalExtra := cellHorizontalExtra(cell.box, box.ContentWidth, collapse)
		minimum := cell.box.IntrinsicMinWidth + horizontalExtra
		maximum := cell.box.IntrinsicMaxWidth + horizontalExtra
		if cell.colSpan <= 1 {
			column := cell.column
			if column >= 0 && column < columnCount {
				if !fixed[column] && minimum > minWidths[column] {
					minWidths[column] = minimum
				}
				if !fixed[column] && maximum > maxWidths[column] {
					maxWidths[column] = maximum
				}
			}
			continue
		}
		distributeSpanWidths(minWidths, maxWidths, fixed, cell.column, cell.colSpan, minimum, maximum)
	}
	for index := range maxWidths {
		if maxWidths[index] < minWidths[index] {
			maxWidths[index] = minWidths[index]
		}
	}
	tableExtra := box.Width - box.ContentWidth
	width := box.ContentWidth
	if model.ParseSize(box.Style.GetString("width")).Auto {
		total := 0.0
		for _, maximum := range maxWidths {
			total += maximum
		}
		if total > 0 {
			width = total
			box.ContentWidth = width
			box.Width = width + tableExtra
		}
	}
	widths := distributeTableWidths(minWidths, maxWidths, width)
	cursorY := box.ContentY
	rowHeights := make([]float64, len(rows))
	rowY := make([]float64, len(rows))
	for rowIndex, row := range rows {
		rowY[rowIndex] = cursorY
		rowHeight := 0.0
		for _, cell := range cellsForRow(cells, rowIndex) {
			cellWidth := spanWidth(widths, cell.column, cell.colSpan)
			cellX := box.ContentX + tableColumnOffset(widths, cell.column)
			layout.layoutTableCell(cell.box, cellX, cursorY, cellWidth, collapse)
			if cell.box.Height > rowHeight {
				rowHeight = cell.box.Height
			}
		}
		if size := model.ParseSize(row.Style.GetString("height")); !size.Auto && size.Keyword == "" {
			minimum := size.Value
			if size.Percent {
				minimum = box.ContentHeight * size.Value / 100
			}
			if minimum > rowHeight {
				rowHeight = minimum
			}
		}
		rowHeights[rowIndex] = rowHeight
		cursorY += rowHeight
	}
	for _, cell := range cells {
		rowHeight := 0.0
		if cell.row >= 0 && cell.row < len(rowHeights) {
			rowHeight = rowHeights[cell.row]
		}
		if cell.rowSpan > 1 {
			for index := 1; index < cell.rowSpan && cell.row+index < len(rowHeights); index++ {
				rowHeight += rowHeights[cell.row+index]
			}
			if rowHeight > cell.box.Height {
				cell.box.ContentHeight += rowHeight - cell.box.Height
				cell.box.Height = rowHeight
			}
			continue
		}
		offset := tableVerticalOffset(cell.box, rowHeight)
		if offset != 0 {
			shiftBox(cell.box, 0, offset)
		}
	}
	for rowIndex, row := range rows {
		row.X = box.ContentX
		row.ContentX = box.ContentX
		row.Y = rowY[rowIndex]
		row.ContentY = rowY[rowIndex]
		row.Width = box.ContentWidth
		row.ContentWidth = box.ContentWidth
		row.Height = rowHeights[rowIndex]
		row.ContentHeight = rowHeights[rowIndex]
	}
	for _, child := range box.Children {
		switch displayOf(child) {
		case "table-row-group", "table-header-group", "table-footer-group":
			layoutRowGroup(child, rows, rowY, rowHeights, box.ContentX, box.ContentWidth)
		}
	}
	if len(rows) > 0 {
		box.ContentHeight = math.Max(0, cursorY-box.ContentY)
	}
}

func collectTableRows(box *model.Box) []*model.Box {
	rows := []*model.Box{}
	for _, child := range box.Children {
		if isOutOfFlow(child) {
			continue
		}
		switch displayOf(child) {
		case "table-row":
			rows = append(rows, child)
		case "table-row-group", "table-header-group", "table-footer-group":
			for _, row := range child.Children {
				if displayOf(row) == "table-row" {
					rows = append(rows, row)
					continue
				}
				if isTableCell(row) {
					rows = append(rows, anonymousTableRow(box, []*model.Box{row}))
				}
			}
		case "table-cell":
			rows = append(rows, anonymousTableRow(box, []*model.Box{child}))
		}
	}
	return rows
}

func anonymousTableRow(table *model.Box, cells []*model.Box) *model.Box {
	row := &model.Box{
		Style:    table.Style,
		Kind:     model.BlockBox,
		Children: cells,
	}
	table.Children = append(table.Children, row)
	for _, cell := range cells {
		cell.Parent = row
	}
	return row
}

func isTableCell(box *model.Box) bool {
	return displayOf(box) == "table-cell"
}

func buildTableGrid(rows []*model.Box) ([]tableCellInfo, int) {
	cells := []tableCellInfo{}
	occupied := map[[2]int]bool{}
	columnCount := 0
	for rowIndex, row := range rows {
		column := 0
		for _, cell := range row.Children {
			if !isTableCell(cell) {
				continue
			}
			for occupied[[2]int{rowIndex, column}] {
				column++
			}
			colSpan := tableSpanAttribute(cell, "colspan")
			rowSpan := tableSpanAttribute(cell, "rowspan")
			if colSpan < 1 {
				colSpan = 1
			}
			if rowSpan < 1 {
				rowSpan = 1
			}
			cells = append(cells, tableCellInfo{
				box:     cell,
				row:     rowIndex,
				column:  column,
				colSpan: colSpan,
				rowSpan: rowSpan,
			})
			for currentRow := rowIndex; currentRow < rowIndex+rowSpan; currentRow++ {
				for currentColumn := column; currentColumn < column+colSpan; currentColumn++ {
					occupied[[2]int{currentRow, currentColumn}] = true
					if currentColumn+1 > columnCount {
						columnCount = currentColumn + 1
					}
				}
			}
			column += colSpan
		}
	}
	return cells, columnCount
}

func tableSpanAttribute(cell *model.Box, name string) int {
	if cell == nil || cell.Node == nil {
		return 1
	}
	value, ok := cell.Node.GetAttribute(name)
	if !ok {
		return 1
	}
	span, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || span < 1 {
		return 1
	}
	return span
}

func applyColumnWidths(table *model.Box, minWidths, maxWidths []float64, fixed []bool, available float64) {
	column := 0
	for _, child := range table.Children {
		if displayOf(child) == "table-column-group" {
			for _, columnBox := range child.Children {
				if displayOf(columnBox) == "table-column" {
					column = applyColumnWidth(columnBox, column, minWidths, maxWidths, fixed, available)
				}
			}
			continue
		}
		if displayOf(child) == "table-column" {
			column = applyColumnWidth(child, column, minWidths, maxWidths, fixed, available)
		}
	}
}

func applyColumnWidth(columnBox *model.Box, column int, minWidths, maxWidths []float64, fixed []bool, available float64) int {
	span := tableSpanAttribute(columnBox, "span")
	if span < 1 {
		span = 1
	}
	size := model.ParseSize(columnBox.Style.GetString("width"))
	if size.Auto || size.Keyword != "" {
		return column + span
	}
	value := size.Value
	if size.Percent {
		value = available * size.Value / 100
	}
	for index := 0; index < span && column+index < len(minWidths); index++ {
		minWidths[column+index] = value / float64(span)
		maxWidths[column+index] = value / float64(span)
		fixed[column+index] = true
	}
	return column + span
}

func distributeSpanWidths(minWidths, maxWidths []float64, fixed []bool, column, span int, minimum, maximum float64) {
	if column < 0 || span < 1 {
		return
	}
	end := column + span
	if end > len(minWidths) {
		end = len(minWidths)
	}
	if end <= column {
		return
	}
	totalMin := 0.0
	totalMax := 0.0
	flexible := 0
	for index := column; index < end; index++ {
		totalMin += minWidths[index]
		totalMax += maxWidths[index]
		if !fixed[index] {
			flexible++
		}
	}
	if flexible == 0 {
		return
	}
	if minimum > totalMin {
		extra := (minimum - totalMin) / float64(flexible)
		for index := column; index < end; index++ {
			if !fixed[index] {
				minWidths[index] += extra
			}
		}
	}
	if maximum > totalMax {
		extra := (maximum - totalMax) / float64(flexible)
		for index := column; index < end; index++ {
			if !fixed[index] {
				maxWidths[index] += extra
			}
		}
	}
}

func cellsForRow(cells []tableCellInfo, row int) []tableCellInfo {
	result := []tableCellInfo{}
	for _, cell := range cells {
		if cell.row == row {
			result = append(result, cell)
		}
	}
	return result
}

func tableColumnOffset(widths []float64, column int) float64 {
	offset := 0.0
	for index := 0; index < column && index < len(widths); index++ {
		offset += widths[index]
	}
	return offset
}

func spanWidth(widths []float64, column, span int) float64 {
	width := 0.0
	for index := 0; index < span && column+index < len(widths); index++ {
		width += widths[column+index]
	}
	return width
}

func isCollapsedTable(box *model.Box) bool {
	return strings.EqualFold(strings.TrimSpace(box.Style.GetString("border-collapse")), "collapse")
}

func (layout *Layout) layoutTableCell(cell *model.Box, x, y, width float64, collapse bool) {
	marginLeft := cell.Margin.Left.Resolve(width)
	marginTop := cell.Margin.Top.Resolve(width)
	paddingLeft := cell.Padding.Left.Resolve(width)
	paddingRight := cell.Padding.Right.Resolve(width)
	paddingTop := cell.Padding.Top.Resolve(width)
	paddingBottom := cell.Padding.Bottom.Resolve(width)
	borderLeft := cellBorder(cell.Border.Left.Resolve(width), collapse)
	borderRight := cellBorder(cell.Border.Right.Resolve(width), collapse)
	borderTop := cellBorder(cell.Border.Top.Resolve(width), collapse)
	borderBottom := cellBorder(cell.Border.Bottom.Resolve(width), collapse)
	horizontalExtra := paddingLeft + paddingRight + borderLeft + borderRight
	verticalExtra := paddingTop + paddingBottom + borderTop + borderBottom
	cell.ContentWidth = math.Max(0, width-horizontalExtra)
	cell.Width = width
	cell.X = x + marginLeft
	cell.ContentX = cell.X + borderLeft + paddingLeft
	cell.Y = y + marginTop
	cell.ContentY = cell.Y + borderTop + paddingTop
	layout.layoutFlexItemContent(cell)
	cell.ContentHeight = layout.resolveContentHeight(cell, cell.ContentHeight, 0, verticalExtra)
	cell.Height = cell.ContentHeight + verticalExtra
}

func cellBorder(width float64, collapse bool) float64 {
	if collapse {
		return width / 2
	}
	return width
}

func tableVerticalOffset(cell *model.Box, rowHeight float64) float64 {
	switch strings.ToLower(strings.TrimSpace(cell.Style.GetString("vertical-align"))) {
	case "middle", "center":
		return (rowHeight - cell.Height) / 2
	case "bottom":
		return rowHeight - cell.Height
	}
	return 0
}

func layoutRowGroup(group *model.Box, rows []*model.Box, rowY, rowHeights []float64, x, width float64) {
	first := -1
	last := -1
	for index, row := range rows {
		if row.Parent != group {
			continue
		}
		if first == -1 {
			first = index
		}
		last = index
	}
	if first == -1 {
		return
	}
	group.X = x
	group.ContentX = x
	group.Width = width
	group.ContentWidth = width
	group.Y = rowY[first]
	group.ContentY = rowY[first]
	group.ContentHeight = rowY[last] + rowHeights[last] - rowY[first]
	group.Height = group.ContentHeight
}

func tableCells(row *model.Box) []*model.Box {
	cells := []*model.Box{}
	for _, child := range row.Children {
		if displayOf(child) == "table-cell" {
			cells = append(cells, child)
		}
	}
	return cells
}

func cellHorizontalExtra(cell *model.Box, base float64, collapse ...bool) float64 {
	borderScale := 1.0
	if len(collapse) > 0 && collapse[0] {
		borderScale = 0.5
	}
	return cell.Padding.Horizontal(base) + cell.Border.Horizontal(base)*borderScale + cell.Margin.Horizontal(base)
}

func distributeTableWidths(minWidths, maxWidths []float64, width float64) []float64 {
	widths := make([]float64, len(minWidths))
	totalMin := 0.0
	totalMax := 0.0
	for index := range minWidths {
		totalMin += minWidths[index]
		totalMax += maxWidths[index]
	}
	if totalMax <= 0 {
		if len(widths) == 0 {
			return widths
		}
		for index := range widths {
			widths[index] = width / float64(len(widths))
		}
		return widths
	}
	switch {
	case width >= totalMax:
		extra := width - totalMax
		for index := range widths {
			widths[index] = maxWidths[index] + extra*maxWidths[index]/totalMax
		}
	case width <= totalMin:
		for index := range widths {
			widths[index] = minWidths[index] * width / totalMin
		}
	default:
		delta := totalMax - totalMin
		for index := range widths {
			widths[index] = minWidths[index] + (maxWidths[index]-minWidths[index])*(width-totalMin)/delta
		}
	}
	return widths
}
