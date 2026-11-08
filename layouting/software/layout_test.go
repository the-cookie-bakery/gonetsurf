package software

import (
	"gonetsurf/layouting/model"
	"math"
	"testing"

	"gonetsurf/dom"
	"gonetsurf/parsers/css/cssom"
	"gonetsurf/styles"

	"golang.org/x/text/unicode/bidi"
)

func testDocument() (*dom.Document, *dom.Node) {
	document := dom.NewDocument()
	html := document.CreateElement("html")
	body := document.CreateElement("body")
	html.AppendChild(body)
	document.AppendChild(html)
	return document, body
}

func testEngine(document *dom.Document, css string) *styles.Engine {
	engine := styles.NewEngine(document)
	engine.AddStylesheet(cssom.Parse(css))
	return engine
}

func findBox(root *model.Box, node *dom.Node) *model.Box {
	if root == nil {
		return nil
	}
	if root.Node == node {
		return root
	}
	for _, child := range root.Children {
		if found := findBox(child, node); found != nil {
			return found
		}
	}
	return nil
}

func TestBoxTreeConstruction(t *testing.T) {
	document, body := testDocument()
	container := document.CreateElement("div")
	container.SetAttribute("id", "container")
	container.AppendChild(document.CreateTextNode("hello"))
	span := document.CreateElement("span")
	span.AppendChild(document.CreateTextNode("world"))
	container.AppendChild(span)
	paragraph := document.CreateElement("p")
	paragraph.AppendChild(document.CreateTextNode("child"))
	container.AppendChild(paragraph)
	hidden := document.CreateElement("div")
	hidden.SetAttribute("class", "hidden")
	hidden.AppendChild(document.CreateTextNode("hidden"))
	container.AppendChild(hidden)
	contents := document.CreateElement("div")
	contents.SetAttribute("class", "contents")
	promoted := document.CreateElement("p")
	promoted.AppendChild(document.CreateTextNode("promoted"))
	contents.AppendChild(promoted)
	container.AppendChild(contents)
	body.AppendChild(container)

	engine := testEngine(document, `
		html, body { margin:0; padding:0 }
		div { display:block }
		span { display:inline }
		.hidden { display:none }
		.contents { display:contents }
	`)
	root := model.BuildDocument(document, engine)
	containerBox := findBox(root, container)
	if containerBox == nil {
		t.Fatal("container box missing")
	}
	if len(containerBox.Children) != 3 {
		t.Fatalf("container children = %d, want 3", len(containerBox.Children))
	}
	anonymous := containerBox.Children[0]
	if anonymous.Kind != model.AnonymousBlockBox || len(anonymous.Children) != 2 {
		t.Errorf("anonymous box = %#v", anonymous)
	}
	if anonymous.Children[0].Kind != model.TextBox || anonymous.Children[0].Text != "hello" {
		t.Errorf("text box = %#v", anonymous.Children[0])
	}
	if anonymous.Children[1].Kind != model.InlineBox || anonymous.Children[1].Node != span {
		t.Errorf("inline box = %#v", anonymous.Children[1])
	}
	if containerBox.Children[1].Node != paragraph || containerBox.Children[1].Kind != model.BlockBox {
		t.Errorf("paragraph box = %#v", containerBox.Children[1])
	}
	if containerBox.Children[2].Node != promoted {
		t.Errorf("promoted contents box = %#v", containerBox.Children[2])
	}
	if findBox(root, hidden) != nil {
		t.Error("display:none box must not be generated")
	}
}

func TestBoxModel(t *testing.T) {
	document, body := testDocument()
	boxElement := document.CreateElement("div")
	boxElement.SetAttribute("class", "edges box")
	autoElement := document.CreateElement("div")
	autoElement.SetAttribute("class", "edges auto")
	borderElement := document.CreateElement("div")
	borderElement.SetAttribute("class", "edges border")
	body.AppendChild(boxElement)
	body.AppendChild(autoElement)
	body.AppendChild(borderElement)

	engine := testEngine(document, `
		html, body { margin:0; padding:0 }
		.edges { display:block; margin:5px; padding:10px; border:2px solid }
		.box { width:100px; height:50px }
		.border { width:100px; box-sizing:border-box }
	`)
	root := model.BuildDocument(document, engine)
	layout := New(400, 600)
	layout.Layout(root)

	box := findBox(root, boxElement)
	if box.ContentWidth != 100 || box.Width != 124 {
		t.Errorf("content-box width = %v / %v", box.ContentWidth, box.Width)
	}
	if box.X != 5 || box.ContentX != 17 {
		t.Errorf("content-box x = %v / %v", box.X, box.ContentX)
	}
	if box.ContentHeight != 50 || box.Height != 74 {
		t.Errorf("content-box height = %v / %v", box.ContentHeight, box.Height)
	}

	auto := findBox(root, autoElement)
	if auto.ContentWidth != 366 || auto.Width != 390 {
		t.Errorf("auto width = %v / %v", auto.ContentWidth, auto.Width)
	}
	if auto.ContentX != 17 {
		t.Errorf("auto content x = %v", auto.ContentX)
	}

	border := findBox(root, borderElement)
	if border.ContentWidth != 76 || border.Width != 100 {
		t.Errorf("border-box width = %v / %v", border.ContentWidth, border.Width)
	}
}

func TestBlockLayoutAndMarginCollapse(t *testing.T) {
	document, body := testDocument()
	first := document.CreateElement("div")
	first.SetAttribute("class", "a")
	second := document.CreateElement("div")
	second.SetAttribute("class", "b")
	body.AppendChild(first)
	body.AppendChild(second)

	engine := testEngine(document, `
		html, body { margin:0; padding:0 }
		.a { display:block; margin:0; margin-bottom:10px; height:50px }
		.b { display:block; margin:0; margin-top:20px; height:30px }
	`)
	root := model.BuildDocument(document, engine)
	layout := New(400, 600)
	layout.Layout(root)

	firstBox := findBox(root, first)
	secondBox := findBox(root, second)
	if firstBox.Y != 0 || firstBox.Height != 50 {
		t.Errorf("first = y:%v h:%v", firstBox.Y, firstBox.Height)
	}
	if secondBox.Y != 70 {
		t.Errorf("second y = %v, want 70", secondBox.Y)
	}
	bodyBox := findBox(root, body)
	if bodyBox.ContentHeight != 100 {
		t.Errorf("body content height = %v, want 100", bodyBox.ContentHeight)
	}
}

func TestInlineLineLayout(t *testing.T) {
	document, body := testDocument()
	paragraph := document.CreateElement("p")
	paragraph.AppendChild(document.CreateTextNode("hello world hello world"))
	body.AppendChild(paragraph)

	engine := testEngine(document, `
		html, body { margin:0; padding:0 }
		p { display:block; width:100px; font-size:16px; line-height:19.2px; margin:0; padding:0; border:0 }
	`)
	root := model.BuildDocument(document, engine)
	layout := New(400, 600)
	layout.Layout(root)

	p := findBox(root, paragraph)
	if math.Abs(p.ContentHeight-38.4) > 1e-9 {
		t.Errorf("content height = %v, want 38.4", p.ContentHeight)
	}
	if len(p.Children) != 4 {
		t.Fatalf("word boxes = %d, want 4", len(p.Children))
	}
	for index, word := range p.Children {
		wantY := 0.0
		if index >= 2 {
			wantY = 19.2
		}
		if math.Abs(word.Y-wantY) > 1e-9 {
			t.Errorf("word %d y = %v, want %v", index, word.Y, wantY)
		}
	}
	if p.Children[0].Width != 40 || p.Children[1].Width != 40 {
		t.Errorf("word widths = %v, %v", p.Children[0].Width, p.Children[1].Width)
	}
}

func TestInlineAlignmentAndAtomicBoxes(t *testing.T) {
	document, body := testDocument()
	centered := document.CreateElement("p")
	centered.SetAttribute("class", "center")
	centered.AppendChild(document.CreateTextNode("hello"))
	right := document.CreateElement("p")
	right.SetAttribute("class", "right")
	right.AppendChild(document.CreateTextNode("hello"))
	nowrap := document.CreateElement("p")
	nowrap.SetAttribute("class", "nowrap")
	nowrap.AppendChild(document.CreateTextNode("hello world"))
	atomic := document.CreateElement("p")
	atomic.SetAttribute("class", "atomic")
	span := document.CreateElement("span")
	span.SetAttribute("class", "block")
	atomic.AppendChild(span)
	body.AppendChild(centered)
	body.AppendChild(right)
	body.AppendChild(nowrap)
	body.AppendChild(atomic)

	engine := testEngine(document, `
		html, body { margin:0; padding:0 }
		p { display:block; width:100px; font-size:16px; line-height:19.2px; margin:0; padding:0; border:0 }
		.center { text-align:center }
		.right { text-align:right }
		.nowrap { white-space:nowrap; width:50px }
		.block { display:inline-block; width:20px; height:10px; margin:0; padding:0; border:0 }
	`)
	root := model.BuildDocument(document, engine)
	layout := New(400, 600)
	layout.Layout(root)

	if got := findBox(root, centered).Children[0].X; got != 30 {
		t.Errorf("centered x = %v, want 30", got)
	}
	if got := findBox(root, right).Children[0].X; got != 60 {
		t.Errorf("right x = %v, want 60", got)
	}
	if got := findBox(root, nowrap).ContentHeight; math.Abs(got-19.2) > 1e-9 {
		t.Errorf("nowrap height = %v, want 19.2", got)
	}
	atomicBox := findBox(root, atomic)
	if len(atomicBox.Children) != 1 || atomicBox.Children[0] != findBox(root, span) {
		t.Fatalf("atomic children = %#v", atomicBox.Children)
	}
	if got := findBox(root, span).Width; got != 20 {
		t.Errorf("inline-block width = %v, want 20", got)
	}
	if got := findBox(root, span).Height; got != 10 {
		t.Errorf("inline-block height = %v, want 10", got)
	}
}

