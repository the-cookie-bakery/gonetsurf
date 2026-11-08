package layouts

import (
	"gonetsurf/dom"
	"gonetsurf/styles"
)

// DocumentLayout caches a laid-out box tree for a document and invalidates it
// when the document mutation version changes or the viewport is updated.
type DocumentLayout struct {
	Document       *dom.Document
	Engine         *styles.Engine
	ViewportWidth  float64
	ViewportHeight float64
	Shaper         TextShaper
	Metrics        func(*styles.Style) FontMetrics

	box              *Box
	layout           *Layout
	version          uint64
	structureVersion uint64
	hasLayout        bool
}

// NewDocumentLayout creates a layout cache for a document.
func NewDocumentLayout(document *dom.Document, engine *styles.Engine, viewportWidth, viewportHeight float64) *DocumentLayout {
	return &DocumentLayout{
		Document:       document,
		Engine:         engine,
		ViewportWidth:  viewportWidth,
		ViewportHeight: viewportHeight,
	}
}

// Invalidate discards the cached layout.
func (documentLayout *DocumentLayout) Invalidate() {
	documentLayout.hasLayout = false
	documentLayout.box = nil
}

// Box returns the cached box tree, or nil before the first reflow.
func (documentLayout *DocumentLayout) Box() *Box {
	return documentLayout.box
}

// Reflow returns the laid-out box tree, rebuilding only when the document has
// mutated or the cache was invalidated.
func (documentLayout *DocumentLayout) Reflow() *Box {
	if documentLayout.Document == nil {
		return nil
	}
	if documentLayout.hasLayout && documentLayout.version == documentLayout.Document.MutationVersion() {
		return documentLayout.box
	}

	if documentLayout.hasLayout && documentLayout.Engine != nil &&
		!documentLayout.Engine.HasGeneratedContent() &&
		documentLayout.structureVersion == documentLayout.Document.StructuralVersion() {
		dirtyNodes := documentLayout.Document.DirtyNodes()
		styleMap := documentLayout.Engine.ComputeDocumentTree(documentLayout.Document)
		refreshBoxStyles(documentLayout.box, styleMap)
		documentLayout.layout = New(documentLayout.ViewportWidth, documentLayout.ViewportHeight)
		documentLayout.layout.Shaper = documentLayout.Shaper
		documentLayout.layout.Metrics = documentLayout.Metrics
		documentLayout.layout.Dirty = dirtyBoxSet(documentLayout.box, dirtyNodes)
		documentLayout.layout.Layout(documentLayout.box)
		documentLayout.Document.ClearDirty()
		documentLayout.version = documentLayout.Document.MutationVersion()
		return documentLayout.box
	}
	documentLayout.layout = New(documentLayout.ViewportWidth, documentLayout.ViewportHeight)
	documentLayout.layout.Shaper = documentLayout.Shaper
	documentLayout.layout.Metrics = documentLayout.Metrics
	documentLayout.box = BuildDocument(documentLayout.Document, documentLayout.Engine)
	documentLayout.layout.Layout(documentLayout.box)
	documentLayout.version = documentLayout.Document.MutationVersion()
	documentLayout.structureVersion = documentLayout.Document.StructuralVersion()
	documentLayout.hasLayout = true
	return documentLayout.box
}

func refreshBoxStyle(box *Box, style *styles.Style) {
	box.Style = style
	box.Margin = styleEdges(style, "margin")
	box.Padding = styleEdges(style, "padding")
	box.Border = borderEdges(style)
	box.FontSize, box.LineHeight = fontMetrics(style)
	box.Ascent = MetricsForStyle(style).Ascent
	box.ZIndex = parseZIndex(style.GetString("z-index"))
	box.ClipX = clipsOverflow(style.GetString("overflow-x"))
	box.ClipY = clipsOverflow(style.GetString("overflow-y"))
	box.ScrollX = scrollsOverflow(style.GetString("overflow-x"))
	box.ScrollY = scrollsOverflow(style.GetString("overflow-y"))
	box.TextRotation = textRotation(style.GetString("writing-mode"))
	box.intrinsicsComputed = false
	box.IntrinsicMinWidth = 0
	box.IntrinsicMaxWidth = 0
}

func dirtyBoxSet(root *Box, nodes []*dom.Node) map[*dom.Node]bool {
	if len(nodes) == 0 {
		return nil
	}
	dirty := map[*dom.Node]bool{}
	for _, node := range nodes {
		box := findBoxByNode(root, node)
		if box == nil {
			return nil
		}
		for ancestor := box; ancestor != nil; ancestor = ancestor.Parent {
			if ancestor.Node != nil {
				dirty[ancestor.Node] = true
			}
		}
		markBoxSubtreeDirty(box, dirty)
	}
	return dirty
}

func markBoxSubtreeDirty(box *Box, dirty map[*dom.Node]bool) {
	if box == nil {
		return
	}
	if box.Node != nil {
		dirty[box.Node] = true
	}
	for _, child := range box.Children {
		markBoxSubtreeDirty(child, dirty)
	}
}

func findBoxByNode(box *Box, node *dom.Node) *Box {
	if box == nil || node == nil {
		return nil
	}
	if box.Node == node {
		return box
	}
	for _, child := range box.Children {
		if found := findBoxByNode(child, node); found != nil {
			return found
		}
	}
	return nil
}

func refreshBoxStyles(box *Box, styleMap map[*dom.Node]*styles.Style) {
	if box == nil {
		return
	}
	if box.Kind == TextBox {
		if box.Parent != nil {
			box.Style = box.Parent.Style
			box.FontSize, box.LineHeight = fontMetrics(box.Style)
			box.Ascent = MetricsForStyle(box.Style).Ascent
		}
		return
	}
	if box.Node != nil {
		if style := styleMap[box.Node]; style != nil {
			refreshBoxStyle(box, style)
		}
	} else if box.Parent != nil && box.Parent.Style != nil {
		refreshBoxStyle(box, box.Parent.Style)
	}
	for _, child := range box.Children {
		refreshBoxStyles(child, styleMap)
	}
}
