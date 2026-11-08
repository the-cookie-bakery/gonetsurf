package model

import (
	"strings"

	"gonetsurf/styles"

	"golang.org/x/text/unicode/bidi"
)

// TextRun is one shaped text run.
type TextRun struct {
	Text           string
	FontFamily     string
	FontWeight     string
	FontStyle      string
	FontStretch    string
	FontSize       float64
	LetterSpacing  float64
	WordSpacing    float64
	AverageAdvance float64
}

// FontMetrics describes the vertical metrics and average advance for a font.
type FontMetrics struct {
	Ascent         float64
	Descent        float64
	LineGap        float64
	AverageAdvance float64
}

// MetricsForStyle returns simple metrics for the computed font style.
func MetricsForStyle(style *styles.Style) FontMetrics {
	fontSize := 16.0
	family := ""
	if style != nil {
		if value, ok := ParseComputedPixels(style.GetString("font-size")); ok && value > 0 {
			fontSize = value
		}
		family = style.GetString("font-family")
	}
	return MetricsForFamily(family, fontSize)
}

// MetricsForFamily selects metrics by matching the first generic family in a
// font-family list. Actual font files and glyph coverage arrive in Phase 7.
func MetricsForFamily(fontFamily string, fontSize float64) FontMetrics {
	advanceFactor := 0.5
	ascentFactor := 0.8
	family := strings.ToLower(fontFamily)
	switch {
	case strings.Contains(family, "monospace"):
		advanceFactor = 0.6
		ascentFactor = 0.8
	case strings.Contains(family, "cursive"):
		advanceFactor = 0.55
		ascentFactor = 0.75
	case strings.Contains(family, "fantasy"):
		advanceFactor = 0.55
		ascentFactor = 0.78
	case strings.Contains(family, "system-ui"):
		advanceFactor = 0.5
	case strings.Contains(family, "sans-serif"):
		advanceFactor = 0.5
	case strings.Contains(family, "serif"):
		advanceFactor = 0.48
		ascentFactor = 0.78
	}
	return FontMetrics{
		Ascent:         fontSize * ascentFactor,
		Descent:        fontSize * 0.2,
		LineGap:        0,
		AverageAdvance: fontSize * advanceFactor,
	}
}

// Glyph is one shaped character advance.
type Glyph struct {
	Text    string
	Advance float64
}

// TextShaper shapes a text run into glyph advances. A full OpenType shaper
// implements the same interface in a later phase.
type TextShaper interface {
	Shape(run TextRun) []Glyph
}

// SimpleShaper uses a fixed average advance width and spacing properties.
type SimpleShaper struct{}

// Shape implements TextShaper.
func (SimpleShaper) Shape(run TextRun) []Glyph {
	glyphs := make([]Glyph, 0, len([]rune(run.Text)))
	baseAdvance := run.AverageAdvance
	if baseAdvance <= 0 {
		baseAdvance = run.FontSize * 0.5
	}
	for _, character := range run.Text {
		advance := baseAdvance + run.LetterSpacing
		if character == ' ' {
			advance += run.WordSpacing
		}
		glyphs = append(glyphs, Glyph{Text: string(character), Advance: advance})
	}
	return glyphs
}

// kerningAdjustments are conservative pair tightenings in em units. They model
// the most visible Latin pairs until real OpenType tables are loaded.
var kerningAdjustments = map[string]float64{
	"AV": -0.05, "VA": -0.05, "AT": -0.04, "TA": -0.04,
	"LT": -0.05, "To": -0.04, "Ta": -0.03, "Te": -0.03,
	"Wa": -0.04, "We": -0.03, "Yo": -0.04, "PA": -0.04,
	"F,": -0.04, "F.": -0.04, "r.": -0.03, "ry": -0.02,
}

func ApplyKerning(glyphs []Glyph, fontSize float64, fontFamily string) {
	if strings.Contains(strings.ToLower(fontFamily), "monospace") {
		return
	}
	for index := 0; index+1 < len(glyphs); index++ {
		pair := glyphs[index].Text + glyphs[index+1].Text
		if adjustment, ok := kerningAdjustments[pair]; ok {
			glyphs[index].Advance += adjustment * fontSize
		}
	}
}

