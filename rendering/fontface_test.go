package rendering

import (
	"encoding/base64"
	"strings"
	"testing"

	"golang.org/x/image/font/gofont/goregular"
)

func TestAuthorFontFaceRendering(t *testing.T) {
	dataURL := "data:font/ttf;base64," + base64.StdEncoding.EncodeToString(goregular.TTF)
	source := `<!doctype html><html><head><style>
@font-face { font-family: "AuthorTest"; src: url(` + dataURL + `) format("truetype"); }
html, body { margin:0; padding:0 }
body { font-family: "AuthorTest"; font-size:20px; color:#000000 }
</style></head><body><p>Author face renders</p></body></html>`
	surface := renderTestPage(t, source, 240, 60)
	if !hasNonWhitePixel(asSoftwareSurface(t, surface)) {
		t.Fatal("author-font text did not paint")
	}
}

func TestAuthorFontFaceLoaderRendering(t *testing.T) {
	source := `<!doctype html><html><head><style>
@font-face { font-family: "LoaderFace"; src: url("https://example.com/author.ttf") format("truetype"); }
html, body { margin:0; padding:0 }
body { font-family: "LoaderFace"; font-size:20px; color:#000000 }
</style></head><body><p>Loaded face renders</p></body></html>`
	document := mustParseTestPage(t, source)
	engine := buildTestStyleEngine(t, document)
	options := Options{
		Images: DataURLImageProvider{},
		LoadFont: func(resource string) ([]byte, error) {
			if !strings.HasSuffix(resource, "/author.ttf") {
				t.Errorf("font resource = %q", resource)
			}
			return goregular.TTF, nil
		},
	}
	surface, renderErr := RenderDocumentWithOptions(document, engine, 240, 60, options)
	if renderErr != nil {
		t.Fatal(renderErr)
	}
	if !hasNonWhitePixel(asSoftwareSurface(t, surface)) {
		t.Fatal("loaded font text did not paint")
	}
}

func hasNonWhitePixel(surface *Surface) bool {
	for offset := 0; offset < len(surface.Pix); offset += 4 {
		if surface.Pix[offset] != 255 || surface.Pix[offset+1] != 255 || surface.Pix[offset+2] != 255 {
			return true
		}
	}
	return false
}
