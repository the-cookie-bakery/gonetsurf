//go:build vulkan && cgo

package vulkan

import (
	_ "embed"
	"fmt"
	"sync"
	"time"
	"unsafe"

	"gonetsurf/dom"
	"gonetsurf/layouting/interfaces"
	"gonetsurf/layouting/model"
	"gonetsurf/layouting/vulkan/internal/gpu"
	"gonetsurf/styles"
)

//go:embed shaders/layout.comp.spv
var layoutShader []byte

type backendOptions struct {
	ViewportWidth  float64
	ViewportHeight float64
	Shaper         model.TextShaper
	Metrics        func(*styles.Style) model.FontMetrics
	IntrinsicSize  func(*dom.Node, *styles.Style) (float64, float64, bool, bool)
}

// ErrUnavailable reports that the Vulkan layout backend is not available.
var ErrUnavailable error

// Available reports whether the Vulkan layout backend is compiled in.
func Available() bool { return true }

// Backend constructs Vulkan layout engines.
type Backend struct{}

// NewBackend returns the Vulkan layout backend.
func NewBackend() (interfaces.Backend, error) {
	return &Backend{}, nil
}

// Name returns the backend name.
func (backend *Backend) Name() string { return "vulkan" }

// NewEngine returns a Vulkan layout engine.
func (backend *Backend) NewEngine(options interfaces.Options) (interfaces.Engine, error) {
	return &Engine{
		options: backendOptions{
			ViewportWidth:  options.ViewportWidth,
			ViewportHeight: options.ViewportHeight,
			Shaper:         options.Shaper,
			Metrics:        options.Metrics,
			IntrinsicSize:  options.IntrinsicSize,
		},
	}, nil
}

type pushConstants struct {
	mode       int32
	target     int32
	count      int32
	padding    int32
	viewWidth  float32
	viewHeight float32
	queryNode  int32
	queryX     float32
	queryY     float32
	scrollX    float32
	scrollY    float32
}

// Engine runs the compute layout passes.
type Engine struct {
	mutex        sync.Mutex
	options      backendOptions
	context      *gpu.Context
	pipeline     *gpu.Pipeline
	capacity     int
	auxCapacity  int
	lastTree     *encodedTree
	lastIndex    map[*dom.Node]int
	lastValues   []float32
	lastReadback []float32
	passCount    int
	activeBase   int
}

// Name returns the engine name.
func (engine *Engine) Name() string { return "vulkan" }

