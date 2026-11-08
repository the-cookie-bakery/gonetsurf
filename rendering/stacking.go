package rendering

import (
	"gonetsurf/rendering/interfaces"
	"sort"
	"strings"

	"gonetsurf/layouts"
)

type stackingEntry struct {
	box     *layouts.Box
	zIndex  int
	order   int
	offsetX float64
	offsetY float64
	clip    Rect
	hasClip bool
	pseudo  bool
}

func (painter *Painter) paintStackingContext(box *layouts.Box, surface interfaces.Canvas, offsetX, offsetY float64) {
	painter.paintStackingContextWithEffects(box, surface, offsetX, offsetY, true)
}

func (painter *Painter) paintStackingContextWithEffects(box *layouts.Box, surface interfaces.Canvas, offsetX, offsetY float64, applyEffects bool) {
	if box == nil {
		return
	}
	if applyEffects {
		matrix, hasTransform := parseBoxTransformMatrix(box)
		opacity, hasOpacity := boxOpacity(box)
		if (hasTransform && !matrix.isIdentity()) || hasOpacity {
			layer := surface.NewLayer()
			painter.paintStackingContextWithEffects(box, layer, offsetX, offsetY, false)
			surface.CompositeLayer(layer, toGeometryMatrix4(matrix), opacity)
			return
		}
	}
	if box.Kind == layouts.TextBox {
		painter.paintText(box, surface, offsetX, offsetY)
		return
	}

	xPos, yPos, deltaX, deltaY := paintOrigin(box, offsetX, offsetY)
	painter.paintDecorations(box, surface, xPos, yPos)

	childOffsetX := offsetX + deltaX
	childOffsetY := offsetY + deltaY

	if mask := roundedPaddingClipMask(box, surface, childOffsetX, childOffsetY); mask != nil {
		layer := surface.NewLayer()
		layer.SetClip(surface.Clip())
		painter.paintBoxContents(box, layer, childOffsetX, childOffsetY, false)
		surface.CompositeLayerMasked(layer, surface.Clip(), mask, 1)
	} else {
		painter.paintBoxContents(box, surface, childOffsetX, childOffsetY, true)
	}
	painter.paintScrollbars(box, surface, xPos, yPos)
}

func (painter *Painter) paintBoxContents(box *layouts.Box, surface interfaces.Canvas, contentOffsetX, contentOffsetY float64, clipContent bool) {
	clip := surface.Clip()
	hasClip := true
	if box.ClipX || box.ClipY {
		clip = boxClipRect(box, contentOffsetX, contentOffsetY, surface.CanvasWidth(), surface.CanvasHeight()).Intersect(clip)
	}

	var entries []stackingEntry
	counter := 0
	collectStackingEntries(box, contentOffsetX-box.ScrollLeft, contentOffsetY-box.ScrollTop, clip, hasClip, &entries, &counter)
	sort.SliceStable(entries, func(first, second int) bool {
		if entries[first].zIndex == entries[second].zIndex {
			return entries[first].order < entries[second].order
		}
		return entries[first].zIndex < entries[second].zIndex
	})

	for _, entry := range entries {
		if entry.zIndex < 0 {
			painter.paintStackingEntry(surface, entry)
		}
	}
	if clipContent {
		painter.paintContent(box, surface, contentOffsetX, contentOffsetY)
	} else {
		painter.paintContentRaw(box, surface, contentOffsetX, contentOffsetY)
	}
	for _, entry := range entries {
		if entry.zIndex >= 0 {
			painter.paintStackingEntry(surface, entry)
		}
	}
}

func (painter *Painter) paintStackingEntry(surface interfaces.Canvas, entry stackingEntry) {
	previousClip := surface.Clip()
	if entry.hasClip {
		surface.SetClip(entry.clip)
	}
	if entry.pseudo {
		painter.paintPseudoContext(entry.box, surface, entry.offsetX, entry.offsetY)
	} else {
		painter.paintStackingContextWithEffects(entry.box, surface, entry.offsetX, entry.offsetY, true)
	}
	surface.SetClip(previousClip)
}

