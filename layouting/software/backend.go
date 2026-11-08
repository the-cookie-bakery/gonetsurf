package software

import (
	"gonetsurf/dom"
	"gonetsurf/layouting/interfaces"
	"gonetsurf/layouting/model"
)

// Backend constructs software layout engines.
type Backend struct{}

// NewBackend returns the software layout backend.
func NewBackend() interfaces.Backend {
	return &Backend{}
}

// Name returns the backend name.
func (backend *Backend) Name() string { return "software" }

// NewEngine returns a software layout engine.
func (backend *Backend) NewEngine(options interfaces.Options) (interfaces.Engine, error) {
	layout := New(options.ViewportWidth, options.ViewportHeight)
	if options.Shaper != nil {
		layout.Shaper = options.Shaper
	}
	if options.Metrics != nil {
		layout.Metrics = options.Metrics
	}
	if options.MeasureText != nil {
		layout.MeasureText = options.MeasureText
	}
	if options.IntrinsicSize != nil {
		intrinsic := options.IntrinsicSize
		layout.IntrinsicSize = func(box *model.Box) (float64, float64, bool, bool) {
			return intrinsic(box.Node, box.Style)
		}
	}
	return &Engine{layout: layout, options: options}, nil
}

// Engine adapts the software layout algorithm to interfaces.Engine.
type Engine struct {
	layout  *Layout
	options interfaces.Options
}

// Name returns the engine name.
func (engine *Engine) Name() string { return "software" }

// Layout lays out root and reports full damage when geometry may have changed.
func (engine *Engine) Layout(root *model.Box, request interfaces.Request) (interfaces.Result, error) {
	if root == nil {
		return interfaces.Result{Complete: true}, nil
	}
	engine.layout.ViewportWidth = engine.options.ViewportWidth
	engine.layout.ViewportHeight = engine.options.ViewportHeight
	engine.layout.FontEpoch = request.Resources.Fonts
	engine.layout.ImageEpoch = request.Resources.Images
	engine.layout.StyleEpoch = request.Resources.Styles
	engine.layout.Generation++
	if request.Dirty.WholeTree || len(request.Dirty.Nodes) == 0 {
		engine.layout.Dirty = nil
	} else {
		engine.layout.Dirty = dirtyNodes(request.Dirty.Nodes)
	}
	before := geometrySnapshot(root)
	engine.layout.Layout(root)
	result := interfaces.Result{Complete: true}
	result.Damage = changedRects(before, geometrySnapshot(root))
	return result, nil
}

func geometrySnapshot(root *model.Box) map[*model.Box]model.Rect {
	snapshot := map[*model.Box]model.Rect{}
	var visit func(*model.Box)
	visit = func(box *model.Box) {
		if box == nil {
			return
		}
		snapshot[box] = model.Rect{X: box.X, Y: box.Y, Width: box.Width, Height: box.Height}
		for _, child := range box.Children {
			visit(child)
		}
	}
	visit(root)
	return snapshot
}

func changedRects(before, after map[*model.Box]model.Rect) []model.Rect {
	damage := map[*model.Box]model.Rect{}
	for box, previous := range before {
		if current, ok := after[box]; !ok {
			damage[box] = previous
		} else if !previous.Equal(current) {
			damage[box] = previous.Union(current)
		}
	}
	for box, current := range after {
		if _, ok := before[box]; !ok {
			damage[box] = current
		}
	}
	rects := make([]model.Rect, 0, len(damage))
	for _, rect := range damage {
		rects = append(rects, rect)
	}
	return rects
}

// Close releases engine resources.
func (engine *Engine) Close() error { return nil }

func dirtyNodes(nodes []*dom.Node) map[*dom.Node]bool {
	dirty := make(map[*dom.Node]bool, len(nodes))
	for _, node := range nodes {
		dirty[node] = true
	}
	return dirty
}

func treeBounds(root *model.Box) model.Rect {
	if root == nil {
		return model.Rect{}
	}
	bounds := model.Rect{X: root.X, Y: root.Y, Width: root.Width, Height: root.Height}
	for _, child := range root.Children {
		bounds = bounds.Union(treeBounds(child))
	}
	return bounds
}
