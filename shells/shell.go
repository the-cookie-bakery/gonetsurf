// Package shells implements the browser shell: chrome, input routing, tab
// navigation, and the event-driven frame loop.
package shells

import (
	"context"
	"encoding/json"
	"fmt"
	htmlstd "html"
	"image"
	"log"
	"os"
	"strings"
	"time"

	"gonetsurf/adapters"
	"gonetsurf/brokers/network"
	"gonetsurf/brokers/storage"
	"gonetsurf/browsers"
	"gonetsurf/dom"
	"gonetsurf/parsers/css/cssom"
	"gonetsurf/parsers/html"
	"gonetsurf/rendering"
	"gonetsurf/rendering/fonts"
	"gonetsurf/rendering/geometry"
	"gonetsurf/rendering/interfaces"
	"gonetsurf/rendering/software"
	"gonetsurf/styles"
	"gonetsurf/urls"
	"gonetsurf/windowing"
)

const (
	topBarHeight       = 34
	statusBarHeight    = 20
	tabWidth           = 150
	privacyPanelWidth  = 320
	privacyPanelHeight = 210
	privacyButtonWidth = 92
	scrollStep         = 80
)

// BrokerClients groups the capability-scoped broker clients used by a shell.
type BrokerClients struct {
	Network *network.Client
	Storage *storage.Client
}

// Shell owns the browser model and the current page state.
type Shell struct {
	Browser *browsers.Browser
	Network *network.Client
	Storage *storage.Client
	Width   int
	Height  int

	OmniboxFocused bool
	OmniboxText    string
	Caret          int
	Suggestions    []browsers.Suggestion
	Suggestion     int
	Status         string
	Error          string

	PrivacyOpen bool
	Privacy     network.PrivacyStats

	draggingTab bool
	dragFrom    int
	dragTo      int
	pointerX    int
	pointerY    int

	page        *pageState
	Renderer    interfaces.Renderer
	dirty       bool
	dirtySignal chan struct{}

	topBarCache    chromeCache
	statusBarCache chromeCache
	privacyCache   chromeCache
}

type chromeCache struct {
	source  string
	width   int
	height  int
	surface interfaces.Surface
}

func (cache *chromeCache) render(source string, width, height int) (interfaces.Surface, error) {
	if cache.surface != nil && cache.source == source && cache.width == width && cache.height == height {
		return cache.surface, nil
	}
	cache.source = source
	cache.width = width
	cache.height = height
	surface, err := renderChrome(source, width, height)
	if err != nil {
		return nil, err
	}
	cache.surface = surface
	return cache.surface, nil
}

// New returns a shell with one blank tab and optional broker clients.
func New(width, height int, initial *urls.URL, brokers ...BrokerClients) *Shell {
	shell := &Shell{Browser: browsers.New(), Width: width, Height: height, dirty: true, dirtySignal: make(chan struct{}, 1)}
	shell.pointerX = width / 2
	if content := height - topBarHeight - statusBarHeight; content > 0 {
		shell.pointerY = content / 2
	}
	if len(brokers) > 0 {
		shell.Network = brokers[0].Network
		shell.Storage = brokers[0].Storage
	}
	shell.OmniboxText = ""
	if initial != nil {
		shell.navigate(initial.String())
	}
	return shell
}

// NavigateInitial navigates the active tab after broker clients are attached.
func (shell *Shell) NavigateInitial(initial *urls.URL) {
	if initial != nil {
		shell.navigate(initial.String())
	}
}

// Dirty reports whether the shell needs a repaint.
func (shell *Shell) Dirty() bool {
	return shell.dirty
}

// MarkDirty requests a repaint.
func (shell *Shell) MarkDirty() {
	shell.dirty = true
}

func (shell *Shell) notifyDirty() {
	if shell.dirtySignal == nil {
		return
	}
	select {
	case shell.dirtySignal <- struct{}{}:
	default:
	}
}

// SetRenderer installs the rendering backend used for pages and chrome.
func (shell *Shell) SetRenderer(renderer interfaces.Renderer) {
	shell.Renderer = renderer
}

// Resize updates the viewport.
func (shell *Shell) Resize(width, height int) {
	if width <= 0 || height <= 0 {
		return
	}
	shell.Width = width
	shell.Height = height
	shell.dirty = true
}

// Render builds a frame with content, chrome, and the status bar.
func (shell *Shell) Render() (interfaces.Surface, error) {
	prepareStart := time.Now()
	result := software.NewSurface(shell.Width, shell.Height)
	result.Clear(rendering.NewColor(255, 255, 255))
	debugf("frame: prepare %s", time.Since(prepareStart))
	contentHeight := shell.contentHeight()
	if shell.page != nil && contentHeight > 0 {
		resizeStart := time.Now()
		shell.page.page.Resize(shell.contentWidth(), contentHeight)
		debugf("frame: resize %s", time.Since(resizeStart))
		if tab := shell.Browser.ActiveTab(); tab != nil {
			scrollStart := time.Now()
			shell.page.page.ScrollTo(tab.ScrollX, tab.ScrollY)
			debugf("frame: scroll %s", time.Since(scrollStart))
		}
		contentStart := time.Now()
		content, err := shell.page.page.Render()
		debugf("frame: content %s", time.Since(contentStart))
		if err != nil {
			return nil, err
		}
		if tab := shell.Browser.ActiveTab(); tab != nil {
			tab.ScrollX, tab.ScrollY = shell.page.page.Scroll()
		}
		blitStart := time.Now()
		result.Blit(0, topBarHeight, content, geometry.Copy)
		debugf("frame: content blit %s", time.Since(blitStart))
	}
	topBarStart := time.Now()
	topBar, err := shell.topBarCache.render(shell.chromeHTML(), shell.Width, topBarHeight)
	debugf("frame: topbar %s", time.Since(topBarStart))
	if err != nil {
		return nil, err
	}
	result.Blit(0, 0, topBar, geometry.Copy)
	statusStart := time.Now()
	statusBar, err := shell.statusBarCache.render(shell.statusHTML(), shell.Width, statusBarHeight)
	debugf("frame: statusbar %s", time.Since(statusStart))
	if err != nil {
		return nil, err
	}
	result.Blit(0, shell.Height-statusBarHeight, statusBar, geometry.Copy)
	if shell.PrivacyOpen {
		panel, err := shell.privacyCache.render(shell.privacyHTML(), privacyPanelWidth, privacyPanelHeight)
		if err != nil {
			return nil, err
		}
		panelX := shell.Width - privacyPanelWidth
		if panelX < 0 {
			panelX = 0
		}
		result.Blit(panelX, topBarHeight, panel, geometry.Copy)
	}
	return result, nil
}

