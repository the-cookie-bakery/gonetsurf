package rendering

import (
	"gonetsurf/layouting/model"
	"gonetsurf/rendering/interfaces"
	"strconv"
	"strings"

	"gonetsurf/dom"
	"gonetsurf/urls"
)

type imageCandidate struct {
	address string
	width   float64
	density float64
}

func (painter *Painter) paintReplacedContent(box *model.Box, surface interfaces.Canvas, offsetX, offsetY float64) {
	if box.Node == nil || box.Node.Type != dom.ElementNode {
		return
	}
	tag := strings.ToLower(box.Node.Data)
	contentX := box.ContentX + offsetX
	contentY := box.ContentY + offsetY
	contentWidth := box.ContentWidth
	contentHeight := box.ContentHeight
	if contentWidth <= 0 || contentHeight <= 0 {
		contentX = box.X + offsetX
		contentY = box.Y + offsetY
		contentWidth = box.Width
		contentHeight = box.Height
	}
	if contentWidth <= 0 || contentHeight <= 0 {
		return
	}

	var source string
	switch tag {
	case "iframe":
		if painter.Subframes != nil {
			frame, err := painter.Subframes.RenderSubframe(box.Node, int(contentWidth), int(contentHeight))
			if err == nil && frame != nil {
				if painter.framePainted == nil {
					painter.framePainted = make(map[*dom.Node]bool)
				}
				painter.framePainted[box.Node] = true
				contentRect := Rect{X: contentX, Y: contentY, Width: contentWidth, Height: contentHeight}
				surface.DrawImage(frame.Image(), contentRect, contentRect, contentClipMask(box, surface, contentRect), 1)
			}
		}
		return
	case "img":
		source = imageSourceFor(box.Node, contentWidth)
	case "video":
		if poster, ok := box.Node.GetAttribute("poster"); ok {
			source = poster
		}
	case "canvas":
		// Canvas content requires scripting; the element is transparent for now.
		return
	default:
		return
	}
	if strings.TrimSpace(source) == "" {
		return
	}
	if painter.BaseURL != nil {
		if resolved, err := urls.ParseReference(painter.BaseURL, source); err == nil {
			source = resolved.String()
		}
	}
	picture := painter.loadImageURL(source)
	if picture == nil {
		return
	}
	contentRect := Rect{X: contentX, Y: contentY, Width: contentWidth, Height: contentHeight}
	intrinsicWidth := float64(picture.Bounds().Dx())
	intrinsicHeight := float64(picture.Bounds().Dy())
	if intrinsicWidth <= 0 || intrinsicHeight <= 0 {
		return
	}
	fit := "fill"
	if box.Style != nil {
		if value := strings.ToLower(strings.TrimSpace(box.Style.GetString("object-fit"))); value != "" {
			fit = value
		}
	}
	destination := objectFitRect(contentRect, intrinsicWidth, intrinsicHeight, fit)
	surface.DrawImage(picture, destination, contentRect, contentClipMask(box, surface, contentRect), 1)
}

// contentClipMask clips replaced content to the content-edge curve, as
// required by CSS Backgrounds and Borders 3 §4.3 and exercised by WPT
// css/css-backgrounds/border-radius-clip-001.html.
func contentClipMask(box *model.Box, surface interfaces.Canvas, contentRect Rect) *Mask {
	if box == nil || box.Style == nil || surface == nil {
		return nil
	}
	base := box.ContainingWidth
	if base <= 0 {
		base = box.Width
	}
	radii := borderRadii(box.Style, box.Width, box.Height, base)
	contentRadii := insetRadii(radii, areaInsets(box, "content-box", base))
	if !hasNonZeroRadii(contentRadii) {
		return nil
	}
	path := NewPath()
	path.AddRoundedRect(contentRect, contentRadii)
	return RasterizeFill(path, surface.CanvasWidth(), surface.CanvasHeight(), NonZeroFill)
}