// Layout runs the compute layout pipeline and copies geometry back into the
// box tree.
func (engine *Engine) Layout(root *model.Box, request interfaces.Request) (interfaces.Result, error) {
	if root == nil {
		return interfaces.Result{Complete: true}, nil
	}
	engine.mutex.Lock()
	defer engine.mutex.Unlock()

	splitTextNodes(root, engine.options)
	tree := encodeTree(root, engine.options)
	if len(tree.nodes) == 0 {
		return interfaces.Result{Complete: true}, nil
	}
	engine.lastTree = nil
	engine.lastIndex = nil
	if err := engine.ensurePipelineSized(len(tree.nodes), len(tree.aux)); err != nil {
		return interfaces.Result{}, err
	}
	engine.passCount = len(tree.nodes)
	engine.activeBase = -1
	if active := activeNodeList(tree, request.Dirty); len(active) > 0 && len(active) < len(tree.nodes) {
		base := len(tree.aux)
		tree.aux = append(tree.aux, make([]float32, len(active))...)
		for index, nodeIndex := range active {
			tree.aux[base+index] = float32(nodeIndex)
		}
		engine.passCount = len(active)
		engine.activeBase = base
	}
	queryFloats := len(tree.aux) + len(tree.nodes)*3 + 8
	if err := engine.ensureAuxCapacity(queryFloats/4 + 1); err != nil {
		return interfaces.Result{}, err
	}
	uploadStart := 0
	uploadEnd := len(tree.values)
	if len(engine.lastValues) == len(tree.values) {
		first := len(tree.values)
		last := -1
		for index, value := range tree.values {
			if value != engine.lastValues[index] {
				if index < first {
					first = index
				}
				last = index
			}
		}
		if last < first {
			result := interfaces.Result{Complete: true}
			for _, pending := range tree.pending {
				result.Pending = append(result.Pending, interfaces.ResourceRef{Kind: interfaces.ResourceImage, URL: pending.url, Node: pending.node})
			}
			return result, nil
		}
		uploadStart = first
		uploadEnd = last + 1
	}
	if uploadStart == 0 && uploadEnd == len(tree.values) {
		if err := engine.context.Upload(0, floatsToBytes(tree.values)); err != nil {
			return interfaces.Result{}, err
		}
	} else {
		if err := engine.context.UploadRange(0, uploadStart*4, floatsToBytes(tree.values[uploadStart:uploadEnd])); err != nil {
			return interfaces.Result{}, err
		}
	}
	if len(tree.aux) > 0 {
		if err := engine.context.Upload(1, floatsToBytes(tree.aux)); err != nil {
			return interfaces.Result{}, err
		}
	}
	if err := engine.context.Begin(); err != nil {
		return interfaces.Result{}, err
	}
	started := time.Now()
	complete := true
	for target := tree.maxDepth; target >= 0; target-- {
		if err := engine.record(0, target, len(tree.nodes)); err != nil {
			return interfaces.Result{}, err
		}
	}
	for target := 0; target <= tree.maxDepth; target++ {
		if err := engine.record(1, target, len(tree.nodes)); err != nil {
			return interfaces.Result{}, err
		}
	}
	for target := 0; target <= tree.maxDepth; target++ {
		if err := engine.record(5, target, len(tree.nodes)); err != nil {
			return interfaces.Result{}, err
		}
	}
	iterations := 3
	for iteration := 0; iteration < iterations; iteration++ {
		for target := 0; target <= tree.maxDepth; target++ {
			if err := engine.record(6, target, len(tree.nodes)); err != nil {
				return interfaces.Result{}, err
			}
		}
		for target := 0; target <= tree.maxDepth; target++ {
			if err := engine.record(8, target, len(tree.nodes)); err != nil {
				return interfaces.Result{}, err
			}
		}
		for target := 0; target <= tree.maxDepth; target++ {
			if err := engine.record(2, target, len(tree.nodes)); err != nil {
				return interfaces.Result{}, err
			}
		}
		for target := tree.maxDepth; target >= 0; target-- {
			if err := engine.record(3, target, len(tree.nodes)); err != nil {
				return interfaces.Result{}, err
			}
		}
		for target := tree.maxDepth; target >= 0; target-- {
			if err := engine.record(14, target, len(tree.nodes)); err != nil {
				return interfaces.Result{}, err
			}
		}
		if request.Budget > 0 && time.Since(started) > request.Budget {
			complete = false
			break
		}
	}
	for target := 0; target <= tree.maxDepth; target++ {
		if err := engine.record(6, target, len(tree.nodes)); err != nil {
			return interfaces.Result{}, err
		}
	}
	for target := 0; target <= tree.maxDepth; target++ {
		if err := engine.record(8, target, len(tree.nodes)); err != nil {
			return interfaces.Result{}, err
		}
	}
	for target := 0; target <= tree.maxDepth; target++ {
		if err := engine.record(10, target, len(tree.nodes)); err != nil {
			return interfaces.Result{}, err
		}
	}
	for target := 0; target <= tree.maxDepth; target++ {
		if err := engine.record(9, target, len(tree.nodes)); err != nil {
			return interfaces.Result{}, err
		}
	}
	for target := tree.maxDepth; target >= 0; target-- {
		if err := engine.record(14, target, len(tree.nodes)); err != nil {
			return interfaces.Result{}, err
		}
	}
	for target := tree.maxDepth; target >= 0; target-- {
		if err := engine.record(4, target, len(tree.nodes)); err != nil {
			return interfaces.Result{}, err
		}
	}
	if err := engine.context.End(); err != nil {
		return interfaces.Result{}, err
	}
	readback := make([]float32, len(tree.values))
	if err := engine.context.Readback(0, floatsToBytes(readback)); err != nil {
		return interfaces.Result{}, err
	}
	applyGeometry(tree, readback)
	result := interfaces.Result{Complete: complete}
	result.Damage = damageFromReadback(engine.lastReadback, readback, tree, dirtyClosure(tree, request.Dirty))
	engine.lastReadback = append(engine.lastReadback[:0], readback...)
	engine.lastValues = append(engine.lastValues[:0], tree.values...)
	engine.lastTree = tree
	engine.lastIndex = make(map[*dom.Node]int, len(tree.nodes))
	for index, encoded := range tree.nodes {
		if encoded.box.Node != nil {
			if _, exists := engine.lastIndex[encoded.box.Node]; !exists {
				engine.lastIndex[encoded.box.Node] = index
			}
		}
	}
	for _, pending := range tree.pending {
		kind := interfaces.ResourceImage
		result.Pending = append(result.Pending, interfaces.ResourceRef{Kind: kind, URL: pending.url, Node: pending.node})
	}
	return result, nil
}

