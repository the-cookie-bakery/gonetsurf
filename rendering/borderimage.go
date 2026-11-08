package rendering

import (
	"gonetsurf/layouting/model"
	"gonetsurf/rendering/interfaces"
	"image"
	"math"
	"strings"

	"gonetsurf/parsers/css/parser"
	"gonetsurf/parsers/css/tokenizer"
	"gonetsurf/styles"
	"gonetsurf/urls"
)

type borderImageSpec struct {
	source  string
	slice   [4]float64
	fill    bool
	widths  [4]lengthValue
	outset  [4]float64
	repeatX string
	repeatY string
}

type borderImageWidths struct {
	top    float64
	right  float64
	bottom float64
	left   float64
}

func (painter *Painter) paintBorders(box *model.Box, surface interfaces.Canvas, xPos, yPos float64) {
	if painter.paintBorderImage(box, surface, xPos, yPos) {
		return
	}
	paintBorders(box, surface, xPos, yPos)
}

func (painter *Painter) paintBorderImage(box *model.Box, surface interfaces.Canvas, xPos, yPos float64) bool {
	spec := parseBorderImageStyle(box.Style)
	if spec == nil || spec.source == "" {
		return false
	}
	source := painter.resolveImageResource(spec.source)
	picture := painter.loadImageURL(source)
	if picture == nil {
		return false
	}
	base := box.ContainingWidth
	if base <= 0 {
		base = box.Width
	}
	borderWidths := borderImageWidths{
		top:    box.Border.Top.Resolve(base),
		right:  box.Border.Right.Resolve(base),
		bottom: box.Border.Bottom.Resolve(base),
		left:   box.Border.Left.Resolve(base),
	}
	widths := resolveBorderImageWidths(spec.widths, borderWidths, box.Width, box.Height)
	widths = clampBorderImageWidths(widths, box.Width, box.Height)
	outset := resolveBorderImageOutsets(spec.outset, borderWidths)
	sourceBounds := picture.Bounds()
	slices := resolveBorderImageSlices(spec.slice, sourceBounds.Dx(), sourceBounds.Dy())
	if slices[0]+slices[2] >= float64(sourceBounds.Dx()) || slices[1]+slices[3] >= float64(sourceBounds.Dy()) {
		return false
	}

	outerX := xPos - outset[3]
	outerY := yPos - outset[1]
	outerWidth := box.Width + outset[1] + outset[3]
	outerHeight := box.Height + outset[0] + outset[2]
	if outerWidth <= 0 || outerHeight <= 0 {
		return false
	}
	destination := borderImageAreas{
		x: outerX, y: outerY, width: outerWidth, height: outerHeight,
		left: widths.left, right: widths.right, top: widths.top, bottom: widths.bottom,
	}
	sourceAreas := borderImageSourceAreas{
		source: picture,
		left:   int(math.Round(slices[0])),
		right:  int(math.Round(slices[2])),
		top:    int(math.Round(slices[1])),
		bottom: int(math.Round(slices[3])),
	}
	painter.drawBorderImage(surface, sourceAreas, destination, spec)
	return true
}

type borderImageAreas struct {
	x      float64
	y      float64
	width  float64
	height float64
	left   float64
	right  float64
	top    float64
	bottom float64
}

type borderImageSourceAreas struct {
	source *image.NRGBA
	left   int
	right  int
	top    int
	bottom int
}

