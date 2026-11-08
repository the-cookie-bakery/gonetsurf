package layouting_test

import (
	"math"
	"testing"

	"gonetsurf/dom"
	"gonetsurf/layouting"
	"gonetsurf/layouting/model"
	"gonetsurf/parsers/css/cssom"
	"gonetsurf/parsers/html"
	"gonetsurf/styles"
)

func testDocument() (*dom.Document, *dom.Node) {
	document := dom.NewDocument()
	html := document.CreateElement("html")
	body := document.CreateElement("body")
	html.AppendChild(body)
	document.AppendChild(html)
	return document, body
}

func testEngine(document *dom.Document, css string) *styles.Engine {
	engine := styles.NewEngine(document)
	engine.AddStylesheet(cssom.Parse(css))
	return engine
}

func findBox(root *model.Box, node *dom.Node) *model.Box {
	if root == nil {
		return nil
	}
	if root.Node == node {
		return root
	}
	for _, child := range root.Children {
		if found := findBox(child, node); found != nil {
			return found
		}
	}
	return nil
}

func TestDocumentLayoutCache(t *testing.T) {
	document, body := testDocument()
	paragraph := document.CreateElement("p")
	paragraph.SetAttribute("class", "item")
	paragraph.AppendChild(document.CreateTextNode("hello"))
	body.AppendChild(paragraph)
	engine := testEngine(document, `
		html, body { margin:0; padding:0 }
		p { display:block; width:100px; height:10px; margin:0; padding:0; border:0 }
		.wide { width:200px }
	`)
	cache := layouting.NewDocumentLayout(document, engine, 400, 600)
	first := cache.Reflow()
	firstParagraph := findBox(first, paragraph)
	if firstParagraph.Width != 100 {
		t.Errorf("first width = %v, want 100", firstParagraph.Width)
	}
	if cache.Reflow() != first {
		t.Error("unchanged document should reuse the cached box tree")
	}

	paragraph.AddClass("wide")
	second := cache.Reflow()
	if second != first {
		t.Error("style-only mutation should reuse the box tree")
	}
	if got := findBox(second, paragraph).Width; got != 200 {
		t.Errorf("restyled width = %v, want 200", got)
	}

	extra := document.CreateElement("p")
	extra.SetAttribute("class", "item")
	body.AppendChild(extra)
	third := cache.Reflow()
	if third == second {
		t.Error("structural mutation should rebuild the box tree")
	}

	cache.ViewportWidth = 800
	cache.Invalidate()
	fourth := cache.Reflow()
	if fourth == third {
		t.Error("invalidated viewport should rebuild the box tree")
	}
}

func TestIncrementalReflowMatchesFullLayout(t *testing.T) {
	document, body := testDocument()
	first := document.CreateElement("p")
	first.SetAttribute("class", "item first")
	first.SetAttribute("id", "first")
	first.AppendChild(document.CreateTextNode("hello"))
	second := document.CreateElement("p")
	second.SetAttribute("class", "item second")
	second.SetAttribute("id", "second")
	second.AppendChild(document.CreateTextNode("world"))
	body.AppendChild(first)
	body.AppendChild(second)
	engine := testEngine(document, `
		html, body { margin:0; padding:0 }
		p { display:block; width:100px; height:10px; margin:0; padding:0; border:0 }
		.wide { width:200px; height:30px }
	`)
	cache := layouting.NewDocumentLayout(document, engine, 400, 600)
	incremental := cache.Reflow()
	secondBox := findBox(incremental, second)
	first.AddClass("wide")
	incremental = cache.Reflow()
	if findBox(incremental, second) != secondBox {
		t.Error("incremental reflow should reuse clean sibling boxes")
	}
	fullCache := layouting.NewDocumentLayout(document, engine, 400, 600)
	full := fullCache.Reflow()
	compareBoxGeometry(t, full, incremental)
}

func compareBoxGeometry(t *testing.T, full, incremental *model.Box) {
	t.Helper()
	if full == nil || incremental == nil {
		if full != incremental {
			t.Error("geometry trees differ in shape")
		}
		return
	}
	if math.Abs(full.X-incremental.X) > 1e-9 || math.Abs(full.Y-incremental.Y) > 1e-9 ||
		math.Abs(full.Width-incremental.Width) > 1e-9 || math.Abs(full.Height-incremental.Height) > 1e-9 ||
		math.Abs(full.ContentWidth-incremental.ContentWidth) > 1e-9 || math.Abs(full.ContentHeight-incremental.ContentHeight) > 1e-9 {
		t.Errorf("geometry differs: full (%v,%v %vx%v) incremental (%v,%v %vx%v)",
			full.X, full.Y, full.Width, full.Height, incremental.X, incremental.Y, incremental.Width, incremental.Height)
	}
	if len(full.Children) != len(incremental.Children) {
		if len(incremental.Children) < len(full.Children) {
			t.Error("incremental tree has fewer children")
		} else {
			t.Error("incremental tree has more children")
		}
		return
	}
	for index := range full.Children {
		compareBoxGeometry(t, full.Children[index], incremental.Children[index])
	}
}

func BenchmarkIncrementalReflow(b *testing.B) {
	document, err := parseBenchmarkDocument()
	if err != nil {
		b.Fatal(err)
	}
	engine := styles.NewEngine(document)
	engine.AddStylesheet(cssom.Parse(`
		p { margin:0; padding:0; font-size:16px; line-height:19.2px }
		.item { margin-bottom:5px }
		.wide { width:600px }
	`))
	cache := layouting.NewDocumentLayout(document, engine, 800, 600)
	cache.Reflow()
	first := document.GetElementById("first")
	second := document.GetElementById("second")
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		if iteration%2 == 0 {
			first.AddClass("wide")
			second.RemoveClass("wide")
		} else {
			second.AddClass("wide")
			first.RemoveClass("wide")
		}
		cache.Reflow()
	}
}

func parseBenchmarkDocument() (*dom.Document, error) {
	return html.ParseString(benchmarkDocument, html.ParseOptions{})
}

const benchmarkDocument = `<!doctype html>
<html><body style="margin:0">
<div id="outer" style="width:800px">
<p id="first" class="item">hello world hello world hello world</p>
<p id="second" class="item">flexible content for layout measurement</p>
<div id="row" style="display:flex;width:800px">
<div style="flex-grow:1;height:20px"></div>
<div style="flex-grow:2;height:20px"></div>
</div>
<table id="table"><tr><td>a</td><td>bb</td></tr><tr><td>ccc</td><td>d</td></tr></table>
<div id="grid" style="display:grid;width:200px;grid-template-columns:1fr 1fr;gap:10px">
<div style="height:10px"></div><div style="height:20px"></div>
</div>
</div>
</body></html>`
