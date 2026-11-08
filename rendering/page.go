package rendering

import (
	"gonetsurf/layouting"
	"gonetsurf/layouting/model"
	"image"
	"strings"
	"sync"
	"sync/atomic"

	"gonetsurf/dom"
	layoutinterfaces "gonetsurf/layouting/interfaces"
	"gonetsurf/parsers/css/mediaqueries"
	"gonetsurf/rendering/fonts"
	"gonetsurf/rendering/interfaces"
	"gonetsurf/rendering/scene"
	"gonetsurf/rendering/software"
	"gonetsurf/styles"
	"gonetsurf/urls"
)

// Page owns the style engine, cached layout, and viewport state for one
// document. It is safe to reuse across frames so scrolling and hit-testing do
// not repeat parsing or style resolution.
type Page struct {
	Document *dom.Document
	Engine   *styles.Engine
	Options  Options
	Width    int
	Height   int

	layout           *layouting.DocumentLayout
	library          *fonts.Library
	scrollX          float64
	scrollY          float64
	fontLoadCallback func()
	fontLayoutDirty  atomic.Bool
	imageLayoutDirty atomic.Bool
	resourceMutex    sync.RWMutex
	resourceCallback func()
}

// NewPage returns a page for document and engine.
func NewPage(document *dom.Document, engine *styles.Engine, width, height int, options Options) *Page {
	page := &Page{
		Document: document,
		Engine:   engine,
		Options:  options,
		Width:    maxInt(width, 0),
		Height:   maxInt(height, 0),
	}
	page.library = fontLibraryFor(document, engine, options)
	page.library.SetAsyncAuthorFonts(options.AsyncFonts)
	page.updateMediaEnvironment()
	page.layout = layouting.NewDocumentLayoutWithBackend(document, engine, options.LayoutBackend, float64(page.Width), float64(page.Height))
	page.layout.Shaper = fonts.NewShaper(page.library)
	page.layout.Metrics = page.library.Metrics
	page.layout.IntrinsicSize = page.intrinsicSize
	page.attachImageCallback()
	return page
}

// Resize updates the viewport and invalidates the cached layout.
func (page *Page) Resize(width, height int) {
	if page == nil {
		return
	}
	width = maxInt(width, 0)
	height = maxInt(height, 0)
	if width == page.Width && height == page.Height {
		return
	}
	page.Width = width
	page.Height = height
	page.updateMediaEnvironment()
	if page.layout != nil {
		page.layout.ViewportWidth = float64(width)
		page.layout.ViewportHeight = float64(height)
		page.layout.Invalidate()
	}
}

// Invalidate discards the cached layout and style-dependent resources.
func (page *Page) Invalidate() {
	if page == nil {
		return
	}
	if page.layout != nil {
		page.layout.Invalidate()
	}
	page.library = fontLibraryFor(page.Document, page.Engine, page.Options)
	page.library.SetAsyncAuthorFonts(page.Options.AsyncFonts)
	if page.fontLoadCallback != nil {
		page.library.SetLoadCallback(page.fontLoadCallback)
	}
	if page.layout != nil {
		page.layout.Shaper = fonts.NewShaper(page.library)
		page.layout.Metrics = page.library.Metrics
		page.layout.IntrinsicSize = page.intrinsicSize
	}
}

// SetFontLoadedCallback installs a callback invoked after an asynchronous
// author font loads. The cached layout is invalidated so the next frame uses
// the real face.
func (page *Page) SetFontLoadedCallback(callback func()) {
	if page == nil {
		return
	}
	page.resourceMutex.Lock()
	page.resourceCallback = callback
	page.resourceMutex.Unlock()
	page.fontLoadCallback = func() {
		page.fontLayoutDirty.Store(true)
		page.notifyResourceLoaded()
	}
	if page.library != nil {
		page.library.SetLoadCallback(page.fontLoadCallback)
	}
}

func (page *Page) attachImageCallback() {
	setter, ok := page.Options.Images.(interface{ SetLoadCallback(func(string)) })
	if !ok {
		return
	}
	setter.SetLoadCallback(func(string) {
		page.imageLayoutDirty.Store(true)
		page.notifyResourceLoaded()
	})
}

func (page *Page) notifyResourceLoaded() {
	page.resourceMutex.RLock()
	callback := page.resourceCallback
	page.resourceMutex.RUnlock()
	if callback != nil {
		callback()
	}
}

