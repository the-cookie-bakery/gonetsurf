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

// TestVulkanCalcParity checks that calc() width, height, min/max, flex-basis,
// and positioned offsets resolve through the GPU path exactly like software.
func TestVulkanCalcParity(t *testing.T) {
	cases := []struct {
		name string
		html string
		css  string
	}{
		{
			name: "width and height",
			html: `<html><body><div id="a"><div id="b"></div></div></body></html>`,
			css:  `html,body{margin:0;padding:0} #a{width:400px;height:300px} #b{width:calc(100% - 40px);height:calc(50% + 10px)}`,
		},
		{
			name: "min and max",
			html: `<html><body><div id="a"></div></body></html>`,
			css:  `html,body{margin:0;padding:0} #a{width:calc(100% - 10px);min-width:calc(50% + 5px);max-width:calc(80% - 4px);height:calc(25% + 6px);min-height:calc(10% + 1px);max-height:calc(20% + 2px)}`,
		},
		{
			name: "flex basis",
			html: `<html><body><div id="row"><div id="a"></div><div id="b"></div></div></body></html>`,
			css:  `html,body{margin:0;padding:0} #row{display:flex;width:400px} #a{flex:0 1 calc(60% - 20px);height:10px} #b{flex:0 1 calc(40% + 20px);height:10px}`,
		},
		{
			name: "positioned offsets",
			html: `<html><body><div id="outer"><div id="a"></div></div></body></html>`,
			css:  `html,body{margin:0;padding:0} #outer{position:relative;width:400px;height:300px} #a{position:absolute;left:calc(25% + 10px);top:calc(50% - 5px);width:calc(50% - 20px);height:calc(25% + 2px)}`,
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			document, err := html.ParseString(testCase.html, html.ParseOptions{})
			if err != nil {
				t.Fatal(err)
			}
			engine := styles.NewEngine(document)
			engine.AddStylesheet(cssom.Parse(testCase.css))

			referenceRoot := model.BuildDocument(document, engine)
			software.New(800, 600).Layout(referenceRoot)

			vulkanRoot := model.BuildDocument(document, engine)
			backend, err := vulkan.NewBackend()
			if err != nil {
				t.Fatal(err)
			}
			flow, err := backend.NewEngine(interfaces.Options{ViewportWidth: 800, ViewportHeight: 600})
			if err != nil {
				t.Fatal(err)
			}
			defer flow.Close()
			if _, err := flow.Layout(vulkanRoot, interfaces.Request{}); err != nil {
				t.Fatal(err)
			}
			compareBoxesFull(t, referenceRoot, vulkanRoot)
		})
	}
}
