package shells

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"gonetsurf/brokers/network"
	"gonetsurf/dom"
	"gonetsurf/ipc"
	"gonetsurf/storages/cookies"
	"gonetsurf/urls"
	"gonetsurf/windowing"
)

func newBrokerShell(t *testing.T, handler http.Handler) *Shell {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	target, err := urls.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	token, err := ipc.NewCapability()
	if err != nil {
		t.Fatal(err)
	}
	serverSide, clientSide := net.Pipe()
	t.Cleanup(func() {
		serverSide.Close()
		clientSide.Close()
	})
	jar := cookies.NewJar()
	broker := &network.Server{
		Conn:    ipc.NewConn(serverSide),
		Token:   token,
		Fetcher: network.HTTPFetcher{Jar: jar, Cache: network.NewCache(), Session: cookies.NewSessionManager(jar)},
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go broker.Serve(ctx)
	shell := New(400, 300, nil, BrokerClients{Network: &network.Client{Conn: ipc.NewConn(clientSide), Token: token}})
	shell.NavigateInitial(target)
	if shell.page == nil {
		t.Fatalf("page did not load: %q", shell.Error)
	}
	return shell
}

func TestShellScrollsWithWheelAndKeys(t *testing.T) {
	shell := newBrokerShell(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "text/html")
		writer.Write([]byte(`<!doctype html><html><head><style>
			html, body { margin:0; padding:0 }
			div { height:1000px }
		</style></head><body><div></div></body></html>`))
	}))
	shell.HandleEvent(context.Background(), nil, windowing.Event{Type: windowing.EventMousePress, Button: 5, X: 10, Y: 100})
	if tab := shell.Browser.ActiveTab(); tab.ScrollY <= 0 {
		t.Fatalf("wheel scroll = %v", tab.ScrollY)
	}
	before := shell.Browser.ActiveTab().ScrollY
	shell.HandleEvent(context.Background(), nil, keyEvent("Down", "", windowing.Modifiers{}))
	if shell.Browser.ActiveTab().ScrollY <= before {
		t.Errorf("keyboard scroll did not advance: %v -> %v", before, shell.Browser.ActiveTab().ScrollY)
	}
	shell.HandleEvent(context.Background(), nil, keyEvent("Home", "", windowing.Modifiers{}))
	if shell.Browser.ActiveTab().ScrollY != 0 {
		t.Errorf("home scroll = %v", shell.Browser.ActiveTab().ScrollY)
	}
}

func TestShellFollowsLinksAndFragments(t *testing.T) {
	shell := newBrokerShell(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "text/html")
		switch request.URL.Path {
		case "/next":
			writer.Write([]byte(`<!doctype html><title>Next Page</title><body>next</body>`))
		default:
			writer.Write([]byte(`<!doctype html><html><head><style>
				html, body { margin:0; padding:0 }
				a { display:block; width:120px; height:30px }
				#section { height:500px }
			</style></head><body>
				<a id="next" href="/next"></a>
				<a id="jump" href="#section" style="margin-top:10px"></a>
				<div id="section"></div>
			</body></html>`))
		}
	}))
	rootAddress := shell.Browser.ActiveTab().URL.String()
	click := windowing.Event{Type: windowing.EventMousePress, Button: 1, X: 10, Y: topBarHeight + 10}
	shell.HandleEvent(context.Background(), nil, click)
	if path := shell.Browser.ActiveTab().URL.Pathname(); path != "/next" {
		t.Fatalf("path after link click = %q (%s)", path, shell.Error)
	}
	if shell.Browser.ActiveTab().Title != "Next Page" {
		t.Errorf("title = %q", shell.Browser.ActiveTab().Title)
	}
	shell.navigateTo(mustParse(t, rootAddress), false)
	before := shell.page
	jump := before.document.QuerySelector("#jump")
	if jump == nil {
		t.Fatal("jump link missing")
	}
	rect, _ := before.page.BoxRect(jump)
	jumpClick := windowing.Event{Type: windowing.EventMousePress, Button: 1, X: int(rect.X) + 5, Y: topBarHeight + int(rect.Y) + 5}
	shell.HandleEvent(context.Background(), nil, jumpClick)
	if shell.page != before {
		t.Error("fragment navigation reloaded the document")
	}
	if shell.Browser.ActiveTab().URL.Fragment != "section" {
		t.Errorf("fragment = %q", shell.Browser.ActiveTab().URL.Fragment)
	}
	if shell.Browser.ActiveTab().ScrollY <= 0 {
		t.Errorf("fragment scroll = %v", shell.Browser.ActiveTab().ScrollY)
	}
}

