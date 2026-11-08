package software

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"gonetsurf/rendering/geometry"
	"image"
	"math"
	"sort"
	"strconv"
	"strings"

	"gonetsurf/layouts"
	"gonetsurf/parsers/css/parser"
)

// SVGOptions limits SVG decoding work.
type SVGOptions struct {
	MaxPixels    int
	MaxDimension int
	MaxNodes     int
	MaxDepth     int
}

// DefaultSVGOptions returns conservative SVG budgets.
func DefaultSVGOptions() SVGOptions {
	return SVGOptions{MaxPixels: 16 << 20, MaxDimension: 4096, MaxNodes: 100000, MaxDepth: 128}
}

func (options SVGOptions) normalized() SVGOptions {
	defaults := DefaultSVGOptions()
	if options.MaxPixels <= 0 {
		options.MaxPixels = defaults.MaxPixels
	}
	if options.MaxDimension <= 0 {
		options.MaxDimension = defaults.MaxDimension
	}
	if options.MaxNodes <= 0 {
		options.MaxNodes = defaults.MaxNodes
	}
	if options.MaxDepth <= 0 {
		options.MaxDepth = defaults.MaxDepth
	}
	return options
}

// ErrSVGUnsupported reports an SVG feature outside the static subset.
var ErrSVGUnsupported = fmt.Errorf("svg: unsupported feature")

// DecodeSVG renders a static SVG document to an NRGBA image. Scripts,
// foreignObject, event handlers, and external references are ignored.
func DecodeSVG(data []byte, options SVGOptions) (image.Image, error) {
	options = options.normalized()
	root, err := parseSVGTree(data, options)
	if err != nil {
		return nil, err
	}
	if !strings.EqualFold(root.name.Local, "svg") {
		return nil, fmt.Errorf("svg: root element is <%s>", root.name.Local)
	}
	width, height, viewBox, hasViewBox := svgViewport(root)
	if width <= 0 || height <= 0 {
		return nil, fmt.Errorf("svg: invalid viewport")
	}
	if width > float64(options.MaxDimension) || height > float64(options.MaxDimension) {
		return nil, fmt.Errorf("svg: viewport exceeds budget")
	}
	pixelWidth := int(math.Round(width))
	pixelHeight := int(math.Round(height))
	if pixelWidth <= 0 || pixelHeight <= 0 || pixelWidth*pixelHeight > options.MaxPixels {
		return nil, fmt.Errorf("svg: pixel budget exceeded")
	}

	base := geometry.IdentityAffine()
	if hasViewBox {
		base = svgViewBoxMatrix(viewBox, Rect{Width: float64(pixelWidth), Height: float64(pixelHeight)}, svgAttribute(root, "preserveAspectRatio"))
	}

	surface := NewSurface(pixelWidth, pixelHeight)
	gradients := make(map[string]*svgNode)
	collectSVGGradients(root, gradients)
	context := svgContext{
		surface:    surface,
		options:    options,
		gradients:  gradients,
		viewWidth:  float64(pixelWidth),
		viewHeight: float64(pixelHeight),
		style: svgStyle{
			fill:          "black",
			fillOpacity:   1,
			strokeOpacity: 1,
			strokeWidth:   1,
			fillRule:      NonZeroFill,
		},
	}
	context.drawNode(root, base, 1)
	return surface.Image(), nil
}

type svgNode struct {
	name     xml.Name
	attrs    []xml.Attr
	children []*svgNode
}

func (node *svgNode) attribute(name string) (string, bool) {
	for _, attribute := range node.attrs {
		if strings.EqualFold(attribute.Name.Local, name) {
			return attribute.Value, true
		}
	}
	return "", false
}

func svgAttribute(node *svgNode, name string) string {
	value, _ := node.attribute(name)
	return value
}

func parseSVGTree(data []byte, options SVGOptions) (*svgNode, error) {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	decoder.Strict = true
	var root *svgNode
	var stack []*svgNode
	nodes := 0
	for {
		token, err := decoder.Token()
		if err != nil {
			if err.Error() == "EOF" {
				break
			}
			return nil, err
		}
		switch typed := token.(type) {
		case xml.StartElement:
			nodes++
			if nodes > options.MaxNodes || len(stack) > options.MaxDepth {
				return nil, fmt.Errorf("svg: node budget exceeded")
			}
			node := &svgNode{name: typed.Name, attrs: append([]xml.Attr(nil), typed.Attr...)}
			if len(stack) == 0 {
				if root != nil {
					return nil, fmt.Errorf("svg: multiple root elements")
				}
				root = node
			} else {
				parent := stack[len(stack)-1]
				parent.children = append(parent.children, node)
			}
			stack = append(stack, node)
		case xml.EndElement:
			if len(stack) == 0 {
				return nil, fmt.Errorf("svg: unbalanced end element")
			}
			stack = stack[:len(stack)-1]
		case xml.CharData:
			// Text content is not rendered in the static subset.
		case xml.Comment, xml.ProcInst, xml.Directive:
			// Ignore comments, processing instructions, and doctypes.
		}
	}
	if root == nil {
		return nil, fmt.Errorf("svg: empty document")
	}
	return root, nil
}