func TestInlineVerticalAlign(t *testing.T) {
	document, body := testDocument()
	paragraph := document.CreateElement("p")
	paragraph.SetAttribute("class", "line")
	top := document.CreateElement("span")
	top.SetAttribute("class", "top")
	middle := document.CreateElement("span")
	middle.SetAttribute("class", "middle")
	paragraph.AppendChild(top)
	paragraph.AppendChild(middle)
	body.AppendChild(paragraph)
	engine := testEngine(document, `
		html, body { margin:0; padding:0 }
		p { display:block; width:100px; font-size:16px; line-height:20px; margin:0; padding:0; border:0 }
		span { display:inline-block; width:10px; height:10px; margin:0; padding:0; border:0 }
		.top { vertical-align:top }
		.middle { vertical-align:middle }
	`)
	root := model.BuildDocument(document, engine)
	layout := New(400, 600)
	layout.Layout(root)
	if got := findBox(root, top).Y; got != 0 {
		t.Errorf("top aligned y = %v, want 0", got)
	}
	if got := findBox(root, middle).Y; got != 5 {
		t.Errorf("middle aligned y = %v, want 5", got)
	}
}

func TestIntrinsicWidths(t *testing.T) {
	document, body := testDocument()
	minElement := document.CreateElement("p")
	minElement.SetAttribute("class", "min")
	minElement.AppendChild(document.CreateTextNode("hello world"))
	maxElement := document.CreateElement("p")
	maxElement.SetAttribute("class", "max")
	maxElement.AppendChild(document.CreateTextNode("hello world"))
	clampElement := document.CreateElement("p")
	clampElement.SetAttribute("class", "clamp")
	clampElement.AppendChild(document.CreateTextNode("hello world"))
	shrink := document.CreateElement("div")
	shrink.SetAttribute("class", "shrink")
	fitElement := document.CreateElement("p")
	fitElement.SetAttribute("class", "fit")
	fitElement.AppendChild(document.CreateTextNode("hello world"))
	shrink.AppendChild(fitElement)
	body.AppendChild(minElement)
	body.AppendChild(maxElement)
	body.AppendChild(clampElement)
	body.AppendChild(shrink)

	engine := testEngine(document, `
		html, body { margin:0; padding:0 }
		p { display:block; font-size:16px; line-height:19.2px; margin:0; padding:0; border:0 }
		.min { width:min-content }
		.max { width:max-content }
		.clamp { width:max-content; max-width:min-content }
		.shrink { display:block; width:60px }
		.fit { width:fit-content }
	`)
	root := model.BuildDocument(document, engine)
	layout := New(400, 600)
	layout.Layout(root)

	if got := findBox(root, minElement).ContentWidth; got != 40 {
		t.Errorf("min-content width = %v, want 40", got)
	}
	if got := findBox(root, maxElement).ContentWidth; got != 84 {
		t.Errorf("max-content width = %v, want 84", got)
	}
	if got := findBox(root, clampElement).ContentWidth; got != 40 {
		t.Errorf("max-width min-content = %v, want 40", got)
	}
	if got := findBox(root, fitElement).ContentWidth; got != 60 {
		t.Errorf("fit-content width = %v, want 60", got)
	}
}

func TestHeightSizingConstraints(t *testing.T) {
	document, body := testDocument()
	parent := document.CreateElement("div")
	parent.SetAttribute("class", "parent")
	percentage := document.CreateElement("div")
	percentage.SetAttribute("class", "percentage")
	clamped := document.CreateElement("div")
	clamped.SetAttribute("class", "clamped")
	ratio := document.CreateElement("div")
	ratio.SetAttribute("class", "ratio")
	fit := document.CreateElement("p")
	fit.SetAttribute("class", "fit")
	fit.AppendChild(document.CreateTextNode("hello world"))
	parent.AppendChild(percentage)
	parent.AppendChild(clamped)
	parent.AppendChild(ratio)
	body.AppendChild(parent)
	body.AppendChild(fit)

	engine := testEngine(document, `
		html, body { margin:0; padding:0 }
		div, p { display:block; margin:0; padding:0; border:0 }
		.parent { width:200px; height:200px }
		.percentage { height:50% }
		.clamped { height:300px; min-height:50px; max-height:120px }
		.ratio { width:100px; aspect-ratio:16 / 9 }
		.fit { font-size:16px; line-height:19.2px; width:fit-content(100px) }
	`)
	root := model.BuildDocument(document, engine)
	layout := New(400, 600)
	layout.Layout(root)

	if got := findBox(root, percentage).ContentHeight; got != 100 {
		t.Errorf("percentage height = %v, want 100", got)
	}
	if got := findBox(root, clamped).ContentHeight; got != 120 {
		t.Errorf("max-height clamp = %v, want 120", got)
	}
	if got := findBox(root, ratio).ContentHeight; math.Abs(got-56.25) > 1e-9 {
		t.Errorf("aspect-ratio height = %v, want 56.25", got)
	}
	if got := findBox(root, fit).ContentWidth; got != 84 {
		t.Errorf("fit-content() width = %v, want 84", got)
	}
}

func TestMinHeightBorderBox(t *testing.T) {
	document, body := testDocument()
	target := document.CreateElement("div")
	target.SetAttribute("class", "target")
	body.AppendChild(target)
	engine := testEngine(document, `
		html, body { margin:0; padding:0 }
		.target { display:block; box-sizing:border-box; height:10px; min-height:50px; padding:5px; margin:0; border:0 }
	`)
	root := model.BuildDocument(document, engine)
	layout := New(400, 600)
	layout.Layout(root)
	targetBox := findBox(root, target)
	if targetBox.Height != 50 || targetBox.ContentHeight != 40 {
		t.Errorf("border-box min-height = %v/%v, want 50/40", targetBox.Height, targetBox.ContentHeight)
	}
}

func TestMarginCollapseThroughAndNegative(t *testing.T) {
	document, body := testDocument()
	container := document.CreateElement("div")
	container.SetAttribute("class", "container")
	empty := document.CreateElement("div")
	empty.SetAttribute("class", "empty")
	after := document.CreateElement("div")
	after.SetAttribute("class", "after")
	container.AppendChild(empty)
	container.AppendChild(after)

	negative := document.CreateElement("div")
	negative.SetAttribute("class", "negative")
	first := document.CreateElement("div")
	first.SetAttribute("class", "first")
	second := document.CreateElement("div")
	second.SetAttribute("class", "second")
	negative.AppendChild(first)
	negative.AppendChild(second)
	body.AppendChild(container)
	body.AppendChild(negative)

	engine := testEngine(document, `
		html, body { margin:0; padding:0 }
		div { display:block; padding:0; border:0 }
		.container { width:200px }
		.empty { margin:20px 0 30px 0 }
		.after { height:10px; margin:0 }
		.first { height:20px; margin:10px 0 20px 0 }
		.second { height:10px; margin:-30px 0 0 0 }
	`)
	root := model.BuildDocument(document, engine)
	layout := New(400, 600)
	layout.Layout(root)

	containerBox := findBox(root, container)
	if got := findBox(root, after).Y - containerBox.ContentY; got != 30 {
		t.Errorf("collapsed-through after offset = %v, want 30", got)
	}
	negativeBox := findBox(root, negative)
	if got := findBox(root, second).Y - negativeBox.ContentY; got != 10 {
		t.Errorf("negative margin second offset = %v, want 10", got)
	}
}

func TestParentChildMarginCollapse(t *testing.T) {
	document, body := testDocument()
	outer := document.CreateElement("div")
	outer.SetAttribute("class", "outer")
	inner := document.CreateElement("div")
	inner.SetAttribute("class", "inner")
	outer.AppendChild(inner)
	body.AppendChild(outer)

	engine := testEngine(document, `
		html, body { margin:0; padding:0 }
		.outer { display:block; margin:20px 0 0 0; padding:0; border:0 }
		.inner { display:block; margin:30px 0 0 0; padding:0; border:0; height:10px }
	`)
	root := model.BuildDocument(document, engine)
	layout := New(400, 600)
	layout.Layout(root)
	outerBox := findBox(root, outer)
	innerBox := findBox(root, inner)
	if outerBox.Y != 30 {
		t.Errorf("collapsed outer y = %v, want 30", outerBox.Y)
	}
	if innerBox.Y != 30 {
		t.Errorf("collapsed inner y = %v, want 30", innerBox.Y)
	}
	if outerBox.ContentHeight != 10 {
		t.Errorf("outer content height = %v, want 10", outerBox.ContentHeight)
	}
}

