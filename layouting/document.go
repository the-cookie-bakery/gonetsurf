package layouting

import (
	"os"
	"reflect"

	"gonetsurf/dom"
	"gonetsurf/layouting/interfaces"
	"gonetsurf/layouting/model"
	"gonetsurf/layouting/software"
	"gonetsurf/styles"
)

// DocumentLayout caches a laid-out box tree for a document and invalidates it
// when the document mutation version changes or the viewport is updated.
type DocumentLayout struct {
	Document       *dom.Document
	Engine         *styles.Engine
	ViewportWidth  float64
	ViewportHeight float64
	Shaper         model.TextShaper
	Metrics        func(*styles.Style) model.FontMetrics
	IntrinsicSize  func(node *dom.Node, style *styles.Style) (width, height float64, ok bool, pending bool)
	Backend        interfaces.Backend
	Resources      interfaces.ResourceEpochs

	box              *model.Box
	flow             interfaces.Engine
	version          uint64
	structureVersion uint64
	engineWidth      float64
	engineHeight     float64
	engineShaper     model.TextShaper
	engineMetrics    func(*styles.Style) model.FontMetrics
	engineIntrinsic  func(node *dom.Node, style *styles.Style) (float64, float64, bool, bool)
	hasLayout        bool
	resourcesDirty   bool
	damage           []model.Rect
	pending          []interfaces.ResourceRef
	flowErr          error
	geometryDirty    bool
}

// NewDocumentLayout creates a layout cache for a document using the software
// layout backend.
func NewDocumentLayout(document *dom.Document, engine *styles.Engine, viewportWidth, viewportHeight float64) *DocumentLayout {
	return NewDocumentLayoutWithBackend(document, engine, software.NewBackend(), viewportWidth, viewportHeight)
}

// NewDocumentLayoutWithBackend creates a layout cache for a document using the
// supplied layout backend.
func NewDocumentLayoutWithBackend(document *dom.Document, engine *styles.Engine, backend interfaces.Backend, viewportWidth, viewportHeight float64) *DocumentLayout {
	return &DocumentLayout{
		Document:       document,
		Engine:         engine,
		ViewportWidth:  viewportWidth,
		ViewportHeight: viewportHeight,
		Backend:        backend,
	}
}

// Invalidate discards the cached layout.
func (documentLayout *DocumentLayout) Invalidate() {
	documentLayout.hasLayout = false
	documentLayout.box = nil
	documentLayout.flow = nil
	documentLayout.flowErr = nil
	documentLayout.geometryDirty = false
}

// MarkGeometryDirty asks the next reflow to refresh backend-resident geometry
// without rebuilding the box tree. It is used after scroll offsets change.
func (documentLayout *DocumentLayout) MarkGeometryDirty() {
	if documentLayout != nil {
		documentLayout.geometryDirty = true
	}
}

// Error returns the error from the most recent engine construction or reflow.
func (documentLayout *DocumentLayout) Error() error {
	if documentLayout == nil {
		return nil
	}
	return documentLayout.flowErr
}

// Box returns the cached box tree, or nil before the first reflow.
func (documentLayout *DocumentLayout) Box() *model.Box {
	return documentLayout.box
}

// GeometryHandle returns backend-resident geometry when available.
func (documentLayout *DocumentLayout) GeometryHandle() interfaces.GeometryHandle {
	if documentLayout == nil || documentLayout.flow == nil {
		return nil
	}
	provider, _ := documentLayout.flow.(interface {
		GeometryHandle() interfaces.GeometryHandle
	})
	if provider == nil {
		return nil
	}
	return provider.GeometryHandle()
}

// Query returns the backend geometry query interface when available.
func (documentLayout *DocumentLayout) Query() interfaces.GeometryQuery {
	if documentLayout == nil || documentLayout.flow == nil {
		return nil
	}
	query, _ := documentLayout.flow.(interfaces.GeometryQuery)
	return query
}

// Damage returns the rectangles changed by the most recent reflow.
func (documentLayout *DocumentLayout) Damage() []model.Rect {
	return documentLayout.damage
}

