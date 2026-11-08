// Package fonts loads system and embedded fonts, matches CSS family/weight/
// style requests, shapes text with real glyph advances, and rasterizes glyphs
// through a cache.
package fonts

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/gobolditalic"
	"golang.org/x/image/font/gofont/goitalic"
	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/gofont/gomonobold"
	"golang.org/x/image/font/gofont/gomonobolditalic"
	"golang.org/x/image/font/gofont/gomonoitalic"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/font/sfnt"

	"gonetsurf/layouts"
	"gonetsurf/parsers/css/parser"
	"gonetsurf/parsers/css/tokenizer"
	"gonetsurf/styles"
)

// Spec describes a font request.
type Spec struct {
	Family  string
	Size    float64
	Weight  int
	Stretch float64
	Italic  bool
	Oblique bool
}

// SpecFromStyle derives a font request from a computed style.
func SpecFromStyle(style *styles.Style) Spec {
	spec := Spec{Family: "", Size: 16, Weight: 400}
	if style == nil {
		return spec
	}
	spec.Family = style.GetString("font-family")
	spec.Size = parseFontSize(style.GetString("font-size"), 16)
	spec.Weight = parseFontWeight(style.GetString("font-weight"))
	spec.Stretch = parseFontStretch(style.GetString("font-stretch"))
	fontStyle := strings.ToLower(strings.TrimSpace(style.GetString("font-style")))
	spec.Italic = fontStyle == "italic"
	spec.Oblique = fontStyle == "oblique"
	return spec
}

type fontEntry struct {
	family string
	weight int
	italic bool
	data   []byte
	path   string

	author            bool
	partition         string
	descriptor        string
	resourceCache     *fontResourceCache
	async             bool
	minimumWeight     int
	maximumWeight     int
	sources           []fontSource
	unicodeRange      []runeRange
	stretchMin        float64
	stretchMax        float64
	sizeAdjust        float64
	ascentOverride    *float64
	descentOverride   *float64
	lineGapOverride   *float64
	featureSettings   string
	variationSettings string
	fontDisplay       string
	localFont         func(string) (*sfnt.Font, error)
	loadURL           func(string) ([]byte, error)
	onLoad            func()

	mutex    sync.Mutex
	parsed   *sfnt.Font
	parseErr error
	loaded   bool
	loading  bool
	done     chan struct{}
}

var errFontNotReady = fmt.Errorf("fonts: font is not ready")

func (entry *fontEntry) font() (*sfnt.Font, error) {
	entry.mutex.Lock()
	if entry.loaded {
		parsed, parseErr := entry.parsed, entry.parseErr
		entry.mutex.Unlock()
		return parsed, parseErr
	}
	if entry.loading {
		if entry.async {
			entry.mutex.Unlock()
			return nil, errFontNotReady
		}
		done := entry.done
		entry.mutex.Unlock()
		<-done
		entry.mutex.Lock()
		parsed, parseErr := entry.parsed, entry.parseErr
		entry.mutex.Unlock()
		return parsed, parseErr
	}
	entry.loading = true
	done := make(chan struct{})
	entry.done = done
	async := entry.async
	entry.mutex.Unlock()

	load := func() {
		var parsed *sfnt.Font
		var parseErr error
		if entry.author {
			parsed, parseErr = entry.loadAuthorFont()
		} else {
			data := entry.data
			if data == nil && entry.path != "" {
				loaded, err := os.ReadFile(entry.path)
				if err != nil {
					parseErr = err
				} else {
					data = loaded
				}
			}
			if parseErr == nil && data == nil {
				parseErr = fmt.Errorf("fonts: font has no data")
			}
			if parseErr == nil {
				parsed, parseErr = parseFontData(data)
			}
		}
		entry.mutex.Lock()
		entry.parsed = parsed
		entry.parseErr = parseErr
		entry.loaded = true
		entry.loading = false
		entry.mutex.Unlock()
		close(done)
		if async {
			entry.mutex.Lock()
			callback := entry.onLoad
			entry.mutex.Unlock()
			if callback != nil {
				callback()
			}
		}
	}
	if async {
		go load()
		return nil, errFontNotReady
	}
	load()
	entry.mutex.Lock()
	parsed, parseErr := entry.parsed, entry.parseErr
	entry.mutex.Unlock()
	return parsed, parseErr
}

