//go:build vulkan && cgo

package vulkan_test

import (
	"testing"

	"gonetsurf/dom"
	"gonetsurf/layouting/interfaces"
	"gonetsurf/layouting/model"
	"gonetsurf/layouting/software"
	"gonetsurf/layouting/vulkan"
	"gonetsurf/parsers/css/cssom"
	"gonetsurf/parsers/html"
	"gonetsurf/styles"
)

func TestVulkanActiveNodeCompaction(t *testing.T) {
	source := `<html><body style="margin:0;padding:0"><div id="card" style="width:200px;height:200px"><div id="a" style="height:10px"></div><div id="b" style="height:10px"></div></div><div id="other" style="height:10px"></div></body></html>`
	document, err := html.ParseString(source, html.ParseOptions{})
	if err != nil {
		t.Fatal(err)
	}
	engine := styles.NewEngine(document)
	engine.AddStylesheet(cssom.Parse(`html, body { margin:0; padding:0 }`))
	root := model.BuildDocument(document, engine)
	backend, err := vulkan.NewBackend()
	if err != nil {
		t.Fatal(err)
	}
	flow, err := backend.NewEngine(interfaces.Options{ViewportWidth: 400, ViewportHeight: 400})
	if err != nil {
		t.Fatal(err)
	}
	defer flow.Close()
	if _, err := flow.Layout(root, interfaces.Request{}); err != nil {
		t.Fatal(err)
	}
	first := model.FindBox(root, document.GetElementById("a"))
	first.Margin.Top = model.Length{Value: 7}
	result, err := flow.Layout(root, interfaces.Request{Dirty: interfaces.DirtySet{Nodes: []*dom.Node{document.GetElementById("a")}}})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Complete {
		t.Fatal("incremental layout did not complete")
	}
	provider, ok := flow.(interface{ ActiveNodeCount() int })
	if !ok {
		t.Fatal("engine does not report active node count")
	}
	if provider.ActiveNodeCount() >= 6 {
		t.Errorf("active node count = %d, want a compacted subset", provider.ActiveNodeCount())
	}

	referenceDocument, err := html.ParseString(source, html.ParseOptions{})
	if err != nil {
		t.Fatal(err)
	}
	referenceEngine := styles.NewEngine(referenceDocument)
	referenceEngine.AddStylesheet(cssom.Parse(`html, body { margin:0; padding:0 }`))
	referenceRoot := model.BuildDocument(referenceDocument, referenceEngine)
	model.FindBox(referenceRoot, referenceDocument.GetElementById("a")).Margin.Top = model.Length{Value: 7}
	software.New(400, 400).Layout(referenceRoot)
	compareBoxesFull(t, referenceRoot, root)
}