func svgViewport(root *svgNode) (float64, float64, Rect, bool) {
	viewBoxText := svgAttribute(root, "viewBox")
	var viewBox Rect
	hasViewBox := false
	if viewBoxText != "" {
		parts := strings.FieldsFunc(viewBoxText, func(character rune) bool {
			return character == ',' || character == ' ' || character == '\t' || character == '\n'
		})
		if len(parts) == 4 {
			values := make([]float64, 4)
			valid := true
			for index, part := range parts {
				number, err := strconv.ParseFloat(part, 64)
				if err != nil {
					valid = false
					break
				}
				values[index] = number
			}
			if valid && values[2] > 0 && values[3] > 0 {
				viewBox = Rect{X: values[0], Y: values[1], Width: values[2], Height: values[3]}
				hasViewBox = true
			}
		}
	}
	widthText := svgAttribute(root, "width")
	heightText := svgAttribute(root, "height")
	width := 300.0
	height := 150.0
	if hasViewBox {
		width = viewBox.Width
		height = viewBox.Height
	}
	if widthText != "" {
		if parsed, ok := svgLength(widthText, viewBox.Width, 16); ok {
			width = parsed
		}
	}
	if heightText != "" {
		if parsed, ok := svgLength(heightText, viewBox.Height, 16); ok {
			height = parsed
		}
	}
	return width, height, viewBox, hasViewBox
}

func svgViewBoxMatrix(viewBox Rect, viewport Rect, preserve string) geometry.Affine {
	scaleX := viewport.Width / viewBox.Width
	scaleY := viewport.Height / viewBox.Height
	scale := math.Min(scaleX, scaleY)
	align := strings.TrimSpace(preserve)
	if strings.HasPrefix(align, "none") {
		return geometry.TranslationAffine(-viewBox.X*scaleX, -viewBox.Y*scaleY).Multiply(geometry.ScaleAffine(scaleX, scaleY))
	}
	offsetX := (viewport.Width - viewBox.Width*scale) / 2
	offsetY := (viewport.Height - viewBox.Height*scale) / 2
	if strings.Contains(align, "xMin") {
		offsetX = 0
	} else if strings.Contains(align, "xMax") {
		offsetX = viewport.Width - viewBox.Width*scale
	}
	if strings.Contains(align, "YMin") {
		offsetY = 0
	} else if strings.Contains(align, "YMax") {
		offsetY = viewport.Height - viewBox.Height*scale
	}
	return geometry.TranslationAffine(offsetX-viewBox.X*scale, offsetY-viewBox.Y*scale).Multiply(geometry.ScaleAffine(scale, scale))
}

type svgStyle struct {
	fill          string
	stroke        string
	fillOpacity   float64
	strokeOpacity float64
	strokeWidth   float64
	fillRule      FillRule
	lineCap       LineCap
	lineJoin      LineJoin
	opacity       float64
}

type svgContext struct {
	surface    *Surface
	options    SVGOptions
	gradients  map[string]*svgNode
	viewWidth  float64
	viewHeight float64
	style      svgStyle
}

func (context svgContext) drawNode(node *svgNode, matrix geometry.Affine, groupOpacity float64) {
	name := strings.ToLower(node.name.Local)
	switch name {
	case "svg", "g", "a":
		style := context.styleFor(node, context.style)
		local := context.transformFor(node, context.viewWidth, context.viewHeight)
		nextMatrix := matrix.Multiply(local)
		opacity := groupOpacity * style.opacity
		for _, child := range node.children {
			next := context
			next.style = style
			next.drawNode(child, nextMatrix, opacity)
		}
	case "rect", "circle", "ellipse", "line", "polyline", "polygon", "path":
		style := context.styleFor(node, context.style)
		local := context.transformFor(node, context.viewWidth, context.viewHeight)
		path := buildSVGShape(node)
		if path == nil {
			return
		}
		path = path.Transform(matrix.Multiply(local))
		context.paintPath(path, style, groupOpacity*style.opacity)
	case "defs", "title", "desc", "metadata", "script", "foreignObject", "symbol", "clipPath", "mask", "pattern", "marker", "style", "use":
		return
	}
}

func (context svgContext) styleFor(node *svgNode, parent svgStyle) svgStyle {
	style := parent
	style.opacity = 1
	attributes := []string{"fill", "stroke", "fill-opacity", "stroke-opacity", "stroke-width", "fill-rule", "stroke-linecap", "stroke-linejoin", "opacity"}
	for _, name := range attributes {
		if value, ok := node.attribute(name); ok {
			style = applySVGStyle(style, name, value)
		}
	}
	if styleText, ok := node.attribute("style"); ok {
		for _, node := range parser.ParseListOfDeclarations(styleText) {
			declaration, ok := node.(*parser.Declaration)
			if !ok {
				continue
			}
			style = applySVGStyle(style, declaration.Name, parser.SerializeValues(declaration.Value))
		}
	}
	return style
}

