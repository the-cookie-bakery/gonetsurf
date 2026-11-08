package software

import (
	"encoding/json"
	"gonetsurf/layouting/model"
	"math"
	"os"
	"testing"

	"gonetsurf/parsers/css/cssom"
	"gonetsurf/parsers/html"
	"gonetsurf/styles"
)

type layoutCheck struct {
	ID       string             `json:"id"`
	Expected map[string]float64 `json:"-"`
}

func (check *layoutCheck) UnmarshalJSON(data []byte) error {
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	id, _ := raw["id"].(string)
	check.ID = id
	delete(raw, "id")
	check.Expected = map[string]float64{}
	for key, value := range raw {
		if number, ok := value.(float64); ok {
			check.Expected[key] = number
		}
	}
	return nil
}

type layoutCase struct {
	Description string `json:"description"`
	HTML        string `json:"html"`
	CSS         string `json:"css"`
	Viewport    struct {
		Width  float64 `json:"width"`
		Height float64 `json:"height"`
	} `json:"viewport"`
	Checks []layoutCheck `json:"checks"`
}

func TestLayoutConformance(t *testing.T) {
	encoded, err := os.ReadFile("../testdata/cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []layoutCase
	if err := json.Unmarshal(encoded, &cases); err != nil {
		t.Fatal(err)
	}
	for _, testCase := range cases {
		document, err := html.ParseString(testCase.HTML, html.ParseOptions{})
		if err != nil {
			t.Errorf("%s: parse HTML: %v", testCase.Description, err)
			continue
		}
		engine := styles.NewEngine(document)
		engine.AddStylesheet(cssom.Parse(testCase.CSS))
		root := model.BuildDocument(document, engine)
		layout := New(testCase.Viewport.Width, testCase.Viewport.Height)
		layout.Layout(root)
		for _, check := range testCase.Checks {
			node := document.GetElementById(check.ID)
			if node == nil {
				t.Errorf("%s: #%s missing", testCase.Description, check.ID)
				continue
			}
			box := findBox(root, node)
			if box == nil {
				t.Errorf("%s: #%s has no box", testCase.Description, check.ID)
				continue
			}
			for property, want := range check.Expected {
				got := boxValue(box, property)
				if math.Abs(got-want) > 1e-6 {
					t.Errorf("%s: #%s %s = %v, want %v", testCase.Description, check.ID, property, got, want)
				}
			}
		}
	}
}

func boxValue(box *model.Box, property string) float64 {
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
	default:
		return math.NaN()
	}
}