func (shell *Shell) chromeHTML() string {
	var builder strings.Builder
	builder.WriteString(`<!doctype html><html><head><style>
html, body { margin:0; padding:0; font-family:sans-serif; font-size:12px; color:#222222 }
.tab { position:absolute; top:4px; height:26px; box-sizing:border-box; padding:6px 8px; border:1px solid #b8b8bc; background:#e8e8ec; white-space:nowrap; overflow:hidden }
.tab.active { background:#ffffff; border-bottom-color:#ffffff }
.omnibox { position:absolute; top:5px; height:24px; box-sizing:border-box; padding:4px 8px; border:1px solid #888888; background:#ffffff; white-space:nowrap; overflow:hidden }
.privacy { position:absolute; top:8px; right:6px; color:#226622; font-size:11px }
</style></head><body>`)
	for index, tab := range shell.Browser.Tabs {
		title := tab.Title
		if title == "" {
			title = "New Tab"
		}
		if len(title) > 18 {
			title = title[:18]
		}
		class := "tab"
		if index == shell.Browser.Active {
			class += " active"
		}
		builder.WriteString(fmt.Sprintf(`<div class="%s" style="left:%dpx;width:%dpx">%s</div>`,
			class, index*tabWidth, tabWidth, htmlstd.EscapeString(title)))
	}
	omniboxLeft := len(shell.Browser.Tabs) * tabWidth
	omniboxWidth := shell.Width - omniboxLeft - 96
	if omniboxWidth < 80 {
		omniboxWidth = 80
	}
	text := shell.OmniboxText
	if shell.OmniboxFocused {
		caret := shell.Caret
		if caret < 0 {
			caret = 0
		}
		if caret > len(text) {
			caret = len(text)
		}
		text = text[:caret] + "|" + text[caret:]
	}
	builder.WriteString(fmt.Sprintf(`<div class="omnibox" style="left:%dpx;width:%dpx">%s</div>`,
		omniboxLeft, omniboxWidth, htmlstd.EscapeString(text)))
	builder.WriteString(`<div class="privacy">privacy: on</div>`)
	builder.WriteString(`</body></html>`)
	return builder.String()
}

func (shell *Shell) statusHTML() string {
	status := shell.Status
	if shell.Error != "" {
		status = shell.Error
	}
	if status == "" {
		status = "Ready"
	}
	if len(status) > 140 {
		status = status[:140]
	}
	suggestion := ""
	if shell.OmniboxFocused && shell.Suggestion >= 0 && shell.Suggestion < len(shell.Suggestions) {
		suggestion = shell.Suggestions[shell.Suggestion].URL.String()
	}
	if suggestion != "" {
		status = "Tab: " + suggestion
	}
	return `<!doctype html><html><head><style>
html, body { margin:0; padding:0; font-family:sans-serif; font-size:11px; color:#333333 }
.status { position:absolute; left:6px; top:3px; white-space:nowrap }
.mode { position:absolute; right:6px; top:3px; color:#555555 }
</style></head><body><div class="status">` + htmlstd.EscapeString(status) + `</div><div class="mode">archive cache ready</div></body></html>`
}

func (shell *Shell) privacyHTML() string {
	site := "(none)"
	if tab := shell.Browser.ActiveTab(); tab != nil && tab.URL != nil {
		if tab.URL.Site() != "" {
			site = tab.URL.Site()
		} else {
			site = tab.URL.Hostname()
		}
	}
	thirdParty := len(shell.Privacy.ThirdPartyHosts)
	return `<!doctype html><html><head><style>
html, body { margin:0; padding:0; font-family:sans-serif; font-size:11px; color:#222222; background:#f7f7fa }
.title { position:absolute; left:10px; top:8px; font-size:13px; font-weight:bold }
.row { position:absolute; left:10px; white-space:nowrap }
.note { color:#555555 }
.button { position:absolute; left:10px; top:160px; width:280px; height:30px; box-sizing:border-box; padding:7px 10px; border:1px solid #777799; background:#e6e6ee; text-align:center }
</style></head><body>
<div class="title">Privacy dashboard</div>
<div class="row" style="top:34px">site: ` + htmlstd.EscapeString(site) + `</div>
<div class="row" style="top:54px">partition: ` + htmlstd.EscapeString(shell.Privacy.Partition) + `</div>
<div class="row" style="top:74px">cookies: ` + fmt.Sprintf("%d", shell.Privacy.CookieCount) + `</div>
<div class="row" style="top:94px">third-party hosts: ` + fmt.Sprintf("%d", thirdParty) + `</div>
<div class="row" style="top:114px">cached responses: ` + fmt.Sprintf("%d", shell.Privacy.CacheEntries) + `</div>
<div class="row note" style="top:138px">Click to clear tracking cookies and cache.</div>
<div class="button">Reset tracking session</div>
</body></html>`
}

