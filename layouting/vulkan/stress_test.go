//go:build vulkan && cgo

package vulkan_test

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"gonetsurf/layouting/interfaces"
	"gonetsurf/layouting/model"
	"gonetsurf/layouting/software"
	"gonetsurf/layouting/vulkan"
	"gonetsurf/parsers/css/cssom"
	"gonetsurf/parsers/html"
	"gonetsurf/styles"
)

func stressOrSkip(t *testing.T) {
	t.Helper()
	if os.Getenv("GONETSURF_VULKAN_STRESS") == "" {
		t.Skip("set GONETSURF_VULKAN_STRESS=1 to run Vulkan stress tests")
	}
}

// TestVulkanDeepTree exercises layouts deeper than the old fixed 256-entry
// shader stacks.
func TestVulkanDeepTree(t *testing.T) {
	stressOrSkip(t)
	depth := 400
	var builder strings.Builder
	builder.WriteString(`<html><body style="margin:0;padding:0">`)
	for index := 0; index < depth; index++ {
		fmt.Fprintf(&builder, `<div id="d%d">`, index)
	}
	builder.WriteString(`<div id="leaf" style="width:10px;height:10px"></div>`)
	for index := 0; index < depth; index++ {
		builder.WriteString(`</div>`)
	}
	builder.WriteString(`</body></html>`)
	css := `html,body{margin:0;padding:0} div{width:auto;height:auto;padding:1px}`
	compareStressTrees(t, builder.String(), css, 400, 600)
}

// TestVulkanStressGridScratch forces a large grid and span workload through
// the auxiliary scratch buffers.
func TestVulkanStressGridScratch(t *testing.T) {
	stressOrSkip(t)
	var builder strings.Builder
	builder.WriteString(`<html><body style="margin:0;padding:0"><div id="grid">`)
	for index := 0; index < 20*20; index++ {
		fmt.Fprintf(&builder, `<div id="c%d" style="height:4px"></div>`, index)
	}
	builder.WriteString(`</div></body></html>`)
	css := `html,body{margin:0;padding:0} #grid{display:grid;width:400px;grid-template-columns:repeat(20,1fr);gap:1px} #c0{grid-column:span 5} #c7{grid-row:span 3}`
	compareStressTrees(t, builder.String(), css, 800, 1200)
}

func compareStressTrees(t *testing.T, source, css string, width, height float64) {
	t.Helper()
	document, err := html.ParseString(source, html.ParseOptions{})
	if err != nil {
		t.Fatal(err)
	}
	engine := styles.NewEngine(document)
	engine.AddStylesheet(cssom.Parse(css))
	referenceRoot := model.BuildDocument(document, engine)
	software.New(width, height).Layout(referenceRoot)

	vulkanRoot := model.BuildDocument(document, engine)
	backend, err := vulkan.NewBackend()
	if err != nil {
		t.Fatal(err)
	}
	flow, err := backend.NewEngine(interfaces.Options{ViewportWidth: width, ViewportHeight: height})
	if err != nil {
		t.Fatal(err)
	}
	defer flow.Close()
	if _, err := flow.Layout(vulkanRoot, interfaces.Request{}); err != nil {
		t.Fatal(err)
	}
	ids := []string{"leaf", "grid", "c0", "c7", "c399", "d0", "d399"}
	compared := 0
	for _, id := range ids {
		node := document.GetElementById(id)
		if node == nil {
			continue
		}
		expected := model.FindBox(referenceRoot, node)
		actual := model.FindBox(vulkanRoot, node)
		if expected == nil || actual == nil {
			t.Fatalf("#%s missing from a layout", id)
		}
		compareBoxesFull(t, expected, actual)
		compared++
	}
	if compared == 0 {
		t.Fatal("no stress nodes found")
	}
}
