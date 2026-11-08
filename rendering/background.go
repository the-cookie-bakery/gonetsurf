package rendering

import (
	"gonetsurf/rendering/interfaces"
	"image"
	"math"
	"strings"

	"gonetsurf/layouts"
	"gonetsurf/parsers/css/parser"
	"gonetsurf/parsers/css/tokenizer"
	"gonetsurf/styles"
)

type lengthValue struct {
	value   float64
	percent bool
	auto    bool
}

func (length lengthValue) resolve(base float64) float64 {
	if length.auto {
		return 0
	}
	if length.percent {
		return base * length.value / 100
	}
	return length.value
}

type backgroundImage struct {
	resource string
	gradient *gradientSpec
}

type backgroundLayerPaint struct {
	image      string
	gradient   *gradientSpec
	repeatX    bool
	repeatY    bool
	positionX  lengthValue
	positionY  lengthValue
	sizeWidth  lengthValue
	sizeHeight lengthValue
	cover      bool
	contain    bool
	origin     string
	clip       string
	attachment string
}

func parseBackgroundLayers(style *styles.Style) []backgroundLayerPaint {
	if style == nil {
		return nil
	}
	currentColor, ok := ParseColor(style.GetString("color"))
	if !ok {
		currentColor = NewColor(0, 0, 0)
	}
	images := parseBackgroundImageList(style.Get("background-image"), currentColor)
	if len(images) == 0 {
		return nil
	}
	positions := splitCommaGroups(style.Get("background-position"))
	sizes := splitCommaGroups(style.Get("background-size"))
	repeats := splitCommaGroups(style.Get("background-repeat"))
	origins := splitCommaGroups(style.Get("background-origin"))
	clips := splitCommaGroups(style.Get("background-clip"))
	attachments := splitCommaGroups(style.Get("background-attachment"))
	fontSize := 16.0
	if value, ok := parseCSSPixelValue(style.GetString("font-size")); ok && value > 0 {
		fontSize = value
	}

	layers := make([]backgroundLayerPaint, len(images))
	for index, image := range images {
		layer := backgroundLayerPaint{
			image:      image.resource,
			gradient:   image.gradient,
			repeatX:    true,
			repeatY:    true,
			positionX:  lengthValue{value: 0, percent: true},
			positionY:  lengthValue{value: 0, percent: true},
			sizeWidth:  lengthValue{auto: true},
			sizeHeight: lengthValue{auto: true},
			origin:     "padding-box",
			clip:       "border-box",
			attachment: "scroll",
		}
		if index < len(positions) && len(positions[index]) > 0 {
			layer.positionX, layer.positionY = parseBackgroundPosition(positions[index], fontSize)
		}
		if index < len(sizes) && len(sizes[index]) > 0 {
			layer.sizeWidth, layer.sizeHeight, layer.cover, layer.contain = parseBackgroundSize(sizes[index], fontSize)
		}
		if index < len(repeats) && len(repeats[index]) > 0 {
			layer.repeatX, layer.repeatY = parseBackgroundRepeat(repeats[index])
		}
		if index < len(origins) && len(origins[index]) > 0 {
			if area := parseBoxArea(origins[index], "padding-box"); area != "" {
				layer.origin = area
			}
		}
		if index < len(clips) && len(clips[index]) > 0 {
			if area := parseBoxArea(clips[index], "border-box"); area != "" {
				layer.clip = area
			}
		}
		if index < len(attachments) && len(attachments[index]) > 0 {
			for _, value := range attachments[index] {
				token, ok := parser.AsToken(value)
				if !ok || token.Type != tokenizer.TokenIdent {
					continue
				}
				switch strings.ToLower(token.Value) {
				case "scroll", "fixed", "local":
					layer.attachment = strings.ToLower(token.Value)
				}
			}
		}
		layers[index] = layer
	}
	return layers
}

