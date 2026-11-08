// Command netsurfctl provides maintenance and diagnostic subcommands for the
// gonetsurf browser: dependency policy checks, cache management, and inspection
// helpers.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"io"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"

	"gonetsurf/adapters"
	"gonetsurf/dom"
	"gonetsurf/internal/reftest"
	"gonetsurf/layouts"
	parsershtml "gonetsurf/parsers/html"
	"gonetsurf/protocols/http1"
	"gonetsurf/rendering"
	"gonetsurf/rendering/backends"
	"gonetsurf/rendering/fonts"
	"gonetsurf/rendering/geometry"
	"gonetsurf/rendering/interfaces"
	"gonetsurf/styles"
	"gonetsurf/urls"
	"gonetsurf/windowing/x11"
)

var activeRenderer interfaces.Renderer

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: netsurfctl <deps-check|urlparse|fetch|layout|render|show|reftest> [args]")
		os.Exit(2)
	}
	arguments, rendererName, err := extractRenderer(os.Args[2:])
	if err != nil {
		fmt.Fprintln(os.Stderr, "netsurfctl:", err)
		os.Exit(2)
	}
	backend, err := backends.Select(rendererName)
	if err != nil {
		fmt.Fprintln(os.Stderr, "netsurfctl:", err)
		os.Exit(2)
	}
	activeRenderer, err = backend.NewRenderer()
	if err != nil {
		fmt.Fprintln(os.Stderr, "netsurfctl:", err)
		os.Exit(1)
	}
	defer activeRenderer.Close()
	switch os.Args[1] {
	case "deps-check":
		if err := depsCheck(); err != nil {
			fmt.Fprintln(os.Stderr, "deps-check failed:", err)
			os.Exit(1)
		}
		fmt.Println("deps-check: ok")
	case "urlparse":
		if err := urlParse(arguments); err != nil {
			fmt.Fprintln(os.Stderr, "urlparse failed:", err)
			os.Exit(1)
		}
	case "fetch":
		if err := fetch(arguments); err != nil {
			fmt.Fprintln(os.Stderr, "fetch failed:", err)
			os.Exit(1)
		}
	case "layout":
		if err := layout(arguments); err != nil {
			fmt.Fprintln(os.Stderr, "layout failed:", err)
			os.Exit(1)
		}
	case "render":
		if err := render(arguments); err != nil {
			fmt.Fprintln(os.Stderr, "render failed:", err)
			os.Exit(1)
		}
	case "show":
		if err := show(arguments); err != nil {
			fmt.Fprintln(os.Stderr, "show failed:", err)
			os.Exit(1)
		}
	case "reftest":
		if err := reftestCommand(arguments); err != nil {
			fmt.Fprintln(os.Stderr, "reftest failed:", err)
			os.Exit(1)
		}
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n", os.Args[1])
		os.Exit(2)
	}
}

// allowedModulePrefixes lists the module prefixes permitted at runtime. Only
// the standard library, the project's own module, and golang.org/x packages
// may appear in the dependency graph.
var allowedModulePrefixes = []string{
	"golang.org/x/",
}

func depsCheck() error {
	command := exec.Command("go", "list", "-deps",
		"-f", "{{with .Module}}{{.Path}}{{end}}", "./...")
	output, err := command.Output()
	if err != nil {
		return fmt.Errorf("go list: %w", err)
	}
	modules := make(map[string]struct{})
	for _, line := range strings.Split(string(output), "\n") {
		module := strings.TrimSpace(line)
		if module == "" {
			continue
		}
		modules[module] = struct{}{}
	}
	var violations []string
	for module := range modules {
		if isAllowedModule(module) {
			continue
		}
		violations = append(violations, module)
	}
	if len(violations) > 0 {
		sort.Strings(violations)
		return fmt.Errorf("disallowed modules: %s", strings.Join(violations, ", "))
	}
	return nil
}

func isAllowedModule(module string) bool {
	if module == "gonetsurf" || strings.HasPrefix(module, "gonetsurf/") {
		return true
	}
	for _, prefix := range allowedModulePrefixes {
		if strings.HasPrefix(module, prefix) {
			return true
		}
	}
	return false
}

func fetch(arguments []string) error {
	if len(arguments) != 1 {
		return fmt.Errorf("usage: netsurfctl fetch <url>")
	}
	status, body, err := fetchBody(arguments[0])
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "status: %d\n", status)
	_, err = os.Stdout.Write(body)
	return err
}

