package rendering

import (
	layoutinginterfaces "gonetsurf/layouting/interfaces"
	"gonetsurf/layouting/model"
	"gonetsurf/rendering/interfaces"
	"image"
	"math"
	"strings"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"

	"gonetsurf/dom"
	"gonetsurf/rendering/fonts"
	"gonetsurf/urls"
)

// Painter paints a laid-out box tree. The zero value paints with the default
// data-URL image provider and the default system font library.
type Painter struct {
	Geometry   layoutinginterfaces.GeometryHandle
	Images     ImageProvider
	imageCache map[string]*image.NRGBA
	Face       font.Face
	Fonts      *fonts.Library
	Glyphs     *fonts.GlyphCache
	BaseURL    *urls.URL
	Subframes  SubframeProvider
	OffsetX    float64
	OffsetY    float64

	framePainted  map[*dom.Node]bool
	colorCache    map[string]Color
	faceCache     map[fonts.Spec][]font.Face
	runeFaceCache map[fonts.Spec]map[rune]font.Face
}

// NewPainter returns a painter using images and the default font library.
func NewPainter(images ImageProvider) *Painter {
	library := fonts.Default()
	return &Painter{Images: images, Fonts: library, Glyphs: library.GlyphCache()}
}

// Paint paints a laid-out box tree onto surface. The caller owns clearing the
// surface; Paint draws shadows, backgrounds, borders, and text in tree order.
func Paint(root *model.Box, surface interfaces.Canvas) {
	NewPainter(DataURLImageProvider{}).Paint(root, surface)
}

// Paint paints root onto surface.
func (painter *Painter) Paint(root *model.Box, surface interfaces.Canvas) {
	if root == nil || surface == nil {
		return
	}
	if painter.Images == nil {
		painter.Images = DataURLImageProvider{}
	}
	if painter.Fonts == nil {
		painter.Fonts = fonts.Default()
	}
	if painter.Glyphs == nil {
		painter.Glyphs = painter.Fonts.GlyphCache()
	}
	painter.paintStackingContext(root, surface, painter.OffsetX, painter.OffsetY)
}

type borderSide struct {
	width float64
	style string
	color Color
}

func borderSides(box *model.Box, base float64) [4]borderSide {
	var sides [4]borderSide
	names := []string{"top", "right", "bottom", "left"}
	widths := []float64{
		box.Border.Top.Resolve(base),
		box.Border.Right.Resolve(base),
		box.Border.Bottom.Resolve(base),
		box.Border.Left.Resolve(base),
	}
	for index, name := range names {
		side := borderSide{width: widths[index]}
		if box.Style != nil {
			side.style = strings.ToLower(strings.TrimSpace(box.Style.GetString("border-" + name + "-style")))
			color, ok := ParseColor(box.Style.GetString("border-" + name + "-color"))
			if !ok {
				color, ok = ParseColor(box.Style.GetString("color"))
			}
			if !ok {
				color = NewColor(0, 0, 0)
			}
			side.color = color
		}
		sides[index] = side
	}
	return sides
}

func borderVisible(side borderSide) bool {
	return side.width > 0 && side.style != "" && side.style != "none" && side.style != "hidden"
}

func paintBorders(box *model.Box, surface interfaces.Canvas, xPos, yPos float64) {
	if box.Style == nil {
		return
	}
	base := box.ContainingWidth
	if base <= 0 {
		base = box.Width
	}
	sides := borderSides(box, base)
	visible := false
	for _, side := range sides {
		if borderVisible(side) {
			visible = true
			break
		}
	}
	if !visible {
		return
	}
	radii := borderRadii(box.Style, box.Width, box.Height, base)
	if !hasNonZeroRadii(radii) {
		paintRectBorders(surface, xPos, yPos, box.Width, box.Height, sides)
		return
	}
	paintRoundedBorders(surface, xPos, yPos, box.Width, box.Height, sides, radii)
}

