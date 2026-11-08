package fonts

import (
	"encoding/base64"
	"fmt"
	"image"
	"math"
	"net/url"
	"strconv"
	"strings"

	"golang.org/x/image/font"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"

	"gonetsurf/parsers/css/cssom"
	"gonetsurf/parsers/css/parser"
	"gonetsurf/parsers/css/tokenizer"
	"gonetsurf/urls"
)

const maxFontBytes = 32 << 20

// Loader fetches a web-font resource. External fetching is supplied by the
// caller; data: URLs are handled internally.
type Loader func(resource string) ([]byte, error)

// AuthorOptions configures @font-face registration.
type AuthorOptions struct {
	Base      *urls.URL
	Partition string
	Loader    Loader
}

type fontSource struct {
	URL    string
	Local  string
	Format string
}

type runeRange struct {
	Start rune
	End   rune
}

func (rangeValue runeRange) contains(character rune) bool {
	return character >= rangeValue.Start && character <= rangeValue.End
}

// AddFontFaceRules registers author faces from parsed @font-face rules.
func (library *Library) AddFontFaceRules(rules []*cssom.FontFaceRule, options AuthorOptions) {
	if library == nil {
		return
	}
	for _, rule := range rules {
		if rule == nil || strings.TrimSpace(rule.Family) == "" {
			continue
		}
		minimumWeight, maximumWeight := parseWeightRange(rule.FontWeight)
		stretchMinimum, stretchMaximum := parseStretchRange(rule.FontStretch)
		entry := &fontEntry{
			family:            rule.Family,
			author:            true,
			minimumWeight:     minimumWeight,
			maximumWeight:     maximumWeight,
			weight:            (minimumWeight + maximumWeight) / 2,
			italic:            strings.Contains(strings.ToLower(rule.FontStyle), "italic") || strings.Contains(strings.ToLower(rule.FontStyle), "oblique"),
			stretchMin:        stretchMinimum,
			stretchMax:        stretchMaximum,
			sizeAdjust:        1,
			featureSettings:   rule.FeatureSettings,
			variationSettings: rule.VariationSettings,
			fontDisplay:       rule.FontDisplay,
			partition:         options.Partition,
			resourceCache:     library.resourceCache,
		}
		entry.descriptor = fmt.Sprintf("%s|%d-%d|%s|%.3f-%.3f|%s|%s|%s",
			rule.Family, minimumWeight, maximumWeight, rule.FontStyle,
			stretchMinimum, stretchMaximum, rule.FeatureSettings,
			rule.VariationSettings, rule.FontDisplay)
		display := strings.ToLower(strings.TrimSpace(rule.FontDisplay))
		entry.async = display == "swap" || display == "optional" || library.asyncAuthorFonts
		entry.onLoad = library.loadCallback
		if factor, ok := parsePercentageFactor(rule.SizeAdjust); ok {
			entry.sizeAdjust = factor
		}
		if value, ok := parsePercentageFactor(rule.AscentOverride); ok {
			entry.ascentOverride = &value
		}
		if value, ok := parsePercentageFactor(rule.DescentOverride); ok {
			entry.descentOverride = &value
		}
		if value, ok := parsePercentageFactor(rule.LineGapOverride); ok {
			entry.lineGapOverride = &value
		}
		for _, unicodeRange := range rule.UnicodeRange {
			if unicodeRange.End < unicodeRange.Start {
				continue
			}
			entry.unicodeRange = append(entry.unicodeRange, runeRange{
				Start: rune(unicodeRange.Start),
				End:   rune(unicodeRange.End),
			})
		}
		for _, source := range rule.Sources {
			entry.sources = append(entry.sources, fontSource{
				URL:    resolveFontURL(options.Base, source.URL),
				Local:  source.Local,
				Format: source.Format,
			})
		}
		entry.localFont = library.localFont
		entry.loadURL = options.Loader
		library.authorEntries = append(library.authorEntries, entry)
	}
	library.mutex.Lock()
	library.faces = make(map[faceKey]font.Face)
	library.mutex.Unlock()
	library.invalidateFamilyCache()
}

func resolveFontURL(base *urls.URL, address string) string {
	if address == "" {
		return ""
	}
	if base == nil {
		return address
	}
	resolved, err := urls.ParseReference(base, address)
	if err != nil {
		return address
	}
	return resolved.String()
}

func (entry *fontEntry) loadAuthorFont() (*sfnt.Font, error) {
	var lastError error
	for _, source := range entry.sources {
		if source.Local != "" {
			if entry.localFont == nil {
				continue
			}
			parsed, err := entry.localFont(source.Local)
			if err == nil {
				return parsed, nil
			}
			lastError = err
			continue
		}
		if source.URL == "" || strings.EqualFold(source.Format, "woff2") {
			continue
		}
		var data []byte
		var err error
		key := fontResourceKey{partition: entry.partition, url: source.URL, format: source.Format, descriptor: entry.descriptor}
		if cached, cacheErr, ok := entry.resourceCache.get(key); ok {
			data, err = cached, cacheErr
		} else if strings.HasPrefix(strings.ToLower(source.URL), "data:") {
			body, parseErr := decodeFontDataURL(source.URL)
			if parseErr != nil {
				lastError = parseErr
				continue
			}
			data = body
			entry.resourceCache.put(key, data, nil)
		} else {
			if entry.loadURL == nil {
				lastError = fmt.Errorf("fonts: no font loader configured")
				continue
			}
			data, err = entry.loadURL(source.URL)
			if err != nil {
				lastError = err
				entry.resourceCache.put(key, nil, err)
				continue
			}
			entry.resourceCache.put(key, data, nil)
		}
		if err != nil {
			lastError = err
			continue
		}
		if len(data) > maxFontBytes {
			lastError = fmt.Errorf("fonts: font exceeds the size limit")
			entry.resourceCache.put(key, nil, lastError)
			continue
		}
		parsed, err := parseFontData(data)
		if err == nil {
			return parsed, nil
		}
		lastError = err
	}
	if lastError == nil {
		lastError = fmt.Errorf("fonts: no @font-face source could be loaded")
	}
	return nil, lastError
}