func applySVGStyle(style svgStyle, name, value string) svgStyle {
	trimmed := strings.TrimSpace(value)
	switch strings.ToLower(name) {
	case "fill":
		style.fill = trimmed
	case "stroke":
		style.stroke = trimmed
	case "fill-opacity":
		if parsed, ok := svgNumber(trimmed); ok {
			style.fillOpacity = clamp01(parsed)
		}
	case "stroke-opacity":
		if parsed, ok := svgNumber(trimmed); ok {
			style.strokeOpacity = clamp01(parsed)
		}
	case "stroke-width":
		if parsed, ok := svgLength(trimmed, 1, 16); ok {
			style.strokeWidth = parsed
		}
	case "fill-rule":
		if strings.EqualFold(trimmed, "evenodd") {
			style.fillRule = EvenOddFill
		} else {
			style.fillRule = NonZeroFill
		}
	case "stroke-linecap":
		switch strings.ToLower(trimmed) {
		case "round":
			style.lineCap = RoundCap
		case "square":
			style.lineCap = SquareCap
		default:
			style.lineCap = ButtCap
		}
	case "stroke-linejoin":
		switch strings.ToLower(trimmed) {
		case "round":
			style.lineJoin = RoundJoin
		case "bevel":
			style.lineJoin = BevelJoin
		default:
			style.lineJoin = MiterJoin
		}
	case "opacity":
		if parsed, ok := svgNumber(trimmed); ok {
			style.opacity = clamp01(parsed)
		}
	}
	return style
}

func (context svgContext) paintPath(path *Path, style svgStyle, opacity float64) {
	if opacity <= 0 {
		return
	}
	fill := strings.TrimSpace(style.fill)
	if fill != "" && !strings.EqualFold(fill, "none") {
		if strings.HasPrefix(strings.ToLower(fill), "url(") {
			if gradient := context.gradientFor(fill); gradient != nil {
				mask := RasterizeFill(path, context.surface.Width, context.surface.Height, style.fillRule)
				context.paintGradientMask(mask, gradient, path.Bounds(), style.fillOpacity*opacity)
			}
		} else if color, ok := ParseColor(fill); ok {
			color.Alpha = geometry.ToByte(float64(color.Alpha) / 255 * style.fillOpacity * opacity)
			context.surface.FillPath(path, style.fillRule, color)
		}
	}
	stroke := strings.TrimSpace(style.stroke)
	if stroke != "" && !strings.EqualFold(stroke, "none") && style.strokeWidth > 0 {
		if color, ok := ParseColor(stroke); ok {
			color.Alpha = geometry.ToByte(float64(color.Alpha) / 255 * style.strokeOpacity * opacity)
			context.surface.StrokePath(path, StrokeStyle{
				Width:      style.strokeWidth,
				Cap:        style.lineCap,
				Join:       style.lineJoin,
				MiterLimit: 10,
			}, color)
		}
	}
}

func (context svgContext) gradientFor(value string) *svgNode {
	reference := strings.TrimSpace(value)
	reference = strings.TrimSuffix(strings.TrimPrefix(reference, "url("), ")")
	reference = strings.Trim(reference, `"'`)
	reference = strings.TrimPrefix(reference, "#")
	return context.gradients[reference]
}

