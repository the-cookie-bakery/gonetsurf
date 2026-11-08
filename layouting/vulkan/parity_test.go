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

type parityCase struct {
	name string
	html string
	css  string
}

func TestVulkanLayoutBlockParity(t *testing.T) {
	cases := []parityCase{
		{
			name: "auto width block",
			html: `<html><body><div id="a">hello</div></body></html>`,
			css:  `html,body{margin:0;padding:0} div{height:20px}`,
		},
		{
			name: "explicit sizes and margins",
			html: `<html><body><div id="a"></div><div id="b"></div></body></html>`,
			css:  `html,body{margin:0;padding:0} div{width:200px;height:30px;margin:10px;padding:5px;border:2px solid black}`,
		},
		{
			name: "nested blocks",
			html: `<html><body><div id="outer"><div id="inner"></div></div></body></html>`,
			css:  `html,body{margin:0;padding:0} #outer{width:300px;padding:10px} #inner{height:40px;margin:5px}`,
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
			softwareRoot := model.BuildDocument(document, engine)
			softwareLayout := software.New(400, 600)
			softwareLayout.Layout(softwareRoot)

			vulkanRoot := model.BuildDocument(document, engine)
			vulkanEngine, err := vulkan.NewBackend()
			if err != nil {
				t.Fatal(err)
			}
			flow, err := vulkanEngine.NewEngine(layoutOptions())
			if err != nil {
				t.Fatal(err)
			}
			defer flow.Close()
			if _, err := flow.Layout(vulkanRoot, interfaces.Request{}); err != nil {
				t.Fatal(err)
			}
			compareTrees(t, softwareRoot, vulkanRoot)
		})
	}
}

func layoutOptions() interfaces.Options {
	return interfaces.Options{ViewportWidth: 400, ViewportHeight: 600}
}

func compareTrees(t *testing.T, expected, actual *model.Box) {
	t.Helper()
	if expected == nil || actual == nil {
		if expected != actual {
			t.Error("tree shape differs")
		}
		return
	}
	if math.Abs(expected.X-actual.X) > 0.5 || math.Abs(expected.Y-actual.Y) > 0.5 ||
		math.Abs(expected.Width-actual.Width) > 0.5 || math.Abs(expected.Height-actual.Height) > 0.5 {
		t.Errorf("%s: geometry (%v,%v %vx%v) != (%v,%v %vx%v)",
			expected.Node.Data, expected.X, expected.Y, expected.Width, expected.Height,
			actual.X, actual.Y, actual.Width, actual.Height)
	}
	if len(expected.Children) != len(actual.Children) {
		t.Errorf("%s: child count %d != %d", expected.Node.Data, len(expected.Children), len(actual.Children))
		return
	}
	for index := range expected.Children {
		compareTrees(t, expected.Children[index], actual.Children[index])
	}
}

// TestVulkanIncrementalRelayout verifies that an unchanged tree produces no
// damage and reuses the previously uploaded node values.
func TestVulkanIncrementalRelayout(t *testing.T) {
	document, err := html.ParseString(`<html><body style="margin:0"><div id="a" style="width:120px;height:20px"></div></body></html>`, html.ParseOptions{})
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
	first, err := flow.Layout(root, interfaces.Request{})
	if err != nil {
		t.Fatal(err)
	}
	if !first.Complete {
		t.Fatal("first layout was not complete")
	}
	second, err := flow.Layout(root, interfaces.Request{})
	if err != nil {
		t.Fatal(err)
	}
	if !second.Complete {
		t.Fatal("second layout was not complete")
	}
	if len(second.Damage) != 0 {
		t.Errorf("unchanged relayout produced %d damage rectangles", len(second.Damage))
	}
}