func renderChrome(source string, width, height int) (interfaces.Surface, error) {
	if width <= 0 || height <= 0 {
		return software.NewSurface(width, height), nil
	}
	document, err := html.ParseString(source, html.ParseOptions{})
	if err != nil {
		return software.NewSurface(width, height), nil
	}
	engine := styleEngine(document)
	return rendering.RenderDocumentWithOptions(document, engine, width, height, rendering.Options{})
}

func styleEngine(document *dom.Document) *styles.Engine {
	engine := styles.NewEngine(document)
	document.Node.Descendants(func(node *dom.Node) bool {
		if node.Type == dom.ElementNode && node.Data == "style" {
			engine.AddStylesheet(cssom.Parse(node.TextContent()))
		}
		return true
	})
	return engine
}

func (shell *Shell) stylesheetEngine(document *dom.Document, base, topLevel *urls.URL) *styles.Engine {
	loader := adapters.StylesheetLoader{
		Fetch: func(target *urls.URL) ([]byte, error) {
			return shell.fetchResource(target.String(), topLevel)
		},
	}
	loader = loader.Prefetch(document, base)
	return loader.Engine(document, base)
}

// Run processes events and paints frames until the window closes or ctx ends.
func (shell *Shell) Run(ctx context.Context, window windowing.Window) error {
	input, _ := window.(windowing.InputWindow)
	ticker := time.NewTicker(16 * time.Millisecond)
	defer ticker.Stop()
	firstPresent := time.Now().Add(150 * time.Millisecond)
	for {
		var events <-chan windowing.Event
		if input != nil {
			events = input.Events()
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-shell.dirtySignal:
			shell.dirty = true
		case <-ticker.C:
			if shell.dirty && time.Now().After(firstPresent) {
				shell.present(window)
			}
		case event, ok := <-events:
			if !ok {
				return nil
			}
			if event.Type == windowing.EventClose {
				return nil
			}
			if event.Type == windowing.EventResize {
				shell.Resize(event.Width, event.Height)
				firstPresent = time.Now().Add(32 * time.Millisecond)
				continue
			}
			if event.Type == windowing.EventExpose {
				shell.dirty = true
				shell.present(window)
				continue
			}
			shell.HandleEvent(ctx, input, event)
			switch event.Type {
			case windowing.EventKeyPress, windowing.EventMousePress, windowing.EventMouseRelease:
				if shell.dirty {
					shell.present(window)
				}
			}
		}
	}
}

func (shell *Shell) present(window windowing.Window) {
	if width, height := window.Size(); width > 0 && height > 0 && (width != shell.Width || height != shell.Height) {
		debugf("present: resizing %dx%d -> %dx%d", shell.Width, shell.Height, width, height)
		shell.Resize(width, height)
	}
	renderStart := time.Now()
	surface, err := shell.Render()
	debugf("frame: render %s", time.Since(renderStart))
	if err != nil {
		debugf("present: render error: %v", err)
		shell.Error = err.Error()
		return
	}
	damage := image.Rect(0, 0, shell.Width, shell.Height)
	if presenter, ok := window.(windowing.SurfacePresenter); ok {
		if err := presenter.PresentSurface(surface, damage); err != nil {
			shell.Error = err.Error()
		}
	} else {
		window.Surface().Blit(0, 0, surface, geometry.Copy)
		if err := window.Present(damage); err != nil {
			shell.Error = err.Error()
		}
	}
	shell.dirty = false
}

// HandleEvent routes one input event.
func (shell *Shell) HandleEvent(ctx context.Context, input windowing.InputWindow, event windowing.Event) {
	switch event.Type {
	case windowing.EventKeyPress:
		shell.handleKey(ctx, input, event)
	case windowing.EventMousePress:
		shell.handleMousePress(ctx, event)
	case windowing.EventMouseMove:
		shell.handleMouseMove(event)
	case windowing.EventMouseRelease:
		shell.handleMouseRelease(event)
	}
	shell.dirty = true
}