func (context svgContext) paintGradientMask(mask *Mask, gradient *svgNode, bounds Rect, opacity float64) {
	kind := strings.ToLower(gradient.name.Local)
	if kind != "lineargradient" && kind != "radialgradient" {
		return
	}
	units := strings.ToLower(strings.TrimSpace(svgAttribute(gradient, "gradientUnits")))
	objectBoundingBox := units != "userspaceonuse"
	stops := svgGradientStops(gradient, opacity)
	if len(stops) == 0 {
		return
	}
	var start, end, center, focal Point
	var radius float64
	if objectBoundingBox {
		start = Point{X: bounds.X + bounds.Width*svgFraction(svgAttributeOrDefault(gradient, "x1", "0%")), Y: bounds.Y + bounds.Height*svgFraction(svgAttributeOrDefault(gradient, "y1", "0%"))}
		end = Point{X: bounds.X + bounds.Width*svgFraction(svgAttributeOrDefault(gradient, "x2", "100%")), Y: bounds.Y + bounds.Height*svgFraction(svgAttributeOrDefault(gradient, "y2", "0%"))}
		center = Point{X: bounds.X + bounds.Width*svgFraction(svgAttributeOrDefault(gradient, "cx", "50%")), Y: bounds.Y + bounds.Height*svgFraction(svgAttributeOrDefault(gradient, "cy", "50%"))}
		focal = center
		radius = math.Max(bounds.Width, bounds.Height) * svgFraction(svgAttributeOrDefault(gradient, "r", "50%"))
	} else {
		start = Point{X: svgAbsolute(svgAttributeOrDefault(gradient, "x1", "0")), Y: svgAbsolute(svgAttributeOrDefault(gradient, "y1", "0"))}
		end = Point{X: svgAbsolute(svgAttributeOrDefault(gradient, "x2", "100%")), Y: svgAbsolute(svgAttributeOrDefault(gradient, "y2", "0%"))}
		center = Point{X: svgAbsolute(svgAttributeOrDefault(gradient, "cx", "50%")), Y: svgAbsolute(svgAttributeOrDefault(gradient, "cy", "50%"))}
		focal = center
		radius = svgAbsolute(svgAttributeOrDefault(gradient, "r", "50%"))
	}
	if gradientTransform, ok := gradient.attribute("gradientTransform"); ok {
		transform := parseSVGTransform(gradientTransform, context.viewWidth, context.viewHeight)
		start = transform.Apply(start)
		end = transform.Apply(end)
		center = transform.Apply(center)
		focal = transform.Apply(focal)
	}
	minX := int(math.Max(0, math.Floor(bounds.X)))
	minY := int(math.Max(0, math.Floor(bounds.Y)))
	maxX := int(math.Min(float64(context.surface.Width), math.Ceil(bounds.X+bounds.Width)))
	maxY := int(math.Min(float64(context.surface.Height), math.Ceil(bounds.Y+bounds.Height)))
	for yPos := minY; yPos < maxY; yPos++ {
		for xPos := minX; xPos < maxX; xPos++ {
			coverage := mask.At(xPos, yPos)
			if coverage <= 0 {
				continue
			}
			point := Point{X: float64(xPos) + 0.5, Y: float64(yPos) + 0.5}
			position := 0.0
			if kind == "lineargradient" {
				deltaX := end.X - start.X
				deltaY := end.Y - start.Y
				denominator := deltaX*deltaX + deltaY*deltaY
				if denominator <= 0 {
					continue
				}
				position = ((point.X-start.X)*deltaX + (point.Y-start.Y)*deltaY) / denominator
			} else {
				if radius <= 0 {
					continue
				}
				position = math.Hypot(point.X-focal.X, point.Y-focal.Y) / radius
			}
			context.surface.blendPixel(xPos, yPos, gradientColorAt(stops, position, false), coverage)
		}
	}
}

func svgGradientStops(gradient *svgNode, opacity float64) []resolvedGradientStop {
	var stops []resolvedGradientStop
	for _, child := range gradient.children {
		if !strings.EqualFold(child.name.Local, "stop") {
			continue
		}
		stopColor := svgAttributeOrDefault(child, "stop-color", "black")
		color, ok := ParseColor(stopColor)
		if !ok {
			color = NewColor(0, 0, 0)
		}
		stopOpacity := 1.0
		if value := svgAttribute(child, "stop-opacity"); value != "" {
			if parsed, ok := svgNumber(value); ok {
				stopOpacity = clamp01(parsed)
			}
		}
		if styleText := svgAttribute(child, "style"); styleText != "" {
			for _, node := range parser.ParseListOfDeclarations(styleText) {
				declaration, ok := node.(*parser.Declaration)
				if !ok {
					continue
				}
				switch strings.ToLower(declaration.Name) {
				case "stop-color":
					if parsed, ok := ParseColor(strings.TrimSpace(parser.SerializeValues(declaration.Value))); ok {
						color = parsed
					}
				case "stop-opacity":
					if parsed, ok := svgNumber(strings.TrimSpace(parser.SerializeValues(declaration.Value))); ok {
						stopOpacity = clamp01(parsed)
					}
				}
			}
		}
		color.Alpha = geometry.ToByte(float64(color.Alpha) / 255 * stopOpacity * opacity)
		offset := 0.0
		if value, ok := child.attribute("offset"); ok {
			offset = svgFraction(value)
		}
		stops = append(stops, resolvedGradientStop{color: color, position: offset})
	}
	if len(stops) == 0 {
		return nil
	}
	sort.SliceStable(stops, func(first, second int) bool {
		return stops[first].position < stops[second].position
	})
	for index := 1; index < len(stops); index++ {
		if stops[index].position < stops[index-1].position {
			stops[index].position = stops[index-1].position
		}
	}
	return stops
}

func collectSVGGradients(node *svgNode, gradients map[string]*svgNode) {
	if identifier, ok := node.attribute("id"); ok {
		name := strings.ToLower(node.name.Local)
		if name == "lineargradient" || name == "radialgradient" {
			gradients[identifier] = node
		}
	}
	for _, child := range node.children {
		collectSVGGradients(child, gradients)
	}
}

func svgAttributeOrDefault(node *svgNode, name, fallback string) string {
	if value, ok := node.attribute(name); ok && strings.TrimSpace(value) != "" {
		return value
	}
	return fallback
}

