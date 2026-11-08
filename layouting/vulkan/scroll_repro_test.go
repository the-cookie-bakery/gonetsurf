//go:build vulkan && cgo

package vulkan_test

import (
	"image"
	"os"
	"path/filepath"
	"testing"

	"gonetsurf/layouting/model"
	layoutsoftware "gonetsurf/layouting/software"
	"gonetsurf/parsers/css/cssom"
	"gonetsurf/parsers/html"
	"gonetsurf/rendering"
	renderingsoftware "gonetsurf/rendering/software"
	renderingvulkan "gonetsurf/rendering/vulkan"
	"gonetsurf/styles"
)

func TestVulkanScrollContentRepro(t *testing.T) {
	source, err := os.ReadFile(filepath.Join("testdata", "cookie-engineer", "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	stylesheet, err := os.ReadFile(filepath.Join("testdata", "cookie-engineer", "combined.css"))
	if err != nil {
		t.Fatal(err)
	}
	document, err := html.ParseString(string(source), html.ParseOptions{})
	if err != nil {
		t.Fatal(err)
	}
	engine := styles.NewEngine(document)
	engine.AddStylesheet(cssom.Parse(string(stylesheet)))
	backend, err := renderingvulkan.NewBackend()
	if err != nil {
		t.Fatal(err)
	}
	renderer, err := backend.NewRenderer()
	if err != nil {
		t.Fatal(err)
	}
	defer renderer.Close()
	page := rendering.NewPage(document, engine, 1024, 768, rendering.Options{
		Renderer:      renderer,
		LayoutBackend: layoutsoftware.NewBackend(),
	})
	first, err := page.Render()
	if err != nil {
		t.Fatal(err)
	}
	firstDiffering := countDifferingPixels(first.Image())
	t.Logf("first frame differing pixels: %d", firstDiffering)
	scrollBox := findLargestScrollContainer(page.Box())
	if scrollBox == nil {
		t.Fatal("no scroll container found")
	}
	t.Logf("scroll target before=%v max=%v", scrollBox.ScrollTop, scrollBox.MaxScrollTop())
	page.ScrollByAt(scrollBox.X+10, scrollBox.Y+10, 0, 120)
	t.Logf("scroll target after=%v", scrollBox.ScrollTop)
	second, err := page.Render()
	if err != nil {
		t.Fatal(err)
	}
	secondDiffering := countDifferingPixels(second.Image())
	t.Logf("second frame differing pixels: %d", secondDiffering)
	if secondDiffering < 1000 {
		t.Fatalf("content after scroll is blank: %d differing pixels", secondDiffering)
	}
	if countImageDifference(first.Image(), second.Image()) < 1000 {
		t.Fatal("scrolling did not change the rendered content")
	}
}

func countDifferingPixels(picture *image.NRGBA) int {
	count := 0
	for y := 0; y < picture.Bounds().Dy(); y++ {
		for x := 0; x < picture.Bounds().Dx(); x++ {
			pixel := picture.NRGBAAt(x, y)
			if pixel.R != 255 || pixel.G != 255 || pixel.B != 255 {
				count++
			}
		}
	}
	return count
}

func findLargestScrollContainer(box *model.Box) *model.Box {
	best := (*model.Box)(nil)
	var visit func(*model.Box)
	visit = func(current *model.Box) {
		if current == nil {
			return
		}
		if current.ScrollY && current.MaxScrollTop() > 0 {
			if best == nil || current.MaxScrollTop() > best.MaxScrollTop() {
				best = current
			}
		}
		for _, child := range current.Children {
			visit(child)
		}
	}
	visit(box)
	return best
}

func countImageDifference(first, second *image.NRGBA) int {
	width := first.Bounds().Dx()
	height := first.Bounds().Dy()
	if second.Bounds().Dx() < width {
		width = second.Bounds().Dx()
	}
	if second.Bounds().Dy() < height {
		height = second.Bounds().Dy()
	}
	count := 0
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			firstOffset := y*first.Stride + x*4
			secondOffset := y*second.Stride + x*4
			for channel := 0; channel < 4; channel++ {
				delta := int(first.Pix[firstOffset+channel]) - int(second.Pix[secondOffset+channel])
				if delta < 0 {
					delta = -delta
				}
				if delta > 8 {
					count++
					break
				}
			}
		}
	}
	return count
}

var _ = renderingsoftware.NewSurface
