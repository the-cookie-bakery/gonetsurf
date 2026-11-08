package rendering

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"gonetsurf/dom"
	"gonetsurf/parsers/css/cssom"
	"gonetsurf/parsers/html"
	"gonetsurf/styles"
)

func renderTestPage(t *testing.T, source string, width, height int) *Surface {
	t.Helper()
	return renderTestPageWithProvider(t, source, width, height, DataURLImageProvider{})
}

func renderTestPageWithProvider(t *testing.T, source string, width, height int, provider ImageProvider) *Surface {
	t.Helper()
	document := mustParseTestPage(t, source)
	engine := buildTestStyleEngine(t, document)
	surface, err := RenderDocumentWithProvider(document, engine, width, height, provider)
	if err != nil {
		t.Fatal(err)
	}
	return asSoftwareSurface(t, surface)
}

func mustParseTestPage(t *testing.T, source string) *dom.Document {
	t.Helper()
	document, err := html.ParseString(source, html.ParseOptions{})
	if err != nil {
		t.Fatal(err)
	}
	return document
}

func buildTestStyleEngine(t *testing.T, document *dom.Document) *styles.Engine {
	t.Helper()
	engine := styles.NewEngine(document)
	document.Node.Descendants(func(node *dom.Node) bool {
		if node.Type == dom.ElementNode && node.Data == "style" {
			engine.AddStylesheet(cssom.Parse(node.TextContent()))
		}
		return true
	})
	return engine
}

func imageDataURL(t *testing.T, fill color.NRGBA, width, height int) string {
	t.Helper()
	source := image.NewNRGBA(image.Rect(0, 0, width, height))
	for yPos := 0; yPos < height; yPos++ {
		for xPos := 0; xPos < width; xPos++ {
			source.SetNRGBA(xPos, yPos, fill)
		}
	}
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, source); err != nil {
		t.Fatal(err)
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(buffer.Bytes())
}

func TestBackgroundImageRepeat(t *testing.T) {
	dataURL := imageDataURL(t, color.NRGBA{G: 180, A: 255}, 4, 4)
	source := `<!doctype html><html><head><style>
html, body { margin:0; padding:0 }
.box { width:20px; height:20px; background:url(` + dataURL + `) repeat 0 0 / 10px 10px #ffffff }
</style></head><body><div class="box"></div></body></html>`
	surface := renderTestPage(t, source, 40, 40)
	if pixel := pixelAt(surface, 5, 5); pixel.Green < 150 {
		t.Errorf("first tile = %#v", pixel)
	}
	if pixel := pixelAt(surface, 15, 5); pixel.Green < 150 {
		t.Errorf("repeated tile = %#v", pixel)
	}
	if pixel := pixelAt(surface, 25, 5); pixel.Red != 255 || pixel.Green != 255 || pixel.Blue != 255 {
		t.Errorf("outside box = %#v, want white", pixel)
	}
}

func TestBackgroundImageNoRepeatPosition(t *testing.T) {
	dataURL := imageDataURL(t, color.NRGBA{B: 200, A: 255}, 4, 4)
	source := `<!doctype html><html><head><style>
html, body { margin:0; padding:0 }
.box { width:20px; height:20px; background-color:#ffffff; background-image:url(` + dataURL + `); background-size:10px 10px; background-repeat:no-repeat; background-position:right bottom }
</style></head><body><div class="box"></div></body></html>`
	surface := renderTestPage(t, source, 40, 40)
	if pixel := pixelAt(surface, 15, 15); pixel.Blue < 150 {
		t.Errorf("positioned image = %#v", pixel)
	}
	if pixel := pixelAt(surface, 5, 5); pixel.Red != 255 || pixel.Green != 255 || pixel.Blue != 255 {
		t.Errorf("unexpected image at top-left = %#v, want white", pixel)
	}
}