func (painter *Painter) drawBorderImage(surface interfaces.Canvas, sourceAreas borderImageSourceAreas, destination borderImageAreas, spec *borderImageSpec) {
	picture := sourceAreas.source
	bounds := picture.Bounds()
	destinationRight := destination.x + destination.width
	destinationBottom := destination.y + destination.height

	sourceTopLeft := image.Rect(bounds.Min.X, bounds.Min.Y, bounds.Min.X+sourceAreas.left, bounds.Min.Y+sourceAreas.top)
	sourceTopRight := image.Rect(bounds.Max.X-sourceAreas.right, bounds.Min.Y, bounds.Max.X, bounds.Min.Y+sourceAreas.top)
	sourceBottomLeft := image.Rect(bounds.Min.X, bounds.Max.Y-sourceAreas.bottom, bounds.Min.X+sourceAreas.left, bounds.Max.Y)
	sourceBottomRight := image.Rect(bounds.Max.X-sourceAreas.right, bounds.Max.Y-sourceAreas.bottom, bounds.Max.X, bounds.Max.Y)
	sourceTop := image.Rect(bounds.Min.X+sourceAreas.left, bounds.Min.Y, bounds.Max.X-sourceAreas.right, bounds.Min.Y+sourceAreas.top)
	sourceBottom := image.Rect(bounds.Min.X+sourceAreas.left, bounds.Max.Y-sourceAreas.bottom, bounds.Max.X-sourceAreas.right, bounds.Max.Y)
	sourceLeft := image.Rect(bounds.Min.X, bounds.Min.Y+sourceAreas.top, bounds.Min.X+sourceAreas.left, bounds.Max.Y-sourceAreas.bottom)
	sourceRight := image.Rect(bounds.Max.X-sourceAreas.right, bounds.Min.Y+sourceAreas.top, bounds.Max.X, bounds.Max.Y-sourceAreas.bottom)
	sourceCenter := image.Rect(bounds.Min.X+sourceAreas.left, bounds.Min.Y+sourceAreas.top, bounds.Max.X-sourceAreas.right, bounds.Max.Y-sourceAreas.bottom)

	drawRegion := func(source image.Image, destination Rect) {
		if source == nil || destination.Width <= 0 || destination.Height <= 0 {
			return
		}
		surface.DrawImage(source, destination, destination, nil, 1)
	}
	drawRegion(picture.SubImage(sourceTopLeft), Rect{X: destination.x, Y: destination.y, Width: destination.left, Height: destination.top})
	drawRegion(picture.SubImage(sourceTopRight), Rect{X: destinationRight - destination.right, Y: destination.y, Width: destination.right, Height: destination.top})
	drawRegion(picture.SubImage(sourceBottomLeft), Rect{X: destination.x, Y: destinationBottom - destination.bottom, Width: destination.left, Height: destination.bottom})
	drawRegion(picture.SubImage(sourceBottomRight), Rect{X: destinationRight - destination.right, Y: destinationBottom - destination.bottom, Width: destination.right, Height: destination.bottom})

	middleWidth := destination.width - destination.left - destination.right
	middleHeight := destination.height - destination.top - destination.bottom
	painter.drawBorderImageEdge(surface, picture.SubImage(sourceTop), Rect{X: destination.x + destination.left, Y: destination.y, Width: middleWidth, Height: destination.top}, spec.repeatX, true)
	painter.drawBorderImageEdge(surface, picture.SubImage(sourceBottom), Rect{X: destination.x + destination.left, Y: destinationBottom - destination.bottom, Width: middleWidth, Height: destination.bottom}, spec.repeatX, true)
	painter.drawBorderImageEdge(surface, picture.SubImage(sourceLeft), Rect{X: destination.x, Y: destination.y + destination.top, Width: destination.left, Height: middleHeight}, spec.repeatY, false)
	painter.drawBorderImageEdge(surface, picture.SubImage(sourceRight), Rect{X: destinationRight - destination.right, Y: destination.y + destination.top, Width: destination.right, Height: middleHeight}, spec.repeatY, false)
	if spec.fill {
		drawRegion(picture.SubImage(sourceCenter), Rect{X: destination.x + destination.left, Y: destination.y + destination.top, Width: middleWidth, Height: middleHeight})
	}
}