// intrinsicSize reports replaced-element dimensions to the layout engine. A
// pending image uses a provisional box so the first frame can be painted
// before the image decodes.
func (page *Page) intrinsicSize(node *dom.Node, style *styles.Style) (float64, float64, bool, bool) {
	if page == nil || node == nil || node.Type != dom.ElementNode {
		return 0, 0, false, false
	}
	tag := strings.ToLower(node.Data)
	source := ""
	switch tag {
	case "img":
		source = imageSourceFor(node, 0)
	case "video":
		source, _ = node.GetAttribute("poster")
	default:
		return 0, 0, false, false
	}
	if strings.TrimSpace(source) == "" {
		return 0, 0, false, false
	}
	peeker, ok := page.Options.Images.(interface {
		PeekImage(string) (image.Image, bool)
	})
	if !ok {
		return 300, 150, true, false
	}
	loaded, ok := peeker.PeekImage(source)
	if !ok || loaded == nil {
		return 0, 0, false, true
	}
	bounds := loaded.Bounds()
	if bounds.Dx() <= 0 || bounds.Dy() <= 0 {
		return 0, 0, false, true
	}
	return float64(bounds.Dx()), float64(bounds.Dy()), true, false
}

// Damage returns the layout damage from the most recent render.
func (page *Page) Damage() []model.Rect {
	if page == nil || page.layout == nil {
		return nil
	}
	return page.layout.Damage()
}

func (page *Page) updateMediaEnvironment() {
	if page.Engine == nil {
		return
	}
	environment := mediaqueries.DefaultEnvironment()
	environment.Width = float64(page.Width)
	environment.Height = float64(page.Height)
	page.Engine.SetMediaEnvironment(environment)
}

// Box returns the laid-out document box tree.
func (page *Page) Box() *model.Box {
	return page.root()
}

func (page *Page) root() *model.Box {
	if page == nil || page.layout == nil {
		return nil
	}
	return page.layout.Reflow()
}

// Scroll returns the current scroll offsets.
func (page *Page) Scroll() (float64, float64) {
	if page == nil {
		return 0, 0
	}
	return page.scrollX, page.scrollY
}

// MaxScroll returns the maximum scroll offsets for the document.
func (page *Page) MaxScroll() (float64, float64) {
	root := page.root()
	if root == nil {
		return 0, 0
	}
	if page.layout != nil {
		if query := page.layout.Query(); query != nil {
			if width, height, ok, err := query.MaxScroll(root); err == nil && ok {
				return width, height
			}
		}
	}
	return maxFloatValue(0, root.ScrollWidth-float64(page.Width)), maxFloatValue(0, root.ScrollHeight-float64(page.Height))
}

// ScrollTo sets the scroll offsets, clamped to the document's scroll range.
func (page *Page) ScrollTo(left, top float64) {
	if left == 0 && top == 0 && page.scrollX == 0 && page.scrollY == 0 {
		return
	}
	root := page.root()
	if root == nil {
		return
	}
	page.applyScroll(root, left, top)
}

func (page *Page) applyScroll(root *model.Box, left, top float64) {
	maxX := maxFloatValue(0, root.ScrollWidth-float64(page.Width))
	maxY := maxFloatValue(0, root.ScrollHeight-float64(page.Height))
	if left < 0 {
		left = 0
	}
	if top < 0 {
		top = 0
	}
	if left > maxX {
		left = maxX
	}
	if top > maxY {
		top = maxY
	}
	page.scrollX = left
	page.scrollY = top
	root.ScrollLeft = left
	root.ScrollTop = top
	if page.layout != nil {
		page.layout.MarkGeometryDirty()
	}
}

// ScrollBy adjusts the scroll offsets by the given deltas.
func (page *Page) ScrollBy(deltaX, deltaY float64) {
	if page == nil {
		return
	}
	page.ScrollTo(page.scrollX+deltaX, page.scrollY+deltaY)
}

// BoxForNode returns the laid-out box generated for node.
func (page *Page) BoxForNode(node *dom.Node) *model.Box {
	if page == nil || node == nil {
		return nil
	}
	return model.FindBox(page.root(), node)
}

// BoxRect returns the viewport-space rectangle of a node's box.
func (page *Page) BoxRect(node *dom.Node) (model.Rect, bool) {
	if page == nil || node == nil {
		return model.Rect{}, false
	}
	root := page.root()
	if root == nil {
		return model.Rect{}, false
	}
	page.applyScroll(root, page.scrollX, page.scrollY)
	box := model.FindBox(root, node)
	if box == nil {
		return model.Rect{}, false
	}
	if page.layout != nil {
		if query := page.layout.Query(); query != nil {
			if rect, ok, err := query.BoxRect(box, 0, 0); err == nil && ok {
				return rect, true
			}
		}
	}
	return model.BoxRect(root, box)
}