func TestPaddingPreventsMarginCollapse(t *testing.T) {
	document, body := testDocument()
	outer := document.CreateElement("div")
	outer.SetAttribute("class", "outer")
	inner := document.CreateElement("div")
	inner.SetAttribute("class", "inner")
	outer.AppendChild(inner)
	body.AppendChild(outer)

	engine := testEngine(document, `
		html, body { margin:0; padding:0 }
		.outer { display:block; margin-top:20px; padding-top:1px; padding-bottom:0; padding-left:0; padding-right:0; border:0 }
		.inner { display:block; margin-top:30px; margin-bottom:0; margin-left:0; margin-right:0; height:10px; padding:0; border:0 }
	`)
	root := model.BuildDocument(document, engine)
	layout := New(400, 600)
	layout.Layout(root)
	outerBox := findBox(root, outer)
	innerBox := findBox(root, inner)
	if outerBox.Y != 20 || innerBox.Y != 51 {
		t.Errorf("padding case outer y=%v inner y=%v, want 20/51", outerBox.Y, innerBox.Y)
	}
}

func TestOverflowPreventsMarginCollapse(t *testing.T) {
	document, body := testDocument()
	outer := document.CreateElement("div")
	outer.SetAttribute("class", "outer")
	inner := document.CreateElement("div")
	inner.SetAttribute("class", "inner")
	outer.AppendChild(inner)
	body.AppendChild(outer)

	engine := testEngine(document, `
		html, body { margin:0; padding:0 }
		.outer { display:block; margin-top:20px; overflow-x:hidden; overflow-y:hidden; padding:0; border:0 }
		.inner { display:block; margin-top:30px; margin-bottom:0; margin-left:0; margin-right:0; height:10px; padding:0; border:0 }
	`)
	root := model.BuildDocument(document, engine)
	layout := New(400, 600)
	layout.Layout(root)
	if got := findBox(root, inner).Y; got != 50 {
		t.Errorf("BFC inner y = %v, want 50", got)
	}
}

func TestParentChildBottomMarginCollapse(t *testing.T) {
	document, body := testDocument()
	outer := document.CreateElement("div")
	outer.SetAttribute("class", "outer")
	inner := document.CreateElement("div")
	inner.SetAttribute("class", "inner")
	outer.AppendChild(inner)
	next := document.CreateElement("div")
	next.SetAttribute("class", "next")
	body.AppendChild(outer)
	body.AppendChild(next)

	engine := testEngine(document, `
		html, body { margin:0; padding:0 }
		.outer { display:block; margin:0; padding:0; border:0 }
		.inner { display:block; margin:0 0 30px 0; height:10px; padding:0; border:0 }
		.next { display:block; height:10px; margin:0 }
	`)
	root := model.BuildDocument(document, engine)
	layout := New(400, 600)
	layout.Layout(root)
	outerBox := findBox(root, outer)
	innerBox := findBox(root, inner)
	nextBox := findBox(root, next)
	if outerBox.ContentHeight != 10 {
		t.Errorf("outer content height = %v, want 10", outerBox.ContentHeight)
	}
	if innerBox.Y != 0 {
		t.Errorf("inner y = %v, want 0", innerBox.Y)
	}
	if nextBox.Y != 40 {
		t.Errorf("next y = %v, want 40", nextBox.Y)
	}
}

func TestPositioning(t *testing.T) {
	document, body := testDocument()
	container := document.CreateElement("div")
	container.SetAttribute("class", "container")
	relative := document.CreateElement("div")
	relative.SetAttribute("class", "rel")
	child := document.CreateElement("div")
	child.SetAttribute("class", "child")
	relative.AppendChild(child)
	absolute := document.CreateElement("div")
	absolute.SetAttribute("class", "abs")
	stretch := document.CreateElement("div")
	stretch.SetAttribute("class", "stretch")
	absoluteBottom := document.CreateElement("div")
	absoluteBottom.SetAttribute("class", "abs-bottom")
	fixed := document.CreateElement("div")
	fixed.SetAttribute("class", "fixed")
	container.AppendChild(relative)
	container.AppendChild(absolute)
	container.AppendChild(stretch)
	container.AppendChild(absoluteBottom)
	body.AppendChild(container)
	body.AppendChild(fixed)

	engine := testEngine(document, `
		html, body { margin:0; padding:0 }
		div { margin:0; padding:0; border:0 }
		.container { position:relative; display:block; width:200px; height:100px }
		.rel { position:relative; left:10px; top:5px; height:10px }
		.child { height:5px }
		.abs { position:absolute; left:20px; top:10px; width:50px; height:20px }
		.stretch { position:absolute; left:10px; right:10px; top:0; height:10px }
		.abs-bottom { position:absolute; left:0; bottom:10px; width:20px; height:30px }
		.fixed { position:fixed; right:10px; bottom:10px; width:40px; height:20px; z-index:5 }
	`)
	root := model.BuildDocument(document, engine)
	layout := New(400, 600)
	layout.Layout(root)

	relBox := findBox(root, relative)
	childBox := findBox(root, child)
	if relBox.X != 10 || relBox.Y != 5 || childBox.X != 10 || childBox.Y != 5 {
		t.Errorf("relative boxes = rel(%v,%v) child(%v,%v)", relBox.X, relBox.Y, childBox.X, childBox.Y)
	}
	absBox := findBox(root, absolute)
	if absBox.X != 20 || absBox.Y != 10 || absBox.Width != 50 || absBox.Height != 20 {
		t.Errorf("absolute box = (%v,%v) %vx%v", absBox.X, absBox.Y, absBox.Width, absBox.Height)
	}
	stretchBox := findBox(root, stretch)
	if stretchBox.X != 10 || stretchBox.Width != 180 {
		t.Errorf("stretch box x/width = %v/%v, want 10/180", stretchBox.X, stretchBox.Width)
	}
	bottomBox := findBox(root, absoluteBottom)
	if bottomBox.Y != 60 {
		t.Errorf("bottom box y = %v, want 60", bottomBox.Y)
	}
	fixedBox := findBox(root, fixed)
	if fixedBox.X != 350 || fixedBox.Y != 570 {
		t.Errorf("fixed box = (%v,%v), want (350,570)", fixedBox.X, fixedBox.Y)
	}
	if fixedBox.ZIndex != 5 {
		t.Errorf("fixed z-index = %d, want 5", fixedBox.ZIndex)
	}
	if got := findBox(root, container).ContentHeight; got != 100 {
		t.Errorf("container content height = %v, want 100", got)
	}
}

func TestPositioningContainingBlockAndStatic(t *testing.T) {
	document, body := testDocument()
	container := document.CreateElement("div")
	container.SetAttribute("class", "container")
	flow := document.CreateElement("div")
	flow.SetAttribute("class", "flow")
	absolute := document.CreateElement("div")
	absolute.SetAttribute("class", "absolute")
	static := document.CreateElement("div")
	static.SetAttribute("class", "static")
	sticky := document.CreateElement("div")
	sticky.SetAttribute("class", "sticky")
	container.AppendChild(flow)
	container.AppendChild(absolute)
	container.AppendChild(static)
	body.AppendChild(container)
	body.AppendChild(sticky)

	engine := testEngine(document, `
		html, body { margin:0; padding:0 }
		div { margin:0; border:0 }
		.container { position:relative; width:200px; height:100px; padding:10px }
		.flow { height:20px }
		.absolute { position:absolute; left:0; top:0; width:10px; height:10px }
		.static { position:absolute; width:10px; height:10px }
		.sticky { position:sticky; top:5px; height:10px }
	`)
	root := model.BuildDocument(document, engine)
	layout := New(400, 600)
	layout.Layout(root)

	if got := findBox(root, absolute).X; got != 0 {
		t.Errorf("absolute containing-block x = %v, want 0", got)
	}
	if got := findBox(root, absolute).Y; got != 0 {
		t.Errorf("absolute containing-block y = %v, want 0", got)
	}
	if got := findBox(root, static).Y; got != 30 {
		t.Errorf("static position y = %v, want 30", got)
	}
	if got := findBox(root, sticky).Y; got != 120 {
		t.Errorf("sticky flow y = %v, want 120", got)
	}
}

func TestFlexboxRow(t *testing.T) {
	document, body := testDocument()
	flex := document.CreateElement("div")
	flex.SetAttribute("class", "flex row")
	items := []*dom.Node{}
	for index := 0; index < 3; index++ {
		item := document.CreateElement("div")
		item.SetAttribute("class", "item")
		flex.AppendChild(item)
		items = append(items, item)
	}
	body.AppendChild(flex)

	engine := testEngine(document, `
		html, body { margin:0; padding:0 }
		.flex { display:flex; width:300px; height:40px; margin:0; padding:0; border:0; align-items:flex-start }
		.row { justify-content:space-between }
		.item { width:50px; height:10px; margin:0; padding:0; border:0 }
	`)
	root := model.BuildDocument(document, engine)
	layout := New(400, 600)
	layout.Layout(root)

	wantX := []float64{0, 125, 250}
	for index, item := range items {
		box := findBox(root, item)
		if box.X != wantX[index] || box.Y != 0 {
			t.Errorf("row item %d = (%v,%v), want (%v,0)", index, box.X, box.Y, wantX[index])
		}
		if box.Width != 50 {
			t.Errorf("row item %d width = %v, want 50", index, box.Width)
		}
	}
}