func paintRectBorders(surface interfaces.Canvas, xPos, yPos, width, height float64, sides [4]borderSide) {
	top := sides[0].width
	right := sides[1].width
	bottom := sides[2].width
	left := sides[3].width

	paintBorderRectangle(surface, sides[0], "top", Rect{X: xPos, Y: yPos, Width: width, Height: top})
	paintBorderRectangle(surface, sides[2], "bottom", Rect{X: xPos, Y: yPos + height - bottom, Width: width, Height: bottom})
	paintBorderRectangle(surface, sides[3], "left", Rect{X: xPos, Y: yPos + top, Width: left, Height: math.Max(0, height-top-bottom)})
	paintBorderRectangle(surface, sides[1], "right", Rect{X: xPos + width - right, Y: yPos + top, Width: right, Height: math.Max(0, height-top-bottom)})
}

func paintBorderRectangle(surface interfaces.Canvas, side borderSide, name string, rect Rect) {
	if !borderVisible(side) || rect.Width <= 0 || rect.Height <= 0 {
		return
	}
	color := borderColorForStyle(side, name)
	switch side.style {
	case "double":
		paintDoubleBorder(surface, rect, name, side.width, color)
	case "dashed":
		paintDashedBorder(surface, rect, name, side.width, maxFloat(2, side.width*3), maxFloat(2, side.width*3), color)
	case "dotted":
		paintDashedBorder(surface, rect, name, side.width, maxFloat(1, side.width), maxFloat(2, side.width*2), color)
	default:
		surface.FillRect(rect, color)
	}
}

func paintDoubleBorder(surface interfaces.Canvas, rect Rect, name string, thickness float64, color Color) {
	band := thickness / 3
	if name == "top" {
		surface.FillRect(Rect{X: rect.X, Y: rect.Y, Width: rect.Width, Height: band}, color)
		surface.FillRect(Rect{X: rect.X, Y: rect.Y + rect.Height - band, Width: rect.Width, Height: band}, color)
		return
	}
	if name == "bottom" {
		surface.FillRect(Rect{X: rect.X, Y: rect.Y, Width: rect.Width, Height: band}, color)
		surface.FillRect(Rect{X: rect.X, Y: rect.Y + rect.Height - band, Width: rect.Width, Height: band}, color)
		return
	}
	surface.FillRect(Rect{X: rect.X, Y: rect.Y, Width: band, Height: rect.Height}, color)
	surface.FillRect(Rect{X: rect.X + rect.Width - band, Y: rect.Y, Width: band, Height: rect.Height}, color)
}

func paintDashedBorder(surface interfaces.Canvas, rect Rect, name string, thickness, dashLength, gapLength float64, color Color) {
	period := dashLength + gapLength
	if period <= 0 {
		return
	}
	horizontal := name == "top" || name == "bottom"
	if horizontal {
		for start := rect.X; start < rect.X+rect.Width; start += period {
			length := math.Min(dashLength, rect.X+rect.Width-start)
			surface.FillRect(Rect{X: start, Y: rect.Y, Width: length, Height: thickness}, color)
		}
		return
	}
	for start := rect.Y; start < rect.Y+rect.Height; start += period {
		length := math.Min(dashLength, rect.Y+rect.Height-start)
		surface.FillRect(Rect{X: rect.X, Y: start, Width: thickness, Height: length}, color)
	}
}