// ScrollTargetAt returns the nearest scroll container at a viewport point,
// falling back to the document root.
func (page *Page) ScrollTargetAt(x, y float64) *model.Box {
	root := page.root()
	if root == nil {
		return nil
	}
	hit := page.HitTest(x, y)
	for box := hit; box != nil; box = box.Parent {
		if box.ScrollX || box.ScrollY {
			return box
		}
	}
	return root
}

// ScrollByAt scrolls the scroll container under a viewport point.
func (page *Page) ScrollByAt(x, y, deltaX, deltaY float64) {
	target := page.ScrollTargetAt(x, y)
	if target == nil {
		return
	}
	if target == page.root() {
		page.ScrollBy(deltaX, deltaY)
		return
	}
	target.ScrollBy(deltaX, deltaY)
	if page.layout != nil {
		page.layout.MarkGeometryDirty()
	}
}

// ScrollToAt scrolls the container under a viewport point to an offset.
func (page *Page) ScrollToAt(x, y, left, top float64) {
	target := page.ScrollTargetAt(x, y)
	if target == nil {
		return
	}
	if target == page.root() {
		page.ScrollTo(left, top)
		return
	}
	target.ScrollTo(left, top)
	if page.layout != nil {
		page.layout.MarkGeometryDirty()
	}
}

// MaxScrollAt returns the scroll range of the container under a viewport point.
func (page *Page) MaxScrollAt(x, y float64) (float64, float64) {
	target := page.ScrollTargetAt(x, y)
	if target == nil {
		return 0, 0
	}
	if target == page.root() {
		return page.MaxScroll()
	}
	return target.MaxScrollLeft(), target.MaxScrollTop()
}

// HitTest returns the deepest box at a viewport-space point.
func (page *Page) HitTest(x, y float64) *model.Box {
	root := page.root()
	if root == nil {
		return nil
	}
	page.applyScroll(root, page.scrollX, page.scrollY)
	if page.layout != nil {
		if query := page.layout.Query(); query != nil {
			if box, _, found, err := query.HitTest(x, y, 0, 0); err == nil && found {
				return box
			}
		}
	}
	return model.HitTest(root, x, y)
}

// Render lays out the document when necessary and paints it.
func (page *Page) Render() (interfaces.Surface, error) {
	if page == nil {
		return software.NewSurface(0, 0), nil
	}
	if page.fontLayoutDirty.Swap(false) && page.layout != nil {
		page.layout.MarkResource(layoutinterfaces.ResourceFont)
	}
	if page.imageLayoutDirty.Swap(false) && page.layout != nil {
		page.layout.MarkResource(layoutinterfaces.ResourceImage)
	}
	root := page.root()
	renderer := page.Options.Renderer
	if renderer == nil || renderer.Name() == "software" {
		surface := software.NewSurface(page.Width, page.Height)
		surface.Clear(NewColor(255, 255, 255))
		if root != nil {
			page.paint(root, surface)
		}
		return surface, nil
	}
	recorder := scene.New(page.Width, page.Height)
	recorder.Clear(NewColor(255, 255, 255))
	if root != nil {
		page.paint(root, recorder)
	}
	return renderer.Render(recorder.Scene())
}

func (page *Page) paint(root *model.Box, canvas interfaces.Canvas) {
	page.applyScroll(root, page.scrollX, page.scrollY)
	root.ScrollLeft = 0
	root.ScrollTop = 0
	painter := NewPainter(page.Options.Images)
	painter.Fonts = page.library
	painter.Glyphs = page.library.GlyphCache()
	painter.Subframes = page.Options.Subframes
	painter.OffsetX = -page.scrollX
	painter.OffsetY = -page.scrollY
	if handle := page.layout.GeometryHandle(); handle != nil {
		painter.Geometry = handle
		if renderer, ok := page.Options.Renderer.(interfaces.GeometryRenderer); ok {
			_ = renderer.SetGeometry(handle)
		}
	}
	if page.Document != nil {
		painter.BaseURL = page.Document.URL
	}
	painter.Paint(root, canvas)
}

func fontLibraryFor(document *dom.Document, engine *styles.Engine, options Options) *fonts.Library {
	library := fonts.Default()
	if engine == nil {
		return library
	}
	fontFaces := engine.FontFaces()
	if len(fontFaces) == 0 {
		return library
	}
	library = library.Clone()
	partition := ""
	baseURL := (*urls.URL)(nil)
	if document != nil {
		baseURL = document.URL
		if baseURL != nil {
			partition = baseURL.Site()
		}
	}
	library.AddFontFaceRules(fontFaces, fonts.AuthorOptions{Base: baseURL, Partition: partition, Loader: options.LoadFont})
	return library
}

func maxInt(first, second int) int {
	if first > second {
		return first
	}
	return second
}

func maxFloatValue(first, second float64) float64 {
	if first > second {
		return first
	}
	return second
}
