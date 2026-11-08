//go:build vulkan && cgo

package vulkan_test

import (
	"encoding/json"
	"math"
	"os"
	"testing"

	"gonetsurf/layouting/interfaces"
	"gonetsurf/layouting/model"
	"gonetsurf/layouting/software"
	"gonetsurf/layouting/vulkan"
	"gonetsurf/parsers/css/cssom"
	"gonetsurf/parsers/html"
	"gonetsurf/styles"
)

// TestVulkanSoftwareParityCorpus runs every conformance case through both
// engines and compares the complete laid-out tree, not only the expected
// per-case values.
func TestVulkanSoftwareParityCorpus(t *testing.T) {
	encoded, err := os.ReadFile("../testdata/cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []conformanceCase
	if err := json.Unmarshal(encoded, &cases); err != nil {
		t.Fatal(err)
	}
	for _, testCase := range cases {
		t.Run(testCase.Description, func(t *testing.T) {
			document, err := html.ParseString(testCase.HTML, html.ParseOptions{})
			if err != nil {
				t.Fatal(err)
			}
			engine := styles.NewEngine(document)
			engine.AddStylesheet(cssom.Parse(testCase.CSS))

			referenceRoot := model.BuildDocument(document, engine)
			software.New(testCase.Viewport.Width, testCase.Viewport.Height).Layout(referenceRoot)

			vulkanRoot := model.BuildDocument(document, engine)
			backend, err := vulkan.NewBackend()
			if err != nil {
				t.Fatal(err)
			}
			flow, err := backend.NewEngine(interfaces.Options{
				ViewportWidth:  testCase.Viewport.Width,
				ViewportHeight: testCase.Viewport.Height,
			})
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

func compareBoxesFull(t *testing.T, expected, actual *model.Box) {
	t.Helper()
	if expected == nil || actual == nil {
		if expected != actual {
			t.Error("tree shape differs")
		}
		return
	}
	for name, pair := range map[string][2]float64{
		"x":             {expected.X, actual.X},
		"y":             {expected.Y, actual.Y},
		"width":         {expected.Width, actual.Width},
		"height":        {expected.Height, actual.Height},
		"contentX":      {expected.ContentX, actual.ContentX},
		"contentY":      {expected.ContentY, actual.ContentY},
		"contentWidth":  {expected.ContentWidth, actual.ContentWidth},
		"contentHeight": {expected.ContentHeight, actual.ContentHeight},
		"scrollWidth":   {expected.ScrollWidth, actual.ScrollWidth},
		"scrollHeight":  {expected.ScrollHeight, actual.ScrollHeight},
	} {
		if math.Abs(pair[0]-pair[1]) > 0.5 {
			t.Errorf("%s: %s = %v, want %v", boxLabel(expected), name, pair[1], pair[0])
		}
	}
	if expected.HasVerticalScrollbar != actual.HasVerticalScrollbar {
		t.Errorf("%s: vertical scrollbar = %v, want %v", boxLabel(expected), actual.HasVerticalScrollbar, expected.HasVerticalScrollbar)
	}
	if expected.HasHorizontalScrollbar != actual.HasHorizontalScrollbar {
		t.Errorf("%s: horizontal scrollbar = %v, want %v", boxLabel(expected), actual.HasHorizontalScrollbar, expected.HasHorizontalScrollbar)
	}
	if expected.ClipX != actual.ClipX || expected.ClipY != actual.ClipY {
		t.Errorf("%s: clip = (%v,%v), want (%v,%v)", boxLabel(expected), actual.ClipX, actual.ClipY, expected.ClipX, expected.ClipY)
	}
	if len(expected.Children) != len(actual.Children) {
		t.Errorf("%s: child count %d != %d", boxLabel(expected), len(actual.Children), len(expected.Children))
		return
	}
	for index := range expected.Children {
		compareBoxesFull(t, expected.Children[index], actual.Children[index])
	}
}

func boxLabel(box *model.Box) string {
	if box == nil {
		return "<nil>"
	}
	if box.Node != nil {
		return box.Node.Data
	}
	return box.Text
}