func parseBackgroundImageList(values []parser.ComponentValue, currentColor Color) []backgroundImage {
	groups := splitCommaGroups(values)
	if len(groups) == 0 {
		return nil
	}
	images := make([]backgroundImage, 0, len(groups))
	for _, group := range groups {
		image := backgroundImage{}
		for _, value := range group {
			if parser.IsWhitespace(value) {
				continue
			}
			if token, ok := parser.AsToken(value); ok {
				switch token.Type {
				case tokenizer.TokenURL:
					image.resource = "url(" + token.Value + ")"
				case tokenizer.TokenIdent:
					if strings.EqualFold(token.Value, "none") {
						image.resource = ""
						image.gradient = nil
					}
				}
				continue
			}
			if function, ok := value.(*parser.Function); ok {
				name := strings.ToLower(function.Name)
				if strings.Contains(name, "gradient") {
					if gradient, ok := parseGradientFunction(function, currentColor); ok {
						image.gradient = gradient
						image.resource = ""
						continue
					}
				}
				image.resource = parser.SerializeValues([]parser.ComponentValue{function})
			}
		}
		images = append(images, image)
	}
	return images
}

func parseBackgroundPosition(values []parser.ComponentValue, fontSize float64) (lengthValue, lengthValue) {
	groups := nonWhitespaceValues(values)
	xPos := lengthValue{value: 50, percent: true}
	yPos := lengthValue{value: 50, percent: true}
	if len(groups) == 0 {
		return lengthValue{value: 0, percent: true}, lengthValue{value: 0, percent: true}
	}
	if len(groups) == 1 {
		if length, ok := parsePositionValue(groups[0], fontSize); ok {
			if isVerticalKeyword(groups[0]) {
				yPos = length
				xPos = lengthValue{value: 50, percent: true}
			} else {
				xPos = length
				yPos = lengthValue{value: 50, percent: true}
			}
		}
		return xPos, yPos
	}
	if length, ok := parsePositionValue(groups[0], fontSize); ok {
		xPos = length
	}
	if length, ok := parsePositionValue(groups[len(groups)-1], fontSize); ok {
		yPos = length
	}
	return xPos, yPos
}

func parsePositionValue(value parser.ComponentValue, fontSize float64) (lengthValue, bool) {
	if token, ok := parser.AsToken(value); ok && token.Type == tokenizer.TokenIdent {
		switch strings.ToLower(token.Value) {
		case "left", "top":
			return lengthValue{value: 0, percent: true}, true
		case "center":
			return lengthValue{value: 50, percent: true}, true
		case "right", "bottom":
			return lengthValue{value: 100, percent: true}, true
		}
	}
	return parseLengthValue(value, fontSize)
}

func isVerticalKeyword(value parser.ComponentValue) bool {
	token, ok := parser.AsToken(value)
	if !ok || token.Type != tokenizer.TokenIdent {
		return false
	}
	lower := strings.ToLower(token.Value)
	return lower == "top" || lower == "bottom"
}

func parseBackgroundSize(values []parser.ComponentValue, fontSize float64) (lengthValue, lengthValue, bool, bool) {
	groups := nonWhitespaceValues(values)
	if len(groups) == 0 {
		return lengthValue{auto: true}, lengthValue{auto: true}, false, false
	}
	if token, ok := parser.AsToken(groups[0]); ok && token.Type == tokenizer.TokenIdent {
		switch strings.ToLower(token.Value) {
		case "cover":
			return lengthValue{auto: true}, lengthValue{auto: true}, true, false
		case "contain":
			return lengthValue{auto: true}, lengthValue{auto: true}, false, true
		}
	}
	width := lengthValue{auto: true}
	height := lengthValue{auto: true}
	if length, ok := parseLengthValue(groups[0], fontSize); ok {
		width = length
	} else if token, ok := parser.AsToken(groups[0]); ok && token.Type == tokenizer.TokenIdent && strings.EqualFold(token.Value, "auto") {
		width = lengthValue{auto: true}
	}
	if len(groups) > 1 {
		if length, ok := parseLengthValue(groups[1], fontSize); ok {
			height = length
		} else if token, ok := parser.AsToken(groups[1]); ok && token.Type == tokenizer.TokenIdent && strings.EqualFold(token.Value, "auto") {
			height = lengthValue{auto: true}
		}
	}
	return width, height, false, false
}

