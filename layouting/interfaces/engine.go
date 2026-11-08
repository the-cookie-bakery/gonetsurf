// Package interfaces defines the backend-neutral layout contracts shared by
// the software and Vulkan layout pipelines.
package interfaces

import (
	"time"

	"gonetsurf/dom"
	"gonetsurf/layouting/model"
	"gonetsurf/styles"
)

// Backend constructs layout engines for one pipeline.
type Backend interface {
	Name() string
	NewEngine(options Options) (Engine, error)
}

// Engine computes box geometry for a box tree.
type Engine interface {
	Name() string
	// Layout lays out root in place. Implementations may stop early when the
	// request budget is exhausted and report progress through Result.
	Layout(root *model.Box, request Request) (Result, error)
	Close() error
}

// Options configures an engine for one page viewport.
type Options struct {
	ViewportWidth  float64
	ViewportHeight float64
	Shaper         model.TextShaper
	Metrics        func(*styles.Style) model.FontMetrics
	MeasureText    func(text string, style *styles.Style) (width, height float64)
	// IntrinsicSize reports the intrinsic size of a replaced element. pending
	// is true when the resource is still loading; callers use a placeholder.
	IntrinsicSize func(node *dom.Node, style *styles.Style) (width, height float64, ok bool, pending bool)
}

// DirtyReason identifies why a box needs to be laid out again.
type DirtyReason uint32

const (
	// DirtyStyle marks a style-only change.
	DirtyStyle DirtyReason = 1 << iota
	// DirtyStructure marks a tree-shape change.
	DirtyStructure
	// DirtyFont marks a font metric change.
	DirtyFont
	// DirtyImage marks an image intrinsic-size change.
	DirtyImage
	// DirtyViewport marks a viewport size change.
	DirtyViewport
	// DirtyContent marks a text or replaced-content change.
	DirtyContent
)

// DirtySet describes the boxes that need reflow.
type DirtySet struct {
	Nodes     []*dom.Node
	Reason    DirtyReason
	WholeTree bool
}

// ResourceEpochs counts resource generations. Layout caches compare epochs to
// decide whether a measurement is still valid.
type ResourceEpochs struct {
	Fonts  uint64
	Images uint64
	Styles uint64
}

// ResourceKind identifies an asynchronous resource that can change layout.
type ResourceKind uint8

const (
	// ResourceImage is a decoded image.
	ResourceImage ResourceKind = iota
	// ResourceFont is an author font.
	ResourceFont
	// ResourceStylesheet is an external stylesheet.
	ResourceStylesheet
)

// ResourceRef identifies a pending resource.
type ResourceRef struct {
	Kind ResourceKind
	URL  string
	Node *dom.Node
}

// Request asks an engine to lay out a tree.
type Request struct {
	Dirty      DirtySet
	Resources  ResourceEpochs
	Budget     time.Duration
	DamageHint model.Rect
}

// Result reports the outcome of a layout request.
type Result struct {
	// Complete is false when a budget stopped the engine before it finished.
	Complete bool
	// Damage lists viewport-space rectangles whose geometry changed.
	Damage []model.Rect
	// Pending lists resources blocking provisional measurements.
	Pending []ResourceRef
}

// GeometryHandle exposes a backend-resident node geometry buffer.
type GeometryHandle interface {
	NodeIndex(node *dom.Node) (int, bool)
	NodeCount() int
	Buffer() uintptr
	Stride() int
	FieldOffsets() (x, y, width, height int)
}

// GeometryQuery exposes GPU-side geometry queries for backends that keep
// layout geometry resident on the device.
type GeometryQuery interface {
	NodeIndex(node *dom.Node) (int, bool)
	HitTest(x, y, scrollX, scrollY float64) (*model.Box, model.Rect, bool, error)
	BoxRect(box *model.Box, scrollX, scrollY float64) (model.Rect, bool, error)
	MaxScroll(box *model.Box) (width, height float64, ok bool, err error)
}