func TestRoundedBorderAndBackground(t *testing.T) {
	source := `<!doctype html><html><head><style>
html, body { margin:0; padding:0 }
.box { box-sizing:border-box; width:40px; height:40px; border:10px solid #ff0000; border-radius:20px; background-color:#0000ff }
</style></head><body><div class="box"></div></body></html>`
	surface := renderTestPage(t, source, 60, 60)
	if pixel := pixelAt(surface, 1, 1); pixel.Red != 255 || pixel.Green != 255 || pixel.Blue != 255 {
		t.Errorf("rounded corner = %#v, want white", pixel)
	}
	if pixel := pixelAt(surface, 20, 20); pixel.Blue < 200 {
		t.Errorf("interior = %#v, want blue", pixel)
	}
	if pixel := pixelAt(surface, 20, 5); pixel.Red < 200 {
		t.Errorf("top border = %#v, want red", pixel)
	}
}

func TestBoxShadow(t *testing.T) {
	source := `<!doctype html><html><head><style>
html, body { margin:0; padding:0 }
.box { width:20px; height:20px; background-color:#ff0000; box-shadow:4px 4px 0 0 #0000ff }
</style></head><body><div class="box"></div></body></html>`
	surface := renderTestPage(t, source, 40, 40)
	if pixel := pixelAt(surface, 22, 22); pixel.Blue < 150 {
		t.Errorf("shadow pixel = %#v, want blue", pixel)
	}
	if pixel := pixelAt(surface, 2, 2); pixel.Red < 200 {
		t.Errorf("box pixel = %#v, want red", pixel)
	}
}

func TestDashedAndDoubleBorders(t *testing.T) {
	source := `<!doctype html><html><head><style>
html, body { margin:0; padding:0 }
.dashed { width:100px; height:20px; border-top:6px dashed #000000; background-color:#ffffff }
.double { width:100px; height:20px; border-top:9px double #000000; background-color:#ffffff }
</style></head><body><div class="dashed"></div><div class="double"></div></body></html>`
	surface := renderTestPage(t, source, 120, 60)
	if pixel := pixelAt(surface, 4, 3); pixel.Red > 20 {
		t.Errorf("dash pixel = %#v, want black", pixel)
	}
	if pixel := pixelAt(surface, 22, 3); pixel.Red < 200 {
		t.Errorf("dash gap = %#v, want white", pixel)
	}
	if pixel := pixelAt(surface, 40, 26); pixel.Red > 20 {
		t.Errorf("double outer band = %#v, want black", pixel)
	}
	if pixel := pixelAt(surface, 40, 30); pixel.Red < 200 {
		t.Errorf("double gap = %#v, want white", pixel)
	}
	if pixel := pixelAt(surface, 40, 33); pixel.Red > 20 {
		t.Errorf("double inner band = %#v, want black", pixel)
	}
}

func TestPaintFeaturesGolden(t *testing.T) {
	dataURL := imageDataURL(t, color.NRGBA{R: 120, G: 190, B: 90, A: 255}, 4, 4)
	source := `<!doctype html><html><head><style>
html, body { margin:0; padding:0; background-color:#ffffff }
.card { box-sizing:border-box; width:140px; height:90px; margin:10px; border:5px solid #336699; border-radius:16px;
        background-color:#f0d080; background-image:url(` + dataURL + `); background-size:12px 12px; background-repeat:repeat;
        box-shadow:6px 6px 8px 2px rgba(0,0,0,0.35) }
</style></head><body><div class="card"></div></body></html>`
	surface := renderTestPage(t, source, 200, 140)
	path := filepath.Join("testdata", "golden", "features.png")
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
	compareGolden(t, path, surface)
}

func borderImageDataURL(t *testing.T) string {
	t.Helper()
	source := image.NewNRGBA(image.Rect(0, 0, 3, 3))
	for yPos := 0; yPos < 3; yPos++ {
		for xPos := 0; xPos < 3; xPos++ {
			pixelColor := color.NRGBA{G: 200, A: 255}
			if (xPos == 0 || xPos == 2) && (yPos == 0 || yPos == 2) {
				pixelColor = color.NRGBA{R: 220, A: 255}
			} else if xPos == 1 && yPos == 1 {
				pixelColor = color.NRGBA{B: 220, A: 255}
			}
			source.SetNRGBA(xPos, yPos, pixelColor)
		}
	}
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, source); err != nil {
		t.Fatal(err)
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(buffer.Bytes())
}