func svgFraction(value string) float64 {
	trimmed := strings.TrimSpace(value)
	if strings.HasSuffix(trimmed, "%") {
		number, err := strconv.ParseFloat(strings.TrimSuffix(trimmed, "%"), 64)
		if err == nil {
			return number / 100
		}
	}
	number, err := strconv.ParseFloat(trimmed, 64)
	if err != nil {
		return 0
	}
	return number
}

func svgAbsolute(value string) float64 {
	number, _ := svgLength(value, 1, 16)
	return number
}

func svgNumber(value string) (float64, bool) {
	number, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil {
		return 0, false
	}
	return number, true
}

func svgLength(value string, reference, fontSize float64) (float64, bool) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return 0, false
	}
	if strings.HasSuffix(trimmed, "%") {
		number, err := strconv.ParseFloat(strings.TrimSuffix(trimmed, "%"), 64)
		if err != nil {
			return 0, false
		}
		return number / 100 * reference, true
	}
	for _, suffix := range []string{"px", "pt", "pc", "in", "cm", "mm", "em", "ex"} {
		if strings.HasSuffix(trimmed, suffix) {
			number, err := strconv.ParseFloat(strings.TrimSuffix(trimmed, suffix), 64)
			if err != nil {
				return 0, false
			}
			factor := 1.0
			switch suffix {
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
			case "ex":
				factor = fontSize / 2
			}
			return number * factor, true
		}
	}
	number, err := strconv.ParseFloat(trimmed, 64)
	if err != nil {
		return 0, false
	}
	return number, true
}

func clamp01(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}

type svgPointList struct {
	values []Point
}

func buildSVGShape(node *svgNode) *Path {
	path := NewPath()
	switch strings.ToLower(node.name.Local) {
	case "rect":
		xPos := svgAbsolute(svgAttributeOrDefault(node, "x", "0"))
		yPos := svgAbsolute(svgAttributeOrDefault(node, "y", "0"))
		width := svgAbsolute(svgAttributeOrDefault(node, "width", "0"))
		height := svgAbsolute(svgAttributeOrDefault(node, "height", "0"))
		if width <= 0 || height <= 0 {
			return nil
		}
		radiusX := svgAbsolute(svgAttributeOrDefault(node, "rx", "0"))
		radiusY := svgAbsolute(svgAttributeOrDefault(node, "ry", "0"))
		if radiusX <= 0 && radiusY > 0 {
			radiusX = radiusY
		}
		if radiusY <= 0 && radiusX > 0 {
			radiusY = radiusX
		}
		radii := CornerRadii{
			TopLeft: Point{X: radiusX, Y: radiusY}, TopRight: Point{X: radiusX, Y: radiusY},
			BottomRight: Point{X: radiusX, Y: radiusY}, BottomLeft: Point{X: radiusX, Y: radiusY},
		}
		path.AddRoundedRect(Rect{X: xPos, Y: yPos, Width: width, Height: height}, radii)
	case "circle":
		centerX := svgAbsolute(svgAttributeOrDefault(node, "cx", "0"))
		centerY := svgAbsolute(svgAttributeOrDefault(node, "cy", "0"))
		radius := svgAbsolute(svgAttributeOrDefault(node, "r", "0"))
		if radius <= 0 {
			return nil
		}
		addSVGEllipse(path, centerX, centerY, radius, radius)
	case "ellipse":
		centerX := svgAbsolute(svgAttributeOrDefault(node, "cx", "0"))
		centerY := svgAbsolute(svgAttributeOrDefault(node, "cy", "0"))
		radiusX := svgAbsolute(svgAttributeOrDefault(node, "rx", "0"))
		radiusY := svgAbsolute(svgAttributeOrDefault(node, "ry", "0"))
		if radiusX <= 0 || radiusY <= 0 {
			return nil
		}
		addSVGEllipse(path, centerX, centerY, radiusX, radiusY)
	case "line":
		path.MoveTo(svgAbsolute(svgAttributeOrDefault(node, "x1", "0")), svgAbsolute(svgAttributeOrDefault(node, "y1", "0")))
		path.LineTo(svgAbsolute(svgAttributeOrDefault(node, "x2", "0")), svgAbsolute(svgAttributeOrDefault(node, "y2", "0")))
	case "polyline", "polygon":
		points, ok := parseSVGPoints(svgAttributeOrDefault(node, "points", ""))
		if !ok || len(points) < 2 {
			return nil
		}
		path.MoveTo(points[0].X, points[0].Y)
		for _, point := range points[1:] {
			path.LineTo(point.X, point.Y)
		}
		if strings.EqualFold(node.name.Local, "polygon") {
			path.Close()
		}
	case "path":
		data := svgAttributeOrDefault(node, "d", "")
		if data == "" {
			return nil
		}
		if !parseSVGPathData(path, data) {
			return nil
		}
	default:
		return nil
	}
	return path
}