func paintRoundedBorders(surface interfaces.Canvas, xPos, yPos, width, height float64, sides [4]borderSide, radii CornerRadii) {
	outer := Rect{X: xPos, Y: yPos, Width: width, Height: height}
	outerPath := NewPath()
	outerPath.AddRoundedRect(outer, radii)
	outerMask := RasterizeFill(outerPath, surface.CanvasWidth(), surface.CanvasHeight(), NonZeroFill)

	insets := boxInsets{Top: sides[0].width, Right: sides[1].width, Bottom: sides[2].width, Left: sides[3].width}
	innerRect := Rect{
		X:      xPos + insets.Left,
		Y:      yPos + insets.Top,
		Width:  math.Max(0, width-insets.Left-insets.Right),
		Height: math.Max(0, height-insets.Top-insets.Bottom),
	}
	innerRadii := insetRadii(radii, insets)
	innerMask := NewMask(surface.CanvasWidth(), surface.CanvasHeight())
	if innerRect.Width > 0 && innerRect.Height > 0 {
		innerPath := NewPath()
		innerPath.AddRoundedRect(innerRect, innerRadii)
		innerMask = RasterizeFill(innerPath, surface.CanvasWidth(), surface.CanvasHeight(), NonZeroFill)
	}

	minX := int(math.Max(0, math.Floor(xPos)))
	minY := int(math.Max(0, math.Floor(yPos)))
	maxX := int(math.Min(float64(surface.CanvasWidth()), math.Ceil(xPos+width)))
	maxY := int(math.Min(float64(surface.CanvasHeight()), math.Ceil(yPos+height)))
	for yPixel := minY; yPixel < maxY; yPixel++ {
		for xPixel := minX; xPixel < maxX; xPixel++ {
			ring := outerMask.At(xPixel, yPixel) * (1 - innerMask.At(xPixel, yPixel))
			if ring <= 0 {
				continue
			}
			centerX := float64(xPixel) + 0.5
			centerY := float64(yPixel) + 0.5
			sideIndex, normalDistance, along := nearestBorderSide(xPos, yPos, width, height, centerX, centerY)
			side := sides[sideIndex]
			if !borderVisible(side) {
				continue
			}
			if normalDistance > side.width {
				normalDistance = side.width
			}
			color := borderColorForStyle(side, []string{"top", "right", "bottom", "left"}[sideIndex])
			coverage := ring
			switch side.style {
			case "double":
				normalized := normalDistance / side.width
				if normalized > 1.0/3.0 && normalized < 2.0/3.0 {
					coverage = 0
				}
			case "dashed":
				period := maxFloat(2, side.width*3) * 2
				if math.Mod(along, period) > maxFloat(2, side.width*3) {
					coverage = 0
				}
			case "dotted":
				period := maxFloat(1, side.width) * 3
				if math.Mod(along, period) > maxFloat(1, side.width) {
					coverage = 0
				}
			}
			if coverage > 0 {
				surface.BlendPixel(xPixel, yPixel, color, coverage)
			}
		}
	}
}

func nearestBorderSide(xPos, yPos, width, height, centerX, centerY float64) (int, float64, float64) {
	topDistance := centerY - yPos
	bottomDistance := yPos + height - centerY
	leftDistance := centerX - xPos
	rightDistance := xPos + width - centerX
	sideIndex := 0
	distance := topDistance
	along := centerX
	if bottomDistance < distance {
		sideIndex = 2
		distance = bottomDistance
	}
	if leftDistance < distance {
		sideIndex = 3
		distance = leftDistance
		along = centerY
	}
	if rightDistance < distance {
		sideIndex = 1
		distance = rightDistance
		along = centerY
	}
	return sideIndex, distance, along
}

func borderColorForStyle(side borderSide, name string) Color {
	light := name == "top" || name == "left"
	switch side.style {
	case "inset":
		if light {
			return shadeColor(side.color, 0.65, false)
		}
		return shadeColor(side.color, 0.35, true)
	case "outset":
		if light {
			return shadeColor(side.color, 0.35, true)
		}
		return shadeColor(side.color, 0.65, false)
	case "groove", "ridge":
		if light {
			return shadeColor(side.color, 0.55, false)
		}
		return shadeColor(side.color, 0.45, true)
	default:
		return side.color
	}
}

func shadeColor(color Color, factor float64, towardWhite bool) Color {
	flip := func(channel uint8) uint8 {
		value := float64(channel)
		if towardWhite {
			value = value + (255-value)*(1-factor)
		} else {
			value = value * factor
		}
		if value < 0 {
			value = 0
		}
		if value > 255 {
			value = 255
		}
		return uint8(math.Round(value))
	}
	return Color{Red: flip(color.Red), Green: flip(color.Green), Blue: flip(color.Blue), Alpha: color.Alpha}
}

func (painter *Painter) cachedColor(value string) (Color, bool) {
	if painter.colorCache == nil {
		painter.colorCache = make(map[string]Color)
	}
	if color, ok := painter.colorCache[value]; ok {
		return color, color.Alpha != 0
	}
	color, ok := ParseColor(value)
	if ok {
		painter.colorCache[value] = color
	}
	return color, ok
}