func activeNodeList(tree *encodedTree, dirty interfaces.DirtySet) []int {
	if tree == nil || dirty.WholeTree || len(dirty.Nodes) == 0 {
		return nil
	}
	byNode := make(map[*dom.Node][]int)
	for index, node := range tree.nodes {
		if node.box.Node != nil {
			byNode[node.box.Node] = append(byNode[node.box.Node], index)
		}
	}
	active := make([]bool, len(tree.nodes))
	for _, dirtyNode := range dirty.Nodes {
		indices := byNode[dirtyNode]
		if len(indices) == 0 {
			return nil
		}
		for _, start := range indices {
			candidate := -1
			for current := start; current >= 0; current = tree.nodes[current].parent {
				if tree.nodeIsolationSafe(current) {
					candidate = current
				}
			}
			if candidate < 0 {
				return nil
			}
			subtree := tree.nodes[candidate].subtree
			for offset := 0; offset < subtree && candidate+offset < len(active); offset++ {
				active[candidate+offset] = true
			}
			for ancestor := tree.nodes[candidate].parent; ancestor >= 0; ancestor = tree.nodes[ancestor].parent {
				active[ancestor] = true
				if ancestor == 0 {
					break
				}
			}
		}
	}
	list := make([]int, 0, len(active))
	for index, enabled := range active {
		if enabled {
			list = append(list, index)
		}
	}
	if len(list) == 0 {
		return nil
	}
	return list
}

func (tree *encodedTree) nodeIsolationSafe(index int) bool {
	if index < 0 || index >= len(tree.nodes) {
		return false
	}
	base := index * nodeFieldCount
	flags := int(tree.values[base+nodeFlags2]) | int(tree.values[base+nodeFlags3])<<16
	if flags&(flagFlexContainer|flagGridContainer) != 0 {
		return false
	}
	return tree.values[base+nodeModeW] == 1 && tree.values[base+nodeModeH] == 1
}

func dirtyClosure(tree *encodedTree, dirty interfaces.DirtySet) []bool {
	if tree == nil || dirty.WholeTree || len(dirty.Nodes) == 0 {
		return nil
	}
	mask := make([]bool, len(tree.nodes))
	byNode := make(map[*dom.Node][]int)
	for index, node := range tree.nodes {
		if node.box.Node != nil {
			byNode[node.box.Node] = append(byNode[node.box.Node], index)
		}
	}
	for _, dirtyNode := range dirty.Nodes {
		for _, index := range byNode[dirtyNode] {
			subtree := tree.nodes[index].subtree
			for offset := 0; offset < subtree && index+offset < len(mask); offset++ {
				mask[index+offset] = true
			}
			for ancestor := tree.nodes[index].parent; ancestor >= 0; ancestor = tree.nodes[ancestor].parent {
				mask[ancestor] = true
				if ancestor == 0 {
					break
				}
			}
		}
	}
	return mask
}