func addSVGEllipse(path *Path, centerX, centerY, radiusX, radiusY float64) {
	const arc = 0.5522847498307936
	path.MoveTo(centerX+radiusX, centerY)
	path.CubicTo(centerX+radiusX, centerY+radiusY*arc, centerX+radiusX*arc, centerY+radiusY, centerX, centerY+radiusY)
	path.CubicTo(centerX-radiusX*arc, centerY+radiusY, centerX-radiusX, centerY+radiusY*arc, centerX-radiusX, centerY)
	path.CubicTo(centerX-radiusX, centerY-radiusY*arc, centerX-radiusX*arc, centerY-radiusY, centerX, centerY-radiusY)
	path.CubicTo(centerX+radiusX*arc, centerY-radiusY, centerX+radiusX, centerY-radiusY*arc, centerX+radiusX, centerY)
	path.Close()
}

func parseSVGPoints(text string) ([]Point, bool) {
	fields := strings.FieldsFunc(text, func(character rune) bool {
		return character == ',' || character == ' ' || character == '\t' || character == '\n' || character == '\r'
	})
	if len(fields)%2 != 0 {
		return nil, false
	}
	points := make([]Point, 0, len(fields)/2)
	for index := 0; index < len(fields); index += 2 {
		xPos, errX := strconv.ParseFloat(fields[index], 64)
		yPos, errY := strconv.ParseFloat(fields[index+1], 64)
		if errX != nil || errY != nil {
			return nil, false
		}
		points = append(points, Point{X: xPos, Y: yPos})
	}
	return points, true
}

func (context svgContext) transformFor(node *svgNode, width, height float64) geometry.Affine {
	value, ok := node.attribute("transform")
	if !ok || strings.TrimSpace(value) == "" {
		return geometry.IdentityAffine()
	}
	return parseSVGTransform(value, width, height)
}

func parseSVGTransform(value string, width, height float64) geometry.Affine {
	matrix := geometry.IdentityAffine()
	values := parser.ParseComponentValueList(value)
	box := &layouts.Box{Width: width, Height: height}
	for _, item := range values {
		if parser.IsWhitespace(item) {
			continue
		}
		function, ok := item.(*parser.Function)
		if !ok {
			continue
		}
		name := strings.ToLower(function.Name)
		arguments := nonWhitespaceValues(function.Value)
		if name == "rotate" && len(arguments) >= 3 {
			angle, okAngle := transformAngle(arguments, 0)
			centerX, okX := transformLength(arguments, 1, width)
			centerY, okY := transformLength(arguments, 2, height)
			if okAngle && okX && okY {
				next := geometry.TranslationAffine(centerX, centerY).Multiply(geometry.RotationAffine(angle)).Multiply(geometry.TranslationAffine(-centerX, -centerY))
				matrix = matrix.Multiply(next)
			}
			continue
		}
		next, ok := transformFunction(function, box)
		if ok {
			matrix = matrix.Multiply(next)
		}
	}
	return matrix
}

func parseSVGPathData(path *Path, data string) bool {
	parser := &svgPathDataParser{data: data, path: path}
	return parser.parse()
}

type svgPathDataParser struct {
	data           string
	index          int
	path           *Path
	current        Point
	start          Point
	lastControl    Point
	lastCommand    byte
	hasLastControl bool
}