func (painter *Painter) drawBorderImageEdge(surface interfaces.Canvas, source image.Image, destination Rect, repeat string, horizontal bool) {
	sourceBounds := source.Bounds()
	if sourceBounds.Dx() == 0 || sourceBounds.Dy() == 0 || destination.Width <= 0 || destination.Height <= 0 {
		return
	}
	if repeat != "repeat" && repeat != "round" && repeat != "space" {
		surface.DrawImage(source, destination, destination, nil, 1)
		return
	}
	if horizontal {
		tileWidth := float64(sourceBounds.Dx()) * destination.Height / float64(sourceBounds.Dy())
		if tileWidth <= 0 {
			return
		}
		for offset := 0.0; offset < destination.Width; offset += tileWidth {
			width := math.Min(tileWidth, destination.Width-offset)
			tileDestination := Rect{X: destination.X + offset, Y: destination.Y, Width: width, Height: destination.Height}
			surface.DrawImage(source, tileDestination, tileDestination, nil, 1)
		}
		return
	}
	tileHeight := float64(sourceBounds.Dy()) * destination.Width / float64(sourceBounds.Dx())
	if tileHeight <= 0 {
		return
	}
	for offset := 0.0; offset < destination.Height; offset += tileHeight {
		height := math.Min(tileHeight, destination.Height-offset)
		tileDestination := Rect{X: destination.X, Y: destination.Y + offset, Width: destination.Width, Height: height}
		surface.DrawImage(source, tileDestination, tileDestination, nil, 1)
	}
}

func (painter *Painter) resolveImageResource(source string) string {
	address, ok := parseCSSURL(source)
	if !ok {
		return ""
	}
	if painter.BaseURL == nil {
		return address
	}
	resolved, err := urls.ParseReference(painter.BaseURL, address)
	if err != nil {
		return address
	}
	return resolved.String()
}

func parseBorderImageStyle(style *styles.Style) *borderImageSpec {
	if style == nil {
		return nil
	}
	sourceText := strings.TrimSpace(style.GetString("border-image-source"))
	if sourceText == "" || strings.EqualFold(sourceText, "none") {
		return nil
	}
	spec := &borderImageSpec{source: sourceText, repeatX: "stretch", repeatY: "stretch"}
	if slices := parseBorderImageSlice(style.Get("border-image-slice")); slices != nil {
		spec.slice = *slices
		spec.fill = strings.Contains(strings.ToLower(style.GetString("border-image-slice")), "fill")
	} else {
		spec.slice = [4]float64{100, 100, 100, 100}
	}
	if widths := parseBorderImageWidthValues(style.Get("border-image-width")); widths != nil {
		spec.widths = *widths
	} else {
		spec.widths = [4]lengthValue{{auto: true}, {auto: true}, {auto: true}, {auto: true}}
	}
	if outsets := parseBorderImageOutsets(style.Get("border-image-outset")); outsets != nil {
		spec.outset = *outsets
	}
	repeats := nonWhitespaceValues(style.Get("border-image-repeat"))
	if len(repeats) > 0 {
		if token, ok := parser.AsToken(repeats[0]); ok && token.Type == tokenizer.TokenIdent {
			spec.repeatX = strings.ToLower(token.Value)
			spec.repeatY = spec.repeatX
		}
	}
	if len(repeats) > 1 {
		if token, ok := parser.AsToken(repeats[1]); ok && token.Type == tokenizer.TokenIdent {
			spec.repeatY = strings.ToLower(token.Value)
		}
	}
	return spec
}

func parseBorderImageSlice(values []parser.ComponentValue) *[4]float64 {
	items := nonWhitespaceValues(values)
	var parsed []float64
	fill := false
	for _, item := range items {
		token, ok := parser.AsToken(item)
		if !ok {
			continue
		}
		switch token.Type {
		case tokenizer.TokenNumber:
			parsed = append(parsed, token.Number)
		case tokenizer.TokenPercentage:
			parsed = append(parsed, -token.Number)
		case tokenizer.TokenIdent:
			if strings.EqualFold(token.Value, "fill") {
				fill = true
			}
		}
	}
	if len(parsed) == 0 {
		return nil
	}
	result := expandFourValues(parsed)
	_ = fill
	return &result
}