func (shell *Shell) handleKey(ctx context.Context, input windowing.InputWindow, event windowing.Event) {
	control := event.Modifiers.Control
	switch {
	case control && strings.EqualFold(event.Key, "l"):
		shell.focusOmnibox()
	case control && strings.EqualFold(event.Key, "t"):
		shell.captureScroll()
		shell.Browser.NewTab(nil)
		shell.page = nil
		shell.focusOmnibox()
	case control && strings.EqualFold(event.Key, "w"):
		shell.captureScroll()
		_ = shell.Browser.CloseTab(shell.Browser.Active)
		shell.page = nil
		shell.syncActiveTab()
	case control && strings.EqualFold(event.Key, "Tab"):
		shell.cycleTab(1)
	case control && strings.EqualFold(event.Key, "r"), strings.EqualFold(event.Key, "F5"):
		shell.reload()
	case control && strings.EqualFold(event.Key, "c"):
		if input != nil && shell.Browser.ActiveTab() != nil && shell.Browser.ActiveTab().URL != nil {
			_ = input.SetClipboard(shell.Browser.ActiveTab().URL.String())
			shell.Status = "Copied URL"
		}
	case control && strings.EqualFold(event.Key, "v"):
		if input != nil {
			if text, err := input.GetClipboard(ctx); err == nil {
				shell.OmniboxText += text
				shell.Caret = len(shell.OmniboxText)
				shell.OmniboxFocused = true
				shell.updateSuggestions()
			}
		}
	case strings.EqualFold(event.Key, "Escape"):
		if shell.PrivacyOpen {
			shell.PrivacyOpen = false
		} else {
			shell.OmniboxFocused = false
			shell.Suggestions = nil
		}
	case strings.EqualFold(event.Key, "Enter"):
		if shell.OmniboxFocused {
			shell.navigate(shell.OmniboxText)
			shell.OmniboxFocused = false
			shell.Suggestions = nil
		}
	case strings.EqualFold(event.Key, "Tab"):
		if shell.OmniboxFocused {
			shell.acceptSuggestion()
		}
	case strings.EqualFold(event.Key, "BackSpace"):
		if shell.OmniboxFocused {
			shell.deleteBackward()
			shell.updateSuggestions()
		} else {
			shell.captureScroll()
			shell.Browser.Back()
			shell.syncActiveTab()
		}
	case strings.EqualFold(event.Key, "Delete"):
		if shell.OmniboxFocused {
			shell.deleteForward()
			shell.updateSuggestions()
		}
	case strings.EqualFold(event.Key, "Left"):
		if shell.OmniboxFocused {
			if shell.Caret > 0 {
				shell.Caret--
			}
		} else if event.Modifiers.Alt {
			shell.captureScroll()
			shell.Browser.Back()
			shell.syncActiveTab()
		}
	case strings.EqualFold(event.Key, "Right"):
		if shell.OmniboxFocused {
			if shell.Caret < len(shell.OmniboxText) {
				shell.Caret++
			}
		} else if event.Modifiers.Alt {
			shell.captureScroll()
			shell.Browser.Forward()
			shell.syncActiveTab()
		}
	case strings.EqualFold(event.Key, "Down"):
		if shell.OmniboxFocused {
			shell.moveSuggestion(1)
		} else {
			shell.scrollBy(0, scrollStep)
		}
	case strings.EqualFold(event.Key, "Up"):
		if shell.OmniboxFocused {
			shell.moveSuggestion(-1)
		} else {
			shell.scrollBy(0, -scrollStep)
		}
	case strings.EqualFold(event.Key, "PageDown"), strings.EqualFold(event.Key, " ") && !shell.OmniboxFocused:
		shell.scrollBy(0, float64(shell.contentHeight()*4/5))
	case strings.EqualFold(event.Key, "PageUp"):
		if !shell.OmniboxFocused {
			shell.scrollBy(0, -float64(shell.contentHeight()*4/5))
		}
	case strings.EqualFold(event.Key, "Home"):
		if shell.OmniboxFocused {
			shell.Caret = 0
		} else {
			shell.scrollTo(0, 0)
		}
	case strings.EqualFold(event.Key, "End"):
		if shell.OmniboxFocused {
			shell.Caret = len(shell.OmniboxText)
		} else {
			_, maximum := shell.maximumScroll()
			shell.scrollTo(0, maximum)
		}
	default:
		if shell.OmniboxFocused && event.Text != "" {
			shell.insertText(event.Text)
			shell.updateSuggestions()
		}
	}
}

func (shell *Shell) handleMousePress(ctx context.Context, event windowing.Event) {
	shell.pointerX = event.X
	if contentY := event.Y - topBarHeight; contentY >= 0 {
		shell.pointerY = contentY
	}
	if event.Button == 4 {
		shell.scrollBy(0, -scrollStep)
		return
	}
	if event.Button == 5 {
		shell.scrollBy(0, scrollStep)
		return
	}
	if event.Y < topBarHeight {
		if event.X >= shell.Width-privacyButtonWidth {
			shell.togglePrivacy(ctx)
			return
		}
		tabIndex := event.X / tabWidth
		if tabIndex < len(shell.Browser.Tabs) {
			shell.captureScroll()
			_ = shell.Browser.SelectTab(tabIndex)
			shell.syncActiveTab()
			shell.OmniboxFocused = false
			shell.draggingTab = true
			shell.dragFrom = tabIndex
			shell.dragTo = tabIndex
			return
		}
		shell.focusOmnibox()
		return
	}
	if shell.PrivacyOpen && shell.inPrivacyPanel(event.X, event.Y) {
		if shell.inPrivacyButton(event.X, event.Y) {
			shell.resetTrackingSession(ctx)
		}
		return
	}
	shell.OmniboxFocused = false
	contentY := event.Y - topBarHeight
	shell.handleContentClick(ctx, float64(event.X), float64(contentY))
}

func (shell *Shell) handleMouseMove(event windowing.Event) {
	if contentY := event.Y - topBarHeight; contentY >= 0 {
		shell.pointerX = event.X
		shell.pointerY = contentY
	}
	if !shell.draggingTab || len(shell.Browser.Tabs) == 0 {
		return
	}
	target := event.X / tabWidth
	if target < 0 {
		target = 0
	}
	if target >= len(shell.Browser.Tabs) {
		target = len(shell.Browser.Tabs) - 1
	}
	shell.dragTo = target
}

func (shell *Shell) handleMouseRelease(event windowing.Event) {
	if !shell.draggingTab {
		return
	}
	shell.draggingTab = false
	if shell.dragFrom == shell.dragTo {
		return
	}
	if err := shell.Browser.MoveTab(shell.dragFrom, shell.dragTo); err == nil {
		shell.Status = "Reordered tabs"
	}
}

func (shell *Shell) handleContentClick(ctx context.Context, x, y float64) {
	if shell.page == nil {
		return
	}
	shell.clickPage(ctx, shell.page, x, y)
}

