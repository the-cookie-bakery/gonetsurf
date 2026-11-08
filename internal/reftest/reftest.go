// Package reftest implements reftest-style pixel comparison: two HTML
// documents are rendered with the same viewport and compared with a tolerance.
package reftest

import (
	"fmt"

	"gonetsurf/adapters"
	"gonetsurf/internal/pixeltest"
	"gonetsurf/parsers/html"
	"gonetsurf/rendering"
	"gonetsurf/rendering/fonts"
	"gonetsurf/rendering/interfaces"
	"gonetsurf/urls"
)

// Options configures a reftest render.
type Options struct {
	Width          int
	Height         int
	Images         rendering.ImageProvider
	LoadFont       fonts.Loader
	LoadStylesheet func(*urls.URL) ([]byte, error)
	Tolerance      uint8
	MaxDifferent   int
	Renderer       interfaces.Renderer
}

func (options Options) normalized() Options {
	if options.Width <= 0 {
		options.Width = 800
	}
	if options.Height <= 0 {
		options.Height = 600
	}
	if options.Images == nil {
		options.Images = rendering.DataURLImageProvider{}
	}
	return options
}

// Render parses and renders source into a surface.
func Render(source []byte, base *urls.URL, options Options) (interfaces.Surface, error) {
	options = options.normalized()
	document, err := html.Parse(source, html.ParseOptions{URL: base})
	if err != nil {
		return nil, err
	}
	loader := adapters.StylesheetLoader{Fetch: options.LoadStylesheet}
	engine := loader.Engine(document, base)
	surface, err := rendering.RenderDocumentWithOptions(document, engine, options.Width, options.Height, rendering.Options{
		Images:   options.Images,
		LoadFont: options.LoadFont,
		Renderer: options.Renderer,
	})
	if err != nil {
		return nil, err
	}
	return surface, nil
}

// Compare renders the test and reference documents and compares them.
func Compare(testSource, referenceSource []byte, base *urls.URL, options Options) (pixeltest.Result, error) {
	options = options.normalized()
	testSurface, err := Render(testSource, base, options)
	if err != nil {
		return pixeltest.Result{}, fmt.Errorf("reftest: render test: %w", err)
	}
	referenceSurface, err := Render(referenceSource, base, options)
	if err != nil {
		return pixeltest.Result{}, fmt.Errorf("reftest: render reference: %w", err)
	}
	return pixeltest.Compare(referenceSurface.Image(), testSurface.Image(), pixeltest.Options{
		ChannelTolerance: options.Tolerance,
		MaxDifferent:     options.MaxDifferent,
	})
}