func decodeFontDataURL(raw string) ([]byte, error) {
	comma := strings.IndexByte(raw, ',')
	if comma < 0 {
		return nil, fmt.Errorf("fonts: invalid data URL")
	}
	metadata := raw[:comma]
	payload := raw[comma+1:]
	if strings.Contains(strings.ToLower(metadata), ";base64") {
		return base64.StdEncoding.DecodeString(payload)
	}
	decoded, err := url.PathUnescape(payload)
	if err != nil {
		return nil, err
	}
	return []byte(decoded), nil
}

func (library *Library) localFont(name string) (*sfnt.Font, error) {
	target := normalizeFamily(name)
	for _, entry := range library.entries {
		if normalizeFamily(entry.family) == target {
			return entry.font()
		}
	}
	if library.loadSystemFonts {
		library.loadSystem()
		for _, entry := range library.system.entries {
			if normalizeFamily(entry.family) == target {
				return entry.font()
			}
		}
	}
	return nil, fmt.Errorf("fonts: local font %q not found", name)
}

func parseWeightRange(value string) (int, int) {
	values := parser.ParseComponentValueList(value)
	var weights []int
	for _, item := range values {
		if parser.IsWhitespace(item) {
			continue
		}
		token, ok := parser.AsToken(item)
		if !ok {
			continue
		}
		switch token.Type {
		case tokenizer.TokenNumber:
			weights = append(weights, int(math.Round(token.Number)))
		case tokenizer.TokenIdent:
			switch strings.ToLower(token.Value) {
			case "normal":
				weights = append(weights, 400)
			case "bold":
				weights = append(weights, 700)
			}
		}
	}
	if len(weights) == 0 {
		return 400, 400
	}
	minimum, maximum := weights[0], weights[0]
	for _, weight := range weights[1:] {
		if weight < minimum {
			minimum = weight
		}
		if weight > maximum {
			maximum = weight
		}
	}
	return minimum, maximum
}

func parseStretchRange(value string) (float64, float64) {
	values := parser.ParseComponentValueList(value)
	var stretches []float64
	for _, item := range values {
		if parser.IsWhitespace(item) {
			continue
		}
		token, ok := parser.AsToken(item)
		if !ok {
			continue
		}
		switch token.Type {
		case tokenizer.TokenPercentage:
			stretches = append(stretches, token.Number/100)
		case tokenizer.TokenIdent:
			if stretch, ok := stretchKeyword(token.Value); ok {
				stretches = append(stretches, stretch)
			}
		}
	}
	if len(stretches) == 0 {
		return 1, 1
	}
	minimum, maximum := stretches[0], stretches[0]
	for _, stretch := range stretches[1:] {
		if stretch < minimum {
			minimum = stretch
		}
		if stretch > maximum {
			maximum = stretch
		}
	}
	return minimum, maximum
}

func stretchKeyword(value string) (float64, bool) {
	switch strings.ToLower(value) {
	case "ultra-condensed":
		return 0.5, true
	case "extra-condensed":
		return 0.625, true
	case "condensed":
		return 0.75, true
	case "semi-condensed":
		return 0.875, true
	case "normal":
		return 1, true
	case "semi-expanded":
		return 1.125, true
	case "expanded":
		return 1.25, true
	case "extra-expanded":
		return 1.5, true
	case "ultra-expanded":
		return 2, true
	default:
		return 0, false
	}
}

func parsePercentageFactor(value string) (float64, bool) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || strings.EqualFold(trimmed, "normal") {
		return 0, false
	}
	values := parser.ParseComponentValueList(trimmed)
	if len(values) != 1 {
		return 0, false
	}
	token, ok := parser.AsToken(values[0])
	if !ok || token.Type != tokenizer.TokenPercentage {
		return 0, false
	}
	return token.Number / 100, true
}

type rangedFace struct {
	font.Face
	ranges []runeRange
}

func (face *rangedFace) contains(character rune) bool {
	if len(face.ranges) == 0 {
		return true
	}
	for _, rangeValue := range face.ranges {
		if rangeValue.contains(character) {
			return true
		}
	}
	return false
}

func (face *rangedFace) Glyph(dot fixed.Point26_6, character rune) (image.Rectangle, image.Image, image.Point, fixed.Int26_6, bool) {
	if !face.contains(character) {
		return image.Rectangle{}, nil, image.Point{}, 0, false
	}
	return face.Face.Glyph(dot, character)
}

func (face *rangedFace) GlyphBounds(character rune) (fixed.Rectangle26_6, fixed.Int26_6, bool) {
	if !face.contains(character) {
		return fixed.Rectangle26_6{}, 0, false
	}
	return face.Face.GlyphBounds(character)
}

func (face *rangedFace) GlyphAdvance(character rune) (fixed.Int26_6, bool) {
	if !face.contains(character) {
		return 0, false
	}
	return face.Face.GlyphAdvance(character)
}

func (face *rangedFace) Kern(first, second rune) fixed.Int26_6 {
	if !face.contains(first) || !face.contains(second) {
		return 0
	}
	return face.Face.Kern(first, second)
}

var _ = strconv.Atoi