// roundedPaddingClipMask clips overflow to the padding-edge curve. CSS
// Backgrounds and Borders 3 §4.3 requires non-visible overflow to follow the
// curved padding edge.
func roundedPaddingClipMask(box *layouts.Box, surface interfaces.Canvas, offsetX, offsetY float64) *Mask {
	if box == nil || box.Style == nil || surface == nil || !box.ClipX || !box.ClipY {
		return nil
	}
	base := box.ContainingWidth
	if base <= 0 {
		base = box.Width
	}
	radii := borderRadii(box.Style, box.Width, box.Height, base)
	paddingRadii := insetRadii(radii, areaInsets(box, "padding-box", base))
	if !hasNonZeroRadii(paddingRadii) {
		return nil
	}
	paddingRect := boxAreaRect(box, "padding-box", offsetX, offsetY, base)
	if paddingRect.Width <= 0 || paddingRect.Height <= 0 {
		return nil
	}
	path := NewPath()
	path.AddRoundedRect(paddingRect, paddingRadii)
	return RasterizeFill(path, surface.CanvasWidth(), surface.CanvasHeight(), NonZeroFill)
}

func (painter *Painter) paintPseudoContext(box *layouts.Box, surface interfaces.Canvas, offsetX, offsetY float64) {
	if box == nil {
		return
	}
	xPos, yPos, deltaX, deltaY := paintOrigin(box, offsetX, offsetY)
	painter.paintDecorations(box, surface, xPos, yPos)
	painter.paintContent(box, surface, offsetX+deltaX, offsetY+deltaY)
	painter.paintScrollbars(box, surface, xPos, yPos)
}

func (painter *Painter) paintContent(box *layouts.Box, surface interfaces.Canvas, offsetX, offsetY float64) {
	if mask := roundedPaddingClipMask(box, surface, offsetX, offsetY); mask != nil {
		layer := surface.NewLayer()
		layer.SetClip(surface.Clip())
		painter.paintContentRaw(box, layer, offsetX, offsetY)
		surface.CompositeLayerMasked(layer, surface.Clip(), mask, 1)
		return
	}
	painter.paintContentRaw(box, surface, offsetX, offsetY)
}

func (painter *Painter) paintContentRaw(box *layouts.Box, surface interfaces.Canvas, offsetX, offsetY float64) {
	if box != nil && box.Node != nil && painter.framePainted[box.Node] {
		return
	}
	previousClip := surface.Clip()
	clipChanged := false
	if box.ClipX || box.ClipY {
		surface.SetClip(boxClipRect(box, offsetX, offsetY, surface.CanvasWidth(), surface.CanvasHeight()))
		clipChanged = true
	}
	childOffsetX := offsetX - box.ScrollLeft
	childOffsetY := offsetY - box.ScrollTop
	for _, child := range box.Children {
		if child == nil {
			continue
		}
		if child.Kind == layouts.TextBox {
			painter.paintText(child, surface, childOffsetX, childOffsetY)
			continue
		}
		if isStackingContext(child) || isPositioned(child) {
			continue
		}
		xPos, yPos, deltaX, deltaY := paintOrigin(child, childOffsetX, childOffsetY)
		painter.paintDecorations(child, surface, xPos, yPos)
		painter.paintContent(child, surface, childOffsetX+deltaX, childOffsetY+deltaY)
		painter.paintScrollbars(child, surface, xPos, yPos)
	}
	if clipChanged {
		surface.SetClip(previousClip)
	}
}

func paintOrigin(box *layouts.Box, offsetX, offsetY float64) (float64, float64, float64, float64) {
	xPos := box.X + offsetX
	yPos := box.Y + offsetY
	if box.Style == nil || !strings.EqualFold(strings.TrimSpace(box.Style.GetString("position")), "sticky") {
		return xPos, yPos, 0, 0
	}
	container := box.ScrollContainer()
	if container == nil {
		return xPos, yPos, 0, 0
	}
	stickyX, stickyY := box.StickyOffset(container.ScrollLeft, container.ScrollTop)
	return stickyX + offsetX + container.ScrollLeft,
		stickyY + offsetY + container.ScrollTop,
		stickyX - box.X,
		stickyY - box.Y
}

func (painter *Painter) paintDecorations(box *layouts.Box, surface interfaces.Canvas, xPos, yPos float64) {
	if box.Style != nil {
		painter.paintOuterShadows(box, surface, xPos, yPos)
	}
	painter.paintBackground(box, surface, xPos-box.X, yPos-box.Y)
	painter.paintBorders(box, surface, xPos, yPos)
	if box.Style != nil {
		painter.paintInnerShadows(box, surface, xPos, yPos)
	}
	painter.paintReplacedContent(box, surface, xPos-box.X, yPos-box.Y)
}

