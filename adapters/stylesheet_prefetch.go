package adapters

import (
	"strings"
	"sync"

	"gonetsurf/dom"
	"gonetsurf/parsers/css/conditions"
	"gonetsurf/parsers/css/cssom"
	"gonetsurf/parsers/css/parser"
	"gonetsurf/urls"
)

// Prefetch fetches linked stylesheets and their @import chains concurrently,
// then returns a loader whose Fetch serves the preloaded bodies. The existing
// sequential cascade order is preserved because preloading only removes
// network waits; stylesheets are still added in document order.
func (loader StylesheetLoader) Prefetch(document *dom.Document, base *urls.URL) StylesheetLoader {
	if loader.Fetch == nil || document == nil {
		return loader
	}
	originalFetch := loader.Fetch
	cache := make(map[string][]byte)
	visited := make(map[string]bool)
	var mutex sync.Mutex
	var waitGroup sync.WaitGroup
	semaphore := make(chan struct{}, 16)

	var process func(target *urls.URL)
	process = func(target *urls.URL) {
		if target == nil {
			return
		}
		key := target.String()
		mutex.Lock()
		if visited[key] {
			mutex.Unlock()
			return
		}
		visited[key] = true
		mutex.Unlock()
		body, err := originalFetch(target)
		if err != nil {
			return
		}
		mutex.Lock()
		cache[key] = body
		mutex.Unlock()
		for _, imported := range stylesheetImports(body, target) {
			waitGroup.Add(1)
			go func(target *urls.URL) {
				defer waitGroup.Done()
				semaphore <- struct{}{}
				defer func() { <-semaphore }()
				process(target)
			}(imported)
		}
	}

	for _, target := range documentStylesheetURLs(document, base) {
		waitGroup.Add(1)
		go func(target *urls.URL) {
			defer waitGroup.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()
			process(target)
		}(target)
	}
	waitGroup.Wait()

	loader.Fetch = func(target *urls.URL) ([]byte, error) {
		if target != nil {
			mutex.Lock()
			body, ok := cache[target.String()]
			mutex.Unlock()
			if ok {
				return body, nil
			}
		}
		return originalFetch(target)
	}
	return loader
}

func documentStylesheetURLs(document *dom.Document, base *urls.URL) []*urls.URL {
	var targets []*urls.URL
	document.Node.Descendants(func(node *dom.Node) bool {
		if node.Type != dom.ElementNode || node.Namespace != dom.NamespaceHTML {
			return true
		}
		if !strings.EqualFold(node.Data, "link") {
			return true
		}
		if _, disabled := node.GetAttribute("disabled"); disabled {
			return true
		}
		rel, _ := node.GetAttribute("rel")
		if !hasToken(rel, "stylesheet") {
			return true
		}
		href, ok := node.GetAttribute("href")
		if !ok || strings.TrimSpace(href) == "" {
			return true
		}
		if target, err := urls.ParseReference(base, href); err == nil {
			targets = append(targets, target)
		}
		return true
	})
	return targets
}

func stylesheetImports(body []byte, base *urls.URL) []*urls.URL {
	if len(body) == 0 {
		return nil
	}
	stylesheet := cssom.Parse(string(body))
	if stylesheet == nil {
		return nil
	}
	var targets []*urls.URL
	for _, rule := range stylesheet.CSSRules() {
		atRule, ok := rule.(*cssom.AtRule)
		if !ok || !strings.EqualFold(atRule.Name, "import") {
			continue
		}
		importRule, err := conditions.ParseImport(parser.SerializeValues(atRule.Prelude))
		if err != nil || importRule.URL == "" {
			continue
		}
		if target, err := urls.ParseReference(base, importRule.URL); err == nil {
			targets = append(targets, target)
		}
	}
	return targets
}