// fetchBody retrieves a URL and returns its status code and body.
func fetchBody(rawURL string) (int, []byte, error) {
	target, err := urls.Parse(rawURL)
	if err != nil {
		return 0, nil, err
	}
	switch target.Scheme {
	case "http", "https":
	default:
		return 0, nil, fmt.Errorf("unsupported scheme %q", target.Scheme)
	}
	client := &http1.Client{Transport: &http1.Transport{}}
	response, err := client.Do(context.Background(), &http1.Request{Method: "GET", URL: target, Header: http1.Header{}})
	if err != nil {
		return 0, nil, err
	}
	defer response.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 8<<20))
	if err != nil {
		return 0, nil, err
	}
	return response.StatusCode, body, nil
}

func urlParse(arguments []string) error {
	if len(arguments) != 1 {
		return fmt.Errorf("usage: netsurfctl urlparse <url>")
	}
	parsed, err := urls.Parse(arguments[0])
	if err != nil {
		return err
	}
	fmt.Printf("href:     %s\n", parsed.String())
	fmt.Printf("scheme:   %s\n", parsed.Scheme)
	fmt.Printf("origin:   %s\n", parsed.OriginString())
	fmt.Printf("site:     %s\n", parsed.Site())
	fmt.Printf("host:     %s\n", parsed.HostWithPort())
	fmt.Printf("path:     %s\n", parsed.Pathname())
	fmt.Printf("query:    %s\n", parsed.Search())
	fmt.Printf("fragment: %s\n", parsed.Hash())
	return nil
}

// layout parses an HTML file or URL, resolves styles, lays out the document,
// and prints the resulting box geometry as JSON.
func layout(arguments []string) error {
	if len(arguments) != 1 && len(arguments) != 3 {
		return fmt.Errorf("usage: netsurfctl layout <file|url|-> [viewport-width viewport-height]")
	}
	width := 800.0
	height := 600.0
	if len(arguments) == 3 {
		parsedWidth, err := strconv.ParseFloat(arguments[1], 64)
		if err != nil || parsedWidth <= 0 {
			return fmt.Errorf("invalid viewport width %q", arguments[1])
		}
		parsedHeight, err := strconv.ParseFloat(arguments[2], 64)
		if err != nil || parsedHeight <= 0 {
			return fmt.Errorf("invalid viewport height %q", arguments[2])
		}
		width, height = parsedWidth, parsedHeight
	}
	source, documentURL, err := readLayoutSource(arguments[0])
	if err != nil {
		return err
	}
	document, err := parsershtml.Parse(source, parsershtml.ParseOptions{URL: documentURL})
	if err != nil {
		return err
	}
	engine := buildStyleEngine(document, documentURL)
	page := rendering.NewPage(document, engine, int(width), int(height), documentRenderOptions(documentURL))
	root := page.Box()
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(buildLayoutBox(root))
}

// render parses an HTML file or URL, lays it out, and writes the result to a
// PNG file.
func render(arguments []string) error {
	if len(arguments) != 2 && len(arguments) != 4 {
		return fmt.Errorf("usage: netsurfctl render <file|url|-> <output.png> [viewport-width viewport-height]")
	}
	width := 800
	height := 600
	if len(arguments) == 4 {
		parsedWidth, err := strconv.Atoi(arguments[2])
		if err != nil || parsedWidth <= 0 {
			return fmt.Errorf("invalid viewport width %q", arguments[2])
		}
		parsedHeight, err := strconv.Atoi(arguments[3])
		if err != nil || parsedHeight <= 0 {
			return fmt.Errorf("invalid viewport height %q", arguments[3])
		}
		width, height = parsedWidth, parsedHeight
	}
	source, documentURL, err := readLayoutSource(arguments[0])
	if err != nil {
		return err
	}
	document, err := parsershtml.Parse(source, parsershtml.ParseOptions{URL: documentURL})
	if err != nil {
		return err
	}
	surface, err := rendering.RenderDocumentWithOptions(document, buildStyleEngine(document, documentURL), width, height, documentRenderOptions(documentURL))
	if err != nil {
		return err
	}
	file, err := os.Create(arguments[1])
	if err != nil {
		return err
	}
	if err := png.Encode(file, surface.Image()); err != nil {
		file.Close()
		return err
	}
	return file.Close()
}