func parseFontData(data []byte) (*sfnt.Font, error) {
	if len(data) >= 4 && string(data[:4]) == "wOFF" {
		decoded, err := decodeWOFF(data)
		if err != nil {
			return nil, err
		}
		data = decoded
	}
	if len(data) >= 4 && string(data[:4]) == "wOF2" {
		return nil, fmt.Errorf("fonts: WOFF2 requires a Brotli decoder")
	}
	return opentype.Parse(data)
}

type faceKey struct {
	entry   *fontEntry
	sizeKey int
}

type systemState struct {
	once    sync.Once
	entries []*fontEntry
}

// Library is a font collection with face and glyph caches.
type Library struct {
	entries         []*fontEntry
	authorEntries   []*fontEntry
	loadSystemFonts bool
	system          *systemState
	resourceCache   *fontResourceCache

	mutex sync.Mutex
	faces map[faceKey]font.Face

	familyMutex   sync.RWMutex
	familyEntries map[string][]*fontEntry

	glyphCache *GlyphCache

	asyncAuthorFonts bool
	loadCallback     func()
}

// NewLibrary returns a library with the embedded Go fonts only.
var embeddedEntries = sync.OnceValue(func() []*fontEntry {
	embedded := []struct {
		data []byte
	}{
		{goregular.TTF}, {gobold.TTF}, {goitalic.TTF}, {gobolditalic.TTF},
		{gomono.TTF}, {gomonobold.TTF}, {gomonoitalic.TTF}, {gomonobolditalic.TTF},
	}
	entries := make([]*fontEntry, 0, len(embedded))
	for _, item := range embedded {
		if entry, err := describeFont(item.data); err == nil {
			entries = append(entries, entry)
		}
	}
	return entries
})

func NewLibrary() *Library {
	return &Library{
		entries:       append([]*fontEntry(nil), embeddedEntries()...),
		faces:         make(map[faceKey]font.Face),
		familyEntries: make(map[string][]*fontEntry),
		glyphCache:    NewGlyphCache(),
		system:        &systemState{},
		resourceCache: newFontResourceCache(),
	}
}

var defaultLibrary = sync.OnceValue(func() *Library {
	library := NewLibrary()
	library.loadSystemFonts = true
	return library
})

// Default returns the shared library with embedded and system fonts.
func Default() *Library {
	return defaultLibrary()
}

// SetAsyncAuthorFonts makes author fonts load in the background when they are
// first requested, so layout can proceed with a fallback face.
func (library *Library) SetAsyncAuthorFonts(enabled bool) {
	if library == nil {
		return
	}
	library.mutex.Lock()
	library.asyncAuthorFonts = enabled
	library.mutex.Unlock()
	if !enabled {
		return
	}
	for _, entry := range library.authorEntries {
		entry.mutex.Lock()
		if entry.author {
			entry.async = true
		}
		entry.mutex.Unlock()
	}
}

// SetLoadCallback installs a callback invoked after an asynchronous author
// font finishes loading.
func (library *Library) SetLoadCallback(callback func()) {
	if library == nil {
		return
	}
	library.mutex.Lock()
	library.loadCallback = callback
	library.mutex.Unlock()
	for _, entry := range library.authorEntries {
		entry.mutex.Lock()
		entry.onLoad = callback
		entry.mutex.Unlock()
	}
}

// Clone returns a library sharing embedded/system entries but with fresh face
// and glyph caches. Author @font-face entries are copied.
func (library *Library) Clone() *Library {
	if library == nil {
		return NewLibrary()
	}
	clone := &Library{
		entries:          library.entries,
		authorEntries:    append([]*fontEntry(nil), library.authorEntries...),
		loadSystemFonts:  library.loadSystemFonts,
		system:           library.system,
		resourceCache:    library.resourceCache,
		faces:            make(map[faceKey]font.Face),
		familyEntries:    make(map[string][]*fontEntry),
		glyphCache:       NewGlyphCache(),
		asyncAuthorFonts: library.asyncAuthorFonts,
		loadCallback:     library.loadCallback,
	}
	return clone
}

// GlyphCache returns the library's glyph cache.
func (library *Library) GlyphCache() *GlyphCache {
	return library.glyphCache
}

func describeFont(data []byte) (*fontEntry, error) {
	parsed, err := opentype.Parse(data)
	if err != nil {
		return nil, err
	}
	var buffer sfnt.Buffer
	family, _ := parsed.Name(&buffer, sfnt.NameIDFamily)
	subfamily, _ := parsed.Name(&buffer, sfnt.NameIDSubfamily)
	family = strings.TrimSpace(family)
	if family == "" {
		family = strings.TrimSpace(subfamily)
	}
	weight, italic := parseSubfamily(subfamily)
	return &fontEntry{family: family, weight: weight, italic: italic, data: data}, nil
}

