package fonts

import (
	"gonetsurf/layouting/model"
	"testing"
)

func BenchmarkShapeSentence(b *testing.B) {
	library := NewLibrary()
	shaper := NewShaper(library)
	run := model.TextRun{
		Text:       "The quick brown fox jumps over the lazy dog.",
		FontFamily: "Go",
		FontSize:   16,
	}
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		_ = shaper.Shape(run)
	}
}

func BenchmarkGlyphCacheLookup(b *testing.B) {
	library := NewLibrary()
	face := library.Face(Spec{Family: "Go", Size: 16})
	cache := NewGlyphCache()
	cache.Glyph(face, 'A')
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		_, _ = cache.Glyph(face, 'A')
	}
}