func (parser *svgPathDataParser) parse() bool {
	for {
		parser.skipSpace()
		if parser.index >= len(parser.data) {
			return true
		}
		command := parser.data[parser.index]
		if isSVGCommand(command) {
			parser.index++
		} else if parser.lastCommand == 0 {
			return false
		} else {
			command = parser.lastCommand
			if command == 'M' {
				command = 'L'
			} else if command == 'm' {
				command = 'l'
			}
		}
		switch command {
		case 'M', 'm':
			xPos, okX := parser.number()
			yPos, okY := parser.number()
			if !okX || !okY {
				return false
			}
			if command == 'm' {
				xPos += parser.current.X
				yPos += parser.current.Y
			}
			parser.current = Point{X: xPos, Y: yPos}
			parser.start = parser.current
			parser.path.MoveTo(xPos, yPos)
			parser.lastCommand = 'L'
			if command == 'm' {
				parser.lastCommand = 'l'
			}
			parser.hasLastControl = false
		case 'L', 'l':
			for {
				xPos, okX := parser.number()
				yPos, okY := parser.number()
				if !okX || !okY {
					break
				}
				if command == 'l' {
					xPos += parser.current.X
					yPos += parser.current.Y
				}
				parser.path.LineTo(xPos, yPos)
				parser.current = Point{X: xPos, Y: yPos}
				parser.lastCommand = command
				parser.hasLastControl = false
				parser.skipSpace()
				if parser.index >= len(parser.data) || isSVGCommand(parser.data[parser.index]) {
					break
				}
			}
		case 'H', 'h':
			for {
				xPos, ok := parser.number()
				if !ok {
					break
				}
				if command == 'h' {
					xPos += parser.current.X
				}
				parser.path.LineTo(xPos, parser.current.Y)
				parser.current.X = xPos
				parser.lastCommand = command
				parser.hasLastControl = false
				parser.skipSpace()
				if parser.index >= len(parser.data) || isSVGCommand(parser.data[parser.index]) {
					break
				}
			}
		case 'V', 'v':
			for {
				yPos, ok := parser.number()
				if !ok {
					break
				}
				if command == 'v' {
					yPos += parser.current.Y
				}
				parser.path.LineTo(parser.current.X, yPos)
				parser.current.Y = yPos
				parser.lastCommand = command
				parser.hasLastControl = false
				parser.skipSpace()
				if parser.index >= len(parser.data) || isSVGCommand(parser.data[parser.index]) {
					break
				}
			}
		case 'C', 'c':
			for {
				x1, ok1 := parser.number()
				y1, ok2 := parser.number()
				x2, ok3 := parser.number()
				y2, ok4 := parser.number()
				xPos, ok5 := parser.number()
				yPos, ok6 := parser.number()
				if !ok1 || !ok2 || !ok3 || !ok4 || !ok5 || !ok6 {
					break
				}
				if command == 'c' {
					x1 += parser.current.X
					y1 += parser.current.Y
					x2 += parser.current.X
					y2 += parser.current.Y
					xPos += parser.current.X
					yPos += parser.current.Y
				}
				parser.path.CubicTo(x1, y1, x2, y2, xPos, yPos)
				parser.current = Point{X: xPos, Y: yPos}
				parser.lastControl = Point{X: x2, Y: y2}
				parser.hasLastControl = true
				parser.lastCommand = command
				parser.skipSpace()
				if parser.index >= len(parser.data) || isSVGCommand(parser.data[parser.index]) {
					break
				}
			}
		case 'S', 's':
			for {
				x2, ok1 := parser.number()
				y2, ok2 := parser.number()
				xPos, ok3 := parser.number()
				yPos, ok4 := parser.number()
				if !ok1 || !ok2 || !ok3 || !ok4 {
					break
				}
				if command == 's' {
					x2 += parser.current.X
					y2 += parser.current.Y
					xPos += parser.current.X
					yPos += parser.current.Y
				}
				controlX := parser.current.X
				controlY := parser.current.Y
				if parser.hasLastControl {
					controlX = 2*parser.current.X - parser.lastControl.X
					controlY = 2*parser.current.Y - parser.lastControl.Y
				}
				parser.path.CubicTo(controlX, controlY, x2, y2, xPos, yPos)
				parser.current = Point{X: xPos, Y: yPos}
				parser.lastControl = Point{X: x2, Y: y2}
				parser.hasLastControl = true
				parser.lastCommand = command
				parser.skipSpace()
				if parser.index >= len(parser.data) || isSVGCommand(parser.data[parser.index]) {
					break
				}
			}
		case 'Q', 'q':
			for {
				x1, ok1 := parser.number()
				y1, ok2 := parser.number()
				xPos, ok3 := parser.number()
				yPos, ok4 := parser.number()
				if !ok1 || !ok2 || !ok3 || !ok4 {
					break
				}
				if command == 'q' {
					x1 += parser.current.X
					y1 += parser.current.Y
					xPos += parser.current.X
					yPos += parser.current.Y
				}
				parser.path.QuadTo(x1, y1, xPos, yPos)
				parser.current = Point{X: xPos, Y: yPos}
				parser.lastControl = Point{X: x1, Y: y1}
				parser.hasLastControl = true
				parser.lastCommand = command
				parser.skipSpace()
				if parser.index >= len(parser.data) || isSVGCommand(parser.data[parser.index]) {
					break
				}
			}
		case 'T', 't':
			for {
				xPos, okX := parser.number()
				yPos, okY := parser.number()
				if !okX || !okY {
					break
				}
				if command == 't' {
					xPos += parser.current.X
					yPos += parser.current.Y
				}
				controlX := parser.current.X
				controlY := parser.current.Y
				if parser.hasLastControl {
					controlX = 2*parser.current.X - parser.lastControl.X
					controlY = 2*parser.current.Y - parser.lastControl.Y
				}
				parser.path.QuadTo(controlX, controlY, xPos, yPos)
				parser.current = Point{X: xPos, Y: yPos}
				parser.lastControl = Point{X: controlX, Y: controlY}
				parser.hasLastControl = true
				parser.lastCommand = command
				parser.skipSpace()
				if parser.index >= len(parser.data) || isSVGCommand(parser.data[parser.index]) {
					break
				}
			}
		case 'Z', 'z':
			parser.path.Close()
			parser.current = parser.start
			parser.lastCommand = command
			parser.hasLastControl = false
		case 'A', 'a':
			if !parser.arc(command) {
				return false
			}
		default:
			return false
		}
	}
}