func damageFromReadback(previous, current []float32, tree *encodedTree, dirty []bool) []model.Rect {
	if tree == nil {
		return nil
	}
	rects := make([]model.Rect, 0, len(tree.nodes))
	for index := range tree.nodes {
		if dirty != nil && !dirty[index] {
			continue
		}
		offset := index * nodeFieldCount
		if offset+nodeH >= len(current) {
			break
		}
		rect := model.Rect{
			X:      float64(current[offset+nodeX]),
			Y:      float64(current[offset+nodeY]),
			Width:  float64(current[offset+nodeW]),
			Height: float64(current[offset+nodeH]),
		}
		if len(previous) < len(current) || offset+nodeH >= len(previous) {
			rects = append(rects, rect)
			continue
		}
		oldRect := model.Rect{
			X:      float64(previous[offset+nodeX]),
			Y:      float64(previous[offset+nodeY]),
			Width:  float64(previous[offset+nodeW]),
			Height: float64(previous[offset+nodeH]),
		}
		if !oldRect.Equal(rect) {
			rects = append(rects, oldRect.Union(rect))
		}
	}
	return rects
}

func (engine *Engine) ensurePipeline(nodeCount int) error {
	return engine.ensurePipelineSized(nodeCount, 0)
}

func (engine *Engine) ensureAuxCapacity(count int) error {
	capacity := count * 4
	if capacity < 64 {
		capacity = 64
	}
	if capacity > engine.auxCapacity {
		if err := engine.context.CreateBuffer(1, capacity); err != nil {
			return err
		}
		engine.auxCapacity = capacity
	}
	return nil
}

func (engine *Engine) ensurePipelineSized(nodeCount, auxCount int) error {
	if engine.context == nil {
		context, err := gpu.New(2, int(unsafe.Sizeof(pushConstants{})))
		if err != nil {
			return err
		}
		engine.context = context
	}
	capacity := nodeCount * nodeFieldCount * 4
	if capacity < 64 {
		capacity = 64
	}
	if capacity > engine.capacity {
		if err := engine.context.CreateBuffer(0, capacity); err != nil {
			return err
		}
		engine.capacity = capacity
	}
	if err := engine.ensureAuxCapacity(auxCount); err != nil {
		return err
	}
	if engine.pipeline == nil {
		pipeline, err := engine.context.NewPipeline(layoutShader)
		if err != nil {
			return err
		}
		engine.pipeline = pipeline
	}
	return nil
}

func (engine *Engine) dispatch(mode, target, count int) error {
	count = engine.passCount
	const workgroup = 64
	groups := (count + workgroup - 1) / workgroup
	push := pushConstants{
		mode:       int32(mode),
		target:     int32(target),
		count:      int32(count),
		padding:    int32(engine.activeBase),
		viewWidth:  float32(engine.options.ViewportWidth),
		viewHeight: float32(engine.options.ViewportHeight),
	}
	return engine.context.Dispatch(engine.pipeline, groups, 1, pushToBytes(push))
}

func (engine *Engine) record(mode, target, count int) error {
	count = engine.passCount
	const workgroup = 64
	groups := (count + workgroup - 1) / workgroup
	push := pushConstants{
		mode:       int32(mode),
		target:     int32(target),
		count:      int32(count),
		padding:    int32(engine.activeBase),
		viewWidth:  float32(engine.options.ViewportWidth),
		viewHeight: float32(engine.options.ViewportHeight),
	}
	return engine.context.Record(engine.pipeline, groups, 1, pushToBytes(push))
}

// Close releases the Vulkan context and pipeline.
func (engine *Engine) Close() error {
	engine.mutex.Lock()
	defer engine.mutex.Unlock()
	if engine.pipeline != nil {
		engine.pipeline.Destroy()
		engine.pipeline = nil
	}
	if engine.context != nil {
		engine.context.Destroy()
		engine.context = nil
	}
	return nil
}