func (shell *Shell) clickPage(ctx context.Context, state *pageState, x, y float64) bool {
	if state == nil || state.page == nil {
		return false
	}
	box := state.page.HitTest(x, y)
	if box == nil {
		return false
	}
	if iframe := ancestorIframe(box.Node); iframe != nil {
		frame := state.frames[iframe]
		if frame != nil {
			if rect, ok := state.page.BoxRect(iframe); ok {
				shell.clickPage(ctx, frame, x-rect.X, y-rect.Y)
			}
			return true
		}
	}
	link := ancestorLink(box.Node)
	if link == nil {
		return false
	}
	href, ok := link.GetAttribute("href")
	if !ok || strings.TrimSpace(href) == "" {
		return true
	}
	if _, downloads := link.GetAttribute("download"); downloads {
		shell.Status = "Downloads are not supported"
		return true
	}
	address, err := urls.ParseReference(state.url, href)
	if err != nil {
		shell.Error = err.Error()
		return true
	}
	target, _ := link.GetAttribute("target")
	if state.model != nil && !sandboxAllowsNavigation(state.model.Sandbox, target) {
		shell.Status = "Navigation blocked by iframe sandbox"
		return true
	}
	shell.followLink(ctx, state, address, target)
	return true
}

func sandboxAllowsNavigation(sandbox uint32, target string) bool {
	flags := dom.SandboxFlags(sandbox)
	switch strings.ToLower(strings.TrimSpace(target)) {
	case "_blank":
		return flags&dom.SandboxAuxiliaryNavigation == 0
	case "_top", "_parent":
		return flags&dom.SandboxTopLevelNavigation == 0
	default:
		return flags.AllowsNavigation()
	}
}

func (shell *Shell) followLink(ctx context.Context, state *pageState, address *urls.URL, target string) {
	switch strings.ToLower(strings.TrimSpace(target)) {
	case "_blank":
		shell.openTab(address)
		return
	case "_top", "_parent":
		shell.navigateTo(address, false)
		return
	}
	if state == shell.page {
		shell.navigateTo(address, false)
		return
	}
	shell.navigateFrameState(ctx, state, address, false)
}

func (shell *Shell) openTab(address *urls.URL) {
	shell.captureScroll()
	openerID := 0
	if active := shell.Browser.ActiveTab(); active != nil {
		openerID = active.ID
	}
	shell.Browser.NewTabWithOpener(nil, openerID)
	shell.page = nil
	shell.navigateTo(address, false)
}

func ancestorLink(node *dom.Node) *dom.Node {
	for current := node; current != nil; current = current.Parent() {
		if current.Type != dom.ElementNode {
			continue
		}
		if current.Namespace == dom.NamespaceHTML && current.Data == "a" {
			return current
		}
	}
	return nil
}

func ancestorIframe(node *dom.Node) *dom.Node {
	for current := node; current != nil; current = current.Parent() {
		if current.Type != dom.ElementNode {
			continue
		}
		if current.Namespace == dom.NamespaceHTML && current.Data == "iframe" {
			return current
		}
	}
	return nil
}

func (shell *Shell) togglePrivacy(ctx context.Context) {
	shell.PrivacyOpen = !shell.PrivacyOpen
	if shell.PrivacyOpen {
		shell.refreshPrivacy(ctx)
	}
}

func (shell *Shell) refreshPrivacy(ctx context.Context) {
	tab := shell.Browser.ActiveTab()
	if tab == nil || tab.URL == nil || shell.Network == nil {
		return
	}
	partition := tab.URL.Site()
	topLevel := partition
	if partition == "" {
		partition = tab.URL.Hostname()
		topLevel = partition
	}
	stats, err := shell.Network.Privacy(ctx, partition, topLevel)
	if err != nil {
		shell.Error = err.Error()
		return
	}
	shell.Privacy = stats
}

func (shell *Shell) resetTrackingSession(ctx context.Context) {
	tab := shell.Browser.ActiveTab()
	if tab == nil || tab.URL == nil || shell.Network == nil {
		return
	}
	partition := tab.URL.Site()
	topLevel := partition
	if partition == "" {
		partition = tab.URL.Hostname()
		topLevel = partition
	}
	stats, err := shell.Network.ResetTrackingSession(ctx, partition, topLevel)
	if err != nil {
		shell.Error = err.Error()
		return
	}
	shell.Privacy = stats
	shell.Status = fmt.Sprintf("Reset tracking session (%d cookies)", stats.ResetCookies)
}

func (shell *Shell) inPrivacyPanel(x, y int) bool {
	panelX := shell.Width - privacyPanelWidth
	if panelX < 0 {
		panelX = 0
	}
	return x >= panelX && x < panelX+privacyPanelWidth && y >= topBarHeight && y < topBarHeight+privacyPanelHeight
}

func (shell *Shell) inPrivacyButton(x, y int) bool {
	panelX := shell.Width - privacyPanelWidth
	if panelX < 0 {
		panelX = 0
	}
	return x >= panelX+10 && x < panelX+290 && y >= topBarHeight+160 && y < topBarHeight+190
}

func (shell *Shell) scrollBy(deltaX, deltaY float64) {
	if shell.page == nil || shell.page.page == nil {
		return
	}
	shell.page.page.ScrollByAt(float64(shell.pointerX), float64(shell.pointerY), deltaX, deltaY)
	shell.syncScroll()
}

func (shell *Shell) scrollTo(x, y float64) {
	if shell.page == nil || shell.page.page == nil {
		return
	}
	shell.page.page.ScrollToAt(float64(shell.pointerX), float64(shell.pointerY), x, y)
	shell.syncScroll()
}