func TestFlexboxGrow(t *testing.T) {
	document, body := testDocument()
	flex := document.CreateElement("div")
	flex.SetAttribute("class", "flex")
	first := document.CreateElement("div")
	first.SetAttribute("class", "grow1")
	second := document.CreateElement("div")
	second.SetAttribute("class", "grow2")
	flex.AppendChild(first)
	flex.AppendChild(second)
	body.AppendChild(flex)

	engine := testEngine(document, `
		html, body { margin:0; padding:0 }
		.flex { display:flex; width:300px; height:20px; margin:0; padding:0; border:0; align-items:flex-start }
		.grow1 { flex-grow:1; flex-basis:0%; height:10px; margin:0; padding:0; border:0 }
		.grow2 { flex-grow:2; flex-basis:0%; height:10px; margin:0; padding:0; border:0 }
	`)
	root := model.BuildDocument(document, engine)
	layout := New(400, 600)
	layout.Layout(root)

	firstBox := findBox(root, first)
	secondBox := findBox(root, second)
	if firstBox.Width != 100 || firstBox.X != 0 {
		t.Errorf("grow1 = x:%v w:%v, want 0/100", firstBox.X, firstBox.Width)
	}
	if secondBox.Width != 200 || secondBox.X != 100 {
		t.Errorf("grow2 = x:%v w:%v, want 100/200", secondBox.X, secondBox.Width)
	}
}

func TestFlexboxColumn(t *testing.T) {
	document, body := testDocument()
	flex := document.CreateElement("div")
	flex.SetAttribute("class", "flex")
	first := document.CreateElement("div")
	first.SetAttribute("class", "item1")
	second := document.CreateElement("div")
	second.SetAttribute("class", "item2")
	flex.AppendChild(first)
	flex.AppendChild(second)
	body.AppendChild(flex)

	engine := testEngine(document, `
		html, body { margin:0; padding:0 }
		.flex { display:flex; flex-direction:column; width:100px; height:100px; margin:0; padding:0; border:0; align-items:center }
		.item1 { width:20px; height:10px; margin:0; padding:0; border:0 }
		.item2 { width:40px; height:20px; margin:0; padding:0; border:0 }
	`)
	root := model.BuildDocument(document, engine)
	layout := New(400, 600)
	layout.Layout(root)

	firstBox := findBox(root, first)
	secondBox := findBox(root, second)
	if firstBox.X != 40 || firstBox.Y != 0 {
		t.Errorf("column item1 = (%v,%v), want (40,0)", firstBox.X, firstBox.Y)
	}
	if secondBox.X != 30 || secondBox.Y != 10 {
		t.Errorf("column item2 = (%v,%v), want (30,10)", secondBox.X, secondBox.Y)
	}
}

func TestFlexboxGapAndOrder(t *testing.T) {
	document, body := testDocument()
	flex := document.CreateElement("div")
	flex.SetAttribute("class", "flex")
	first := document.CreateElement("div")
	first.SetAttribute("class", "item order2")
	second := document.CreateElement("div")
	second.SetAttribute("class", "item order1")
	third := document.CreateElement("div")
	third.SetAttribute("class", "item")
	flex.AppendChild(first)
	flex.AppendChild(second)
	flex.AppendChild(third)
	body.AppendChild(flex)

	engine := testEngine(document, `
		html, body { margin:0; padding:0 }
		.flex { display:flex; width:300px; height:20px; gap:10px; margin:0; padding:0; border:0; align-items:flex-start; justify-content:flex-start }
		.item { width:50px; height:10px; margin:0; padding:0; border:0 }
		.order1 { order:1 }
		.order2 { order:2 }
	`)
	root := model.BuildDocument(document, engine)
	layout := New(400, 600)
	layout.Layout(root)

	// The un-ordered third item comes first, then order1, then order2.
	if got := findBox(root, third).X; got != 0 {
		t.Errorf("third x = %v, want 0", got)
	}
	if got := findBox(root, second).X; got != 60 {
		t.Errorf("second x = %v, want 60", got)
	}
	if got := findBox(root, first).X; got != 120 {
		t.Errorf("first x = %v, want 120", got)
	}
}

func TestTableLayout(t *testing.T) {
	document, body := testDocument()
	table := document.CreateElement("table")
	table.SetAttribute("class", "table")
	row1 := document.CreateElement("tr")
	cellA := document.CreateElement("td")
	cellA.AppendChild(document.CreateTextNode("a"))
	cellB := document.CreateElement("td")
	cellB.AppendChild(document.CreateTextNode("bb"))
	row1.AppendChild(cellA)
	row1.AppendChild(cellB)
	row2 := document.CreateElement("tr")
	cellC := document.CreateElement("td")
	cellC.AppendChild(document.CreateTextNode("ccc"))
	cellD := document.CreateElement("td")
	cellD.AppendChild(document.CreateTextNode("d"))
	row2.AppendChild(cellC)
	row2.AppendChild(cellD)
	table.AppendChild(row1)
	table.AppendChild(row2)
	body.AppendChild(table)

	engine := testEngine(document, `
		html, body { margin:0; padding:0 }
		table { display:table; margin:0; padding:0; border:0 }
		tr { display:table-row }
		td { display:table-cell; margin:0; padding:0; border:0; font-size:16px; line-height:19.2px }
	`)
	root := model.BuildDocument(document, engine)
	layout := New(400, 600)
	layout.Layout(root)

	tableBox := findBox(root, table)
	if tableBox.ContentWidth != 40 || tableBox.ContentHeight != 38.4 {
		t.Errorf("table = %vx%v, want 40x38.4", tableBox.ContentWidth, tableBox.ContentHeight)
	}
	row1Box := findBox(root, row1)
	row2Box := findBox(root, row2)
	if row1Box.Y != 0 || row2Box.Y != 19.2 {
		t.Errorf("rows = y:%v and y:%v", row1Box.Y, row2Box.Y)
	}
	if findBox(root, cellA).X != 0 || findBox(root, cellA).Width != 24 {
		t.Errorf("cell A = x:%v w:%v, want 0/24", findBox(root, cellA).X, findBox(root, cellA).Width)
	}
	if findBox(root, cellB).X != 24 || findBox(root, cellB).Width != 16 {
		t.Errorf("cell B = x:%v w:%v, want 24/16", findBox(root, cellB).X, findBox(root, cellB).Width)
	}
}

func TestTableSpans(t *testing.T) {
	document, body := testDocument()
	table := document.CreateElement("table")
	table.SetAttribute("class", "table")
	firstRow := document.CreateElement("tr")
	spanned := document.CreateElement("td")
	spanned.SetAttribute("id", "spanned")
	spanned.SetAttribute("colspan", "2")
	spanned.AppendChild(document.CreateTextNode("a"))
	firstRow.AppendChild(spanned)
	secondRow := document.CreateElement("tr")
	left := document.CreateElement("td")
	left.SetAttribute("id", "left")
	left.AppendChild(document.CreateTextNode("a"))
	right := document.CreateElement("td")
	right.SetAttribute("id", "right")
	right.AppendChild(document.CreateTextNode("b"))
	secondRow.AppendChild(left)
	secondRow.AppendChild(right)
	table.AppendChild(firstRow)
	table.AppendChild(secondRow)
	body.AppendChild(table)

	engine := testEngine(document, `
		html, body { margin:0; padding:0 }
		table { display:table; width:100px; border-collapse:separate; margin:0; padding:0; border:0 }
		tr { display:table-row }
		td { display:table-cell; margin:0; padding:0; border:0; font-size:16px; line-height:19.2px }
	`)
	root := model.BuildDocument(document, engine)
	layout := New(400, 600)
	layout.Layout(root)

	if got := findBox(root, spanned).Width; got != 100 {
		t.Errorf("colspan width = %v, want 100", got)
	}
	if got := findBox(root, left).Width; got != 50 {
		t.Errorf("left column width = %v, want 50", got)
	}
	if got := findBox(root, right).X; got != 50 {
		t.Errorf("right column x = %v, want 50", got)
	}
}

func TestTableRowSpanAndBorderCollapse(t *testing.T) {
	document, body := testDocument()
	table := document.CreateElement("table")
	table.SetAttribute("class", "table")
	firstRow := document.CreateElement("tr")
	tall := document.CreateElement("td")
	tall.SetAttribute("id", "tall")
	tall.SetAttribute("rowspan", "2")
	tall.AppendChild(document.CreateTextNode("a"))
	firstRight := document.CreateElement("td")
	firstRight.SetAttribute("id", "first-right")
	firstRight.AppendChild(document.CreateTextNode("b"))
	firstRow.AppendChild(tall)
	firstRow.AppendChild(firstRight)
	secondRow := document.CreateElement("tr")
	secondRight := document.CreateElement("td")
	secondRight.SetAttribute("id", "second-right")
	secondRight.AppendChild(document.CreateTextNode("c"))
	secondRow.AppendChild(secondRight)
	table.AppendChild(firstRow)
	table.AppendChild(secondRow)
	body.AppendChild(table)

	engine := testEngine(document, `
		html, body { margin:0; padding:0 }
		table { display:table; border-collapse:collapse; margin:0; padding:0; border:0 }
		tr { display:table-row }
		td { display:table-cell; margin:0; padding:0; border:2px solid; font-size:16px; line-height:19.2px }
	`)
	root := model.BuildDocument(document, engine)
	layout := New(400, 600)
	layout.Layout(root)

	if got := findBox(root, secondRight).X; got != 10 {
		t.Errorf("rowspan second column x = %v, want 10", got)
	}
	if got := findBox(root, tall).Height; got != 42.4 {
		t.Errorf("rowspan height = %v, want 42.4", got)
	}
	if got := findBox(root, table).Width; got != 20 {
		t.Errorf("collapsed table width = %v, want 20", got)
	}
	if got := findBox(root, tall).ContentWidth; got != 8 {
		t.Errorf("collapsed cell content width = %v, want 8", got)
	}
}