func parseBackgroundRepeat(values []parser.ComponentValue) (bool, bool) {
	groups := nonWhitespaceValues(values)
	repeatX := true
	repeatY := true
	if len(groups) == 0 {
		return repeatX, repeatY
	}
	if token, ok := parser.AsToken(groups[0]); ok && token.Type == tokenizer.TokenIdent {
		switch strings.ToLower(token.Value) {
		case "repeat-x":
			return true, false
		case "repeat-y":
			return false, true
		case "no-repeat":
			return false, false
		case "space", "round", "repeat":
			repeatX, repeatY = true, true
		}
	}
	if len(groups) > 1 {
		repeatX = repeatKeyword(groups[0])
		repeatY = repeatKeyword(groups[1])
	}
	return repeatX, repeatY
}

func repeatKeyword(value parser.ComponentValue) bool {
	token, ok := parser.AsToken(value)
	if !ok || token.Type != tokenizer.TokenIdent {
		return true
	}
	return strings.ToLower(token.Value) != "no-repeat"
}

func parseBoxArea(values []parser.ComponentValue, fallback string) string {
	for _, value := range values {
		token, ok := parser.AsToken(value)
		if !ok || token.Type != tokenizer.TokenIdent {
			continue
		}
		switch strings.ToLower(token.Value) {
		case "border-box", "padding-box", "content-box":
			return strings.ToLower(token.Value)
		}
	}
	return fallback
}

func parseLengthValue(value parser.ComponentValue, fontSize float64) (lengthValue, bool) {
	token, ok := parser.AsToken(value)
	if !ok {
		return lengthValue{}, false
	}
	switch token.Type {
	case tokenizer.TokenPercentage:
		return lengthValue{value: token.Number, percent: true}, true
	case tokenizer.TokenNumber:
		return lengthValue{value: token.Number}, true
	case tokenizer.TokenDimension:
		unit := strings.ToLower(token.Value)
		factor := 1.0
		switch unit {
		case "px":
			factor = 1
		case "pt":
			factor = 96.0 / 72.0
		case "pc":
			factor = 16
		case "in":
			factor = 96
		case "cm":
			factor = 96.0 / 2.54
		case "mm":
			factor = 96.0 / 25.4
		case "em":
			factor = fontSize
		case "rem":
			factor = 16
		case "ex", "ch":
			factor = fontSize / 2
		default:
			return lengthValue{}, false
		}
		return lengthValue{value: token.Number * factor}, true
	}
	return lengthValue{}, false
}

func splitCommaGroups(values []parser.ComponentValue) [][]parser.ComponentValue {
	var groups [][]parser.ComponentValue
	var current []parser.ComponentValue
	for _, value := range values {
		if token, ok := parser.AsToken(value); ok && token.Type == tokenizer.TokenComma {
			groups = append(groups, current)
			current = nil
			continue
		}
		current = append(current, value)
	}
	if len(current) > 0 || len(groups) > 0 {
		groups = append(groups, current)
	}
	return groups
}

func nonWhitespaceValues(values []parser.ComponentValue) []parser.ComponentValue {
	result := make([]parser.ComponentValue, 0, len(values))
	for _, value := range values {
		if !parser.IsWhitespace(value) {
			result = append(result, value)
		}
	}
	return result
}

func parseCSSPixelValue(text string) (float64, bool) {
	values := nonWhitespaceValues(parser.ParseComponentValueList(text))
	if len(values) != 1 {
		return 0, false
	}
	length, ok := parseLengthValue(values[0], 16)
	if !ok || length.percent || length.auto {
		return 0, false
	}
	return length.value, true
}