func parseSubfamily(subfamily string) (int, bool) {
	lower := strings.ToLower(subfamily)
	weight := 400
	switch {
	case strings.Contains(lower, "thin"):
		weight = 100
	case strings.Contains(lower, "extralight") || strings.Contains(lower, "ultralight"):
		weight = 200
	case strings.Contains(lower, "light"):
		weight = 300
	case strings.Contains(lower, "medium"):
		weight = 500
	case strings.Contains(lower, "semibold") || strings.Contains(lower, "demibold"):
		weight = 600
	case strings.Contains(lower, "bold"):
		weight = 700
	case strings.Contains(lower, "extrabold") || strings.Contains(lower, "ultrabold"):
		weight = 800
	case strings.Contains(lower, "black") || strings.Contains(lower, "heavy"):
		weight = 900
	}
	italic := strings.Contains(lower, "italic") || strings.Contains(lower, "oblique")
	return weight, italic
}

func (library *Library) loadSystem() {
	library.system.once.Do(func() {
		defer library.invalidateFamilyCache()
		for _, directory := range systemFontDirectories() {
			filepath.WalkDir(directory, func(path string, entry os.DirEntry, err error) error {
				if err != nil || entry.IsDir() {
					return nil
				}
				extension := strings.ToLower(filepath.Ext(path))
				if extension != ".ttf" && extension != ".otf" {
					return nil
				}
				info, err := entry.Info()
				if err != nil || info.Size() > 20<<20 {
					return nil
				}
				data, err := os.ReadFile(path)
				if err != nil {
					return nil
				}
				described, err := describeFont(data)
				if err != nil {
					return nil
				}
				described.data = nil
				described.path = path
				library.system.entries = append(library.system.entries, described)
				return nil
			})
		}
	})
}

func systemFontDirectories() []string {
	home, _ := os.UserHomeDir()
	directories := []string{
		"/usr/share/fonts",
		"/usr/local/share/fonts",
		filepath.Join(home, ".fonts"),
		filepath.Join(home, ".local", "share", "fonts"),
	}
	if runtime.GOOS == "darwin" {
		directories = append(directories, "/System/Library/Fonts", "/Library/Fonts")
	}
	return directories
}

func (library *Library) entriesFor(family string) []*fontEntry {
	target := normalizeFamily(family)
	library.familyMutex.RLock()
	cached, ok := library.familyEntries[target]
	library.familyMutex.RUnlock()
	if ok {
		return cached
	}
	result := library.computeEntriesFor(target)
	library.familyMutex.Lock()
	if existing, exists := library.familyEntries[target]; exists {
		result = existing
	} else {
		library.familyEntries[target] = result
	}
	library.familyMutex.Unlock()
	return result
}

func (library *Library) computeEntriesFor(target string) []*fontEntry {
	var result []*fontEntry
	for _, entry := range library.authorEntries {
		if normalizeFamily(entry.family) == target {
			result = append(result, entry)
		}
	}
	if len(result) > 0 {
		return result
	}
	for _, entry := range library.entries {
		if normalizeFamily(entry.family) == target {
			result = append(result, entry)
		}
	}
	if len(result) > 0 || !library.loadSystemFonts {
		return result
	}
	library.loadSystem()
	for _, entry := range library.system.entries {
		if normalizeFamily(entry.family) == target {
			result = append(result, entry)
		}
	}
	return result
}

func (library *Library) invalidateFamilyCache() {
	library.familyMutex.Lock()
	library.familyEntries = make(map[string][]*fontEntry)
	library.familyMutex.Unlock()
}

func normalizeFamily(family string) string {
	return strings.ToLower(strings.Trim(strings.TrimSpace(family), `"'`))
}

func genericFamily(family string) string {
	switch normalizeFamily(family) {
	case "monospace":
		return "Go Mono"
	case "sans-serif", "serif", "system-ui", "cursive", "fantasy":
		return "Go"
	default:
		return family
	}
}

func (library *Library) selectEntry(spec Spec) *fontEntry {
	for _, family := range splitFamilyList(spec.Family) {
		family = genericFamily(family)
		candidates := library.entriesFor(family)
		if len(candidates) > 0 {
			return bestEntry(candidates, spec)
		}
	}
	fallback := "Go"
	if strings.Contains(strings.ToLower(spec.Family), "monospace") {
		fallback = "Go Mono"
	}
	if candidates := library.entriesFor(fallback); len(candidates) > 0 {
		return bestEntry(candidates, spec)
	}
	if len(library.entries) > 0 {
		return bestEntry(library.entries, spec)
	}
	return nil
}

