package rendering

import (
	"testing"

	"gonetsurf/dom"
	"gonetsurf/parsers/css/cssom"
	"gonetsurf/parsers/html"
	"gonetsurf/rendering/interfaces"
	"gonetsurf/styles"
)

type testSubframes struct {
	surface *Surface
}

func (provider testSubframes) RenderSubframe(node *dom.Node, width, height int) (interfaces.Surface, error) {
	return provider.surface, nil
}

func testPage(t *testing.T, source string, width, height int) *Page {
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
	return NewPage(document, engine, width, height, Options{Images: DataURLImageProvider{}})
}

func testPixelAt(surface *Surface, x, y int) Color {
	offset := y*surface.Stride + x*4
	return Color{Red: surface.Pix[offset], Green: surface.Pix[offset+1], Blue: surface.Pix[offset+2], Alpha: surface.Pix[offset+3]}
}

func TestPageScrollAndHitTest(t *testing.T) {
	page := testPage(t, `<!doctype html><html><head><style>
		html, body { margin:0; padding:0 }
		div { width:100px; height:100px }
		.first { background:#ff0000 }
		.second { background:#0000ff }
	</style></head><body><div class="first"></div><div class="second"></div></body></html>`, 100, 50)
	if maximumX, maximumY := page.MaxScroll(); maximumX != 0 || maximumY != 150 {
		t.Fatalf("max scroll = %v/%v", maximumX, maximumY)
	}
	page.ScrollTo(0, 100)
	if _, top := page.Scroll(); top != 100 {
		t.Fatalf("scroll top = %v", top)
	}
	rendered, renderErr := page.Render()
	if renderErr != nil {
		t.Fatal(renderErr)
	}
	surface := asSoftwareSurface(t, rendered)
	if pixel := testPixelAt(surface, 10, 10); pixel != (Color{Red: 0, Green: 0, Blue: 255, Alpha: 255}) {
		t.Errorf("pixel = %#v", pixel)
	}
	hit := page.HitTest(10, 10)
	if hit == nil || hit.Node == nil {
		t.Fatal("hit test returned no box")
	}
	if class, _ := hit.Node.GetAttribute("class"); class != "second" {
		t.Errorf("hit class = %q", class)
	}
}

func TestPagePaintsSubframe(t *testing.T) {
	page := testPage(t, `<!doctype html><html><head><style>
		html, body { margin:0; padding:0 }
		iframe { width:100px; height:50px; border:0 }
	</style></head><body><iframe src="about:blank"></iframe></body></html>`, 200, 100)
	frame := NewSurface(100, 50)
	frame.Clear(NewColor(0, 255, 0))
	page.Options.Subframes = testSubframes{surface: frame}
	rendered, renderErr := page.Render()
	if renderErr != nil {
		t.Fatal(renderErr)
	}
	surface := asSoftwareSurface(t, rendered)
	if pixel := testPixelAt(surface, 10, 10); pixel != (Color{Red: 0, Green: 255, Blue: 0, Alpha: 255}) {
		t.Errorf("iframe pixel = %#v", pixel)
	}
}

func TestPageResizeUpdatesMediaQueries(t *testing.T) {
	page := testPage(t, `<!doctype html><html><head><style>
		html, body { margin:0; padding:0 }
		#box { display:block; width:100px; height:10px }
		@media (max-width: 200px) { #box { width:50px } }
	</style></head><body><div id="box"></div></body></html>`, 300, 200)
	if _, renderErr := page.Render(); renderErr != nil {
		t.Fatal(renderErr)
	}
	box := page.Document.GetElementById("box")
	layoutBox := page.BoxForNode(box)
	if layoutBox == nil || layoutBox.Width != 100 {
		t.Fatalf("initial width = %v", layoutBox)
	}
	page.Resize(150, 200)
	if _, renderErr := page.Render(); renderErr != nil {
		t.Fatal(renderErr)
	}
	layoutBox = page.BoxForNode(box)
	if layoutBox == nil || layoutBox.Width != 50 {
		t.Errorf("resized width = %v, want 50", layoutBox)
	}
}