func (shell *Shell) maximumScroll() (float64, float64) {
	if shell.page == nil || shell.page.page == nil {
		return 0, 0
	}
	return shell.page.page.MaxScrollAt(float64(shell.pointerX), float64(shell.pointerY))
}

func (shell *Shell) syncScroll() {
	if shell.page == nil || shell.page.page == nil {
		return
	}
	x, y := shell.page.page.Scroll()
	if tab := shell.Browser.ActiveTab(); tab != nil {
		tab.SetScroll(x, y)
	}
	shell.dirty = true
}

func (shell *Shell) captureScroll() {
	if shell.page == nil || shell.page.page == nil {
		return
	}
	x, y := shell.page.page.Scroll()
	if tab := shell.Browser.ActiveTab(); tab != nil {
		tab.SetScroll(x, y)
	}
}

func (shell *Shell) focusOmnibox() {
	shell.OmniboxFocused = true
	if tab := shell.Browser.ActiveTab(); tab != nil && tab.URL != nil {
		shell.OmniboxText = tab.URL.String()
	}
	shell.Caret = len(shell.OmniboxText)
	shell.updateSuggestions()
}

func (shell *Shell) updateSuggestions() {
	if !shell.OmniboxFocused {
		shell.Suggestions = nil
		shell.Suggestion = -1
		return
	}
	shell.Suggestions = shell.Browser.Suggestions(shell.OmniboxText, 6)
	shell.Suggestion = -1
	if len(shell.Suggestions) > 0 {
		shell.Suggestion = 0
	}
}

func (shell *Shell) moveSuggestion(direction int) {
	if len(shell.Suggestions) == 0 {
		return
	}
	if shell.Suggestion < 0 {
		shell.Suggestion = 0
	} else {
		shell.Suggestion = (shell.Suggestion + direction) % len(shell.Suggestions)
		if shell.Suggestion < 0 {
			shell.Suggestion += len(shell.Suggestions)
		}
	}
	shell.OmniboxText = shell.Suggestions[shell.Suggestion].URL.String()
	shell.Caret = len(shell.OmniboxText)
}

func (shell *Shell) acceptSuggestion() {
	if shell.Suggestion < 0 || shell.Suggestion >= len(shell.Suggestions) {
		return
	}
	shell.OmniboxText = shell.Suggestions[shell.Suggestion].URL.String()
	shell.Caret = len(shell.OmniboxText)
	shell.navigate(shell.OmniboxText)
	shell.OmniboxFocused = false
	shell.Suggestions = nil
}

func (shell *Shell) syncActiveTab() {
	tab := shell.Browser.ActiveTab()
	if tab == nil {
		shell.OmniboxText = ""
		shell.Caret = 0
		shell.page = nil
		return
	}
	if tab.URL != nil {
		shell.OmniboxText = tab.URL.String()
	} else {
		shell.OmniboxText = ""
	}
	shell.Caret = len(shell.OmniboxText)
	shell.loadActiveTab(false)
}

func (shell *Shell) cycleTab(direction int) {
	if len(shell.Browser.Tabs) == 0 {
		return
	}
	shell.captureScroll()
	index := (shell.Browser.Active + direction) % len(shell.Browser.Tabs)
	if index < 0 {
		index += len(shell.Browser.Tabs)
	}
	_ = shell.Browser.SelectTab(index)
	shell.syncActiveTab()
}

func (shell *Shell) insertText(text string) {
	caret := shell.Caret
	if caret < 0 {
		caret = 0
	}
	if caret > len(shell.OmniboxText) {
		caret = len(shell.OmniboxText)
	}
	shell.OmniboxText = shell.OmniboxText[:caret] + text + shell.OmniboxText[caret:]
	shell.Caret += len(text)
}

func (shell *Shell) deleteBackward() {
	if shell.Caret <= 0 || shell.Caret > len(shell.OmniboxText) {
		return
	}
	shell.OmniboxText = shell.OmniboxText[:shell.Caret-1] + shell.OmniboxText[shell.Caret:]
	shell.Caret--
}

func (shell *Shell) deleteForward() {
	if shell.Caret < 0 || shell.Caret >= len(shell.OmniboxText) {
		return
	}
	shell.OmniboxText = shell.OmniboxText[:shell.Caret] + shell.OmniboxText[shell.Caret+1:]
}

func (shell *Shell) navigate(input string) {
	address := strings.TrimSpace(input)
	if address == "" {
		return
	}
	tab := shell.Browser.ActiveTab()
	if strings.HasPrefix(address, "#") && tab != nil && tab.URL != nil {
		if parsed, err := urls.ParseReference(tab.URL, address); err == nil {
			shell.navigateSameDocument(parsed)
			return
		}
	}
	parsed, err := browsers.NavigationTarget(address, browsers.DefaultSearchTemplate)
	if err != nil {
		shell.Error = err.Error()
		return
	}
	shell.navigateTo(parsed, false)
}

func (shell *Shell) navigateTo(target *urls.URL, reload bool) {
	if target == nil {
		return
	}
	if target.Scheme != "http" && target.Scheme != "https" && target.Scheme != "about" && target.Scheme != "data" {
		shell.Error = "unsupported scheme " + target.Scheme
		return
	}
	tab := shell.Browser.ActiveTab()
	if tab == nil {
		tab = shell.Browser.NewTab(nil)
	}
	shell.captureScroll()
	if browsers.ClassifyNavigation(tab.URL, target) {
		shell.navigateSameDocument(target)
		return
	}
	_ = shell.Browser.Navigate(tab, target)
	shell.OmniboxText = target.String()
	shell.Caret = len(shell.OmniboxText)
	shell.Status = "Loading " + target.String()
	shell.Error = ""
	shell.loadActiveTab(reload)
	if tab.URL != nil {
		shell.OmniboxText = tab.URL.String()
		shell.Caret = len(shell.OmniboxText)
	}
}