// BidiRun is a directional run of text.
type BidiRun struct {
	Text      string
	Direction bidi.Direction
	Start     int
	End       int
}

// SegmentBidi splits text into directional runs. The ordering is the bidi
// visual ordering from x/text; line layout integration is a later refinement.
func SegmentBidi(text string, defaultDirection bidi.Direction) []BidiRun {
	var paragraph bidi.Paragraph
	if _, err := paragraph.SetString(text, bidi.DefaultDirection(defaultDirection)); err != nil {
		return []BidiRun{{Text: text, Direction: defaultDirection}}
	}
	ordering, err := paragraph.Order()
	if err != nil || ordering.NumRuns() == 0 {
		return []BidiRun{{Text: text, Direction: defaultDirection}}
	}
	runs := make([]BidiRun, 0, ordering.NumRuns())
	for index := 0; index < ordering.NumRuns(); index++ {
		run := ordering.Run(index)
		start, end := run.Pos()
		runs = append(runs, BidiRun{
			Text:      run.String(),
			Direction: run.Direction(),
			Start:     start,
			End:       end,
		})
	}
	return runs
}

type visualWord struct {
	Text   string
	Visual string
}

func VisualWords(text, direction string) []visualWord {
	defaultDirection := bidi.LeftToRight
	if strings.EqualFold(strings.TrimSpace(direction), "rtl") {
		defaultDirection = bidi.RightToLeft
	}
	runs := SegmentBidi(text, defaultDirection)
	words := []visualWord{}
	for _, run := range runs {
		runWords := strings.Fields(run.Text)
		if run.Direction == bidi.RightToLeft {
			for left, right := 0, len(runWords)-1; left < right; left, right = left+1, right-1 {
				runWords[left], runWords[right] = runWords[right], runWords[left]
			}
			for _, word := range runWords {
				words = append(words, visualWord{Text: word, Visual: reverseRunes(word)})
			}
			continue
		}
		for _, word := range runWords {
			words = append(words, visualWord{Text: word, Visual: word})
		}
	}
	return words
}

func reverseRunes(text string) string {
	runes := []rune(text)
	for left, right := 0, len(runes)-1; left < right; left, right = left+1, right-1 {
		runes[left], runes[right] = runes[right], runes[left]
	}
	return string(runes)
}

// ShapeText shapes a text run with shaper and the computed font style.
func ShapeText(shaper TextShaper, text string, style *styles.Style) []Glyph {
	fontSize, _ := FontMetricsForStyle(style)
	if fontSize <= 0 {
		fontSize = 16
	}
	if shaper == nil {
		shaper = SimpleShaper{}
	}
	letterSpacing := 0.0
	wordSpacing := 0.0
	fontFamily := ""
	fontWeight := ""
	fontStyle := ""
	fontStretch := ""
	if style != nil {
		if value, ok := ParseComputedPixels(style.GetString("letter-spacing")); ok {
			letterSpacing = value
		}
		if value, ok := ParseComputedPixels(style.GetString("word-spacing")); ok {
			wordSpacing = value
		}
		fontFamily = style.GetString("font-family")
		fontWeight = style.GetString("font-weight")
		fontStyle = style.GetString("font-style")
		fontStretch = style.GetString("font-stretch")
	}
	metrics := MetricsForStyle(style)
	glyphs := shaper.Shape(TextRun{
		Text:           text,
		FontFamily:     fontFamily,
		FontWeight:     fontWeight,
		FontStyle:      fontStyle,
		FontStretch:    fontStretch,
		FontSize:       fontSize,
		LetterSpacing:  letterSpacing,
		WordSpacing:    wordSpacing,
		AverageAdvance: metrics.AverageAdvance,
	})
	if _, simple := shaper.(SimpleShaper); simple && style != nil {
		if kerning := strings.ToLower(strings.TrimSpace(style.GetString("font-kerning"))); kerning != "none" {
			ApplyKerning(glyphs, fontSize, style.GetString("font-family"))
		}
	}
	return glyphs
}
