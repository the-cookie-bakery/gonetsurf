//go:build vulkan && cgo

package vulkan_test

import (
	"testing"

	"gonetsurf/layouting/interfaces"
	"gonetsurf/layouting/model"
	"gonetsurf/layouting/vulkan"
	"gonetsurf/parsers/css/cssom"
	"gonetsurf/parsers/html"
	"gonetsurf/rendering/geometry"
	renderinginterfaces "gonetsurf/rendering/interfaces"
	renderingvulkan "gonetsurf/rendering/vulkan"
	"gonetsurf/styles"
)

func TestVulkanGeometryHandoff(t *testing.T) {
	document, err := html.ParseString(`<html><body style="margin:0;padding:0"><div id="target" style="width:20px;height:10px"></div></body></html>`, html.ParseOptions{})
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
	flow, err := backend.NewEngine(interfaces.Options{ViewportWidth: 64, ViewportHeight: 64})
	if err != nil {
		t.Fatal(err)
	}
	defer flow.Close()
	if _, err := flow.Layout(root, interfaces.Request{}); err != nil {
		t.Fatal(err)
	}
	provider, ok := flow.(interface {
		GeometryHandle() interfaces.GeometryHandle
	})
	if !ok {
		t.Fatal("layout engine does not expose a geometry handle")
	}
	handle := provider.GeometryHandle()
	if handle == nil {
		t.Fatal("geometry handle is nil")
	}
	target := model.FindBox(root, document.GetElementById("target"))
	index, ok := handle.NodeIndex(target.Node)
	if !ok {
		t.Fatal("target node is missing from the geometry handle")
	}
	rendererBackend, err := renderingvulkan.NewBackend()
	if err != nil {
		t.Fatal(err)
	}
	renderer, err := rendererBackend.NewRenderer()
	if err != nil {
		t.Fatal(err)
	}
	defer renderer.Close()
	geometryRenderer, ok := renderer.(renderinginterfaces.GeometryRenderer)
	if !ok {
		t.Fatal("renderer does not accept geometry handles")
	}
	if err := geometryRenderer.SetGeometry(handle); err != nil {
		t.Fatal(err)
	}
	scene := &renderinginterfaces.Scene{Width: 64, Height: 64}
	scene.Commands = []renderinginterfaces.Command{
		{Kind: renderinginterfaces.CommandClear, Color: geometry.NewColor(255, 255, 255)},
		{Kind: renderinginterfaces.CommandFillRect, Rect: geometry.Rect{X: 40, Y: 40, Width: 10, Height: 10}, Color: geometry.NewColor(255, 0, 0), Clip: geometry.Rect{Width: 64, Height: 64}, UseNode: true, NodeIndex: index},
	}
	surface, err := renderer.Render(scene)
	if err != nil {
		t.Fatal(err)
	}
	pixelX := int(target.X) + 2
	pixelY := int(target.Y) + 2
	pixel := surface.Image().NRGBAAt(pixelX, pixelY)
	if pixel.R < 250 || pixel.G > 5 || pixel.B > 5 || pixel.A < 250 {
		t.Fatalf("node draw pixel = %#v, want red at node rect", pixel)
	}
}