func (painter *Painter) paintBackground(box *layouts.Box, surface interfaces.Canvas, offsetX, offsetY float64) {
	if box.Style == nil || box.Kind == layouts.AnonymousBlockBox {
		return
	}
	layers := parseBackgroundLayers(box.Style)
	base := box.ContainingWidth
	if base <= 0 {
		base = box.Width
	}
	radii := borderRadii(box.Style, box.Width, box.Height, base)
	color, hasColor := ParseColor(box.Style.GetString("background-color"))

	if hasColor && color.Alpha > 0 {
		clip := "border-box"
		if len(layers) > 0 {
			clip = layers[len(layers)-1].clip
		}
		clipRect := boxAreaRect(box, clip, offsetX, offsetY, base)
		clipRadii := insetRadii(radii, areaInsets(box, clip, base))
		fillRoundedArea(surface, clipRect, clipRadii, color)
	}
	if painter == nil {
		return
	}
	for index := len(layers) - 1; index >= 0; index-- {
		painter.paintBackgroundLayer(box, surface, layers[index], radii, offsetX, offsetY, base)
	}
}

func (painter *Painter) paintBackgroundLayer(box *layouts.Box, surface interfaces.Canvas, layer backgroundLayerPaint, radii CornerRadii, offsetX, offsetY, base float64) {
	if layer.image == "" && layer.gradient == nil {
		return
	}
	area := boxAreaRect(box, layer.origin, offsetX, offsetY, base)
	clip := boxAreaRect(box, layer.clip, offsetX, offsetY, base)
	if strings.EqualFold(layer.attachment, "fixed") {
		area = Rect{Width: float64(surface.CanvasWidth()), Height: float64(surface.CanvasHeight())}
	}
	if area.Width <= 0 || area.Height <= 0 || clip.Width <= 0 || clip.Height <= 0 {
		return
	}
	clipRadii := insetRadii(radii, areaInsets(box, layer.clip, base))
	var mask *Mask
	if hasNonZeroRadii(clipRadii) {
		path := NewPath()
		path.AddRoundedRect(clip, clipRadii)
		mask = RasterizeFill(path, surface.CanvasWidth(), surface.CanvasHeight(), NonZeroFill)
	}
	if layer.gradient != nil {
		tileWidth := area.Width
		tileHeight := area.Height
		if !layer.sizeWidth.auto {
			tileWidth = layer.sizeWidth.resolve(area.Width)
		}
		if !layer.sizeHeight.auto {
			tileHeight = layer.sizeHeight.resolve(area.Height)
		}
		if layer.cover || layer.contain {
			tileWidth = area.Width
			tileHeight = area.Height
		}
		if tileWidth <= 0 || tileHeight <= 0 {
			return
		}
		positionX := area.X + (area.Width-tileWidth)*layer.positionX.value/100
		if !layer.positionX.percent {
			positionX = area.X + layer.positionX.value
		}
		positionY := area.Y + (area.Height-tileHeight)*layer.positionY.value/100
		if !layer.positionY.percent {
			positionY = area.Y + layer.positionY.value
		}
		startX := positionX
		if layer.repeatX {
			startX = positionX - math.Ceil((positionX-clip.X)/tileWidth)*tileWidth
		}
		startY := positionY
		if layer.repeatY {
			startY = positionY - math.Ceil((positionY-clip.Y)/tileHeight)*tileHeight
		}
		drawX := startX
		tilesX := 0
		for {
			if !layer.repeatX && tilesX > 0 {
				break
			}
			drawY := startY
			tilesY := 0
			for {
				if !layer.repeatY && tilesY > 0 {
					break
				}
				tile := Rect{X: drawX, Y: drawY, Width: tileWidth, Height: tileHeight}
				if tile.X < clip.X+clip.Width && tile.Y < clip.Y+clip.Height && tile.X+tile.Width > clip.X && tile.Y+tile.Height > clip.Y {
					painter.paintGradient(layer.gradient, surface, tile, clip, mask)
				}
				tilesY++
				drawY += tileHeight
				if !layer.repeatY || drawY >= clip.Y+clip.Height || tilesY > 10000 {
					break
				}
			}
			tilesX++
			drawX += tileWidth
			if !layer.repeatX || drawX >= clip.X+clip.Width || tilesX > 10000 {
				break
			}
		}
		return
	}
	source := painter.loadImage(layer.image)
	if source == nil {
		return
	}
	sourceBounds := source.Bounds()
	intrinsicWidth := float64(sourceBounds.Dx())
	intrinsicHeight := float64(sourceBounds.Dy())
	if intrinsicWidth <= 0 || intrinsicHeight <= 0 {
		return
	}

	tileWidth := layer.sizeWidth.resolve(area.Width)
	tileHeight := layer.sizeHeight.resolve(area.Height)
	switch {
	case layer.cover:
		scale := math.Max(area.Width/intrinsicWidth, area.Height/intrinsicHeight)
		tileWidth = intrinsicWidth * scale
		tileHeight = intrinsicHeight * scale
	case layer.contain:
		scale := math.Min(area.Width/intrinsicWidth, area.Height/intrinsicHeight)
		tileWidth = intrinsicWidth * scale
		tileHeight = intrinsicHeight * scale
	default:
		if layer.sizeWidth.auto && layer.sizeHeight.auto {
			tileWidth = intrinsicWidth
			tileHeight = intrinsicHeight
		} else if layer.sizeWidth.auto {
			tileWidth = tileHeight * intrinsicWidth / intrinsicHeight
		} else if layer.sizeHeight.auto {
			tileHeight = tileWidth * intrinsicHeight / intrinsicWidth
		}
	}
	if tileWidth <= 0 || tileHeight <= 0 {
		return
	}

	positionX := area.X + (area.Width-tileWidth)*layer.positionX.value/100
	if !layer.positionX.percent {
		positionX = area.X + layer.positionX.value
	}
	positionY := area.Y + (area.Height-tileHeight)*layer.positionY.value/100
	if !layer.positionY.percent {
		positionY = area.Y + layer.positionY.value
	}

	startX := positionX
	if layer.repeatX {
		startX = positionX - math.Ceil((positionX-clip.X)/tileWidth)*tileWidth
	}
	startY := positionY
	if layer.repeatY {
		startY = positionY - math.Ceil((positionY-clip.Y)/tileHeight)*tileHeight
	}

	drawX := startX
	tilesX := 0
	for {
		if !layer.repeatX && tilesX > 0 {
			break
		}
		drawY := startY
		tilesY := 0
		for {
			if !layer.repeatY && tilesY > 0 {
				break
			}
			if drawX < clip.X+clip.Width && drawY < clip.Y+clip.Height && drawX+tileWidth > clip.X && drawY+tileHeight > clip.Y {
				surface.DrawImage(source, Rect{X: drawX, Y: drawY, Width: tileWidth, Height: tileHeight}, clip, mask, 1)
			}
			tilesY++
			drawY += tileHeight
			if !layer.repeatY || drawY >= clip.Y+clip.Height || tilesY > 10000 {
				break
			}
		}
		tilesX++
		drawX += tileWidth
		if !layer.repeatX || drawX >= clip.X+clip.Width || tilesX > 10000 {
			break
		}
	}
}

