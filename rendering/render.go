package rendering

import (
	"gonetsurf/dom"
	"gonetsurf/layouts"
	"gonetsurf/rendering/fonts"
	"gonetsurf/rendering/interfaces"
	"gonetsurf/rendering/software"
	"gonetsurf/styles"
	"gonetsurf/urls"
)

// Options configures a render pass.
type Options struct {
	Images     ImageProvider
	LoadFont   fonts.Loader
	Subframes  SubframeProvider
	Renderer   interfaces.Renderer
	AsyncFonts bool
}

// SubframeProvider supplies rendered documents for nested browsing contexts.
type SubframeProvider interface {
	RenderSubframe(node *dom.Node, width, height int) (interfaces.Surface, error)
}

// RenderDocument resolves styles, lays out document, and paints it into a new
// surface. The canvas is cleared to white first.
func RenderDocument(document *dom.Document, engine *styles.Engine, width, height int) (interfaces.Surface, error) {
	return RenderDocumentWithOptions(document, engine, width, height, Options{Images: DataURLImageProvider{}})
}

// RenderDocumentWithProvider is RenderDocument with an explicit image provider.
func RenderDocumentWithProvider(document *dom.Document, engine *styles.Engine, width, height int, provider ImageProvider) (interfaces.Surface, error) {
	return RenderDocumentWithOptions(document, engine, width, height, Options{Images: provider})
}

// RenderDocumentWithOptions is RenderDocument with explicit image, font, and
// renderer options.
func RenderDocumentWithOptions(document *dom.Document, engine *styles.Engine, width, height int, options Options) (interfaces.Surface, error) {
	if document == nil || engine == nil {
		return software.NewSurface(width, height), nil
	}
	if options.Images == nil {
		options.Images = DataURLImageProvider{}
	}
	return NewPage(document, engine, width, height, options).Render()
}

// RenderBox lays out an existing box tree and paints it into a new surface.
func RenderBox(root *layouts.Box, width, height int) (interfaces.Surface, error) {
	return renderBoxWithOptions(root, width, height, DataURLImageProvider{}, nil, fonts.Default())
}

func renderBoxWithOptions(root *layouts.Box, width, height int, provider ImageProvider, baseURL *urls.URL, library *fonts.Library) (interfaces.Surface, error) {
	surface := software.NewSurface(width, height)
	surface.Clear(NewColor(255, 255, 255))
	if root == nil {
		return surface, nil
	}
	if library == nil {
		library = fonts.Default()
	}
	layout := layouts.New(float64(width), float64(height))
	layout.Shaper = fonts.NewShaper(library)
	layout.Metrics = library.Metrics
	layout.Layout(root)
	painter := NewPainter(provider)
	painter.Fonts = library
	painter.Glyphs = library.GlyphCache()
	painter.BaseURL = baseURL
	painter.Paint(root, surface)
	return surface, nil
}