func imageSourceFor(node *dom.Node, contentWidth float64) string {
	candidatesText := ""
	if node.Parent() != nil && strings.EqualFold(node.Parent().Data, "picture") {
		for _, child := range node.Parent().Children() {
			if child.Type != dom.ElementNode || !strings.EqualFold(child.Data, "source") {
				continue
			}
			if value, ok := child.GetAttribute("srcset"); ok && strings.TrimSpace(value) != "" {
				candidatesText = value
				break
			}
		}
	}
	if candidatesText == "" {
		if value, ok := node.GetAttribute("srcset"); ok {
			candidatesText = value
		}
	}
	if candidatesText != "" {
		if chosen := chooseImageCandidate(candidatesText, contentWidth); chosen != "" {
			return chosen
		}
	}
	if source, ok := node.GetAttribute("src"); ok {
		return source
	}
	return ""
}

func chooseImageCandidate(srcset string, contentWidth float64) string {
	candidates := parseImageCandidates(srcset)
	if len(candidates) == 0 {
		return ""
	}
	hasWidth := false
	for _, candidate := range candidates {
		if candidate.width > 0 {
			hasWidth = true
			break
		}
	}
	if hasWidth {
		best := candidates[0]
		bestWidth := 0.0
		for _, candidate := range candidates {
			if candidate.width <= 0 {
				continue
			}
			if candidate.width >= contentWidth && (bestWidth == 0 || candidate.width < bestWidth) {
				best = candidate
				bestWidth = candidate.width
			}
		}
		if bestWidth == 0 {
			for _, candidate := range candidates {
				if candidate.width > bestWidth {
					best = candidate
					bestWidth = candidate.width
				}
			}
		}
		return best.address
	}
	best := candidates[0]
	bestDistance := 1.0
	first := true
	for _, candidate := range candidates {
		density := candidate.density
		if density <= 0 {
			density = 1
		}
		distance := density - 1
		if distance < 0 {
			distance = -distance
		}
		if first || distance < bestDistance {
			best = candidate
			bestDistance = distance
			first = false
		}
	}
	return best.address
}

func parseImageCandidates(srcset string) []imageCandidate {
	var candidates []imageCandidate
	for _, part := range strings.Split(srcset, ",") {
		fields := strings.Fields(strings.TrimSpace(part))
		if len(fields) == 0 {
			continue
		}
		candidate := imageCandidate{address: fields[0], density: 1}
		if len(fields) > 1 {
			descriptor := strings.ToLower(fields[1])
			switch {
			case strings.HasSuffix(descriptor, "w"):
				if value, err := strconv.ParseFloat(strings.TrimSuffix(descriptor, "w"), 64); err == nil {
					candidate.width = value
					candidate.density = 0
				}
			case strings.HasSuffix(descriptor, "x"):
				if value, err := strconv.ParseFloat(strings.TrimSuffix(descriptor, "x"), 64); err == nil {
					candidate.density = value
				}
			}
		}
		candidates = append(candidates, candidate)
	}
	return candidates
}

func objectFitRect(content Rect, intrinsicWidth, intrinsicHeight float64, fit string) Rect {
	switch fit {
	case "contain":
		scale := minFloat(content.Width/intrinsicWidth, content.Height/intrinsicHeight)
		return centeredRect(content, intrinsicWidth*scale, intrinsicHeight*scale)
	case "cover":
		scale := maxFloat(content.Width/intrinsicWidth, content.Height/intrinsicHeight)
		return centeredRect(content, intrinsicWidth*scale, intrinsicHeight*scale)
	case "none":
		return centeredRect(content, intrinsicWidth, intrinsicHeight)
	case "scale-down":
		scale := minFloat(1, minFloat(content.Width/intrinsicWidth, content.Height/intrinsicHeight))
		return centeredRect(content, intrinsicWidth*scale, intrinsicHeight*scale)
	default:
		return content
	}
}

func centeredRect(content Rect, width, height float64) Rect {
	return Rect{
		X:      content.X + (content.Width-width)/2,
		Y:      content.Y + (content.Height-height)/2,
		Width:  width,
		Height: height,
	}
}

func minFloat(first, second float64) float64 {
	if first < second {
		return first
	}
	return second
}
