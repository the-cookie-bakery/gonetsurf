package software

import (
	"gonetsurf/layouting/model"
	"testing"
)

func TestHitTestAndBoxRectWithScroll(t *testing.T) {
	document, body := testDocument()
	first := document.CreateElement("div")
	first.SetAttribute("id", "first")
	second := document.CreateElement("div")
	second.SetAttribute("id", "second")
	body.AppendChild(first)
	body.AppendChild(second)
	engine := testEngine(document, `
		html, body { margin:0; padding:0 }
		div { display:block; width:100px; height:100px }
	`)
	root := model.BuildDocument(document, engine)
	layout := New(200, 50)
	layout.Layout(root)
	root.ScrollTop = 80
	hit := model.HitTest(root, 10, 30)
	if hit == nil || hit.Node != second {
		t.Fatalf("hit = %#v", hit)
	}
	secondBox := model.FindBox(root, second)
	rect, ok := model.BoxRect(root, secondBox)
	if !ok {
		t.Fatal("second box has no rectangle")
	}
	if rect.X != 0 || rect.Y != 20 || rect.Width != 100 || rect.Height != 100 {
		t.Errorf("rect = %#v", rect)
	}
	if model.FindBox(root, first) == nil {
		t.Error("first box missing")
	}
}
