//go:build vulkan && cgo

package vulkan_test

import (
	"testing"

	"gonetsurf/layouting/interfaces"
	"gonetsurf/layouting/model"
	"gonetsurf/layouting/vulkan"
	"gonetsurf/parsers/css/cssom"
	"gonetsurf/parsers/html"
	"gonetsurf/styles"
)

// TestVulkanNoSoftwareFallback verifies that calc()-based pages are laid out by
// the GPU pipeline even when software fallback is disabled.
func TestVulkanNoSoftwareFallback(t *testing.T) {
	t.Setenv("GONETSURF_VULKAN_NO_FALLBACK", "1")
	source := `<html><body style="margin:0"><div id="outer" style="width:400px"><div id="target" style="width:calc(100% - 40px);height:calc(50% + 5px)"></div></div></body></html>`
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
	flow, err := backend.NewEngine(interfaces.Options{ViewportWidth: 400, ViewportHeight: 600})
	if err != nil {
		t.Fatal(err)
	}
	defer flow.Close()
	result, err := flow.Layout(root, interfaces.Request{})
	if err != nil {
		t.Fatalf("calc layout must not use the software fallback: %v", err)
	}
	if !result.Complete {
		t.Fatal("layout was not complete")
	}
	target := model.FindBox(root, document.GetElementById("target"))
	if target == nil {
		t.Fatal("target box missing")
	}
	if target.Width < 359.5 || target.Width > 360.5 {
		t.Errorf("target width = %v, want 360", target.Width)
	}
}
