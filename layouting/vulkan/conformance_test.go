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

type conformanceCheck struct {
	ID       string             `json:"id"`
	Expected map[string]float64 `json:"-"`
}

func (check *conformanceCheck) UnmarshalJSON(data []byte) error {
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	check.ID, _ = raw["id"].(string)
	delete(raw, "id")
	check.Expected = map[string]float64{}
	for key, value := range raw {
		if number, ok := value.(float64); ok {
			check.Expected[key] = number
		}
	}
	return nil
}

type conformanceCase struct {
	Description string `json:"description"`
	HTML        string `json:"html"`
	CSS         string `json:"css"`
	Viewport    struct {
		Width  float64 `json:"width"`
		Height float64 `json:"height"`
	} `json:"viewport"`
	Checks []conformanceCheck `json:"checks"`
}

func TestVulkanConformanceGeometry(t *testing.T) {
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
			referenceLayout := software.New(testCase.Viewport.Width, testCase.Viewport.Height)
			referenceLayout.Layout(referenceRoot)

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
			for _, check := range testCase.Checks {
				node := document.GetElementById(check.ID)
				if node == nil {
					t.Errorf("#%s missing", check.ID)
					continue
				}
				referenceBox := model.FindBox(referenceRoot, node)
				vulkanBox := model.FindBox(vulkanRoot, node)
				if referenceBox == nil || vulkanBox == nil {
					t.Errorf("#%s has no box", check.ID)
					continue
				}
				for property, want := range check.Expected {
					got := boxProperty(vulkanBox, property)
					if math.Abs(got-want) > 0.5 {
						t.Errorf("#%s %s = %v, want %v", check.ID, property, got, want)
					}
				}
			}
		})
	}
}

func boxProperty(box *model.Box, property string) float64 {
	switch property {
	case "x":
		return box.X
	case "y":
		return box.Y
	case "width":
		return box.Width
	case "height":
		return box.Height
	case "contentWidth":
		return box.ContentWidth
	case "contentHeight":
		return box.ContentHeight
	case "scrollWidth":
		return box.ScrollWidth
	case "scrollHeight":
		return box.ScrollHeight
	case "scrollportWidth":
		return box.ScrollportWidth()
	case "scrollportHeight":
		return box.ScrollportHeight()
	case "maxScrollLeft":
		return box.MaxScrollLeft()
	case "maxScrollTop":
		return box.MaxScrollTop()
	case "clipX":
		if box.ClipX {
			return 1
		}
		return 0
	case "clipY":
		if box.ClipY {
			return 1
		}
		return 0
	case "firstChildX":
		if len(box.Children) > 0 {
			return box.Children[0].X
		}
		return math.NaN()
	case "firstChildY":
		if len(box.Children) > 0 {
			return box.Children[0].Y
		}
		return math.NaN()
	case "firstChildWidth":
		if len(box.Children) > 0 {
			return box.Children[0].Width
		}
		return math.NaN()
	case "firstChildHeight":
		if len(box.Children) > 0 {
			return box.Children[0].Height
		}
		return math.NaN()
	}
	return math.NaN()
}
