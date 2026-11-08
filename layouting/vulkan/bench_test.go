//go:build vulkan && cgo

package vulkan_test

import (
	"testing"

	"gonetsurf/layouting/interfaces"
	"gonetsurf/layouting/model"
	"gonetsurf/layouting/software"
	"gonetsurf/layouting/vulkan"
	"gonetsurf/parsers/css/cssom"
	"gonetsurf/parsers/html"
	"gonetsurf/styles"
)

const benchmarkHTML = `<html><body><div style="width:800px">
<p>hello world hello world hello world</p>
<div style="display:flex;width:800px"><div style="flex-grow:1;height:20px"></div><div style="flex-grow:2;height:20px"></div></div>
<div style="display:grid;width:200px;grid-template-columns:1fr 1fr;gap:10px"><div style="height:10px"></div><div style="height:20px"></div></div>
</div></body></html>`

func benchmarkTree(b *testing.B) *model.Box {
	b.Helper()
	document, err := html.ParseString(benchmarkHTML, html.ParseOptions{})
	if err != nil {
		b.Fatal(err)
	}
	engine := styles.NewEngine(document)
	engine.AddStylesheet(cssom.Parse(`html, body { margin:0; padding:0 } p { margin:0; font-size:16px; line-height:19.2px }`))
	return model.BuildDocument(document, engine)
}

func BenchmarkSoftwareLayout(b *testing.B) {
	root := benchmarkTree(b)
	layout := software.New(800, 600)
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		layout.Layout(root)
	}
}

func BenchmarkVulkanLayout(b *testing.B) {
	root := benchmarkTree(b)
	backend, err := vulkan.NewBackend()
	if err != nil {
		b.Fatal(err)
	}
	flow, err := backend.NewEngine(interfaces.Options{ViewportWidth: 800, ViewportHeight: 600})
	if err != nil {
		b.Fatal(err)
	}
	defer flow.Close()
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		if _, err := flow.Layout(root, interfaces.Request{}); err != nil {
			b.Fatal(err)
		}
	}
}