func TestTableExplicitWidth(t *testing.T) {
	document, body := testDocument()
	table := document.CreateElement("table")
	table.SetAttribute("class", "table")
	row := document.CreateElement("tr")
	first := document.CreateElement("td")
	first.AppendChild(document.CreateTextNode("a"))
	second := document.CreateElement("td")
	second.AppendChild(document.CreateTextNode("bb"))
	row.AppendChild(first)
	row.AppendChild(second)
	table.AppendChild(row)
	body.AppendChild(table)

	engine := testEngine(document, `
		html, body { margin:0; padding:0 }
		table { display:table; width:100px; margin:0; padding:0; border:0 }
		tr { display:table-row }
		td { display:table-cell; margin:0; padding:0; border:0; font-size:16px; line-height:19.2px }
	`)
	root := model.BuildDocument(document, engine)
	layout := New(400, 600)
	layout.Layout(root)
	tableBox := findBox(root, table)
	if tableBox.ContentWidth != 100 {
		t.Errorf("table width = %v, want 100", tableBox.ContentWidth)
	}
	if got := findBox(root, first).Width; math.Abs(got-100.0/3) > 1e-9 {
		t.Errorf("first width = %v, want %v", got, 100.0/3)
	}
	if got := findBox(root, second).Width; math.Abs(got-200.0/3) > 1e-9 {
		t.Errorf("second width = %v, want %v", got, 200.0/3)
	}
}

func TestGridLayout(t *testing.T) {
	document, body := testDocument()
	grid := document.CreateElement("div")
	grid.SetAttribute("class", "grid")
	items := []*dom.Node{}
	for index := 0; index < 3; index++ {
		item := document.CreateElement("div")
		item.SetAttribute("class", "item")
		grid.AppendChild(item)
		items = append(items, item)
	}
	body.AppendChild(grid)

	engine := testEngine(document, `
		html, body { margin:0; padding:0 }
		.grid { display:grid; width:160px; height:100px; margin:0; padding:0; border:0; grid-template-columns:50px 100px; grid-template-rows:20px 20px; column-gap:10px; row-gap:10px }
		.item { margin:0; padding:0; border:0; height:20px }
	`)
	root := model.BuildDocument(document, engine)
	layout := New(400, 600)
	layout.Layout(root)

	wantX := []float64{0, 60, 0}
	wantY := []float64{0, 0, 30}
	wantW := []float64{50, 100, 50}
	for index, item := range items {
		box := findBox(root, item)
		if box.X != wantX[index] || box.Y != wantY[index] || box.Width != wantW[index] {
			t.Errorf("grid item %d = x:%v y:%v w:%v, want %v/%v/%v", index, box.X, box.Y, box.Width, wantX[index], wantY[index], wantW[index])
		}
	}
}

func TestGridFrAndPlacement(t *testing.T) {
	document, body := testDocument()
	grid := document.CreateElement("div")
	grid.SetAttribute("class", "fr")
	first := document.CreateElement("div")
	first.SetAttribute("class", "item")
	second := document.CreateElement("div")
	second.SetAttribute("class", "item")
	grid.AppendChild(first)
	grid.AppendChild(second)
	body.AppendChild(grid)

	engine := testEngine(document, `
		html, body { margin:0; padding:0 }
		.fr { display:grid; width:300px; height:20px; margin:0; padding:0; border:0; grid-template-columns:1fr 2fr; gap:0 }
		.item { margin:0; padding:0; border:0; height:20px }
	`)
	root := model.BuildDocument(document, engine)
	layout := New(400, 600)
	layout.Layout(root)
	if got := findBox(root, first).Width; got != 100 {
		t.Errorf("fr first width = %v, want 100", got)
	}
	if got := findBox(root, second).Width; got != 200 {
		t.Errorf("fr second width = %v, want 200", got)
	}
	if got := findBox(root, second).X; got != 100 {
		t.Errorf("fr second x = %v, want 100", got)
	}

	document, body = testDocument()
	placed := document.CreateElement("div")
	placed.SetAttribute("class", "placed")
	explicit := document.CreateElement("div")
	explicit.SetAttribute("class", "explicit")
	auto := document.CreateElement("div")
	auto.SetAttribute("class", "auto")
	placed.AppendChild(explicit)
	placed.AppendChild(auto)
	body.AppendChild(placed)
	engine = testEngine(document, `
		html, body { margin:0; padding:0 }
		.placed { display:grid; width:160px; height:100px; margin:0; padding:0; border:0; grid-template-columns:50px 100px; grid-template-rows:20px 20px; column-gap:10px; row-gap:10px }
		.explicit, .auto { margin:0; padding:0; border:0; height:20px }
		.explicit { grid-column-start:2; grid-row-start:1 }
	`)
	root = model.BuildDocument(document, engine)
	layout = New(400, 600)
	layout.Layout(root)
	if got := findBox(root, explicit).X; got != 60 {
		t.Errorf("explicit x = %v, want 60", got)
	}
	if got := findBox(root, auto).X; got != 0 {
		t.Errorf("auto x = %v, want 0", got)
	}
}

func TestGridImplicitTracksSpansAndAlignment(t *testing.T) {
	document, body := testDocument()
	grid := document.CreateElement("div")
	grid.SetAttribute("class", "grid")
	explicit := document.CreateElement("div")
	explicit.SetAttribute("class", "explicit")
	spanning := document.CreateElement("div")
	spanning.SetAttribute("class", "spanning")
	centered := document.CreateElement("div")
	centered.SetAttribute("class", "centered")
	grid.AppendChild(explicit)
	grid.AppendChild(spanning)
	grid.AppendChild(centered)
	body.AppendChild(grid)

	engine := testEngine(document, `
		html, body { margin:0; padding:0 }
		.grid { display:grid; width:120px; height:40px; grid-template-columns:minmax(40px, 1fr) minmax(40px, 1fr); grid-template-rows:20px 20px; column-gap:0; row-gap:0; justify-items:center; align-items:center; margin:0; padding:0; border:0 }
		.explicit { grid-column-start:3; justify-self:start; width:20px; height:10px; margin:0; padding:0; border:0 }
		.spanning { grid-column-start:span 2; justify-self:stretch; width:20px; height:10px; margin:0; padding:0; border:0 }
		.centered { width:20px; height:10px; margin:0; padding:0; border:0 }
	`)
	root := model.BuildDocument(document, engine)
	layout := New(400, 600)
	layout.Layout(root)

	if got := findBox(root, explicit).X; got != 120 {
		t.Errorf("implicit column x = %v, want 120", got)
	}
	if got := findBox(root, spanning).Width; got != 120 {
		t.Errorf("span width = %v, want 120", got)
	}
	centeredBox := findBox(root, centered)
	if centeredBox.X != 20 || centeredBox.Y != 25 {
		t.Errorf("centered grid item = (%v,%v), want (20,25)", centeredBox.X, centeredBox.Y)
	}
}

func TestFloatLayout(t *testing.T) {
	document, body := testDocument()
	left := document.CreateElement("div")
	left.SetAttribute("class", "float-left")
	right := document.CreateElement("div")
	right.SetAttribute("class", "float-right")
	after := document.CreateElement("div")
	after.SetAttribute("class", "after")
	body.AppendChild(left)
	body.AppendChild(right)
	body.AppendChild(after)

	engine := testEngine(document, `
		html, body { margin:0; padding:0 }
		.float-left { float:left; width:50px; height:20px; margin:0; padding:0; border:0 }
		.float-right { float:right; width:50px; height:20px; margin:0; padding:0; border:0 }
		.after { display:block; height:10px; margin:0; padding:0; border:0 }
	`)
	root := model.BuildDocument(document, engine)
	layout := New(400, 600)
	layout.Layout(root)

	leftBox := findBox(root, left)
	rightBox := findBox(root, right)
	afterBox := findBox(root, after)
	if leftBox.X != 0 || leftBox.Y != 0 {
		t.Errorf("left float = (%v,%v)", leftBox.X, leftBox.Y)
	}
	if rightBox.X != 350 || rightBox.Y != 0 {
		t.Errorf("right float = (%v,%v)", rightBox.X, rightBox.Y)
	}
	if afterBox.Y != 20 {
		t.Errorf("after y = %v, want 20", afterBox.Y)
	}
}

func TestFloatStackingAndClear(t *testing.T) {
	document, body := testDocument()
	container := document.CreateElement("div")
	container.SetAttribute("class", "container")
	first := document.CreateElement("div")
	first.SetAttribute("class", "first")
	second := document.CreateElement("div")
	second.SetAttribute("class", "second")
	cleared := document.CreateElement("div")
	cleared.SetAttribute("class", "cleared")
	container.AppendChild(first)
	container.AppendChild(second)
	container.AppendChild(cleared)
	body.AppendChild(container)
	engine := testEngine(document, `
		html, body { margin:0; padding:0 }
		.container { display:block; width:200px; margin:0; padding:0; border:0 }
		.first, .second { float:left; width:50px; height:20px; margin:0; padding:0; border:0 }
		.cleared { clear:left; height:10px; margin:0; padding:0; border:0 }
	`)
	root := model.BuildDocument(document, engine)
	layout := New(400, 600)
	layout.Layout(root)

	if got := findBox(root, second).X; got != 50 {
		t.Errorf("second float x = %v, want 50", got)
	}
	if got := findBox(root, second).Y; got != 0 {
		t.Errorf("second float y = %v, want 0", got)
	}
	if got := findBox(root, cleared).Y; got != 20 {
		t.Errorf("cleared y = %v, want 20", got)
	}
	if got := findBox(root, container).Height; got != 30 {
		t.Errorf("float container height = %v, want 30", got)
	}
}

