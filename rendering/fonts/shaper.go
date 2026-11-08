package fonts

import (
	"gonetsurf/layouting/model"
	"strings"

	"golang.org/x/image/font"
)

// Shaper implements model.TextShaper with real font advances and kerning.
type Shaper struct {
	Library *Library
}

// NewShaper returns a shaper backed by library.
func NewShaper(library *Library) *Shaper {
	if library == nil {
		library = Default()
	}
	return &Shaper{Library: library}
}

// Shape implements model.TextShaper.
func (shaper *Shaper) Shape(run model.TextRun) []model.Glyph {
	library := shaper.Library
	if library == nil {
		library = Default()
	}
	spec := Spec{
		Family:  run.FontFamily,
		Size:    run.FontSize,
		Weight:  parseFontWeight(run.FontWeight),
		Italic:  strings.EqualFold(strings.TrimSpace(run.FontStyle), "italic"),
		Oblique: strings.EqualFold(strings.TrimSpace(run.FontStyle), "oblique"),
	}
	if spec.Size <= 0 {
		spec.Size = 16
	}
	characters := []rune(run.Text)
	glyphs := make([]model.Glyph, 0, len(characters))
	faces := library.Faces(spec)
	var previousFace font.Face
	var previousCharacter rune = -1
	for _, character := range characters {
		face := FaceForRuneChain(faces, character)
		advance := run.AverageAdvance
		if face != nil {
			if measured, ok := face.GlyphAdvance(character); ok {
				advance = float64(measured) / 64
			}
			if previousCharacter >= 0 && previousFace == face {
				kern := face.Kern(previousCharacter, character)
				advance += float64(kern) / 64
			}
		}
		advance += run.LetterSpacing
		if character == ' ' {
			advance += run.WordSpacing
		}
		glyphs = append(glyphs, model.Glyph{Text: string(character), Advance: advance})
		previousFace = face
		previousCharacter = character
	}
	return glyphs
}
