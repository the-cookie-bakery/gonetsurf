package rendering

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"gonetsurf/dom"
	"gonetsurf/internal/pixeltest"
	"gonetsurf/parsers/css/cssom"
	"gonetsurf/parsers/html"
	"gonetsurf/styles"
)

const renderFixture = `<!doctype html>
<html><head><style>
html, body { margin: 0; padding: 0; }
body { background: #ffffff; }
.card {
  margin: 12px;
  padding: 10px;
  width: 200px;
  border: 4px solid #336699;
  background-color: #f0d080;
  color: #202020;
}
</style></head>
<body><div class="card">Hello renderer</div></body></html>`

func TestRenderDocumentGolden(t *testing.T) {
	document, err := html.ParseString(renderFixture, html.ParseOptions{})
	if err != nil {
		t.Fatal(err)
	}
	engine := styles.NewEngine(document)
	document.Node.Descendants(func(node *dom.Node) bool {
		if node.Type == dom.ElementNode && node.Data == "style" {
			engine.AddStylesheet(cssom.Parse(node.TextContent()))
		}
		return true
	})
	rendered, renderErr := RenderDocument(document, engine, 320, 200)
	if renderErr != nil {
		t.Fatal(renderErr)
	}
	surface := asSoftwareSurface(t, rendered)
	path := filepath.Join("testdata", "golden", "page.png")

	if *updateGolden {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		file, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := png.Encode(file, surface.Image()); err != nil {
			file.Close()
			t.Fatal(err)
		}
		if err := file.Close(); err != nil {
			t.Fatal(err)
		}
		return
	}

	result, err := pixeltest.CompareFile(path, surface.Image(), pixeltest.Options{MaxDifferent: 0})
	if err != nil {
		t.Fatalf("compare golden: %v (run go test ./rendering -run TestRenderDocumentGolden -update)", err)
	}
	if !result.OK(pixeltest.Options{MaxDifferent: 0}) {
		t.Fatalf("render golden mismatch: %s", result)
	}
}

func TestRenderDocumentPixels(t *testing.T) {
	document, err := html.ParseString(renderFixture, html.ParseOptions{})
	if err != nil {
		t.Fatal(err)
	}
	engine := styles.NewEngine(document)
	document.Node.Descendants(func(node *dom.Node) bool {
		if node.Type == dom.ElementNode && node.Data == "style" {
			engine.AddStylesheet(cssom.Parse(node.TextContent()))
		}
		return true
	})
	rendered, renderErr := RenderDocument(document, engine, 320, 200)
	if renderErr != nil {
		t.Fatal(renderErr)
	}
	surface := asSoftwareSurface(t, rendered)

	background := surface.Pix[3]
	if background != 255 {
		t.Errorf("canvas alpha = %d, want 255", background)
	}
	corner := pixelAt(surface, 0, 0)
	if corner.Red != 255 || corner.Green != 255 || corner.Blue != 255 {
		t.Errorf("canvas corner = %#v, want white", corner)
	}
	border := pixelAt(surface, 12, 12)
	if border.Red != 51 || border.Green != 102 || border.Blue != 153 {
		t.Errorf("border pixel = %#v, want #336699", border)
	}
}

func pixelAt(surface *Surface, xPos, yPos int) Color {
	offset := yPos*surface.Stride + xPos*4
	return Color{
		Red:   surface.Pix[offset],
		Green: surface.Pix[offset+1],
		Blue:  surface.Pix[offset+2],
		Alpha: surface.Pix[offset+3],
	}
}

func compareGolden(t *testing.T, path string, surface *Surface) {
	t.Helper()
	result, err := pixeltest.CompareFile(path, surface.Image(), pixeltest.Options{MaxDifferent: 0})
	if err != nil {
		t.Fatalf("compare golden: %v (run go test ./rendering -update)", err)
	}
	if !result.OK(pixeltest.Options{MaxDifferent: 0}) {
		t.Fatalf("render golden mismatch: %s", result)
	}
}
