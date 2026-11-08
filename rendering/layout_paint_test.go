package rendering

import (
	"gonetsurf/layouting/model"
	layoutsoftware "gonetsurf/layouting/software"
	"testing"

	"gonetsurf/dom"
	"gonetsurf/parsers/css/cssom"
	"gonetsurf/parsers/html"
	"gonetsurf/styles"
)

func buildPaintTree(t *testing.T, source string, width, height int) (*model.Box, *styles.Engine, *dom.Document) {
	t.Helper()
	document, err := html.ParseString(source, html.ParseOptions{})
	if err != nil {
		t.Fatal(err)
	}
	engine := styles.NewEngine(document)
	document.Node.Descendants(func(node *dom.Node) bool {
		if node.Type == dom.ElementNode && node.Data == "style" {
			engine.AddStylesheet(cssom.Parse(node.TextContent()))
		}
		return true
	})
	root := model.BuildDocument(document, engine)
	layout := layoutsoftware.New(float64(width), float64(height))
	layout.Layout(root)
	return root, engine, document
}

func findPaintBox(t *testing.T, box *model.Box, id string) *model.Box {
	t.Helper()
	if box == nil {
		return nil
	}
	if box.Node != nil {
		if value, ok := box.Node.GetAttribute("id"); ok && value == id {
			return box
		}
	}
	for _, child := range box.Children {
		if found := findPaintBox(t, child, id); found != nil {
			return found
		}
	}
	return nil
}

func TestStickyPaintIntegration(t *testing.T) {
	source := `<!doctype html><html><head><style>
html, body { margin:0; padding:0 }
.scroller { width:100px; height:100px; overflow:auto; background-color:#ffffff }
.wrapper { height:200px }
.sticky { position:sticky; top:0; left:0; width:20px; height:20px; background-color:#ff0000 }
</style></head><body><div id="scroller" class="scroller"><div class="wrapper"><div id="sticky" class="sticky"></div></div></div></body></html>`
	root, _, _ := buildPaintTree(t, source, 200, 200)
	scroller := findPaintBox(t, root, "scroller")
	sticky := findPaintBox(t, root, "sticky")
	if scroller == nil || sticky == nil {
		t.Fatal("missing scroll/sticky boxes")
	}
	scroller.ScrollTop = 50
	surface := NewSurface(200, 200)
	surface.Clear(NewColor(255, 255, 255))
	Paint(root, surface)
	if pixel := pixelAt(surface, 5, 55); pixel.Red < 200 {
		t.Errorf("sticky paint pixel = %#v, want red at the scrolled position", pixel)
	}
}

func TestScrollbarPainting(t *testing.T) {
	source := `<!doctype html><html><head><style>
html, body { margin:0; padding:0 }
.scroller { width:100px; height:100px; overflow:scroll }
.content { height:300px; width:300px; background-color:#ffffff }
</style></head><body><div id="scroller" class="scroller"><div class="content"></div></div></body></html>`
	root, _, _ := buildPaintTree(t, source, 200, 200)
	scroller := findPaintBox(t, root, "scroller")
	if scroller == nil || !scroller.HasVerticalScrollbar || !scroller.HasHorizontalScrollbar {
		t.Fatal("scroller did not expose scrollbars")
	}
	scroller.ScrollTop = 100
	scroller.ScrollLeft = 100
	surface := NewSurface(200, 200)
	surface.Clear(NewColor(255, 255, 255))
	Paint(root, surface)
	trackX := int(scroller.ContentX + scroller.ScrollportWidth() + model.ScrollbarThickness/2)
	trackY := int(scroller.ContentY + 5)
	pixel := pixelAt(surface, trackX, trackY)
	if pixel.Red == 255 && pixel.Green == 255 && pixel.Blue == 255 {
		t.Errorf("vertical scrollbar was not painted at (%d,%d)", trackX, trackY)
	}
	horizontalX := int(scroller.ContentX + 5)
	horizontalY := int(scroller.ContentY + scroller.ScrollportHeight() + model.ScrollbarThickness/2)
	pixel = pixelAt(surface, horizontalX, horizontalY)
	if pixel.Red == 255 && pixel.Green == 255 && pixel.Blue == 255 {
		t.Errorf("horizontal scrollbar was not painted at (%d,%d)", horizontalX, horizontalY)
	}
}
