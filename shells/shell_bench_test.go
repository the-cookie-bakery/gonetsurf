package shells

import (
	"strings"
	"testing"

	"gonetsurf/parsers/html"
	"gonetsurf/urls"
)

func BenchmarkShellScrollRender(b *testing.B) {
	var source strings.Builder
	source.WriteString(`<!doctype html><html><head><style>
		html, body { margin:0; padding:0 }
		article { width:1000px; margin:0 auto }
		p { font-size:16px; line-height:1.5 }
	</style></head><body>`)
	for index := 0; index < 200; index++ {
		source.WriteString(`<article><h2>Heading</h2><p>Lorem ipsum dolor sit amet, consectetur adipiscing elit. Sed do eiusmod tempor incididunt ut labore et dolore magna aliqua.</p></article>`)
	}
	source.WriteString(`</body></html>`)
	base, err := urls.Parse("https://example.com/")
	if err != nil {
		b.Fatal(err)
	}
	document, err := html.ParseString(source.String(), html.ParseOptions{URL: base})
	if err != nil {
		b.Fatal(err)
	}
	shell := New(1280, 800, nil)
	tab := shell.Browser.ActiveTab()
	tab.URL = base
	tab.History = []*urls.URL{base}
	shell.page = shell.newPageState(document, styleEngine(document), base, base, 1280, 746, tab, 0)
	_, _ = shell.Render()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		tab.SetScroll(0, float64(index%2000))
		_, _ = shell.Render()
	}
}