func applyGeometry(tree *encodedTree, values []float32) {
	for index, encoded := range tree.nodes {
		box := encoded.box
		box.X = float64(values[index*nodeFieldCount+nodeX])
		box.Y = float64(values[index*nodeFieldCount+nodeY])
		box.Width = float64(values[index*nodeFieldCount+nodeW])
		box.Height = float64(values[index*nodeFieldCount+nodeH])
		box.ContentX = float64(values[index*nodeFieldCount+nodeCx])
		box.ContentY = float64(values[index*nodeFieldCount+nodeCy])
		box.ContentWidth = float64(values[index*nodeFieldCount+nodeCw])
		box.ContentHeight = float64(values[index*nodeFieldCount+nodeCh])
		box.ScrollWidth = float64(values[index*nodeFieldCount+nodeScrollW])
		box.ScrollHeight = float64(values[index*nodeFieldCount+nodeScrollH])
		scrollFlags := int(values[index*nodeFieldCount+nodeScrollFlags] + 0.5)
		box.HasVerticalScrollbar = scrollFlags&1 != 0
		box.HasHorizontalScrollbar = scrollFlags&2 != 0
		box.LaidOut = true
		box.Provisional = values[index*nodeFieldCount+nodeReplFlags] != 0
	}
}

func floatsToBytes(values []float32) []byte {
	if len(values) == 0 {
		return nil
	}
	return unsafe.Slice((*byte)(unsafe.Pointer(&values[0])), len(values)*4)
}

func pushToBytes(push pushConstants) []byte {
	return unsafe.Slice((*byte)(unsafe.Pointer(&push)), int(unsafe.Sizeof(push)))
}

type engineGeometryHandle struct {
	buffer uintptr
	count  int
	stride int
	x      int
	y      int
	width  int
	height int
	index  map[*dom.Node]int
}

func (handle *engineGeometryHandle) NodeIndex(node *dom.Node) (int, bool) {
	index, ok := handle.index[node]
	return index, ok
}

func (handle *engineGeometryHandle) NodeCount() int { return handle.count }

func (handle *engineGeometryHandle) Buffer() uintptr { return handle.buffer }

func (handle *engineGeometryHandle) Stride() int { return handle.stride }

func (handle *engineGeometryHandle) FieldOffsets() (int, int, int, int) {
	return handle.x, handle.y, handle.width, handle.height
}

// ActiveNodeCount reports how many nodes the last layout dispatched.
func (engine *Engine) ActiveNodeCount() int {
	engine.mutex.Lock()
	defer engine.mutex.Unlock()
	return engine.passCount
}

// GeometryHandle returns a handle to the backend-resident node geometry.
func (engine *Engine) GeometryHandle() interfaces.GeometryHandle {
	engine.mutex.Lock()
	defer engine.mutex.Unlock()
	if engine.lastTree == nil || engine.context == nil {
		return nil
	}
	buffer := engine.context.BufferHandle(0)
	if buffer == 0 {
		return nil
	}
	return &engineGeometryHandle{
		buffer: buffer,
		count:  len(engine.lastTree.nodes),
		stride: nodeFieldCount,
		x:      nodeX,
		y:      nodeY,
		width:  nodeW,
		height: nodeH,
		index:  engine.lastIndex,
	}
}

// NodeIndex returns the GPU node index for a DOM node.
func (engine *Engine) NodeIndex(node *dom.Node) (int, bool) {
	engine.mutex.Lock()
	defer engine.mutex.Unlock()
	if engine.lastIndex == nil {
		return 0, false
	}
	index, ok := engine.lastIndex[node]
	return index, ok
}

func (engine *Engine) boxIndex(box *model.Box) int {
	if engine.lastTree == nil {
		return -1
	}
	for index, encoded := range engine.lastTree.nodes {
		if encoded.box == box {
			return index
		}
	}
	return -1
}