func sortEntries(candidates []*fontEntry, spec Spec) {
	sort.SliceStable(candidates, func(first, second int) bool {
		return entryScore(candidates[first], spec) < entryScore(candidates[second], spec)
	})
}

func bestEntry(candidates []*fontEntry, spec Spec) *fontEntry {
	best := candidates[0]
	bestScore := entryScore(best, spec)
	for _, candidate := range candidates[1:] {
		score := entryScore(candidate, spec)
		if score < bestScore {
			best = candidate
			bestScore = score
		}
	}
	return best
}

func entryScore(entry *fontEntry, spec Spec) int {
	weight := entry.weight
	if weight < entry.weightMin() {
		weight = entry.weightMin()
	}
	if weight > entry.weightMax() {
		weight = entry.weightMax()
	}
	score := absInt(weight - spec.Weight)
	stretch := spec.Stretch
	if stretch <= 0 {
		stretch = 1
	}
	minimumStretch := entry.stretchMin
	maximumStretch := entry.stretchMax
	if minimumStretch <= 0 {
		minimumStretch = 1
	}
	if maximumStretch <= 0 {
		maximumStretch = 1
	}
	if stretch < minimumStretch {
		score += int((minimumStretch - stretch) * 1000)
	} else if stretch > maximumStretch {
		score += int((stretch - maximumStretch) * 1000)
	}
	wantsItalic := spec.Italic || spec.Oblique
	if wantsItalic && !entry.italic {
		score += 1000
	}
	if !wantsItalic && entry.italic {
		score += 500
	}
	return score
}

func (entry *fontEntry) weightMin() int {
	if entry.minimumWeight > 0 {
		return entry.minimumWeight
	}
	return entry.weight
}

func (entry *fontEntry) weightMax() int {
	if entry.maximumWeight > 0 {
		return entry.maximumWeight
	}
	return entry.weight
}

func absInt(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

func splitFamilyList(value string) []string {
	values := parser.ParseComponentValueList(value)
	var families []string
	current := ""
	for _, item := range values {
		if parser.IsWhitespace(item) {
			if current != "" {
				current += " "
			}
			continue
		}
		if token, ok := parser.AsToken(item); ok {
			switch token.Type {
			case tokenizer.TokenComma:
				if strings.TrimSpace(current) != "" {
					families = append(families, strings.TrimSpace(current))
				}
				current = ""
				continue
			case tokenizer.TokenString, tokenizer.TokenIdent:
				current += token.Value
				continue
			}
		}
	}
	if strings.TrimSpace(current) != "" {
		families = append(families, strings.TrimSpace(current))
	}
	if len(families) == 0 {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			families = append(families, trimmed)
		}
	}
	return families
}

func parseFontWeight(value string) int {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "normal":
		return 400
	case "bold", "bolder":
		return 700
	case "lighter":
		return 300
	}
	number, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return 400
	}
	return number
}

func parseFontStretch(value string) float64 {
	values := parser.ParseComponentValueList(value)
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
			if token.Number > 0 {
				return token.Number / 100
			}
		case tokenizer.TokenIdent:
			if stretch, ok := stretchKeyword(token.Value); ok {
				return stretch
			}
		}
	}
	return 1
}

func parseFontSize(value string, fallback float64) float64 {
	values := parser.ParseComponentValueList(value)
	if len(values) != 1 {
		return fallback
	}
	token, ok := parser.AsToken(values[0])
	if !ok {
		return fallback
	}
	switch token.Type {
	case tokenizer.TokenNumber:
		if token.Number > 0 {
			return token.Number
		}
	case tokenizer.TokenDimension:
		factor := 1.0
		switch strings.ToLower(token.Value) {
		case "px":
			factor = 1
		case "pt":
			factor = 96.0 / 72.0
		case "pc":
			factor = 16
		case "in":
			factor = 96
		case "cm":
			factor = 96.0 / 2.54
		case "mm":
			factor = 96.0 / 25.4
		case "rem":
			factor = 16
		default:
			return fallback
		}
		if token.Number > 0 {
			return token.Number * factor
		}
	}
	return fallback
}

// Face returns the best matching face for spec.
func (library *Library) Face(spec Spec) font.Face {
	return library.faceForEntry(library.selectEntry(spec), spec)
}