// show renders an HTML file or URL into an X11 window and runs its event loop.
func show(arguments []string) error {
	if len(arguments) != 1 && len(arguments) != 3 {
		return fmt.Errorf("usage: netsurfctl show <file|url|-> [viewport-width viewport-height]")
	}
	width := 800
	height := 600
	if len(arguments) == 3 {
		parsedWidth, err := strconv.Atoi(arguments[1])
		if err != nil || parsedWidth <= 0 {
			return fmt.Errorf("invalid viewport width %q", arguments[1])
		}
		parsedHeight, err := strconv.Atoi(arguments[2])
		if err != nil || parsedHeight <= 0 {
			return fmt.Errorf("invalid viewport height %q", arguments[2])
		}
		width, height = parsedWidth, parsedHeight
	}
	source, documentURL, err := readLayoutSource(arguments[0])
	if err != nil {
		return err
	}
	document, err := parsershtml.Parse(source, parsershtml.ParseOptions{URL: documentURL})
	if err != nil {
		return err
	}
	engine := buildStyleEngine(document, documentURL)
	options := documentRenderOptions(documentURL)
	surface, err := rendering.RenderDocumentWithOptions(document, engine, width, height, options)
	if err != nil {
		return err
	}
	title := "gonetsurf: " + arguments[0]
	if documentURL != nil {
		title = "gonetsurf: " + documentURL.String()
	}
	display := x11.New(x11.Options{Width: width, Height: height, Title: title})
	opened, err := display.Open()
	if err != nil {
		return err
	}
	defer display.Close()
	window, ok := opened.(*x11.Window)
	if !ok {
		return fmt.Errorf("unexpected window implementation")
	}
	window.SetResizeHandler(func(width, height int) {
		rendered, renderErr := rendering.RenderDocumentWithOptions(document, engine, width, height, options)
		if renderErr != nil {
			return
		}
		window.Surface().Blit(0, 0, rendered, geometry.Copy)
		window.Present(image.Rect(0, 0, width, height))
	})
	window.Surface().Blit(0, 0, surface, geometry.Copy)
	if err := window.Present(image.Rect(0, 0, width, height)); err != nil {
		return err
	}
	return display.EventLoop(context.Background())
}

func reftestCommand(arguments []string) error {
	if len(arguments) != 2 && len(arguments) != 4 {
		return fmt.Errorf("usage: netsurfctl reftest <test|url|-> <reference|url|-> [viewport-width viewport-height]")
	}
	width := 800
	height := 600
	if len(arguments) == 4 {
		parsedWidth, err := strconv.Atoi(arguments[2])
		if err != nil || parsedWidth <= 0 {
			return fmt.Errorf("invalid viewport width %q", arguments[2])
		}
		parsedHeight, err := strconv.Atoi(arguments[3])
		if err != nil || parsedHeight <= 0 {
			return fmt.Errorf("invalid viewport height %q", arguments[3])
		}
		width, height = parsedWidth, parsedHeight
	}
	testSource, testURL, err := readLayoutSource(arguments[0])
	if err != nil {
		return err
	}
	referenceSource, referenceURL, err := readLayoutSource(arguments[1])
	if err != nil {
		return err
	}
	baseURL := testURL
	if baseURL == nil {
		baseURL = referenceURL
	}
	options := reftest.Options{
		Width:          width,
		Height:         height,
		Images:         documentImageProvider(baseURL),
		LoadFont:       documentFontLoader(),
		LoadStylesheet: fetchStylesheet,
		Tolerance:      0,
		Renderer:       activeRenderer,
	}
	result, err := reftest.Compare(testSource, referenceSource, baseURL, options)
	if err != nil {
		return err
	}
	fmt.Printf("reftest: %s\n", result)
	if result.Different != 0 {
		return fmt.Errorf("reftest mismatch")
	}
	return nil
}

func documentFontLoader() fonts.Loader {
	return func(resource string) ([]byte, error) {
		status, body, err := fetchBody(resource)
		if err != nil {
			return nil, err
		}
		if status < 200 || status >= 300 {
			return nil, fmt.Errorf("font fetch returned status %d", status)
		}
		return body, nil
	}
}