func (shell *Shell) navigateSameDocument(target *urls.URL) {
	tab := shell.Browser.ActiveTab()
	if tab == nil || target == nil {
		return
	}
	shell.captureScroll()
	_ = shell.Browser.NavigateSameDocument(tab, target)
	if shell.page != nil {
		shell.page.topLevel = target
		shell.scrollToFragmentState(shell.page, target)
	}
	tab.URL = target
	shell.OmniboxText = target.String()
	shell.Caret = len(shell.OmniboxText)
	shell.Status = "Navigated to " + target.String()
	shell.Error = ""
	shell.dirty = true
}

func (shell *Shell) navigateFrameState(ctx context.Context, state *pageState, target *urls.URL, reload bool) {
	if state == nil || target == nil {
		return
	}
	if browsers.ClassifyNavigation(state.url, target) {
		if state.model != nil && state.tab != nil {
			_ = shell.Browser.NavigateFrame(state.tab, state.model, target)
		}
		shell.scrollToFragmentState(state, target)
		shell.dirty = true
		return
	}
	topLevel := state.topLevel
	if topLevel == nil {
		topLevel = state.url
	}
	body, pageURL, err := shell.fetchURL(target, reload, topLevel)
	if err != nil {
		shell.Error = err.Error()
		return
	}
	document, err := html.Parse(body, html.ParseOptions{URL: pageURL})
	if err != nil {
		shell.Error = err.Error()
		return
	}
	width, height := state.page.Width, state.page.Height
	if state.model != nil && state.tab != nil {
		_ = shell.Browser.NavigateFrame(state.tab, state.model, pageURL)
	}
	replacement := &pageState{
		document: document,
		engine:   shell.stylesheetEngine(document, pageURL, topLevel),
		url:      pageURL,
		topLevel: topLevel,
		tab:      state.tab,
		model:    state.model,
		frames:   make(map[*dom.Node]*pageState),
	}
	options, fontCache, rawLoader := shell.resourceOptions(pageURL, topLevel)
	replacement.page = rendering.NewPage(document, replacement.engine, width, height, options)
	replacement.page.Options.Subframes = subframeProvider{shell: shell, owner: replacement}
	replacement.page.SetFontLoadedCallback(shell.notifyDirty)
	shell.prefetchResources(document, replacement.engine, options, fontCache, rawLoader)
	shell.loadFrames(replacement, topLevel, reload, 1, nil)
	*state = *replacement
	shell.dirty = true
}

func (shell *Shell) reload() {
	tab := shell.Browser.ActiveTab()
	if tab == nil || tab.URL == nil {
		return
	}
	shell.captureScroll()
	shell.Browser.Reload()
	shell.loadActiveTab(true)
}

func debugf(format string, arguments ...any) {
	if os.Getenv("GONETSURF_DEBUG") != "" {
		log.Printf(format, arguments...)
	}
}

func (shell *Shell) loadActiveTab(reload bool) {
	tab := shell.Browser.ActiveTab()
	if tab == nil || tab.URL == nil {
		shell.page = nil
		return
	}
	if tab.URL.Scheme == "about" {
		shell.page = nil
		shell.Status = tab.URL.String()
		return
	}
	shell.Status = "Loading " + tab.URL.String()
	loadStart := time.Now()
	source, pageURL, err := shell.fetchURL(tab.URL, reload, tab.URL)
	debugf("load: document fetch %s (%d bytes)", time.Since(loadStart), len(source))
	if err != nil {
		shell.Error = err.Error()
		shell.page = nil
		shell.dirty = true
		return
	}
	document, err := html.Parse(source, html.ParseOptions{URL: pageURL})
	if err != nil {
		shell.Error = err.Error()
		shell.page = nil
		shell.dirty = true
		return
	}
	stylesStart := time.Now()
	engine := shell.stylesheetEngine(document, pageURL, pageURL)
	debugf("load: stylesheets %s", time.Since(stylesStart))
	pageStart := time.Now()
	state := shell.newPageState(document, engine, pageURL, pageURL, shell.contentWidth(), shell.contentHeight(), tab, 0)
	debugf("load: page setup %s", time.Since(pageStart))
	state.page.ScrollTo(tab.ScrollX, tab.ScrollY)
	tab.SetScroll(state.page.Scroll())
	shell.page = state
	if tab.URL != nil {
		tab.URL = pageURL
	}
	if title := documentTitle(document); title != "" {
		tab.Title = title
	} else if pageURL != nil {
		tab.Title = pageURL.Hostname()
	}
	shell.Status = fmt.Sprintf("Loaded %s", tab.Title)
	shell.Error = ""
	shell.dirty = true
}

func (shell *Shell) scrollToFragmentState(state *pageState, target *urls.URL) {
	if state == nil || state.page == nil || target == nil {
		return
	}
	state.document.URL = target
	state.url = target
	if !target.HasFragment || target.Fragment == "" {
		state.page.ScrollTo(0, 0)
		if state == shell.page {
			shell.syncScroll()
		}
		return
	}
	identifier := decodeFragment(target.Fragment)
	node := state.document.GetElementById(identifier)
	if node == nil {
		state.document.Node.Descendants(func(candidate *dom.Node) bool {
			if node != nil || candidate.Type != dom.ElementNode {
				return node == nil
			}
			if value, ok := candidate.GetAttribute("name"); ok && value == identifier {
				node = candidate
				return false
			}
			return true
		})
	}
	if node == nil {
		return
	}
	if box := state.page.BoxForNode(node); box != nil {
		state.page.ScrollTo(0, box.Y)
		if state == shell.page {
			shell.syncScroll()
		}
	}
}

