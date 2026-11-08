package rendering

import (
	"encoding/base64"
	"testing"

	"gonetsurf/rendering/software"
)

func TestSVGShapesAndViewBox(t *testing.T) {
	source := `<svg xmlns="http://www.w3.org/2000/svg" width="40" height="20" viewBox="0 0 20 10">
<rect x="0" y="0" width="10" height="10" fill="#ff0000"/>
<circle cx="15" cy="5" r="4" fill="#0000ff"/>
</svg>`
	image, err := software.DecodeSVG([]byte(source), software.SVGOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if image.Bounds().Dx() != 40 || image.Bounds().Dy() != 20 {
		t.Fatalf("bounds = %v", image.Bounds())
	}
	red := image.At(5, 5)
	redR, _, _, _ := red.RGBA()
	if redR>>8 < 200 {
		t.Errorf("rect pixel = %#v", red)
	}
	blue := image.At(30, 10)
	_, _, blueB, _ := blue.RGBA()
	if blueB>>8 < 200 {
		t.Errorf("circle pixel = %#v", blue)
	}
}

func TestSVGPathTransform(t *testing.T) {
	source := `<svg xmlns="http://www.w3.org/2000/svg" width="40" height="40" viewBox="0 0 40 40">
<path d="M0 0 L10 0 L10 10 Z" fill="#00aa00" transform="translate(5 5)"/>
</svg>`
	image, err := software.DecodeSVG([]byte(source), software.SVGOptions{})
	if err != nil {
		t.Fatal(err)
	}
	pixel := image.At(10, 8)
	_, green, _, _ := pixel.RGBA()
	if green>>8 < 150 {
		t.Errorf("path pixel = %#v", pixel)
	}
	outsideR, outsideG, outsideB, outsideA := image.At(2, 2).RGBA()
	if outsideA>>8 == 0 {
		// Transparent is expected outside the path.
		return
	}
	if outsideR>>8 == 0 && outsideG>>8 == 0 && outsideB>>8 == 0 {
		t.Errorf("outside path is black")
	}
}

func TestSVGStroke(t *testing.T) {
	source := `<svg xmlns="http://www.w3.org/2000/svg" width="30" height="20">
<line x1="0" y1="10" x2="30" y2="10" stroke="#000000" stroke-width="6"/>
</svg>`
	image, err := software.DecodeSVG([]byte(source), software.SVGOptions{})
	if err != nil {
		t.Fatal(err)
	}
	red, green, blue, alpha := image.At(15, 10).RGBA()
	if alpha>>8 < 200 || red>>8 > 20 || green>>8 > 20 || blue>>8 > 20 {
		t.Errorf("stroke pixel = (%d,%d,%d,%d)", red>>8, green>>8, blue>>8, alpha>>8)
	}
}

func TestSVGLinearGradient(t *testing.T) {
	source := `<svg xmlns="http://www.w3.org/2000/svg" width="40" height="10">
<defs><linearGradient id="grad"><stop offset="0%" stop-color="#ff0000"/><stop offset="100%" stop-color="#0000ff"/></linearGradient></defs>
<rect x="0" y="0" width="40" height="10" fill="url(#grad)"/>
</svg>`
	image, err := software.DecodeSVG([]byte(source), software.SVGOptions{})
	if err != nil {
		t.Fatal(err)
	}
	leftR, _, leftB, _ := image.At(2, 5).RGBA()
	rightR, _, rightB, _ := image.At(38, 5).RGBA()
	if leftR>>8 < 150 || leftB>>8 > 100 {
		t.Errorf("gradient left = (%d,%d)", leftR>>8, leftB>>8)
	}
	if rightB>>8 < 150 || rightR>>8 > 100 {
		t.Errorf("gradient right = (%d,%d)", rightR>>8, rightB>>8)
	}
}

func TestSVGIgnoresScriptAndForeignObject(t *testing.T) {
	source := `<svg xmlns="http://www.w3.org/2000/svg" width="20" height="20">
<script>alert(1)</script>
<foreignObject width="20" height="20"><div xmlns="http://www.w3.org/1999/xhtml">text</div></foreignObject>
<rect width="20" height="20" fill="#ff0000"/>
</svg>`
	image, err := software.DecodeSVG([]byte(source), software.SVGOptions{})
	if err != nil {
		t.Fatal(err)
	}
	red, green, blue, alpha := image.At(10, 10).RGBA()
	if red>>8 < 200 || green>>8 > 50 || blue>>8 > 50 || alpha>>8 < 200 {
		t.Errorf("SVG with script = (%d,%d,%d,%d)", red>>8, green>>8, blue>>8, alpha>>8)
	}
}

func TestSVGBudget(t *testing.T) {
	source := `<svg xmlns="http://www.w3.org/2000/svg" width="5000" height="5000"><rect width="5000" height="5000"/></svg>`
	if _, err := software.DecodeSVG([]byte(source), software.SVGOptions{MaxDimension: 1024}); err == nil {
		t.Fatal("expected a dimension budget error")
	}
	if _, err := software.DecodeSVG([]byte(source), software.SVGOptions{MaxPixels: 100}); err == nil {
		t.Fatal("expected a pixel budget error")
	}
}

func TestSVGImageProvider(t *testing.T) {
	source := `<svg xmlns="http://www.w3.org/2000/svg" width="10" height="10"><rect width="10" height="10" fill="#00cc00"/></svg>`
	dataURL := "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString([]byte(source))
	provider := URLImageProvider{}
	loaded, err := provider.LoadImage("url(" + dataURL + ")")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Bounds().Dx() != 10 || loaded.Bounds().Dy() != 10 {
		t.Errorf("bounds = %v", loaded.Bounds())
	}
}

func TestSVGReplacedImage(t *testing.T) {
	source := `<svg xmlns="http://www.w3.org/2000/svg" width="10" height="10"><rect width="10" height="10" fill="#00aa00"/></svg>`
	dataURL := "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString([]byte(source))
	page := `<!doctype html><html><head><style>html, body { margin:0 } img { display:block; width:20px; height:20px }</style></head><body><img src="` + dataURL + `"></body></html>`
	surface := renderTestPage(t, page, 40, 40)
	if pixel := pixelAt(surface, 10, 10); pixel.Green < 150 {
		t.Errorf("SVG img pixel = %#v", pixel)
	}
}