func (library *Library) faceForEntry(entry *fontEntry, spec Spec) font.Face {
	if entry == nil {
		return nil
	}
	size := spec.Size
	if size <= 0 {
		size = 16
	}
	if entry.sizeAdjust > 0 {
		size *= entry.sizeAdjust
	}
	sizeKey := int(math.Round(size * 64))
	key := faceKey{entry: entry, sizeKey: sizeKey}
	library.mutex.Lock()
	if face, ok := library.faces[key]; ok {
		library.mutex.Unlock()
		return face
	}
	library.mutex.Unlock()
	parsed, err := entry.font()
	if err != nil {
		return nil
	}
	created, err := opentype.NewFace(parsed, &opentype.FaceOptions{
		Size:    size,
		DPI:     72,
		Hinting: font.HintingFull,
	})
	if err != nil {
		return nil
	}
	var result font.Face = created
	if len(entry.unicodeRange) > 0 {
		result = &rangedFace{Face: created, ranges: entry.unicodeRange}
	}
	library.mutex.Lock()
	if existing, ok := library.faces[key]; ok {
		library.mutex.Unlock()
		return existing
	}
	library.faces[key] = result
	library.mutex.Unlock()
	return result
}

// Faces returns the primary face followed by deterministic fallbacks.
func (library *Library) Faces(spec Spec) []font.Face {
	var faces []font.Face
	seen := make(map[font.Face]bool)
	appendFace := func(face font.Face) {
		if face == nil {
			return
		}
		if cacheableFace(face) {
			if seen[face] {
				return
			}
			seen[face] = true
		}
		faces = append(faces, face)
	}
	for _, family := range splitFamilyList(spec.Family) {
		family = genericFamily(family)
		candidates := library.entriesFor(family)
		if len(candidates) == 0 {
			continue
		}
		sorted := append([]*fontEntry(nil), candidates...)
		sortEntries(sorted, spec)
		for _, entry := range sorted {
			appendFace(library.faceForEntry(entry, spec))
		}
		break
	}
	if len(faces) == 0 {
		appendFace(library.Face(spec))
	}
	appendFace(library.Face(Spec{Family: "Go", Size: spec.Size, Weight: 400}))
	if strings.Contains(strings.ToLower(spec.Family), "monospace") {
		appendFace(library.Face(Spec{Family: "Go Mono", Size: spec.Size, Weight: 400}))
	}
	return faces
}

// FaceForRune returns the first face in the fallback chain containing glyph.
func (library *Library) FaceForRune(spec Spec, character rune) font.Face {
	return FaceForRuneChain(library.Faces(spec), character)
}

// FaceForRuneChain selects the first face in faces that contains character.
func FaceForRuneChain(faces []font.Face, character rune) font.Face {
	for _, face := range faces {
		if face == nil {
			continue
		}
		if _, _, ok := face.GlyphBounds(character); ok {
			return face
		}
	}
	return nil
}

// MetricsForSpec returns font metrics for spec.
func (library *Library) MetricsForSpec(spec Spec) layouts.FontMetrics {
	entry := library.selectEntry(spec)
	face := library.faceForEntry(entry, spec)
	if face == nil {
		face = library.Face(Spec{Family: "Go", Size: spec.Size, Weight: spec.Weight})
	}
	if face == nil {
		return layouts.MetricsForStyle(nil)
	}
	metrics := face.Metrics()
	average := spec.Size * 0.5
	if advance, ok := face.GlyphAdvance('x'); ok {
		average = float64(advance) / 64
	}
	ascent := float64(metrics.Ascent) / 64
	descent := float64(metrics.Descent) / 64
	lineGap := float64(metrics.Height-metrics.Ascent-metrics.Descent) / 64
	if entry != nil {
		if entry.ascentOverride != nil {
			ascent = *entry.ascentOverride * spec.Size
		}
		if entry.descentOverride != nil {
			descent = *entry.descentOverride * spec.Size
		}
		if entry.lineGapOverride != nil {
			lineGap = *entry.lineGapOverride * spec.Size
		}
	}
	return layouts.FontMetrics{
		Ascent:         ascent,
		Descent:        descent,
		LineGap:        lineGap,
		AverageAdvance: average,
	}
}

// Metrics returns font metrics for style.
func (library *Library) Metrics(style *styles.Style) layouts.FontMetrics {
	return library.MetricsForSpec(SpecFromStyle(style))
}
