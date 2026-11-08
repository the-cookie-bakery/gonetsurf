package rendering

import (
	"strings"

	"gonetsurf/dom"
	"gonetsurf/parsers/css/parser"
	"gonetsurf/parsers/css/tokenizer"
	"gonetsurf/styles"
	"gonetsurf/urls"
)

var imageProperties = []string{
	"background-image",
	"border-image-source",
	"list-style-image",
	"content",
	"mask-image",
	"-webkit-mask-image",
	"shape-outside",
}

// CollectDocumentImageResources returns image URLs referenced by DOM
// attributes, inline styles, and inline <style> text. It does not compute the
// full style tree, so it is cheap enough to run before layout.
func CollectDocumentImageResources(document *dom.Document) []string {
	if document == nil {
		return nil
	}
	base := document.URL
	found := make(map[string]struct{})
	document.Node.Descendants(func(node *dom.Node) bool {
		if node.Type != dom.ElementNode {
			return true
		}
		if strings.EqualFold(node.Data, "style") {
			collectURLValues(parser.ParseComponentValueList(node.TextContent()), base, found)
		}
		collectNodeImageAttributes(node, base, found)
		if value, ok := node.GetAttribute("style"); ok {
			collectURLValues(parser.ParseComponentValueList(value), base, found)
		}
		return true
	})
	result := make([]string, 0, len(found))
	for resource := range found {
		result = append(result, resource)
	}
	return result
}

// CollectImageResources returns the absolute image URLs referenced by the
// document's computed styles and image-bearing attributes.
func CollectImageResources(document *dom.Document, engine *styles.Engine) []string {
	if document == nil {
		return nil
	}
	base := document.URL
	found := make(map[string]struct{})
	if engine != nil {
		for _, style := range engine.ComputeDocumentTree(document) {
			for _, property := range imageProperties {
				collectURLValues(style.Get(property), base, found)
			}
		}
	}
	document.Node.Descendants(func(node *dom.Node) bool {
		if node.Type != dom.ElementNode {
			return true
		}
		collectNodeImageAttributes(node, base, found)
		if value, ok := node.GetAttribute("style"); ok {
			collectURLValues(parser.ParseComponentValueList(value), base, found)
		}
		return true
	})
	result := make([]string, 0, len(found))
	for resource := range found {
		result = append(result, resource)
	}
	return result
}

func collectNodeImageAttributes(node *dom.Node, base *urls.URL, found map[string]struct{}) {
	tag := strings.ToLower(node.Data)
	switch tag {
	case "img":
		addAttributeURL(node, "src", base, found)
		if value, ok := node.GetAttribute("srcset"); ok {
			for _, candidate := range parseImageCandidates(value) {
				addResolvedURL(candidate.address, base, found)
			}
		}
	case "input":
		inputType, _ := node.GetAttribute("type")
		if strings.EqualFold(inputType, "image") {
			addAttributeURL(node, "src", base, found)
		}
	case "video":
		addAttributeURL(node, "poster", base, found)
	case "source":
		if value, ok := node.GetAttribute("srcset"); ok {
			for _, candidate := range parseImageCandidates(value) {
				addResolvedURL(candidate.address, base, found)
			}
		}
	case "image":
		addAttributeURL(node, "href", base, found)
		addAttributeURL(node, "xlink:href", base, found)
	}
}

func addAttributeURL(node *dom.Node, name string, base *urls.URL, found map[string]struct{}) {
	value, ok := node.GetAttribute(name)
	if !ok || strings.TrimSpace(value) == "" {
		return
	}
	addResolvedURL(value, base, found)
}

func addResolvedURL(value string, base *urls.URL, found map[string]struct{}) {
	value = strings.TrimSpace(value)
	if value == "" || strings.HasPrefix(value, "#") || strings.HasPrefix(value, "data:") {
		return
	}
	lower := strings.ToLower(value)
	if strings.HasPrefix(lower, "javascript:") || strings.HasPrefix(lower, "mailto:") {
		return
	}
	if target, err := urls.ParseReference(base, value); err == nil {
		found[target.String()] = struct{}{}
	}
}

func collectURLValues(values []parser.ComponentValue, base *urls.URL, found map[string]struct{}) {
	for _, value := range values {
		switch typed := value.(type) {
		case parser.Token:
			if typed.Type == tokenizer.TokenURL {
				addResolvedURL(typed.Value, base, found)
			}
		case *parser.Function:
			if strings.EqualFold(typed.Name, "url") {
				for _, argument := range typed.Value {
					if token, ok := parser.AsToken(argument); ok {
						addResolvedURL(token.Value, base, found)
					}
				}
				continue
			}
			collectURLValues(typed.Value, base, found)
		case *parser.Block:
			collectURLValues(typed.Value, base, found)
		}
	}
}

// CollectFontResources returns external @font-face source URLs.
func CollectFontResources(engine *styles.Engine) []string {
	if engine == nil {
		return nil
	}
	found := make(map[string]struct{})
	for _, rule := range engine.FontFaces() {
		if rule == nil {
			continue
		}
		for _, source := range rule.Sources {
			if strings.TrimSpace(source.URL) == "" {
				continue
			}
			found[source.URL] = struct{}{}
		}
	}
	result := make([]string, 0, len(found))
	for resource := range found {
		result = append(result, resource)
	}
	return result
}
