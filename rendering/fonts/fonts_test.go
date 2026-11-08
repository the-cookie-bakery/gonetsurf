package fonts

import (
	"gonetsurf/layouting/model"
	"testing"
)

func TestEmbeddedFonts(t *testing.T) {
	library := NewLibrary()
	if len(library.entries) != 8 {
		t.Fatalf("embedded entries = %d, want 8", len(library.entries))
	}
	face := library.Face(Spec{Family: "Go", Size: 16, Weight: 400})
	if face == nil {
		t.Fatal("Go face is nil")
	}
	metrics := face.Metrics()
	if metrics.Ascent <= 0 || metrics.Height <= 0 {
		t.Errorf("metrics = %#v", metrics)
	}
	if _, advance, ok := face.GlyphBounds('A'); !ok || advance <= 0 {
		t.Errorf("glyph A = ok %v advance %v", ok, advance)
	}
}

func TestFamilyFallback(t *testing.T) {
	library := NewLibrary()
	face := library.Face(Spec{Family: "Definitely Not Installed", Size: 14, Weight: 400})
	if face == nil {
		t.Fatal("fallback face is nil")
	}
	faces := library.Faces(Spec{Family: "Definitely Not Installed", Size: 14, Weight: 400})
	if len(faces) == 0 {
		t.Fatal("fallback chain is empty")
	}
	if library.FaceForRune(Spec{Family: "Definitely Not Installed", Size: 14}, 'A') == nil {
		t.Fatal("fallback FaceForRune failed")
	}
}

func TestWeightAndStyleSelection(t *testing.T) {
	library := NewLibrary()
	regular := library.Face(Spec{Family: "Go", Size: 16, Weight: 400})
	bold := library.Face(Spec{Family: "Go", Size: 16, Weight: 700})
	italic := library.Face(Spec{Family: "Go", Size: 16, Weight: 400, Italic: true})
	if regular == nil || bold == nil || italic == nil {
		t.Fatal("missing faces")
	}
	if regular == bold {
		t.Error("regular and bold resolved to the same face")
	}
	if regular == italic {
		t.Error("regular and italic resolved to the same face")
	}
}

func TestMetricsForSpec(t *testing.T) {
	library := NewLibrary()
	metrics := library.MetricsForSpec(Spec{Family: "Go", Size: 16})
	if metrics.Ascent <= 0 || metrics.Descent <= 0 || metrics.AverageAdvance <= 0 {
		t.Errorf("metrics = %#v", metrics)
	}
}

func TestShaperAdvancesAndKerning(t *testing.T) {
	library := NewLibrary()
	shaper := NewShaper(library)
	glyphs := shaper.Shape(model.TextRun{Text: "AV", FontFamily: "Go", FontWeight: "bold", FontSize: 20})
	if len(glyphs) != 2 {
		t.Fatalf("glyphs = %d", len(glyphs))
	}
	if glyphs[0].Advance <= 0 || glyphs[1].Advance <= 0 {
		t.Errorf("advances = %#v", glyphs)
	}
	face := library.Face(Spec{Family: "Go", Weight: 700, Size: 20})
	firstAdvance, firstOK := face.GlyphAdvance('A')
	secondAdvance, secondOK := face.GlyphAdvance('V')
	if !firstOK || !secondOK {
		t.Fatal("missing advances")
	}
	expected := (float64(firstAdvance) + float64(secondAdvance) + float64(face.Kern('A', 'V'))) / 64
	total := glyphs[0].Advance + glyphs[1].Advance
	if total != expected {
		t.Errorf("shaped total = %v, want %v", total, expected)
	}
}

func TestGlyphCacheReusesMask(t *testing.T) {
	library := NewLibrary()
	face := library.Face(Spec{Family: "Go", Size: 16})
	cache := NewGlyphCache()
	first, ok := cache.Glyph(face, 'A')
	if !ok {
		t.Fatal("first glyph lookup failed")
	}
	second, ok := cache.Glyph(face, 'A')
	if !ok {
		t.Fatal("second glyph lookup failed")
	}
	if first.Mask != second.Mask {
		t.Error("glyph cache did not reuse the mask")
	}
	nonzero := false
	for _, alpha := range first.Mask.Pix {
		if alpha != 0 {
			nonzero = true
			break
		}
	}
	if !nonzero {
		t.Error("glyph mask is empty")
	}
}

func TestSpecParsingHelpers(t *testing.T) {
	if got := parseFontWeight("bold"); got != 700 {
		t.Errorf("bold weight = %d", got)
	}
	if got := parseFontWeight("350"); got != 350 {
		t.Errorf("numeric weight = %d", got)
	}
	if got := parseFontSize("12px", 16); got != 12 {
		t.Errorf("12px size = %v", got)
	}
	if got := parseFontSize("1.5rem", 16); got != 24 {
		t.Errorf("1.5rem size = %v", got)
	}
	families := splitFamilyList(`"Go", "Go Mono", monospace`)
	if len(families) != 3 || families[0] != "Go" || families[1] != "Go Mono" || families[2] != "monospace" {
		t.Errorf("families = %#v", families)
	}
}
