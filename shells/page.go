package shells

import (
	"gonetsurf/browsers"
	"gonetsurf/dom"
	"gonetsurf/parsers/html"
	"gonetsurf/rendering"
	"gonetsurf/rendering/fonts"
	"gonetsurf/rendering/interfaces"
	"gonetsurf/styles"
	"gonetsurf/urls"
	"log"
	"os"
)

const maximumFrameDepth = 4

type pageState struct {
	document *dom.Document
	engine   *styles.Engine
	url      *urls.URL
	topLevel *urls.URL
	tab      *browsers.Tab
	model    *browsers.Frame
	page     *rendering.Page
	frames   map[*dom.Node]*pageState
}

func (shell *Shell) newPageState(document *dom.Document, engine *styles.Engine, address, topLevel *urls.URL, width, height int, tab *browsers.Tab, depth int) *pageState {
	state := &pageState{
		document: document,
		engine:   engine,
		url:      address,
		topLevel: topLevel,
		tab:      tab,
		frames:   make(map[*dom.Node]*pageState),
	}
	options, fontCache, rawLoader := shell.resourceOptions(address, topLevel)
	state.page = rendering.NewPage(document, engine, width, height, options)
	state.page.Options.Subframes = subframeProvider{shell: shell, owner: state}
	state.page.SetFontLoadedCallback(shell.notifyDirty)
	shell.prefetchResources(document, engine, options, fontCache, rawLoader)
	if os.Getenv("GONETSURF_DEBUG") != "" {
		log.Printf("load: page setup mutation=%d", document.MutationVersion())
	}
	if depth < maximumFrameDepth {
		shell.loadFrames(state, topLevel, false, depth, tab)
	}
	return state
}

func (shell *Shell) loadFrames(state *pageState, topLevel *urls.URL, reload bool, depth int, tab *browsers.Tab) {
	if state == nil || state.document == nil || state.url == nil {
		return
	}
	if tab != nil && depth == 0 {
		tab.Frames = nil
	}
	state.document.Node.Descendants(func(node *dom.Node) bool {
		if node.Type != dom.ElementNode || node.Namespace != dom.NamespaceHTML || node.Data != "iframe" {
			return true
		}
		source, ok := node.GetAttribute("src")
		if !ok || source == "" {
			return true
		}
		address, err := urls.ParseReference(state.url, source)
		if err != nil {
			return true
		}
		name, _ := node.GetAttribute("name")
		sandbox := uint32(0)
		if value, exists := node.GetAttribute("sandbox"); exists {
			sandbox = uint32(dom.ParseSandboxDirective(value))
		}
		var model *browsers.Frame
		if tab != nil && depth == 0 {
			model = tab.NewFrame(name, address, sandbox)
		}
		body, pageURL, err := shell.fetchURL(address, reload, topLevel)
		if err != nil {
			return true
		}
		document, err := html.Parse(body, html.ParseOptions{URL: pageURL})
		if err != nil {
			return true
		}
		engine := shell.stylesheetEngine(document, pageURL, topLevel)
		frame := &pageState{
			document: document,
			engine:   engine,
			url:      pageURL,
			topLevel: topLevel,
			tab:      tab,
			model:    model,
			frames:   make(map[*dom.Node]*pageState),
		}
		options, fontCache, rawLoader := shell.resourceOptions(pageURL, topLevel)
		frame.page = rendering.NewPage(document, engine, 300, 150, options)
		frame.page.Options.Subframes = subframeProvider{shell: shell, owner: frame}
		frame.page.SetFontLoadedCallback(shell.notifyDirty)
		shell.prefetchResources(document, engine, options, fontCache, rawLoader)
		if depth+1 < maximumFrameDepth {
			shell.loadFrames(frame, topLevel, reload, depth+1, nil)
		}
		state.frames[node] = frame
		return true
	})
}

type subframeProvider struct {
	shell *Shell
	owner *pageState
}

func (provider subframeProvider) RenderSubframe(node *dom.Node, width, height int) (interfaces.Surface, error) {
	if provider.owner == nil {
		return nil, nil
	}
	frame := provider.owner.frames[node]
	if frame == nil || frame.page == nil {
		return nil, nil
	}
	if width < 1 {
		width = 1
	}
	if height < 1 {
		height = 1
	}
	frame.page.Resize(width, height)
	return frame.page.Render()
}

func (shell *Shell) contentWidth() int {
	width := shell.Width
	if width < 1 {
		width = 1
	}
	return width
}

func (shell *Shell) contentHeight() int {
	height := shell.Height - topBarHeight - statusBarHeight
	if height < 1 {
		height = 1
	}
	return height
}

func (shell *Shell) prefetchResources(document *dom.Document, engine *styles.Engine, options rendering.Options, fontCache *fonts.LoaderCache, rawLoader fonts.Loader) {
	if prefetcher, ok := options.Images.(interface{ Prefetch([]string) }); ok {
		resources := rendering.CollectDocumentImageResources(document)
		if len(resources) > 0 {
			go prefetcher.Prefetch(resources)
		}
	}
	if fontCache != nil {
		resources := rendering.CollectFontResources(engine)
		if len(resources) > 0 {
			go fontCache.Prefetch(resources, rawLoader)
		}
	}
}