func TestOverflowMetrics(t *testing.T) {
	document, body := testDocument()
	parent := document.CreateElement("div")
	parent.SetAttribute("class", "parent")
	child := document.CreateElement("div")
	child.SetAttribute("class", "child")
	parent.AppendChild(child)
	absolute := document.CreateElement("div")
	absolute.SetAttribute("class", "absolute")
	parent.AppendChild(absolute)
	body.AppendChild(parent)

	engine := testEngine(document, `
		html, body { margin:0; padding:0 }
		.parent { position:relative; display:block; width:50px; height:20px; margin:0; padding:0; border:0 }
		.child { display:block; width:40px; height:40px; margin:0; padding:0; border:0 }
		.absolute { position:absolute; left:60px; top:0; width:20px; height:10px; margin:0; padding:0; border:0 }
	`)
	root := model.BuildDocument(document, engine)
	layout := New(400, 600)
	layout.Layout(root)
	parentBox := findBox(root, parent)
	if parentBox.ScrollWidth != 80 || parentBox.ScrollHeight != 40 {
		t.Errorf("parent scroll = %vx%v, want 80x40", parentBox.ScrollWidth, parentBox.ScrollHeight)
	}
}

func TestTextShaper(t *testing.T) {
	glyphs := (model.SimpleShaper{}).Shape(model.TextRun{Text: "ab", FontSize: 16})
	if len(glyphs) != 2 || glyphs[0].Advance != 8 || glyphs[1].Advance != 8 {
		t.Errorf("glyphs = %#v", glyphs)
	}
	glyphs = (model.SimpleShaper{}).Shape(model.TextRun{Text: "a b", FontSize: 16, LetterSpacing: 2, WordSpacing: 4})
	if len(glyphs) != 3 || glyphs[0].Advance != 10 || glyphs[1].Advance != 14 || glyphs[2].Advance != 10 {
		t.Errorf("spaced glyphs = %#v", glyphs)
	}

	document, body := testDocument()
	paragraph := document.CreateElement("p")
	paragraph.AppendChild(document.CreateTextNode("a b"))
	body.AppendChild(paragraph)
	engine := testEngine(document, `
		html, body { margin:0; padding:0 }
		p { display:block; font-size:16px; line-height:19.2px; letter-spacing:2px; word-spacing:4px; margin:0; padding:0; border:0 }
	`)
	styleMap := engine.ComputeDocumentTree(document)
	layout := New(400, 600)
	width, height := layout.measureText("a b", styleMap[paragraph])
	if width != 34 || height != 19.2 {
		t.Errorf("measured = %vx%v, want 34x19.2", width, height)
	}
}

func TestBidiSegmentation(t *testing.T) {
	runs := model.SegmentBidi("hello", bidi.LeftToRight)
	if len(runs) != 1 || runs[0].Text != "hello" || runs[0].Direction != bidi.LeftToRight {
		t.Errorf("ltr runs = %#v", runs)
	}
	runs = model.SegmentBidi("abc \u05d0\u05d1\u05d2", bidi.LeftToRight)
	if len(runs) < 2 {
		t.Fatalf("mixed runs = %#v", runs)
	}
	hasRTL := false
	for _, run := range runs {
		if run.Direction == bidi.RightToLeft && run.Text != "" {
			hasRTL = true
		}
	}
	if !hasRTL {
		t.Errorf("mixed runs have no RTL run: %#v", runs)
	}
}

func TestFlexColumnWrapAndMaxWidth(t *testing.T) {
	document, body := testDocument()
	column := document.CreateElement("div")
	column.SetAttribute("class", "column")
	columnItems := []*dom.Node{}
	for index := 0; index < 3; index++ {
		item := document.CreateElement("div")
		item.SetAttribute("class", "column-item")
		column.AppendChild(item)
		columnItems = append(columnItems, item)
	}
	row := document.CreateElement("div")
	row.SetAttribute("class", "row")
	growing := document.CreateElement("div")
	growing.SetAttribute("class", "growing")
	row.AppendChild(growing)
	body.AppendChild(column)
	body.AppendChild(row)

	engine := testEngine(document, `
		html, body { margin:0; padding:0 }
		.column { display:flex; flex-direction:column; flex-wrap:wrap; align-items:flex-start; width:100px; height:40px; margin:0; padding:0; border:0 }
		.column-item { width:20px; height:30px; margin:0; padding:0; border:0 }
		.row { display:flex; width:100px; height:20px; margin:0; padding:0; border:0 }
		.growing { flex-grow:1; max-width:60px; height:20px; margin:0; padding:0; border:0 }
	`)
	root := model.BuildDocument(document, engine)
	layout := New(400, 600)
	layout.Layout(root)

	for index, item := range columnItems {
		box := findBox(root, item)
		if box.X != float64(index*20) || box.Y != 0 {
			t.Errorf("column wrap item %d = (%v,%v), want (%v,0)", index, box.X, box.Y, index*20)
		}
	}
	if got := findBox(root, growing).Width; got != 60 {
		t.Errorf("flex max-width = %v, want 60", got)
	}
}

func TestFlexWrapAndBaseline(t *testing.T) {
	document, body := testDocument()
	flex := document.CreateElement("div")
	flex.SetAttribute("class", "wrap")
	items := []*dom.Node{}
	for index := 0; index < 3; index++ {
		item := document.CreateElement("div")
		item.SetAttribute("class", "item")
		flex.AppendChild(item)
		items = append(items, item)
	}
	body.AppendChild(flex)
	engine := testEngine(document, `
		html, body { margin:0; padding:0 }
		.wrap { display:flex; flex-wrap:wrap; width:120px; column-gap:10px; row-gap:0; align-items:flex-start }
		.item { width:50px; height:20px; margin:0; padding:0; border:0 }
	`)
	root := model.BuildDocument(document, engine)
	layout := New(400, 600)
	layout.Layout(root)
	if findBox(root, items[0]).X != 0 || findBox(root, items[1]).X != 60 {
		t.Errorf("first line x = %v/%v", findBox(root, items[0]).X, findBox(root, items[1]).X)
	}
	if findBox(root, items[2]).X != 0 || findBox(root, items[2]).Y != 20 {
		t.Errorf("wrapped item = (%v,%v), want (0,20)", findBox(root, items[2]).X, findBox(root, items[2]).Y)
	}

	document, body = testDocument()
	baseline := document.CreateElement("div")
	baseline.SetAttribute("class", "baseline")
	small := document.CreateElement("span")
	small.SetAttribute("class", "small")
	small.AppendChild(document.CreateTextNode("x"))
	large := document.CreateElement("span")
	large.SetAttribute("class", "large")
	large.AppendChild(document.CreateTextNode("x"))
	baseline.AppendChild(small)
	baseline.AppendChild(large)
	body.AppendChild(baseline)
	engine = testEngine(document, `
		html, body { margin:0; padding:0 }
		.baseline { display:flex; width:200px; align-items:baseline }
		.small { width:50px; margin:0; padding:0; border:0; font-size:16px }
		.large { width:50px; margin:0; padding:0; border:0; font-size:24px }
	`)
	root = model.BuildDocument(document, engine)
	layout = New(400, 600)
	layout.Layout(root)
	if got := findBox(root, small).Y; math.Abs(got-6.4) > 1e-9 {
		t.Errorf("small baseline y = %v, want 6.4", got)
	}
	if got := findBox(root, large).Y; got != 0 {
		t.Errorf("large baseline y = %v, want 0", got)
	}
}