func TestShellOpensTargetBlankTabWithOpener(t *testing.T) {
	shell := newBrokerShell(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "text/html")
		if request.URL.Path == "/next" {
			writer.Write([]byte(`<!doctype html><title>Popup</title><body>popup</body>`))
			return
		}
		writer.Write([]byte(`<!doctype html><html><head><style>
			html, body { margin:0; padding:0 }
			a { display:block; width:120px; height:30px }
		</style></head><body><a href="/next" target="_blank"></a></body></html>`))
	}))
	opener := shell.Browser.ActiveTab()
	shell.HandleEvent(context.Background(), nil, windowing.Event{Type: windowing.EventMousePress, Button: 1, X: 10, Y: topBarHeight + 10})
	if len(shell.Browser.Tabs) != 2 {
		t.Fatalf("tabs = %d", len(shell.Browser.Tabs))
	}
	opened := shell.Browser.ActiveTab()
	if opened == opener || opened.OpenerID != opener.ID {
		t.Errorf("opened tab = %#v opener %d", opened, opener.ID)
	}
	if opened.URL.Pathname() != "/next" {
		t.Errorf("opened URL = %v", opened.URL)
	}
}

func TestShellRendersInlineFrame(t *testing.T) {
	shell := newBrokerShell(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "text/html")
		if request.URL.Path == "/frame" {
			writer.Write([]byte(`<!doctype html><html><head><style>
				html, body { margin:0; padding:0; height:100%; background:#00ff00 }
			</style></head><body></body></html>`))
			return
		}
		writer.Write([]byte(`<!doctype html><html><head><style>
			html, body { margin:0; padding:0 }
			iframe { width:120px; height:80px; border:0 }
		</style></head><body><iframe src="/frame"></iframe></body></html>`))
	}))
	if len(shell.page.frames) != 1 {
		t.Fatalf("frames = %d", len(shell.page.frames))
	}
	rendered, renderErr := shell.Render()
	if renderErr != nil {
		t.Fatal(renderErr)
	}
	surface := asSoftwareSurface(t, rendered)
	pixel := surface.Pix[(topBarHeight+10)*surface.Stride+10*4:]
	if pixel[0] != 0 || pixel[1] != 255 || pixel[2] != 0 {
		t.Errorf("iframe pixel = %v", pixel[:4])
	}
}

func TestShellTabDragReorder(t *testing.T) {
	shell := New(400, 300, nil)
	first := shell.Browser.ActiveTab()
	second := shell.Browser.NewTab(nil)
	shell.HandleEvent(context.Background(), nil, windowing.Event{Type: windowing.EventMousePress, Button: 1, X: 10, Y: 10})
	shell.HandleEvent(context.Background(), nil, windowing.Event{Type: windowing.EventMouseMove, X: tabWidth + 10, Y: 10})
	shell.HandleEvent(context.Background(), nil, windowing.Event{Type: windowing.EventMouseRelease, Button: 1, X: tabWidth + 10, Y: 10})
	if shell.Browser.Tabs[0] != second || shell.Browser.Tabs[1] != first {
		t.Errorf("tab order = %#v", shell.Browser.Tabs)
	}
}

func TestShellPrivacyDashboardReset(t *testing.T) {
	shell := newBrokerShell(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "text/html")
		writer.Header().Set("Cache-Control", "max-age=60")
		http.SetCookie(writer, &http.Cookie{Name: "session", Value: "abc", Path: "/"})
		writer.Write([]byte(`<!doctype html><title>Privacy</title><body>privacy</body>`))
	}))
	shell.HandleEvent(context.Background(), nil, windowing.Event{Type: windowing.EventMousePress, Button: 1, X: shell.Width - 20, Y: 10})
	if !shell.PrivacyOpen {
		t.Fatal("privacy dashboard did not open")
	}
	if shell.Privacy.CookieCount == 0 || shell.Privacy.CacheEntries == 0 {
		t.Fatalf("privacy stats = %#v", shell.Privacy)
	}
	panelX := shell.Width - privacyPanelWidth
	shell.HandleEvent(context.Background(), nil, windowing.Event{
		Type:   windowing.EventMousePress,
		Button: 1,
		X:      panelX + 20,
		Y:      topBarHeight + 170,
	})
	if shell.Privacy.ResetCookies == 0 || shell.Privacy.CookieCount != 0 || shell.Privacy.CacheEntries != 0 {
		t.Errorf("privacy reset = %#v (%s)", shell.Privacy, shell.Error)
	}
}

func mustParse(t *testing.T, input string) *urls.URL {
	t.Helper()
	parsed, err := urls.Parse(input)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}