func decodeFragment(value string) string {
	var builder strings.Builder
	for index := 0; index < len(value); index++ {
		if value[index] == '%' && index+2 < len(value) {
			high, highOK := hexValue(value[index+1])
			low, lowOK := hexValue(value[index+2])
			if highOK && lowOK {
				builder.WriteByte(high<<4 | low)
				index += 2
				continue
			}
		}
		builder.WriteByte(value[index])
	}
	return builder.String()
}

func hexValue(value byte) (byte, bool) {
	switch {
	case value >= '0' && value <= '9':
		return value - '0', true
	case value >= 'a' && value <= 'f':
		return value - 'a' + 10, true
	case value >= 'A' && value <= 'F':
		return value - 'A' + 10, true
	default:
		return 0, false
	}
}

func documentTitle(document *dom.Document) string {
	title := ""
	document.Node.Descendants(func(node *dom.Node) bool {
		if node.Type == dom.ElementNode && node.Data == "title" && title == "" {
			title = strings.TrimSpace(node.TextContent())
		}
		return true
	})
	return title
}

func (shell *Shell) fetch(target *urls.URL, reload bool) ([]byte, *urls.URL, error) {
	return shell.fetchURL(target, reload, target)
}

func (shell *Shell) fetchURL(target *urls.URL, reload bool, topLevel *urls.URL) ([]byte, *urls.URL, error) {
	if target == nil {
		return nil, target, fmt.Errorf("nil URL")
	}
	switch target.Scheme {
	case "data":
		_, body, ok := target.DataURL()
		if !ok {
			return nil, target, fmt.Errorf("invalid data URL")
		}
		return body, target, nil
	case "about":
		return []byte("<!doctype html><html><body></body></html>"), target, nil
	case "http", "https":
		if shell.Network == nil {
			return nil, target, fmt.Errorf("network broker is unavailable")
		}
		partition := ""
		if topLevel != nil {
			partition = topLevel.Site()
		}
		if partition == "" {
			partition = target.Site()
		}
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		response, err := shell.Network.Fetch(ctx, network.Request{
			URL:       target.String(),
			Method:    "GET",
			Partition: partition,
			TopLevel:  partition,
			Reload:    reload,
		})
		if err != nil {
			return nil, target, err
		}
		if response.Status < 200 || response.Status >= 300 {
			return nil, target, fmt.Errorf("HTTP %d", response.Status)
		}
		if len(response.Body) > 8<<20 {
			return nil, target, fmt.Errorf("response is too large")
		}
		return response.Body, target, nil
	default:
		return nil, target, fmt.Errorf("unsupported scheme %q", target.Scheme)
	}
}

func (shell *Shell) fetchResource(resource string, topLevel *urls.URL) ([]byte, error) {
	target, err := urls.Parse(resource)
	if err != nil {
		return nil, err
	}
	if target.Scheme != "http" && target.Scheme != "https" && target.Scheme != "data" {
		return nil, fmt.Errorf("unsupported resource scheme %q", target.Scheme)
	}
	if target.Scheme == "data" {
		_, body, ok := target.DataURL()
		if !ok {
			return nil, fmt.Errorf("invalid data URL")
		}
		return body, nil
	}
	if shell.Network == nil {
		return nil, fmt.Errorf("network broker is unavailable")
	}
	partition := ""
	if topLevel != nil {
		partition = topLevel.Site()
	}
	if partition == "" {
		partition = target.Site()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	response, err := shell.Network.Fetch(ctx, network.Request{
		URL:       target.String(),
		Method:    "GET",
		Partition: partition,
		TopLevel:  partition,
	})
	if err != nil {
		return nil, err
	}
	if response.Status < 200 || response.Status >= 300 {
		return nil, fmt.Errorf("HTTP %d", response.Status)
	}
	return response.Body, nil
}

// SaveSession stores the browser session through the storage broker.
func (shell *Shell) SaveSession(ctx context.Context) error {
	if shell.Storage == nil {
		return nil
	}
	data, err := json.Marshal(shell.Browser.Session())
	if err != nil {
		return err
	}
	return shell.Storage.Set(ctx, "session", "gonetsurf", "active", data)
}

// LoadSession restores a session from the storage broker when available.
func (shell *Shell) LoadSession(ctx context.Context) error {
	if shell.Storage == nil {
		return nil
	}
	data, found, err := shell.Storage.Get(ctx, "session", "gonetsurf", "active")
	if err != nil || !found {
		return err
	}
	var session browsers.Session
	if err := json.Unmarshal(data, &session); err != nil {
		return err
	}
	restored, err := browsers.Restore(session)
	if err != nil {
		return err
	}
	shell.Browser = restored
	shell.syncActiveTab()
	return nil
}

func (shell *Shell) resourceOptions(base, topLevel *urls.URL) (rendering.Options, *fonts.LoaderCache, fonts.Loader) {
	loader := func(resource string) ([]byte, error) {
		return shell.fetchResource(resource, topLevel)
	}
	provider := rendering.URLImageProvider{
		Base:  base,
		Fetch: loader,
	}
	fontCache := fonts.NewLoaderCache()
	return rendering.Options{
		Renderer:   shell.Renderer,
		AsyncFonts: true,
		Images: rendering.CachedImageProvider{
			Provider: provider,
			Cache:    rendering.NewImageCache(),
			Base:     base,
		},
		LoadFont: fontCache.Loader(loader),
	}, fontCache, loader
}
