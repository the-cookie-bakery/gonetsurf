//go:build vulkan && cgo

package vulkan_test

import (
	"image"
	"image/png"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	layoutinterfaces "gonetsurf/layouting/interfaces"
	layoutsoftware "gonetsurf/layouting/software"
	"gonetsurf/layouting/vulkan"
	"gonetsurf/parsers/css/cssom"
	"gonetsurf/parsers/html"
	"gonetsurf/rendering"
	renderinginterfaces "gonetsurf/rendering/interfaces"
	renderingsoftware "gonetsurf/rendering/software"
	renderingvulkan "gonetsurf/rendering/vulkan"
	"gonetsurf/styles"
)

// TestWebsiteSnapshotParity renders the vendored cookie.engineer snapshot
// through the software and Vulkan pipelines. It runs without network access and
// is the primary local layout/render regression gate.
func TestWebsiteSnapshotParity(t *testing.T) {
	if os.Getenv("GONETSURF_VULKAN_NO_FALLBACK") == "" && os.Getenv("GONETSURF_E2E") == "" {
		t.Skip("set GONETSURF_VULKAN_NO_FALLBACK=1 (or GONETSURF_E2E=1) to run the offline snapshot parity test")
	}
	source, err := os.ReadFile(filepath.Join("testdata", "cookie-engineer", "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	stylesheet, err := os.ReadFile(filepath.Join("testdata", "cookie-engineer", "combined.css"))
	if err != nil {
		t.Fatal(err)
	}
	render := func(layoutBackend layoutinterfaces.Backend, renderer renderinginterfaces.Renderer) *image.NRGBA {
		t.Helper()
		document, err := html.ParseString(string(source), html.ParseOptions{})
		if err != nil {
			t.Fatal(err)
		}
		engine := styles.NewEngine(document)
		engine.AddStylesheet(cssom.Parse(string(stylesheet)))
		surface, err := rendering.RenderDocumentWithOptions(document, engine, 1024, 768, rendering.Options{
			Renderer:      renderer,
			LayoutBackend: layoutBackend,
		})
		if err != nil {
			t.Fatal(err)
		}
		return surface.Image()
	}
	software := render(layoutsoftware.NewBackend(), renderingsoftware.NewRenderer())
	vulkanLayout, err := vulkan.NewBackend()
	if err != nil {
		t.Fatal(err)
	}
	vulkanRendererBackend, err := renderingvulkan.NewBackend()
	if err != nil {
		t.Fatal(err)
	}
	vulkanRenderer, err := vulkanRendererBackend.NewRenderer()
	if err != nil {
		t.Fatal(err)
	}
	defer vulkanRenderer.Close()
	accelerated := render(vulkanLayout, vulkanRenderer)
	rootMeanSquare, differing := compareImages(software, accelerated)
	t.Logf("offline snapshot parity: rmse=%.4f differing=%.3f%%", rootMeanSquare, differing*100)
	if rootMeanSquare > 0.08 {
		t.Errorf("offline snapshot parity rmse %.4f exceeds 0.08 (%.2f%% pixels differ)", rootMeanSquare, differing*100)
	}
}

// TestWebsiteParity renders a live page through the software and Vulkan
// pipelines and compares the resulting pixels. It is opt-in because it needs
// network access:
//
//	GONETSURF_E2E=1 go test -tags vulkan ./layouting/vulkan/ -run TestWebsiteParity -v
func TestWebsiteParity(t *testing.T) {
	if os.Getenv("GONETSURF_E2E") == "" {
		t.Skip("set GONETSURF_E2E=1 to run the live website layout/render parity test")
	}
	address := os.Getenv("GONETSURF_E2E_URL")
	if address == "" {
		address = "https://cookie.engineer/index.html"
	}
	directory := t.TempDir()
	softwarePath := filepath.Join(directory, "software.png")
	vulkanPath := filepath.Join(directory, "vulkan.png")
	run := func(arguments ...string) {
		command := exec.Command("go", append([]string{"run"}, arguments...)...)
		command.Dir = filepath.Join("..", "..")
		output, err := command.CombinedOutput()
		if err != nil {
			t.Fatalf("%v: %v\n%s", arguments, err, output)
		}
	}
	run("./cmd/netsurfctl", "render", "--renderer=software", address, softwarePath, "1024", "768")
	run("-tags", "vulkan", "./cmd/netsurfctl", "render", "--renderer=vulkan", address, vulkanPath, "1024", "768")
	software := decodePNG(t, softwarePath)
	vulkan := decodePNG(t, vulkanPath)
	rootMeanSquare, differing := compareImages(software, vulkan)
	t.Logf("website parity: %s rmse=%.4f differing=%.3f%%", address, rootMeanSquare, differing*100)
	if rootMeanSquare > 0.08 {
		t.Errorf("website parity rmse %.4f exceeds 0.08 (%.2f%% pixels differ)", rootMeanSquare, differing*100)
	}
}

func decodePNG(t *testing.T, path string) *image.NRGBA {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	decoded, err := png.Decode(file)
	if err != nil {
		t.Fatal(err)
	}
	bounds := decoded.Bounds()
	result := image.NewNRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	for y := 0; y < bounds.Dy(); y++ {
		for x := 0; x < bounds.Dx(); x++ {
			result.Set(x, y, decoded.At(bounds.Min.X+x, bounds.Min.Y+y))
		}
	}
	return result
}

func compareImages(first, second *image.NRGBA) (float64, float64) {
	width := minInt(first.Bounds().Dx(), second.Bounds().Dx())
	height := minInt(first.Bounds().Dy(), second.Bounds().Dy())
	total := float64(width * height)
	squared := 0.0
	differing := 0.0
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			firstOffset := y*first.Stride + x*4
			secondOffset := y*second.Stride + x*4
			different := false
			for channel := 0; channel < 4; channel++ {
				delta := float64(first.Pix[firstOffset+channel]) - float64(second.Pix[secondOffset+channel])
				squared += delta * delta
				if math.Abs(delta) > 8 {
					different = true
				}
			}
			if different {
				differing++
			}
		}
	}
	rootMeanSquare := math.Sqrt(squared / (total * 4 * 255 * 255))
	return rootMeanSquare, differing / total
}

func minInt(first, second int) int {
	if first < second {
		return first
	}
	return second
}