func collectStackingEntries(box *layouts.Box, offsetX, offsetY float64, clip Rect, hasClip bool, entries *[]stackingEntry, counter *int) {
	for _, child := range box.Children {
		if child == nil || child.Kind == layouts.TextBox {
			continue
		}
		if isStackingContext(child) {
			*entries = append(*entries, stackingEntry{
				box:     child,
				zIndex:  effectiveZIndex(child),
				order:   *counter,
				offsetX: offsetX,
				offsetY: offsetY,
				clip:    clip,
				hasClip: hasClip,
			})
			*counter++
			continue
		}
		if isPositioned(child) {
			if zIndexAuto(child.Style.GetString("z-index")) {
				*entries = append(*entries, stackingEntry{
					box:     child,
					zIndex:  0,
					order:   *counter,
					offsetX: offsetX,
					offsetY: offsetY,
					clip:    clip,
					hasClip: hasClip,
					pseudo:  true,
				})
				*counter++
				childClip := clip
				childHasClip := hasClip
				if child.ClipX || child.ClipY {
					rect := boxClipRect(child, offsetX, offsetY, 1<<30, 1<<30)
					childClip = rect.Intersect(clip)
					childHasClip = true
				}
				collectStackingEntries(child, offsetX-child.ScrollLeft, offsetY-child.ScrollTop, childClip, childHasClip, entries, counter)
				continue
			}
			*entries = append(*entries, stackingEntry{
				box:     child,
				zIndex:  effectiveZIndex(child),
				order:   *counter,
				offsetX: offsetX,
				offsetY: offsetY,
				clip:    clip,
				hasClip: hasClip,
			})
			*counter++
			continue
		}
		childClip := clip
		childHasClip := hasClip
		if child.ClipX || child.ClipY {
			rect := boxClipRect(child, offsetX, offsetY, 1<<30, 1<<30)
			childClip = rect.Intersect(clip)
			childHasClip = true
		}
		nextOffsetX := offsetX - child.ScrollLeft
		nextOffsetY := offsetY - child.ScrollTop
		collectStackingEntries(child, nextOffsetX, nextOffsetY, childClip, childHasClip, entries, counter)
	}
}

func boxClipRect(box *layouts.Box, offsetX, offsetY float64, surfaceWidth, surfaceHeight int) Rect {
	xPos := box.X + offsetX
	yPos := box.Y + offsetY
	rect := Rect{X: xPos, Y: yPos, Width: box.Width, Height: box.Height}
	if box.ClipX && !box.ClipY {
		rect.Y = -float64(surfaceHeight)
		rect.Height = float64(surfaceHeight * 2)
	}
	if box.ClipY && !box.ClipX {
		rect.X = -float64(surfaceWidth)
		rect.Width = float64(surfaceWidth * 2)
	}
	return rect
}

func isPositioned(box *layouts.Box) bool {
	if box == nil || box.Style == nil {
		return false
	}
	position := strings.ToLower(strings.TrimSpace(box.Style.GetString("position")))
	return position != "" && position != "static"
}

func isStackingContext(box *layouts.Box) bool {
	if box == nil || box.Style == nil {
		return false
	}
	if hasBoxTransform(box) {
		return true
	}
	if _, hasOpacity := boxOpacity(box); hasOpacity {
		return true
	}
	position := strings.ToLower(strings.TrimSpace(box.Style.GetString("position")))
	if position == "fixed" || position == "sticky" {
		return true
	}
	if position != "" && position != "static" && !zIndexAuto(box.Style.GetString("z-index")) {
		return true
	}
	if strings.EqualFold(strings.TrimSpace(box.Style.GetString("isolation")), "isolate") {
		return true
	}
	return false
}

func effectiveZIndex(box *layouts.Box) int {
	if box == nil || box.Style == nil {
		return 0
	}
	if zIndexAuto(box.Style.GetString("z-index")) {
		return 0
	}
	return box.ZIndex
}

func zIndexAuto(value string) bool {
	trimmed := strings.ToLower(strings.TrimSpace(value))
	return trimmed == "" || trimmed == "auto"
}

func boxOpacity(box *layouts.Box) (float64, bool) {
	if box == nil || box.Style == nil {
		return 1, false
	}
	value, ok := parseOpacityValue(box.Style.GetString("opacity"))
	if !ok || value >= 1 {
		return 1, false
	}
	return value, true
}
