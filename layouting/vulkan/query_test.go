//go:build vulkan && cgo

package vulkan_test

import (
	"math"
	"testing"

	"gonetsurf/layouting/interfaces"
	"gonetsurf/layouting/model"
	"gonetsurf/layouting/software"
	"gonetsurf/layouting/vulkan"
	"gonetsurf/parsers/css/cssom"
	"gonetsurf/parsers/html"
	"gonetsurf/styles"
)

func TestVulkanGeometryQueries(t *testing.T) {
	source := `<html><body style="margin:0"><div id="target" style="width:200px;height:100px"><div id="inner" style="width:50px;height:20px"></div></div></body></html>`
	document, err := html.ParseString(source, html.ParseOptions{})
	if err != nil {
		t.Fatal(err)
	}
	engine := styles.NewEngine(document)
	engine.AddStylesheet(cssom.Parse(`html, body { margin:0; padding:0 }`))
	referenceRoot := model.BuildDocument(document, engine)
	reference := software.New(400, 600)
	reference.Layout(referenceRoot)

	vulkanRoot := model.BuildDocument(document, engine)
	backend, err := vulkan.NewBackend()
	if err != nil {
		t.Fatal(err)
	}
	flow, err := backend.NewEngine(interfaces.Options{ViewportWidth: 400, ViewportHeight: 600})
	if err != nil {
		t.Fatal(err)
	}
	defer flow.Close()
	if _, err := flow.Layout(vulkanRoot, interfaces.Request{}); err != nil {
		t.Fatal(err)
	}
	query, ok := flow.(interfaces.GeometryQuery)
	if !ok {
		t.Fatal("engine does not implement GeometryQuery")
	}
	box, _, found, err := query.HitTest(10, 10, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	expectedHit := model.HitTest(referenceRoot, 10, 10)
	if !found || box == nil || expectedHit == nil || box.Node != expectedHit.Node {
		t.Fatalf("hit-test mismatch: got %v want %v", box, expectedHit)
	}
	target := model.FindBox(vulkanRoot, document.GetElementById("target"))
	rect, ok, err := query.BoxRect(target, 0, 0)
	if err != nil || !ok {
		t.Fatalf("box rect failed: %v", err)
	}
	expectedRect, _ := model.BoxRect(referenceRoot, model.FindBox(referenceRoot, document.GetElementById("target")))
	if math.Abs(rect.X-expectedRect.X) > 0.5 || math.Abs(rect.Y-expectedRect.Y) > 0.5 ||
		math.Abs(rect.Width-expectedRect.Width) > 0.5 || math.Abs(rect.Height-expectedRect.Height) > 0.5 {
		t.Errorf("box rect = %+v, want %+v", rect, expectedRect)
	}
}

func TestVulkanNestedScrollQueries(t *testing.T) {
	source := `<html><body style="margin:0;padding:0"><div id="outer" style="width:200px;height:150px;overflow:auto"><div id="inner" style="width:300px;height:400px;overflow:auto"><div id="leaf" style="width:80px;height:20px"></div></div></div></body></html>`
	document, err := html.ParseString(source, html.ParseOptions{})
	if err != nil {
		t.Fatal(err)
	}
	engine := styles.NewEngine(document)
	engine.AddStylesheet(cssom.Parse(`html, body { margin:0; padding:0 }`))
	referenceRoot := model.BuildDocument(document, engine)
	software.New(400, 600).Layout(referenceRoot)

	vulkanRoot := model.BuildDocument(document, engine)
	backend, err := vulkan.NewBackend()
	if err != nil {
		t.Fatal(err)
	}
	flow, err := backend.NewEngine(interfaces.Options{ViewportWidth: 400, ViewportHeight: 600})
	if err != nil {
		t.Fatal(err)
	}
	defer flow.Close()
	if _, err := flow.Layout(vulkanRoot, interfaces.Request{}); err != nil {
		t.Fatal(err)
	}
	query, ok := flow.(interfaces.GeometryQuery)
	if !ok {
		t.Fatal("engine does not implement GeometryQuery")
	}
	referenceInner := model.FindBox(referenceRoot, document.GetElementById("inner"))
	vulkanInner := model.FindBox(vulkanRoot, document.GetElementById("inner"))
	if referenceInner == nil || vulkanInner == nil {
		t.Fatal("inner box missing")
	}
	referenceInner.ScrollLeft = 40
	referenceInner.ScrollTop = 60
	vulkanInner.ScrollLeft = 40
	vulkanInner.ScrollTop = 60
	if _, err := flow.Layout(vulkanRoot, interfaces.Request{}); err != nil {
		t.Fatal(err)
	}
	points := [][2]float64{{10, 10}, {100, 100}, {150, 120}, {250, 200}, {190, 20}}
	for _, point := range points {
		want := model.HitTest(referenceRoot, point[0], point[1])
		got, _, found, err := query.HitTest(point[0], point[1], 0, 0)
		if err != nil {
			t.Fatal(err)
		}
		if want == nil {
			if found && got != nil {
				t.Errorf("hit-test at %v = %v, want no hit", point, got)
			}
			continue
		}
		if !found || got == nil || got.Node != want.Node {
			t.Errorf("hit-test at %v = %v, want %v", point, got, want)
		}
	}
	referenceLeaf := model.FindBox(referenceRoot, document.GetElementById("leaf"))
	vulkanLeaf := model.FindBox(vulkanRoot, document.GetElementById("leaf"))
	wantRect, _ := model.BoxRect(referenceRoot, referenceLeaf)
	gotRect, ok, err := query.BoxRect(vulkanLeaf, 0, 0)
	if err != nil || !ok {
		t.Fatalf("leaf box rect failed: %v", err)
	}
	if math.Abs(gotRect.X-wantRect.X) > 0.5 || math.Abs(gotRect.Y-wantRect.Y) > 0.5 ||
		math.Abs(gotRect.Width-wantRect.Width) > 0.5 || math.Abs(gotRect.Height-wantRect.Height) > 0.5 {
		t.Errorf("leaf rect = %+v, want %+v", gotRect, wantRect)
	}
	wantLeft, wantTop := referenceInner.MaxScrollLeft(), referenceInner.MaxScrollTop()
	gotLeft, gotTop, ok, err := query.MaxScroll(vulkanInner)
	if err != nil || !ok {
		t.Fatalf("max scroll failed: %v", err)
	}
	if math.Abs(gotLeft-wantLeft) > 0.5 || math.Abs(gotTop-wantTop) > 0.5 {
		t.Errorf("max scroll = %v,%v, want %v,%v", gotLeft, gotTop, wantLeft, wantTop)
	}
}