func documentRenderOptions(documentURL *urls.URL) rendering.Options {
	return rendering.Options{
		Renderer: activeRenderer,
		Images:   documentImageProvider(documentURL),
		LoadFont: documentFontLoader(),
	}
}

func documentImageProvider(documentURL *urls.URL) rendering.ImageProvider {
	return rendering.URLImageProvider{
		Base: documentURL,
		Fetch: func(resource string) ([]byte, error) {
			status, body, err := fetchBody(resource)
			if err != nil {
				return nil, err
			}
			if status < 200 || status >= 300 {
				return nil, fmt.Errorf("image fetch returned status %d", status)
			}
			return body, nil
		},
	}
}

func buildStyleEngine(document *dom.Document, base *urls.URL) *styles.Engine {
	loader := adapters.StylesheetLoader{Fetch: fetchStylesheet}
	return loader.Engine(document, base)
}

func fetchStylesheet(target *urls.URL) ([]byte, error) {
	status, body, err := fetchBody(target.String())
	if err != nil {
		return nil, err
	}
	if status < 200 || status >= 300 {
		return nil, fmt.Errorf("stylesheet fetch returned status %d", status)
	}
	return body, nil
}

func readLayoutSource(source string) ([]byte, *urls.URL, error) {
	if source == "-" {
		data, err := io.ReadAll(io.LimitReader(os.Stdin, 16<<20))
		return data, nil, err
	}
	parsed, err := urls.Parse(source)
	if err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") {
		_, body, fetchErr := fetchBody(source)
		if fetchErr != nil {
			return nil, nil, fetchErr
		}
		return body, parsed, nil
	}
	data, err := os.ReadFile(source)
	if err != nil {
		return nil, nil, err
	}
	return data, nil, nil
}

type layoutBoxJSON struct {
	Tag           string          `json:"tag,omitempty"`
	ID            string          `json:"id,omitempty"`
	Class         string          `json:"class,omitempty"`
	Text          string          `json:"text,omitempty"`
	X             float64         `json:"x"`
	Y             float64         `json:"y"`
	Width         float64         `json:"width"`
	Height        float64         `json:"height"`
	ContentX      float64         `json:"contentX"`
	ContentY      float64         `json:"contentY"`
	ContentWidth  float64         `json:"contentWidth"`
	ContentHeight float64         `json:"contentHeight"`
	ScrollWidth   float64         `json:"scrollWidth,omitempty"`
	ScrollHeight  float64         `json:"scrollHeight,omitempty"`
	ClipX         bool            `json:"clipX,omitempty"`
	ClipY         bool            `json:"clipY,omitempty"`
	Children      []layoutBoxJSON `json:"children,omitempty"`
}

func buildLayoutBox(box *layouts.Box) layoutBoxJSON {
	if box == nil {
		return layoutBoxJSON{}
	}
	result := layoutBoxJSON{
		Text:          box.Text,
		X:             box.X,
		Y:             box.Y,
		Width:         box.Width,
		Height:        box.Height,
		ContentX:      box.ContentX,
		ContentY:      box.ContentY,
		ContentWidth:  box.ContentWidth,
		ContentHeight: box.ContentHeight,
		ScrollWidth:   box.ScrollWidth,
		ScrollHeight:  box.ScrollHeight,
		ClipX:         box.ClipX,
		ClipY:         box.ClipY,
	}
	if box.Node != nil && box.Node.Type == dom.ElementNode {
		result.Tag = strings.ToLower(box.Node.Data)
		result.ID, _ = box.Node.GetAttribute("id")
		result.Class, _ = box.Node.GetAttribute("class")
	}
	for _, child := range box.Children {
		result.Children = append(result.Children, buildLayoutBox(child))
	}
	return result
}

func extractRenderer(arguments []string) ([]string, string, error) {
	name := "auto"
	remaining := make([]string, 0, len(arguments))
	for index := 0; index < len(arguments); index++ {
		argument := arguments[index]
		if argument == "--renderer" {
			if index+1 >= len(arguments) {
				return nil, "", fmt.Errorf("--renderer requires a value")
			}
			name = arguments[index+1]
			index++
			continue
		}
		if strings.HasPrefix(argument, "--renderer=") {
			name = strings.TrimPrefix(argument, "--renderer=")
			continue
		}
		remaining = append(remaining, argument)
	}
	return remaining, name, nil
}