func (painter *Painter) loadImage(resource string) *image.NRGBA {
	if painter.Images == nil {
		return nil
	}
	if painter.imageCache == nil {
		painter.imageCache = make(map[string]*image.NRGBA)
	}
	if cached, ok := painter.imageCache[resource]; ok {
		return cached
	}
	loaded, err := painter.Images.LoadImage(resource)
	if err != nil || loaded == nil {
		painter.imageCache[resource] = nil
		return nil
	}
	converted := toNRGBA(loaded)
	painter.imageCache[resource] = converted
	return converted
}

func boxAreaRect(box *layouts.Box, area string, offsetX, offsetY, base float64) Rect {
	switch area {
	case "content-box":
		return Rect{X: box.ContentX + offsetX, Y: box.ContentY + offsetY, Width: box.ContentWidth, Height: box.ContentHeight}
	case "padding-box":
		insets := boxBorderInsets(box, base)
		return Rect{
			X:      box.X + offsetX + insets.Left,
			Y:      box.Y + offsetY + insets.Top,
			Width:  math.Max(0, box.Width-insets.Left-insets.Right),
			Height: math.Max(0, box.Height-insets.Top-insets.Bottom),
		}
	default:
		return Rect{X: box.X + offsetX, Y: box.Y + offsetY, Width: box.Width, Height: box.Height}
	}
}