// Pending returns the resources blocking the most recent reflow.
func (documentLayout *DocumentLayout) Pending() []interfaces.ResourceRef {
	return documentLayout.pending
}

// MarkResource records that a resource changed and marks the document dirty.
func (documentLayout *DocumentLayout) MarkResource(kind interfaces.ResourceKind) {
	switch kind {
	case interfaces.ResourceFont:
		documentLayout.Resources.Fonts++
	case interfaces.ResourceImage:
		documentLayout.Resources.Images++
	case interfaces.ResourceStylesheet:
		documentLayout.Resources.Styles++
	}
	documentLayout.resourcesDirty = true
}

// Reflow returns the laid-out box tree, rebuilding only when the document has
// mutated or the cache was invalidated.
func (documentLayout *DocumentLayout) Reflow() *model.Box {
	if documentLayout.Document == nil {
		return nil
	}
	if documentLayout.hasLayout && !documentLayout.resourcesDirty && !documentLayout.geometryDirty &&
		documentLayout.version == documentLayout.Document.MutationVersion() {
		return documentLayout.box
	}

	request := interfaces.Request{}
	if documentLayout.resourcesDirty && documentLayout.hasLayout {
		request.Dirty = interfaces.DirtySet{Reason: interfaces.DirtyImage, WholeTree: true}
	} else if documentLayout.hasLayout && documentLayout.Engine != nil &&
		!documentLayout.Engine.HasGeneratedContent() &&
		documentLayout.structureVersion == documentLayout.Document.StructuralVersion() {
		dirtyNodes := documentLayout.Document.DirtyNodes()
		styleMap := documentLayout.Engine.ComputeDocumentTree(documentLayout.Document)
		refreshBoxStyles(documentLayout.box, styleMap)
		request.Dirty = interfaces.DirtySet{Nodes: expandDirtyNodes(documentLayout.box, dirtyNodes), Reason: interfaces.DirtyStyle}
	} else {
		documentLayout.box = model.BuildDocument(documentLayout.Document, documentLayout.Engine)
		request.Dirty = interfaces.DirtySet{Reason: interfaces.DirtyStructure, WholeTree: true}
		documentLayout.structureVersion = documentLayout.Document.StructuralVersion()
	}

	documentLayout.ensureEngine()
	request.Resources = documentLayout.Resources
	if documentLayout.flow == nil {
		documentLayout.Document.ClearDirty()
		documentLayout.version = documentLayout.Document.MutationVersion()
		documentLayout.hasLayout = false
		documentLayout.resourcesDirty = false
		return nil
	}
	result, err := documentLayout.flow.Layout(documentLayout.box, request)
	if err == nil {
		documentLayout.flowErr = nil
		documentLayout.damage = result.Damage
		documentLayout.pending = result.Pending
		documentLayout.geometryDirty = !result.Complete
	} else {
		documentLayout.flowErr = err
	}
	documentLayout.Document.ClearDirty()
	documentLayout.version = documentLayout.Document.MutationVersion()
	documentLayout.hasLayout = true
	documentLayout.resourcesDirty = false
	return documentLayout.box
}

func (documentLayout *DocumentLayout) ensureEngine() {
	backend := documentLayout.Backend
	if backend == nil {
		backend = software.NewBackend()
	}
	if documentLayout.flow != nil &&
		documentLayout.engineWidth == documentLayout.ViewportWidth &&
		documentLayout.engineHeight == documentLayout.ViewportHeight &&
		sameInterface(documentLayout.engineShaper, documentLayout.Shaper) &&
		sameFunc(documentLayout.engineMetrics, documentLayout.Metrics) &&
		sameIntrinsic(documentLayout.engineIntrinsic, documentLayout.IntrinsicSize) {
		return
	}
	flow, err := backend.NewEngine(interfaces.Options{
		ViewportWidth:  documentLayout.ViewportWidth,
		ViewportHeight: documentLayout.ViewportHeight,
		Shaper:         documentLayout.Shaper,
		Metrics:        documentLayout.Metrics,
		IntrinsicSize:  documentLayout.IntrinsicSize,
	})
	if err != nil {
		if os.Getenv("GONETSURF_VULKAN_NO_FALLBACK") != "" {
			documentLayout.flow = nil
			documentLayout.flowErr = err
			return
		}
		flow, _ = software.NewBackend().NewEngine(interfaces.Options{
			ViewportWidth:  documentLayout.ViewportWidth,
			ViewportHeight: documentLayout.ViewportHeight,
			Shaper:         documentLayout.Shaper,
			Metrics:        documentLayout.Metrics,
			IntrinsicSize:  documentLayout.IntrinsicSize,
		})
	}
	documentLayout.flowErr = nil
	documentLayout.flow = flow
	documentLayout.engineWidth = documentLayout.ViewportWidth
	documentLayout.engineHeight = documentLayout.ViewportHeight
	documentLayout.engineShaper = documentLayout.Shaper
	documentLayout.engineMetrics = documentLayout.Metrics
	documentLayout.engineIntrinsic = documentLayout.IntrinsicSize
}