func TestBorderImageSlices(t *testing.T) {
	dataURL := borderImageDataURL(t)
	source := `<!doctype html><html><head><style>
html, body { margin:0; padding:0; background:#ffffff }
.box { box-sizing:border-box; width:40px; height:40px; border:10px solid transparent;
       border-image-source:url(` + dataURL + `); border-image-slice:1 fill; border-image-width:10px; border-image-repeat:stretch }
</style></head><body><div class="box"></div></body></html>`
	surface := renderTestPage(t, source, 60, 60)
	if pixel := pixelAt(surface, 5, 5); pixel.Red < 150 || pixel.Green > 80 {
		t.Errorf("corner slice = %#v, want red", pixel)
	}
	if pixel := pixelAt(surface, 20, 4); pixel.Green < 150 {
		t.Errorf("top edge slice = %#v, want green", pixel)
	}
	if pixel := pixelAt(surface, 20, 20); pixel.Blue < 150 {
		t.Errorf("center fill = %#v, want blue", pixel)
	}
}

// Covers CSS Backgrounds and Borders 3 §4.3 and the equivalent of WPT
// css/css-backgrounds/border-radius-clip-001.html for replaced elements.
func TestRoundedReplacedContentClipping(t *testing.T) {
	source := `<!doctype html><html><head><style>
html, body { margin:0; padding:0 }
img { display:block; width:40px; height:40px; border-radius:50%; }
</style></head><body><img id="image"></body></html>`
	document := mustParseTestPage(t, source)
	image := document.GetElementById("image")
	image.SetAttribute("src", imageDataURL(t, color.NRGBA{R: 255, A: 255}, 40, 40))
	surface := renderTestPage(t, source, 40, 40)
	if pixel := pixelAt(surface, 1, 1); pixel != (Color{Red: 255, Green: 255, Blue: 255, Alpha: 255}) {
		t.Errorf("rounded image corner = %#v, want white", pixel)
	}
	if pixel := pixelAt(surface, 20, 20); pixel.Red < 200 {
		t.Errorf("rounded image center = %#v, want image red", pixel)
	}
}

func TestRoundedReplacedContentRespectsBorderAndPadding(t *testing.T) {
	source := `<!doctype html><html><head><style>
html, body { margin:0; padding:0 }
img { display:block; box-sizing:border-box; width:60px; height:60px; border:10px solid #0000ff; padding:5px; border-radius:30px; }
</style></head><body><img id="image"></body></html>`
	document := mustParseTestPage(t, source)
	image := document.GetElementById("image")
	image.SetAttribute("src", imageDataURL(t, color.NRGBA{R: 255, A: 255}, 60, 60))
	surface := renderTestPage(t, source, 60, 60)
	if pixel := pixelAt(surface, 16, 16); pixel.Red > 200 && pixel.Blue < 50 {
		t.Errorf("image leaked into border curve corner = %#v", pixel)
	}
	if pixel := pixelAt(surface, 5, 30); pixel.Blue < 200 || pixel.Red > 50 {
		t.Errorf("border pixel = %#v, want blue", pixel)
	}
	if pixel := pixelAt(surface, 30, 30); pixel.Red < 200 {
		t.Errorf("content center = %#v, want red", pixel)
	}
}

func TestRoundedOverflowClipsDescendantContent(t *testing.T) {
	source := `<!doctype html><html><head><style>
html, body { margin:0; padding:0 }
#box { display:block; box-sizing:border-box; width:60px; height:60px; border:10px solid #0000ff; border-radius:30px; overflow:hidden }
#child { display:block; width:40px; height:40px; background:#ff0000 }
</style></head><body><div id="box"><div id="child"></div></div></body></html>`
	surface := renderTestPage(t, source, 60, 60)
	if pixel := pixelAt(surface, 1, 1); pixel != (Color{Red: 255, Green: 255, Blue: 255, Alpha: 255}) {
		t.Errorf("rounded overflow corner = %#v, want white", pixel)
	}
	if pixel := pixelAt(surface, 11, 11); pixel.Blue < 150 {
		t.Errorf("rounded border arc = %#v, want blue", pixel)
	}
	if pixel := pixelAt(surface, 5, 30); pixel.Blue < 200 {
		t.Errorf("border pixel = %#v, want blue", pixel)
	}
	if pixel := pixelAt(surface, 30, 30); pixel.Red < 200 {
		t.Errorf("content center = %#v, want red", pixel)
	}
}
