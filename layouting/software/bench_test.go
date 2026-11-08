package software

import (
	"gonetsurf/layouting/model"
	"testing"

	"gonetsurf/parsers/css/cssom"
	"gonetsurf/parsers/html"
	"gonetsurf/styles"
)

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

func benchmarkLayout(t testing.TB) (*styles.Engine, *model.Box) {
	t.Helper()
	document, err := html.ParseString(benchmarkDocument, html.ParseOptions{})
	if err != nil {
		t.Fatal(err)
	}
	engine := styles.NewEngine(document)
	engine.AddStylesheet(cssom.Parse(`
		p { margin:0; padding:0; font-size:16px; line-height:19.2px }
		.item { margin-bottom:5px }
		table { display:table; border-collapse:collapse }
		tr { display:table-row }
		td { display:table-cell; padding:2px }
	`))
	root := model.BuildDocument(document, engine)
	return engine, root
}

func BenchmarkFullLayout(b *testing.B) {
	_, root := benchmarkLayout(b)
	layout := New(800, 600)
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		layout.Layout(root)
	}
}