type boxInsets struct {
	Top    float64
	Right  float64
	Bottom float64
	Left   float64
}

func boxBorderInsets(box *layouts.Box, base float64) boxInsets {
	return boxInsets{
		Top:    box.Border.Top.Resolve(base),
		Right:  box.Border.Right.Resolve(base),
		Bottom: box.Border.Bottom.Resolve(base),
		Left:   box.Border.Left.Resolve(base),
	}
}

func boxPaddingInsets(box *layouts.Box, base float64) boxInsets {
	return boxInsets{
		Top:    box.Padding.Top.Resolve(base),
		Right:  box.Padding.Right.Resolve(base),
		Bottom: box.Padding.Bottom.Resolve(base),
		Left:   box.Padding.Left.Resolve(base),
	}
}

func areaInsets(box *layouts.Box, area string, base float64) boxInsets {
	border := boxBorderInsets(box, base)
	if area == "padding-box" {
		return border
	}
	if area == "content-box" {
		padding := boxPaddingInsets(box, base)
		return boxInsets{
			Top:    border.Top + padding.Top,
			Right:  border.Right + padding.Right,
			Bottom: border.Bottom + padding.Bottom,
			Left:   border.Left + padding.Left,
		}
	}
	return boxInsets{}
}

func insetRadii(radii CornerRadii, insets boxInsets) CornerRadii {
	adjust := func(radius Point, horizontal, vertical float64) Point {
		return Point{X: math.Max(0, radius.X-horizontal), Y: math.Max(0, radius.Y-vertical)}
	}
	return CornerRadii{
		TopLeft:     adjust(radii.TopLeft, insets.Left, insets.Top),
		TopRight:    adjust(radii.TopRight, insets.Right, insets.Top),
		BottomRight: adjust(radii.BottomRight, insets.Right, insets.Bottom),
		BottomLeft:  adjust(radii.BottomLeft, insets.Left, insets.Bottom),
	}
}

func hasNonZeroRadii(radii CornerRadii) bool {
	return radii.TopLeft.X > 0 || radii.TopLeft.Y > 0 ||
		radii.TopRight.X > 0 || radii.TopRight.Y > 0 ||
		radii.BottomRight.X > 0 || radii.BottomRight.Y > 0 ||
		radii.BottomLeft.X > 0 || radii.BottomLeft.Y > 0
}

func fillRoundedArea(surface interfaces.Canvas, rect Rect, radii CornerRadii, color Color) {
	if rect.Width <= 0 || rect.Height <= 0 {
		return
	}
	if !hasNonZeroRadii(radii) {
		surface.FillRect(rect, color)
		return
	}
	path := NewPath()
	path.AddRoundedRect(rect, radii)
	surface.FillPath(path, NonZeroFill, color)
}

func borderRadii(style *styles.Style, width, height, base float64) CornerRadii {
	if style == nil {
		return CornerRadii{}
	}
	parse := func(property string, horizontalBase, verticalBase float64) Point {
		values := nonWhitespaceValues(style.Get(property))
		if len(values) == 0 {
			return Point{}
		}
		horizontal, ok := parseLengthValue(values[0], 16)
		if !ok {
			return Point{}
		}
		vertical := horizontal
		if len(values) > 1 {
			if parsed, ok := parseLengthValue(values[1], 16); ok {
				vertical = parsed
			}
		}
		return Point{X: horizontal.resolve(horizontalBase), Y: vertical.resolve(verticalBase)}
	}
	return CornerRadii{
		TopLeft:     parse("border-top-left-radius", width, height),
		TopRight:    parse("border-top-right-radius", width, height),
		BottomRight: parse("border-bottom-right-radius", width, height),
		BottomLeft:  parse("border-bottom-left-radius", width, height),
	}
}
