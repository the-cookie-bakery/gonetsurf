package rendering

import (
	"bytes"
	"fmt"
	"image"
	"image/draw"
	"strconv"
	"strings"

	"gonetsurf/parsers/css/parser"
	"gonetsurf/parsers/css/tokenizer"
	"gonetsurf/rendering/images"
	"gonetsurf/rendering/software"
	"gonetsurf/urls"
)

// ImageProvider loads images referenced by CSS or HTML.
type ImageProvider interface {
	LoadImage(resource string) (image.Image, error)
}

// DataURLImageProvider decodes data: URLs with the standard library decoders.
type DataURLImageProvider struct{}

// LoadImage decodes a url(...) or data: image source.
func (DataURLImageProvider) LoadImage(resource string) (image.Image, error) {
	return URLImageProvider{}.LoadImage(resource)
}

// URLImageProvider loads data: URLs directly and delegates other URLs to Fetch.
// Base resolves relative URLs when set.
type URLImageProvider struct {
	Base    *urls.URL
	Fetch   func(resource string) ([]byte, error)
	Options images.Options
}

// LoadImage decodes the resource after resolving it against Base.
func (provider URLImageProvider) LoadImage(resource string) (image.Image, error) {
	address, ok := parseCSSURL(resource)
	if !ok {
		return nil, fmt.Errorf("rendering: unsupported image source %q", resource)
	}
	var parsed *urls.URL
	var err error
	if provider.Base != nil {
		parsed, err = urls.ParseReference(provider.Base, address)
	} else {
		parsed, err = urls.Parse(address)
	}
	if err != nil {
		return nil, err
	}
	var data []byte
	mediaType := ""
	switch parsed.Scheme {
	case "data":
		var body []byte
		var isData bool
		mediaType, body, isData = parsed.DataURL()
		if !isData {
			return nil, fmt.Errorf("rendering: invalid data URL")
		}
		data = body
	case "http", "https":
		if provider.Fetch == nil {
			return nil, fmt.Errorf("rendering: no image fetcher configured")
		}
		data, err = provider.Fetch(parsed.String())
		if err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("rendering: unsupported image scheme %q", parsed.Scheme)
	}
	if strings.Contains(strings.ToLower(mediaType), "svg") || looksLikeSVG(data) {
		maxPixels := int64(provider.Options.MaxPixels)
		if maxPixels <= 0 || maxPixels > 1<<31-1 {
			maxPixels = int64(software.DefaultSVGOptions().MaxPixels)
		}
		maxDimension := provider.Options.MaxDimension
		if maxDimension <= 0 {
			maxDimension = software.DefaultSVGOptions().MaxDimension
		}
		return software.DecodeSVG(data, software.SVGOptions{MaxPixels: int(maxPixels), MaxDimension: maxDimension})
	}
	decoded, err := images.Decode(data, provider.Options)
	if err != nil {
		return nil, err
	}
	if len(decoded.Frames) == 0 {
		return nil, fmt.Errorf("rendering: image has no frames")
	}
	return decoded.Frames[0].Image, nil
}

func looksLikeSVG(data []byte) bool {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return false
	}
	lower := strings.ToLower(string(trimmed[:minInt(len(trimmed), 1024)]))
	return strings.HasPrefix(lower, "<svg") ||
		(strings.HasPrefix(lower, "<?xml") && strings.Contains(lower, "<svg"))
}

func minInt(first, second int) int {
	if first < second {
		return first
	}
	return second
}

func (painter *Painter) loadImageURL(address string) *image.NRGBA {
	if address == "" {
		return nil
	}
	return painter.loadImage("url(" + strconv.Quote(address) + ")")
}

func parseCSSURL(resource string) (string, bool) {
	trimmed := strings.TrimSpace(resource)
	if trimmed == "" {
		return "", false
	}
	if !strings.HasPrefix(strings.ToLower(trimmed), "url(") {
		if strings.HasPrefix(strings.ToLower(trimmed), "data:") {
			return trimmed, true
		}
		return "", false
	}
	values := parser.ParseComponentValueList(trimmed)
	for _, value := range values {
		if parser.IsWhitespace(value) {
			continue
		}
		if token, ok := parser.AsToken(value); ok && token.Type == tokenizer.TokenURL {
			return token.Value, true
		}
		if function, ok := value.(*parser.Function); ok && strings.EqualFold(function.Name, "url") {
			for _, argument := range function.Value {
				if token, ok := parser.AsToken(argument); ok && (token.Type == tokenizer.TokenString || token.Type == tokenizer.TokenURL) {
					return token.Value, true
				}
			}
		}
	}
	return "", false
}

func toNRGBA(source image.Image) *image.NRGBA {
	if converted, ok := source.(*image.NRGBA); ok {
		return converted
	}
	bounds := source.Bounds()
	converted := image.NewNRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	draw.Draw(converted, converted.Bounds(), source, bounds.Min, draw.Src)
	return converted
}