func TestScrollContainer(t *testing.T) {
	document, body := testDocument()
	parent := document.CreateElement("div")
	parent.SetAttribute("class", "scroller")
	child := document.CreateElement("div")
	child.SetAttribute("class", "child")
	parent.AppendChild(child)
	body.AppendChild(parent)
	engine := testEngine(document, `
		html, body { margin:0; padding:0 }
		.scroller { display:block; width:50px; height:20px; overflow:auto; margin:0; padding:0; border:0 }
		.child { display:block; width:100px; height:40px; margin:0; padding:0; border:0 }
	`)
	root := model.BuildDocument(document, engine)
	layout := New(400, 600)
	layout.Layout(root)
	scroller := findBox(root, parent)
	if !scroller.ClipX || !scroller.ClipY {
		t.Error("overflow:auto should clip both axes")
	}
	if scroller.ScrollWidth != 100 || scroller.ScrollHeight != 40 {
		t.Errorf("scroll size = %vx%v, want 100x40", scroller.ScrollWidth, scroller.ScrollHeight)
	}
	if !scroller.HasVerticalScrollbar || !scroller.HasHorizontalScrollbar {
		t.Error("overflow:auto with overflow should expose scrollbars")
	}
	if scroller.MaxScrollLeft() != 65 || scroller.MaxScrollTop() != 35 {
		t.Errorf("max scroll = %v/%v, want 65/35", scroller.MaxScrollLeft(), scroller.MaxScrollTop())
	}
	scroller.ScrollTo(100, 100)
	if scroller.ScrollLeft != 65 || scroller.ScrollTop != 35 {
		t.Errorf("clamped scroll = %v/%v, want 65/35", scroller.ScrollLeft, scroller.ScrollTop)
	}
	scroller.ScrollBy(-10, -5)
	if scroller.ScrollLeft != 55 || scroller.ScrollTop != 30 {
		t.Errorf("relative scroll = %v/%v, want 55/30", scroller.ScrollLeft, scroller.ScrollTop)
	}

	document, body = testDocument()
	visible := document.CreateElement("div")
	visible.SetAttribute("class", "visible")
	visible.AppendChild(document.CreateTextNode("x"))
	body.AppendChild(visible)
	engine = testEngine(document, `
		html, body { margin:0; padding:0 }
		.visible { display:block; width:50px; height:20px; overflow:visible; margin:0; padding:0; border:0 }
	`)
	root = model.BuildDocument(document, engine)
	layout = New(400, 600)
	layout.Layout(root)
	if findBox(root, visible).ClipX || findBox(root, visible).ClipY {
		t.Error("overflow:visible should not clip")
	}

	document, body = testDocument()
	hidden := document.CreateElement("div")
	hidden.SetAttribute("class", "hidden")
	hiddenChild := document.CreateElement("div")
	hiddenChild.SetAttribute("class", "child")
	hidden.AppendChild(hiddenChild)
	fits := document.CreateElement("div")
	fits.SetAttribute("class", "fits")
	fitsChild := document.CreateElement("div")
	fitsChild.SetAttribute("class", "small")
	fits.AppendChild(fitsChild)
	body.AppendChild(hidden)
	body.AppendChild(fits)
	engine = testEngine(document, `
		html, body { margin:0; padding:0 }
		.hidden { display:block; width:50px; height:20px; overflow:hidden; margin:0; padding:0; border:0 }
		.child { display:block; width:100px; height:40px; margin:0; padding:0; border:0 }
		.fits { display:block; width:50px; height:20px; overflow:auto; margin:0; padding:0; border:0 }
		.small { display:block; width:40px; height:10px; margin:0; padding:0; border:0 }
	`)
	root = model.BuildDocument(document, engine)
	layout = New(400, 600)
	layout.Layout(root)
	hiddenBox := findBox(root, hidden)
	if !hiddenBox.ClipX || !hiddenBox.ClipY {
		t.Error("overflow:hidden should clip")
	}
	if hiddenBox.HasHorizontalScrollbar || hiddenBox.HasVerticalScrollbar {
		t.Error("overflow:hidden should not create scrollbars")
	}
	if hiddenBox.MaxScrollLeft() != 50 || hiddenBox.MaxScrollTop() != 20 {
		t.Errorf("hidden max scroll = %v/%v, want 50/20", hiddenBox.MaxScrollLeft(), hiddenBox.MaxScrollTop())
	}
	fitsBox := findBox(root, fits)
	if fitsBox.HasHorizontalScrollbar || fitsBox.HasVerticalScrollbar {
		t.Error("overflow:auto without overflow should not reserve scrollbars")
	}
	if fitsBox.MaxScrollLeft() != 0 || fitsBox.MaxScrollTop() != 0 {
		t.Errorf("fits max scroll = %v/%v, want 0/0", fitsBox.MaxScrollLeft(), fitsBox.MaxScrollTop())
	}
}

func TestInlineBidiOrdering(t *testing.T) {
	document, body := testDocument()
	ltr := document.CreateElement("p")
	ltr.SetAttribute("class", "ltr")
	ltr.AppendChild(document.CreateTextNode("hello world"))
	rtl := document.CreateElement("p")
	rtl.SetAttribute("class", "rtl")
	rtl.AppendChild(document.CreateTextNode("שלום עולם"))
	body.AppendChild(ltr)
	body.AppendChild(rtl)
	engine := testEngine(document, `
		html, body { margin:0; padding:0 }
		p { display:block; width:400px; font-size:16px; line-height:19.2px; margin:0; padding:0; border:0 }
		.ltr { direction:ltr }
		.rtl { direction:rtl }
	`)
	root := model.BuildDocument(document, engine)
	layout := New(800, 600)
	layout.Layout(root)
	ltrBox := findBox(root, ltr)
	if len(ltrBox.Children) != 2 || ltrBox.Children[0].Text != "hello" || ltrBox.Children[1].Text != "world" {
		t.Errorf("ltr words = %#v", ltrBox.Children)
	}
	if ltrBox.Children[0].VisualText != "hello" {
		t.Errorf("ltr visual text = %q", ltrBox.Children[0].VisualText)
	}
	rtlBox := findBox(root, rtl)
	if len(rtlBox.Children) != 2 || rtlBox.Children[0].Text != "עולם" || rtlBox.Children[1].Text != "שלום" {
		t.Errorf("rtl words = %#v", rtlBox.Children)
	}
	if rtlBox.Children[0].VisualText != "םלוע" || rtlBox.Children[1].VisualText != "םולש" {
		t.Errorf("rtl visual text = %q/%q", rtlBox.Children[0].VisualText, rtlBox.Children[1].VisualText)
	}
}

func TestFontMetrics(t *testing.T) {
	document, body := testDocument()
	paragraph := document.CreateElement("p")
	body.AppendChild(paragraph)
	engine := testEngine(document, `
		html, body { margin:0; padding:0 }
		p { display:block; font-family: monospace; font-size:16px; line-height:19.2px; margin:0; padding:0; border:0 }
	`)
	styleMap := engine.ComputeDocumentTree(document)
	metrics := model.MetricsForStyle(styleMap[paragraph])
	if metrics.Ascent != 12.8 || metrics.Descent != 3.2 || metrics.AverageAdvance != 9.6 {
		t.Errorf("metrics = %#v", metrics)
	}
	layout := New(400, 600)
	width, _ := layout.measureText("ab", styleMap[paragraph])
	if math.Abs(width-19.2) > 1e-9 {
		t.Errorf("monospace width = %v, want 19.2", width)
	}
}

func TestLogicalSizingAndVerticalMargins(t *testing.T) {
	document, body := testDocument()
	container := document.CreateElement("div")
	container.SetAttribute("class", "vertical")
	first := document.CreateElement("div")
	first.SetAttribute("class", "first")
	second := document.CreateElement("div")
	second.SetAttribute("class", "second")
	container.AppendChild(first)
	container.AppendChild(second)
	body.AppendChild(container)
	engine := testEngine(document, `
		html, body { margin:0; padding:0 }
		.vertical { display:block; writing-mode:vertical-rl; inline-size:40px; block-size:60px; margin:0; padding:0; border:0 }
		.first { width:20px; height:10px; margin:0; margin-block-start:10px; padding:0; border:0 }
		.second { width:20px; height:10px; margin:0; padding:0; border:0 }
	`)
	root := model.BuildDocument(document, engine)
	layout := New(400, 600)
	layout.Layout(root)
	containerBox := findBox(root, container)
	if containerBox.Width != 60 || containerBox.Height != 40 {
		t.Errorf("logical size = %vx%v, want 60x40", containerBox.Width, containerBox.Height)
	}
	if got := findBox(root, first).X; got != 30 {
		t.Errorf("vertical margin-block-start x = %v, want 30", got)
	}
	if got := findBox(root, second).X; got != 10 {
		t.Errorf("second vertical block x = %v, want 10", got)
	}
}

func TestFontFamilyMetricsAndKerning(t *testing.T) {
	if got := model.MetricsForFamily("serif", 16).AverageAdvance; math.Abs(got-7.68) > 1e-9 {
		t.Errorf("serif advance = %v, want 7.68", got)
	}
	if got := model.MetricsForFamily("monospace", 16).AverageAdvance; got != 9.6 {
		t.Errorf("monospace advance = %v, want 9.6", got)
	}
	document, body := testDocument()
	kerned := document.CreateElement("p")
	kerned.SetAttribute("class", "kerned")
	plain := document.CreateElement("p")
	plain.SetAttribute("class", "plain")
	body.AppendChild(kerned)
	body.AppendChild(plain)
	engine := testEngine(document, `
		html, body { margin:0; padding:0 }
		p { display:block; font-family:sans-serif; font-size:16px; line-height:19.2px; margin:0; padding:0; border:0 }
		.kerned { font-kerning:normal }
		.plain { font-kerning:none }
	`)
	styleMap := engine.ComputeDocumentTree(document)
	layout := New(400, 600)
	kernedWidth, _ := layout.measureText("AV", styleMap[kerned])
	plainWidth, _ := layout.measureText("AV", styleMap[plain])
	if math.Abs(kernedWidth-15.2) > 1e-9 {
		t.Errorf("kerned width = %v, want 15.2", kernedWidth)
	}
	if plainWidth != 16 {
		t.Errorf("plain width = %v, want 16", plainWidth)
	}
}

func TestVerticalWritingModeBlocks(t *testing.T) {
	document, body := testDocument()
	container := document.CreateElement("div")
	container.SetAttribute("class", "vertical-rl")
	first := document.CreateElement("div")
	first.SetAttribute("class", "a")
	second := document.CreateElement("div")
	second.SetAttribute("class", "b")
	container.AppendChild(first)
	container.AppendChild(second)
	body.AppendChild(container)
	engine := testEngine(document, `
		html, body { margin:0; padding:0 }
		.vertical-rl { display:block; writing-mode:vertical-rl; width:60px; height:40px; margin:0; padding:0; border:0 }
		.a { display:block; width:20px; height:10px; margin:0; padding:0; border:0 }
		.b { display:block; width:30px; height:10px; margin:0; padding:0; border:0 }
	`)
	root := model.BuildDocument(document, engine)
	layout := New(400, 600)
	layout.Layout(root)
	if got := findBox(root, first).X; got != 40 {
		t.Errorf("vertical-rl first x = %v, want 40", got)
	}
	if got := findBox(root, second).X; got != 10 {
		t.Errorf("vertical-rl second x = %v, want 10", got)
	}

	document, body = testDocument()
	container = document.CreateElement("div")
	container.SetAttribute("class", "vertical-lr")
	first = document.CreateElement("div")
	first.SetAttribute("class", "a")
	second = document.CreateElement("div")
	second.SetAttribute("class", "b")
	container.AppendChild(first)
	container.AppendChild(second)
	body.AppendChild(container)
	engine = testEngine(document, `
		html, body { margin:0; padding:0 }
		.vertical-lr { display:block; writing-mode:vertical-lr; width:60px; height:40px; margin:0; padding:0; border:0 }
		.a { display:block; width:20px; height:10px; margin:0; padding:0; border:0 }
		.b { display:block; width:30px; height:10px; margin:0; padding:0; border:0 }
	`)
	root = model.BuildDocument(document, engine)
	layout = New(400, 600)
	layout.Layout(root)
	if got := findBox(root, first).X; got != 0 {
		t.Errorf("vertical-lr first x = %v, want 0", got)
	}
	if got := findBox(root, second).X; got != 20 {
		t.Errorf("vertical-lr second x = %v, want 20", got)
	}
}