func parseBorderImageWidthValues(values []parser.ComponentValue) *[4]lengthValue {
	items := nonWhitespaceValues(values)
	var parsed []lengthValue
	for _, item := range items {
		token, ok := parser.AsToken(item)
		if !ok {
			continue
		}
		if token.Type == tokenizer.TokenIdent && strings.EqualFold(token.Value, "auto") {
			parsed = append(parsed, lengthValue{auto: true})
			continue
		}
		if length, ok := parseLengthValue(item, 16); ok {
			if token.Type == tokenizer.TokenNumber {
				length = lengthValue{value: -length.value} // number is a border-width multiple
			}
			parsed = append(parsed, length)
		}
	}
	if len(parsed) == 0 {
		return nil
	}
	result := expandFourLengthValues(parsed)
	return &result
}

func parseBorderImageOutsets(values []parser.ComponentValue) *[4]float64 {
	items := nonWhitespaceValues(values)
	var parsed []float64
	for _, item := range items {
		token, ok := parser.AsToken(item)
		if !ok {
			continue
		}
		if token.Type == tokenizer.TokenNumber || token.Type == tokenizer.TokenDimension {
			if length, ok := parseLengthValue(item, 16); ok {
				if token.Type == tokenizer.TokenNumber {
					parsed = append(parsed, -length.value) // number is a border-width multiple
				} else {
					parsed = append(parsed, length.value)
				}
			}
		}
	}
	if len(parsed) == 0 {
		return nil
	}
	result := expandFourValues(parsed)
	return &result
}

func resolveBorderImageSlices(slice [4]float64, width, height int) [4]float64 {
	return [4]float64{
		resolveSliceValue(slice[0], float64(width)),
		resolveSliceValue(slice[1], float64(height)),
		resolveSliceValue(slice[2], float64(width)),
		resolveSliceValue(slice[3], float64(height)),
	}
}

func resolveSliceValue(value, reference float64) float64 {
	if value < 0 {
		return -value / 100 * reference
	}
	return value
}

func resolveBorderImageWidths(widths [4]lengthValue, border borderImageWidths, boxWidth, boxHeight float64) borderImageWidths {
	resolve := func(value lengthValue, borderValue, reference float64) float64 {
		if value.auto {
			return borderValue
		}
		if value.value < 0 {
			return -value.value * borderValue
		}
		return value.resolve(reference)
	}
	return borderImageWidths{
		top:    resolve(widths[0], border.top, boxHeight),
		right:  resolve(widths[1], border.right, boxWidth),
		bottom: resolve(widths[2], border.bottom, boxHeight),
		left:   resolve(widths[3], border.left, boxWidth),
	}
}

func resolveBorderImageOutsets(outsets [4]float64, border borderImageWidths) [4]float64 {
	resolve := func(value, borderValue float64) float64 {
		if value < 0 {
			return -value * borderValue
		}
		return value
	}
	return [4]float64{
		resolve(outsets[0], border.top),
		resolve(outsets[1], border.right),
		resolve(outsets[2], border.bottom),
		resolve(outsets[3], border.left),
	}
}

func clampBorderImageWidths(widths borderImageWidths, width, height float64) borderImageWidths {
	if widths.left+widths.right > width && widths.left+widths.right > 0 {
		scale := width / (widths.left + widths.right)
		widths.left *= scale
		widths.right *= scale
	}
	if widths.top+widths.bottom > height && widths.top+widths.bottom > 0 {
		scale := height / (widths.top + widths.bottom)
		widths.top *= scale
		widths.bottom *= scale
	}
	return widths
}

func expandFourValues(values []float64) [4]float64 {
	var result [4]float64
	switch len(values) {
	case 1:
		result = [4]float64{values[0], values[0], values[0], values[0]}
	case 2:
		result = [4]float64{values[0], values[1], values[0], values[1]}
	case 3:
		result = [4]float64{values[0], values[1], values[2], values[1]}
	default:
		copy(result[:], values)
	}
	return result
}

func expandFourLengthValues(values []lengthValue) [4]lengthValue {
	var result [4]lengthValue
	switch len(values) {
	case 1:
		result = [4]lengthValue{values[0], values[0], values[0], values[0]}
	case 2:
		result = [4]lengthValue{values[0], values[1], values[0], values[1]}
	case 3:
		result = [4]lengthValue{values[0], values[1], values[2], values[1]}
	default:
		copy(result[:], values)
	}
	return result
}