func (painter *Painter) cachedFaces(spec fonts.Spec) []font.Face {
	if painter.faceCache == nil {
		painter.faceCache = make(map[fonts.Spec][]font.Face)
	}
	if faces, ok := painter.faceCache[spec]; ok {
		return faces
	}
	faces := painter.Fonts.Faces(spec)
	painter.faceCache[spec] = faces
	return faces
}

func (painter *Painter) cachedRuneFace(spec fonts.Spec, faces []font.Face, character rune) font.Face {
	if painter.runeFaceCache == nil {
		painter.runeFaceCache = make(map[fonts.Spec]map[rune]font.Face)
	}
	cache := painter.runeFaceCache[spec]
	if cache == nil {
		cache = make(map[rune]font.Face)
		painter.runeFaceCache[spec] = cache
	}
	if face, ok := cache[character]; ok {
		return face
	}
	face := fonts.FaceForRuneChain(faces, character)
	if face == nil && len(faces) > 0 {
		face = faces[0]
	}
	cache[character] = face
	return face
}

func (painter *Painter) paintText(box *model.Box, surface interfaces.Canvas, offsetX, offsetY float64) {
	text := box.VisualText
	if text == "" {
		text = box.Text
	}
	if text == "" {
		return
	}
	clip := surface.Clip()
	left := box.ContentX + offsetX
	top := box.ContentY + offsetY
	if left+box.Width <= clip.X || top+box.Height <= clip.Y ||
		left >= clip.X+clip.Width || top >= clip.Y+clip.Height {
		return
	}
	color := NewColor(0, 0, 0)
	if box.Style != nil {
		value := box.Style.GetString("color")
		if parsed, ok := painter.cachedColor(value); ok {
			color = parsed
		}
	}
	if color.Alpha == 0 {
		return
	}
	spec := fonts.Spec{Size: box.FontSize}
	if box.Style != nil {
		spec = fonts.SpecFromStyle(box.Style)
	}
	if spec.Size <= 0 {
		spec.Size = box.FontSize
	}
	if spec.Size <= 0 {
		spec.Size = 16
	}
	var faces []font.Face
	if painter.Fonts != nil {
		faces = painter.cachedFaces(spec)
	}
	if len(faces) == 0 {
		if painter.Face == nil {
			painter.Face = basicfont.Face7x13
		}
		faces = []font.Face{painter.Face}
	}
	primary := faces[0]
	ascent := box.Ascent
	if ascent <= 0 {
		ascent = float64(primary.Metrics().Ascent) / 64
	}
	letterSpacing := 0.0
	wordSpacing := 0.0
	if box.Style != nil {
		if value, ok := parseCSSPixelValue(box.Style.GetString("letter-spacing")); ok {
			letterSpacing = value
		}
		if value, ok := parseCSSPixelValue(box.Style.GetString("word-spacing")); ok {
			wordSpacing = value
		}
	}

	dotX := math.Round(box.ContentX + offsetX)
	dotY := math.Round(box.ContentY + offsetY + ascent)
	var previousCharacter rune = -1
	var previousFace font.Face
	for _, character := range text {
		if character == '\n' || character == '\r' {
			continue
		}
		face := primary
		if selected := painter.cachedRuneFace(spec, faces, character); selected != nil {
			face = selected
		}
		if previousCharacter >= 0 && face == previousFace {
			dotX += float64(face.Kern(previousCharacter, character)) / 64
		}
		if painter.Glyphs != nil {
			if glyph, ok := painter.Glyphs.Glyph(face, character); ok {
				drawCachedGlyph(surface, glyph, int(math.Round(dotX)), int(dotY), color)
			}
		}
		advance := 0.0
		if measured, ok := face.GlyphAdvance(character); ok {
			advance = float64(measured) / 64
		}
		advance += letterSpacing
		if character == ' ' {
			advance += wordSpacing
		}
		dotX += advance
		previousCharacter = character
		previousFace = face
	}
}

func drawCachedGlyph(surface interfaces.Canvas, glyph fonts.CachedGlyph, dotX, dotY int, color Color) {
	if glyph.Mask == nil {
		return
	}
	surface.DrawGlyphMask(glyph.Mask, dotX+glyph.Bounds.Min.X, dotY+glyph.Bounds.Min.Y, color)
}

func maxFloat(first, second float64) float64 {
	if first > second {
		return first
	}
	return second
}