func TestVerticalInlineText(t *testing.T) {
	document, body := testDocument()
	paragraph := document.CreateElement("p")
	paragraph.SetAttribute("class", "vertical")
	paragraph.AppendChild(document.CreateTextNode("ab cd"))
	body.AppendChild(paragraph)
	engine := testEngine(document, `
		html, body { margin:0; padding:0 }
		p.vertical { display:block; writing-mode:vertical-rl; width:40px; height:20px; font-size:16px; line-height:19.2px; margin:0; padding:0; border:0 }
	`)
	root := model.BuildDocument(document, engine)
	layout := New(400, 600)
	layout.Layout(root)
	p := findBox(root, paragraph)
	if len(p.Children) != 2 {
		t.Fatalf("vertical words = %d, want 2", len(p.Children))
	}
	if math.Abs(p.Children[0].X-20.8) > 1e-9 || p.Children[0].Y != 0 {
		t.Errorf("first vertical word = (%v,%v), want (20.8,0)", p.Children[0].X, p.Children[0].Y)
	}
	if math.Abs(p.Children[1].X-1.6) > 1e-9 || p.Children[1].Y != 0 {
		t.Errorf("second vertical word = (%v,%v), want (1.6,0)", p.Children[1].X, p.Children[1].Y)
	}
}

func TestStickyOffset(t *testing.T) {
	document, body := testDocument()
	scroller := document.CreateElement("div")
	scroller.SetAttribute("class", "scroller")
	wrapper := document.CreateElement("div")
	wrapper.SetAttribute("class", "wrapper")
	sticky := document.CreateElement("div")
	sticky.SetAttribute("class", "sticky")
	wrapper.AppendChild(sticky)
	scroller.AppendChild(wrapper)
	body.AppendChild(scroller)
	engine := testEngine(document, `
		html, body { margin:0; padding:0 }
		.scroller { display:block; width:100px; height:100px; overflow:auto; margin:0; padding:0; border:0 }
		.wrapper { display:block; height:200px; margin:0; padding:0; border:0 }
		.sticky { position:sticky; top:0; left:0; width:20px; height:20px; margin:0; padding:0; border:0 }
	`)
	root := model.BuildDocument(document, engine)
	layout := New(400, 600)
	layout.Layout(root)
	stickyBox := findBox(root, sticky)
	x, y := stickyBox.StickyOffset(0, 50)
	if x != 0 || y != 50 {
		t.Errorf("sticky at scroll 50 = (%v,%v), want (0,50)", x, y)
	}
	_, y = stickyBox.StickyOffset(0, 190)
	if y != 180 {
		t.Errorf("sticky clamped y = %v, want 180", y)
	}
}

func TestTextRotationMetadata(t *testing.T) {
	document, body := testDocument()
	for _, mode := range []string{"horizontal-tb", "vertical-rl", "vertical-lr", "sideways-rl", "sideways-lr"} {
		element := document.CreateElement("div")
		element.SetAttribute("class", "rot-"+mode)
		body.AppendChild(element)
	}
	engine := testEngine(document, `
		html, body { margin:0; padding:0 }
		.rot-horizontal-tb { writing-mode:horizontal-tb; display:block; height:10px }
		.rot-vertical-rl { writing-mode:vertical-rl; display:block; width:10px }
		.rot-vertical-lr { writing-mode:vertical-lr; display:block; width:10px }
		.rot-sideways-rl { writing-mode:sideways-rl; display:block; width:10px }
		.rot-sideways-lr { writing-mode:sideways-lr; display:block; width:10px }
	`)
	root := model.BuildDocument(document, engine)
	want := map[string]float64{
		"horizontal-tb": 0, "vertical-rl": 90, "vertical-lr": -90,
		"sideways-rl": 90, "sideways-lr": -90,
	}
	for mode, rotation := range want {
		for _, child := range root.Children {
			for _, box := range child.Children {
				if box.Node != nil && box.Node.HasClass("rot-"+mode) {
					if box.TextRotation != rotation {
						t.Errorf("%s rotation = %v, want %v", mode, box.TextRotation, rotation)
					}
				}
			}
		}
	}
}

func TestReplacedInlineSizing(t *testing.T) {
	document, body := testDocument()
	image := document.CreateElement("img")
	image.SetAttribute("class", "image")
	image.SetAttribute("width", "50")
	image.SetAttribute("height", "25")
	body.AppendChild(image)
	engine := testEngine(document, `
		html, body { margin:0; padding:0 }
	`)
	root := model.BuildDocument(document, engine)
	layout := New(400, 600)
	layout.Layout(root)
	imageBox := findBox(root, image)
	if imageBox == nil || imageBox.Width != 50 || imageBox.Height != 25 {
		t.Errorf("replaced box = %#v", imageBox)
	}
}

func TestBidiLineOrdering(t *testing.T) {
	document, body := testDocument()
	paragraph := document.CreateElement("p")
	paragraph.SetAttribute("class", "rtl")
	paragraph.AppendChild(document.CreateTextNode("\u05d0\u05d1 \u05d2\u05d3 \u05d4\u05d5"))
	body.AppendChild(paragraph)
	engine := testEngine(document, `
		html, body { margin:0; padding:0 }
		p { display:block; width:40px; direction:rtl; font-size:16px; line-height:19.2px; margin:0; padding:0; border:0 }
	`)
	root := model.BuildDocument(document, engine)
	layout := New(400, 600)
	layout.Layout(root)
	words := findBox(root, paragraph).Children
	if len(words) != 3 {
		t.Fatalf("bidi words = %#v", words)
	}
	if words[0].Text != "\u05d2\u05d3" || words[1].Text != "\u05d0\u05d1" || words[2].Text != "\u05d4\u05d5" {
		t.Errorf("bidi line order = %q/%q/%q", words[0].Text, words[1].Text, words[2].Text)
	}
	if words[0].VisualText != "\u05d3\u05d2" || words[1].VisualText != "\u05d1\u05d0" {
		t.Errorf("bidi visual order = %q/%q", words[0].VisualText, words[1].VisualText)
	}
}

func TestMixedBidiRuns(t *testing.T) {
	document, body := testDocument()
	paragraph := document.CreateElement("p")
	paragraph.SetAttribute("class", "mixed")
	paragraph.AppendChild(document.CreateTextNode("abc אב"))
	body.AppendChild(paragraph)
	engine := testEngine(document, `
		html, body { margin:0; padding:0 }
		p.mixed { display:block; width:400px; direction:ltr; font-size:16px; line-height:19.2px; margin:0; padding:0; border:0 }
	`)
	root := model.BuildDocument(document, engine)
	layout := New(800, 600)
	layout.Layout(root)
	p := findBox(root, paragraph)
	if len(p.Children) != 2 {
		t.Fatalf("mixed words = %#v", p.Children)
	}
	if p.Children[0].Text != "abc" || p.Children[0].VisualText != "abc" {
		t.Errorf("ltr run = %#v", p.Children[0])
	}
	if p.Children[1].Text != "אב" || p.Children[1].VisualText != "בא" {
		t.Errorf("rtl run = %#v", p.Children[1])
	}
}

func TestReplacedIntrinsicSizingUsesProvisionalSize(t *testing.T) {
	document, body := testDocument()
	imageNode := document.CreateElement("img")
	body.AppendChild(imageNode)
	engine := testEngine(document, `
		html, body { margin:0; padding:0 }
		img { display:inline-block }
	`)
	root := model.BuildDocument(document, engine)
	layout := New(400, 600)
	layout.IntrinsicSize = func(box *model.Box) (float64, float64, bool, bool) {
		if box.Node == imageNode {
			return 0, 0, false, true
		}
		return 0, 0, false, false
	}
	layout.Layout(root)
	imageBox := findBox(root, imageNode)
	if imageBox == nil {
		t.Fatal("image box missing")
	}
	if imageBox.Width != 300 || imageBox.Height != 150 || !imageBox.Provisional {
		t.Errorf("provisional box = %vx%v provisional=%v", imageBox.Width, imageBox.Height, imageBox.Provisional)
	}

	layout.IntrinsicSize = func(box *model.Box) (float64, float64, bool, bool) {
		if box.Node == imageNode {
			return 50, 25, true, false
		}
		return 0, 0, false, false
	}
	layout.Layout(root)
	imageBox = findBox(root, imageNode)
	if imageBox.Width != 50 || imageBox.Height != 25 || imageBox.Provisional {
		t.Errorf("loaded box = %vx%v provisional=%v", imageBox.Width, imageBox.Height, imageBox.Provisional)
	}
}