func TestShellOmniboxSuggestions(t *testing.T) {
	shell := New(400, 300, nil)
	tab := shell.Browser.ActiveTab()
	if err := shell.Browser.Navigate(tab, mustParse(t, "https://example.com/guide")); err != nil {
		t.Fatal(err)
	}
	shell.OmniboxFocused = true
	shell.OmniboxText = "exam"
	shell.Caret = 4
	shell.updateSuggestions()
	if len(shell.Suggestions) == 0 || shell.Suggestion != 0 {
		t.Fatalf("suggestions = %#v index %d", shell.Suggestions, shell.Suggestion)
	}
	shell.moveSuggestion(1)
	if shell.OmniboxText != "https://example.com/guide" || shell.Caret != len(shell.OmniboxText) {
		t.Errorf("completed omnibox = %q caret %d", shell.OmniboxText, shell.Caret)
	}
}

func TestSandboxNavigationRules(t *testing.T) {
	if sandboxAllowsNavigation(0, "") != true {
		t.Error("unsandboxed frame navigation was blocked")
	}
	sandboxed := uint32(dom.ParseSandboxDirective(""))
	if sandboxAllowsNavigation(sandboxed, "") {
		t.Error("sandboxed frame self-navigation was allowed")
	}
	withPopups := uint32(dom.ParseSandboxDirective("allow-popups"))
	if !sandboxAllowsNavigation(withPopups, "_blank") {
		t.Error("allow-popups did not enable auxiliary navigation")
	}
}

func TestShellLoadsLinkedStylesheetsAndImports(t *testing.T) {
	shell := newBrokerShell(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/style.css":
			writer.Header().Set("Content-Type", "text/css")
			writer.Write([]byte(`@import "/theme.css"; #box { width:100px; height:80px }`))
		case "/theme.css":
			writer.Header().Set("Content-Type", "text/css")
			writer.Write([]byte(`#box { background-color:#ff0000 }`))
		default:
			writer.Header().Set("Content-Type", "text/html")
			writer.Write([]byte(`<!doctype html><html><head><link rel="stylesheet" href="/style.css"></head><body><div id="box"></div></body></html>`))
		}
	}))
	rendered, renderErr := shell.Render()
	if renderErr != nil {
		t.Fatal(renderErr)
	}
	surface := asSoftwareSurface(t, rendered)
	pixel := surface.Pix[(topBarHeight+10)*surface.Stride+10*4:]
	if pixel[0] != 255 || pixel[1] != 0 || pixel[2] != 0 {
		t.Errorf("stylesheet pixel = %v", pixel[:4])
	}
}

func TestShellResizeRelayoutsPage(t *testing.T) {
	shell := newBrokerShell(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "text/html")
		writer.Write([]byte(`<!doctype html><html><head><style>
			html, body { margin:0; padding:0 }
			#box { display:block; width:700px; height:20px; background:#ff0000 }
			@media (max-width: 600px) { #box { width:300px } }
		</style></head><body><div id="box"></div></body></html>`))
	}))
	shell.Resize(1000, 500)
	if _, renderErr := shell.Render(); renderErr != nil {
		t.Fatal(renderErr)
	}
	box := shell.page.document.GetElementById("box")
	layoutBox := shell.page.page.BoxForNode(box)
	if layoutBox == nil || layoutBox.Width != 700 {
		t.Fatalf("wide layout = %v", layoutBox)
	}
	shell.Resize(500, 400)
	rendered, renderErr := shell.Render()
	if renderErr != nil {
		t.Fatal(renderErr)
	}
	surface := asSoftwareSurface(t, rendered)
	layoutBox = shell.page.page.BoxForNode(box)
	if layoutBox == nil || layoutBox.Width != 300 {
		t.Errorf("narrow layout = %v", layoutBox)
	}
	if shell.page.page.Width != 500 || shell.page.page.Height != 400-topBarHeight-statusBarHeight {
		t.Errorf("page viewport = %dx%d", shell.page.page.Width, shell.page.page.Height)
	}
	_ = surface
}

func TestShellScrollsNestedContainerUnderPointer(t *testing.T) {
	shell := newBrokerShell(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "text/html")
		writer.Write([]byte(`<!doctype html><html><head><style>
			html, body { margin:0; padding:0; height:100% }
			#scroller { display:block; width:200px; height:100px; overflow:auto }
			#content { display:block; width:200px; height:800px }
		</style></head><body><div id="scroller"><div id="content"></div></div></body></html>`))
	}))
	shell.HandleEvent(context.Background(), nil, windowing.Event{Type: windowing.EventMouseMove, X: 20, Y: topBarHeight + 20})
	shell.HandleEvent(context.Background(), nil, windowing.Event{Type: windowing.EventMousePress, Button: 5, X: 20, Y: topBarHeight + 20})
	scroller := shell.page.page.BoxForNode(shell.page.document.GetElementById("scroller"))
	if scroller == nil || scroller.ScrollTop == 0 {
		t.Errorf("nested scroll = %v", scroller)
	}
	if _, rootMax := shell.page.page.MaxScroll(); rootMax != 0 {
		t.Errorf("root scrolled instead of inner container: %v", rootMax)
	}
}