func TestPageScrollsNestedContainerUnderPointer(t *testing.T) {
	page := testPage(t, `<!doctype html><html><head><style>
		html, body { margin:0; padding:0; height:100% }
		#scroller { display:block; width:100px; height:50px; overflow:auto }
		#content { display:block; width:100px; height:500px }
	</style></head><body><div id="scroller"><div id="content"></div></div></body></html>`, 100, 100)
	if _, renderErr := page.Render(); renderErr != nil {
		t.Fatal(renderErr)
	}
	if _, maximum := page.MaxScroll(); maximum != 0 {
		t.Fatalf("root max scroll = %v", maximum)
	}
	page.ScrollByAt(10, 10, 0, 80)
	scroller := page.Document.GetElementById("scroller")
	box := page.BoxForNode(scroller)
	if box == nil || box.ScrollTop != 80 {
		t.Errorf("nested scroll = %v", box)
	}
}

func TestFixedElementsStayDuringNestedScroll(t *testing.T) {
	page := testPage(t, `<!doctype html><html><head><style>
		html, body { margin:0; padding:0; height:100% }
		#header { position:fixed; top:0; left:0; width:100px; height:10px; background:#ff0000 }
		#scroller { display:block; margin-top:10px; width:100px; height:50px; overflow:auto }
		#content { display:block; width:100px; height:300px; background:#00ff00 }
		#footer { position:fixed; bottom:0; left:0; width:100px; height:10px; background:#0000ff }
	</style></head><body>
		<div id="header"></div><div id="scroller"><div id="content"></div></div><div id="footer"></div>
	</body></html>`, 100, 100)
	if _, renderErr := page.Render(); renderErr != nil {
		t.Fatal(renderErr)
	}
	page.ScrollByAt(10, 20, 0, 80)
	scroller := page.BoxForNode(page.Document.GetElementById("scroller"))
	if scroller == nil || scroller.ScrollTop != 80 {
		t.Fatalf("nested scroll = %v", scroller)
	}
	rendered, renderErr := page.Render()
	if renderErr != nil {
		t.Fatal(renderErr)
	}
	surface := asSoftwareSurface(t, rendered)
	if pixel := testPixelAt(surface, 5, 5); pixel != (Color{Red: 255, Green: 0, Blue: 0, Alpha: 255}) {
		t.Errorf("header pixel = %#v", pixel)
	}
	if pixel := testPixelAt(surface, 5, 95); pixel != (Color{Red: 0, Green: 0, Blue: 255, Alpha: 255}) {
		t.Errorf("footer pixel = %#v", pixel)
	}
}

func TestNestedScrollPaintsContentAtSingleOffset(t *testing.T) {
	page := testPage(t, `<!doctype html><html><head><style>
		html, body { margin:0; padding:0 }
		#scroller { display:block; width:100px; height:80px; overflow:auto }
		.first { display:block; width:100px; height:100px; background:#ff0000 }
		.second { display:block; width:100px; height:100px; background:#0000ff }
	</style></head><body><div id="scroller"><div class="first"></div><div class="second"></div></div></body></html>`, 100, 100)
	if _, renderErr := page.Render(); renderErr != nil {
		t.Fatal(renderErr)
	}
	page.ScrollByAt(10, 10, 0, 100)
	rendered, renderErr := page.Render()
	if renderErr != nil {
		t.Fatal(renderErr)
	}
	surface := asSoftwareSurface(t, rendered)
	if pixel := testPixelAt(surface, 10, 10); pixel != (Color{Red: 0, Green: 0, Blue: 255, Alpha: 255}) {
		t.Errorf("scrolled pixel = %#v, want blue", pixel)
	}
}

func TestPagePaintsGeneratedContent(t *testing.T) {
	page := testPage(t, `<!doctype html><html><head><style>
		html, body { margin:0; padding:0 }
		#box { display:block }
		#box::before { content:""; display:inline-block; width:20px; height:20px; background:#ff0000 }
	</style></head><body><div id="box"></div></body></html>`, 100, 100)
	rendered, renderErr := page.Render()
	if renderErr != nil {
		t.Fatal(renderErr)
	}
	surface := asSoftwareSurface(t, rendered)
	if pixel := testPixelAt(surface, 5, 5); pixel != (Color{Red: 255, Green: 0, Blue: 0, Alpha: 255}) {
		t.Errorf("generated content pixel = %#v", pixel)
	}
}