func sameInterface(first, second model.TextShaper) bool {
	if first == nil || second == nil {
		return first == nil && second == nil
	}
	return reflect.TypeOf(first) == reflect.TypeOf(second) &&
		reflect.ValueOf(first).Kind() == reflect.Ptr &&
		reflect.ValueOf(first).Pointer() == reflect.ValueOf(second).Pointer()
}

func sameFunc(first, second func(*styles.Style) model.FontMetrics) bool {
	if first == nil || second == nil {
		return first == nil && second == nil
	}
	return reflect.ValueOf(first).Pointer() == reflect.ValueOf(second).Pointer()
}

func sameIntrinsic(first, second func(*dom.Node, *styles.Style) (float64, float64, bool, bool)) bool {
	if first == nil || second == nil {
		return first == nil && second == nil
	}
	return reflect.ValueOf(first).Pointer() == reflect.ValueOf(second).Pointer()
}

func expandDirtyNodes(root *model.Box, nodes []*dom.Node) []*dom.Node {
	if len(nodes) == 0 {
		return nil
	}
	seen := map[*dom.Node]bool{}
	var result []*dom.Node
	mark := func(node *dom.Node) {
		if node == nil || seen[node] {
			return
		}
		seen[node] = true
		result = append(result, node)
	}
	var markSubtree func(*model.Box)
	markSubtree = func(box *model.Box) {
		if box == nil {
			return
		}
		mark(box.Node)
		for _, child := range box.Children {
			markSubtree(child)
		}
	}
	for _, node := range nodes {
		box := model.FindBox(root, node)
		if box == nil {
			return nil
		}
		for ancestor := box; ancestor != nil; ancestor = ancestor.Parent {
			mark(ancestor.Node)
		}
		markSubtree(box)
	}
	return result
}

func refreshBoxStyle(box *model.Box, style *styles.Style) {
	box.Style = style
	box.Margin = model.StyleEdges(style, "margin")
	box.Padding = model.StyleEdges(style, "padding")
	box.Border = model.BorderEdges(style)
	box.FontSize, box.LineHeight = model.FontMetricsForStyle(style)
	box.Ascent = model.MetricsForStyle(style).Ascent
	box.ZIndex = model.ParseZIndex(style.GetString("z-index"))
	box.ClipX = model.ClipsOverflow(style.GetString("overflow-x"))
	box.ClipY = model.ClipsOverflow(style.GetString("overflow-y"))
	box.ScrollX = model.ScrollsOverflow(style.GetString("overflow-x"))
	box.ScrollY = model.ScrollsOverflow(style.GetString("overflow-y"))
	box.TextRotation = model.TextRotation(style.GetString("writing-mode"))
	box.IntrinsicsComputed = false
	box.IntrinsicMinWidth = 0
	box.IntrinsicMaxWidth = 0
}

func refreshBoxStyles(box *model.Box, styleMap map[*dom.Node]*styles.Style) {
	if box == nil {
		return
	}
	if box.Kind == model.TextBox {
		if box.Parent != nil {
			box.Style = box.Parent.Style
			box.FontSize, box.LineHeight = model.FontMetricsForStyle(box.Style)
			box.Ascent = model.MetricsForStyle(box.Style).Ascent
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