func (engine *Engine) queryGeometry(mode, node int, x, y, scrollX, scrollY float64) ([]float32, error) {
	if engine.context == nil || engine.pipeline == nil {
		return nil, fmt.Errorf("vulkan: no layout pipeline")
	}
	scratch := 0
	if mode == 11 && engine.lastTree != nil {
		scratch = len(engine.lastTree.aux)
		needed := scratch + len(engine.lastTree.nodes)*3 + 8
		if needed > engine.auxCapacity {
			if err := engine.ensureAuxCapacity(needed/4 + 1); err != nil {
				return nil, err
			}
		}
	}
	push := pushConstants{
		mode:       int32(mode),
		count:      1,
		padding:    -1,
		target:     int32(scratch),
		queryNode:  int32(node),
		queryX:     float32(x),
		queryY:     float32(y),
		scrollX:    float32(scrollX),
		scrollY:    float32(scrollY),
		viewWidth:  float32(engine.options.ViewportWidth),
		viewHeight: float32(engine.options.ViewportHeight),
	}
	if err := engine.context.Begin(); err != nil {
		return nil, err
	}
	if err := engine.context.Record(engine.pipeline, 1, 1, pushToBytes(push)); err != nil {
		return nil, err
	}
	if err := engine.context.End(); err != nil {
		return nil, err
	}
	values := make([]float32, 8)
	if err := engine.context.Readback(1, floatsToBytes(values)); err != nil {
		return nil, err
	}
	return values, nil
}

// HitTest returns the deepest box containing the viewport point.
func (engine *Engine) HitTest(x, y, scrollX, scrollY float64) (*model.Box, model.Rect, bool, error) {
	engine.mutex.Lock()
	defer engine.mutex.Unlock()
	values, err := engine.queryGeometry(11, 0, x, y, scrollX, scrollY)
	if err != nil {
		return nil, model.Rect{}, false, err
	}
	index := int(values[0]+0.5) - 1
	if index < 0 || engine.lastTree == nil || index >= len(engine.lastTree.nodes) {
		return nil, model.Rect{}, false, nil
	}
	box := engine.lastTree.nodes[index].box
	return box, model.Rect{X: float64(values[1]), Y: float64(values[2]), Width: float64(values[3]), Height: float64(values[4])}, true, nil
}

// BoxRect returns the viewport-space rectangle of a box.
func (engine *Engine) BoxRect(box *model.Box, scrollX, scrollY float64) (model.Rect, bool, error) {
	engine.mutex.Lock()
	defer engine.mutex.Unlock()
	index := engine.boxIndex(box)
	if index < 0 {
		return model.Rect{}, false, nil
	}
	values, err := engine.queryGeometry(12, index, 0, 0, scrollX, scrollY)
	if err != nil {
		return model.Rect{}, false, err
	}
	if values[0] < 0.5 {
		return model.Rect{}, false, nil
	}
	return model.Rect{X: float64(values[1]), Y: float64(values[2]), Width: float64(values[3]), Height: float64(values[4])}, true, nil
}

// MaxScroll returns the scroll extent of a box.
func (engine *Engine) MaxScroll(box *model.Box) (float64, float64, bool, error) {
	engine.mutex.Lock()
	defer engine.mutex.Unlock()
	index := engine.boxIndex(box)
	if index < 0 {
		return 0, 0, false, nil
	}
	values, err := engine.queryGeometry(13, index, 0, 0, 0, 0)
	if err != nil {
		return 0, 0, false, err
	}
	if values[0] < 0.5 {
		return 0, 0, false, nil
	}
	scrollWidth := float64(values[1])
	scrollHeight := float64(values[2])
	scrollportWidth := float64(values[3])
	scrollportHeight := float64(values[4])
	scrollFlags := int(values[5] + 0.5)
	if scrollFlags&1 != 0 {
		scrollportWidth = nonNegative(scrollportWidth - model.ScrollbarThickness)
	}
	if scrollFlags&2 != 0 {
		scrollportHeight = nonNegative(scrollportHeight - model.ScrollbarThickness)
	}
	return nonNegative(scrollWidth - scrollportWidth), nonNegative(scrollHeight - scrollportHeight), true, nil
}

func nonNegative(value float64) float64 {
	if value < 0 {
		return 0
	}
	return value
}