func (parser *svgPathDataParser) arc(command byte) bool {
	radiusX, ok1 := parser.number()
	radiusY, ok2 := parser.number()
	rotation, ok3 := parser.number()
	largeArc, ok4 := parser.number()
	sweep, ok5 := parser.number()
	xPos, ok6 := parser.number()
	yPos, ok7 := parser.number()
	if !ok1 || !ok2 || !ok3 || !ok4 || !ok5 || !ok6 || !ok7 {
		return false
	}
	if command == 'a' {
		xPos += parser.current.X
		yPos += parser.current.Y
	}
	end := Point{X: xPos, Y: yPos}
	points := svgArcPoints(parser.current, end, math.Abs(radiusX), math.Abs(radiusY), rotation, largeArc != 0, sweep != 0)
	for _, point := range points {
		parser.path.LineTo(point.X, point.Y)
	}
	parser.current = end
	parser.lastCommand = command
	parser.hasLastControl = false
	return true
}

func svgArcPoints(start, end Point, radiusX, radiusY, rotation float64, largeArc, sweep bool) []Point {
	if radiusX == 0 || radiusY == 0 || (start.X == end.X && start.Y == end.Y) {
		return []Point{end}
	}
	phi := rotation * math.Pi / 180
	cosPhi := math.Cos(phi)
	sinPhi := math.Sin(phi)
	deltaX := (start.X - end.X) / 2
	deltaY := (start.Y - end.Y) / 2
	x1 := cosPhi*deltaX + sinPhi*deltaY
	y1 := -sinPhi*deltaX + cosPhi*deltaY
	lambda := (x1*x1)/(radiusX*radiusX) + (y1*y1)/(radiusY*radiusY)
	if lambda > 1 {
		scale := math.Sqrt(lambda)
		radiusX *= scale
		radiusY *= scale
	}
	numerator := radiusX*radiusX*radiusY*radiusY - radiusX*radiusX*y1*y1 - radiusY*radiusY*x1*x1
	denominator := radiusX*radiusX*y1*y1 + radiusY*radiusY*x1*x1
	if denominator == 0 {
		return []Point{end}
	}
	factor := math.Sqrt(math.Max(0, numerator/denominator))
	if largeArc == sweep {
		factor = -factor
	}
	centerX := factor * radiusX * y1 / radiusY
	centerY := -factor * radiusY * x1 / radiusX
	middleX := (start.X + end.X) / 2
	middleY := (start.Y + end.Y) / 2
	center := Point{
		X: cosPhi*centerX - sinPhi*centerY + middleX,
		Y: sinPhi*centerX + cosPhi*centerY + middleY,
	}
	startAngle := math.Atan2((y1-centerY)/radiusY, (x1-centerX)/radiusX)
	endAngle := math.Atan2((-y1-centerY)/radiusY, (-x1-centerX)/radiusX)
	deltaAngle := endAngle - startAngle
	if !sweep && deltaAngle > 0 {
		deltaAngle -= 2 * math.Pi
	} else if sweep && deltaAngle < 0 {
		deltaAngle += 2 * math.Pi
	}
	segments := int(math.Ceil(math.Abs(deltaAngle) / (math.Pi / 8)))
	if segments < 1 {
		segments = 1
	}
	points := make([]Point, 0, segments)
	for index := 1; index <= segments; index++ {
		angle := startAngle + deltaAngle*float64(index)/float64(segments)
		pointX := center.X + radiusX*math.Cos(angle)*cosPhi - radiusY*math.Sin(angle)*sinPhi
		pointY := center.Y + radiusX*math.Cos(angle)*sinPhi + radiusY*math.Sin(angle)*cosPhi
		points = append(points, Point{X: pointX, Y: pointY})
	}
	return points
}

func isSVGCommand(character byte) bool {
	switch character {
	case 'M', 'm', 'L', 'l', 'H', 'h', 'V', 'v', 'C', 'c', 'S', 's', 'Q', 'q', 'T', 't', 'A', 'a', 'Z', 'z':
		return true
	default:
		return false
	}
}

func (parser *svgPathDataParser) skipSpace() {
	for parser.index < len(parser.data) {
		character := parser.data[parser.index]
		if character == ' ' || character == '\t' || character == '\n' || character == '\r' || character == ',' {
			parser.index++
			continue
		}
		break
	}
}

func (parser *svgPathDataParser) number() (float64, bool) {
	parser.skipSpace()
	start := parser.index
	if parser.index < len(parser.data) && (parser.data[parser.index] == '+' || parser.data[parser.index] == '-') {
		parser.index++
	}
	for parser.index < len(parser.data) {
		character := parser.data[parser.index]
		if (character >= '0' && character <= '9') || character == '.' || character == 'e' || character == 'E' || character == '+' || character == '-' {
			parser.index++
			continue
		}
		break
	}
	if start == parser.index {
		return 0, false
	}
	value, err := strconv.ParseFloat(parser.data[start:parser.index], 64)
	if err != nil {
		return 0, false
	}
	return value, true
}
