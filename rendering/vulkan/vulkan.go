//go:build vulkan && cgo

package vulkan

import (
	_ "embed"
	"errors"
	"fmt"
	"image"
	"image/draw"
	"log"
	"math"
	"os"
	"reflect"
	"sync"
	"unsafe"

	layoutinginterfaces "gonetsurf/layouting/interfaces"
	"gonetsurf/rendering/geometry"
	"gonetsurf/rendering/interfaces"
	"gonetsurf/rendering/software"
	"gonetsurf/rendering/vulkan/internal/gpu"
)

// ErrSoftwareFallbackDisabled reports that the Vulkan renderer would have
// needed the software renderer while GONETSURF_VULKAN_NO_FALLBACK is set.
var ErrSoftwareFallbackDisabled = errors.New("vulkan renderer: software fallback disabled")

func fallbackDisabled() bool {
	return os.Getenv("GONETSURF_VULKAN_NO_FALLBACK") != ""
}

//go:embed shaders/solid.vert.spv
var vertexShader []byte

//go:embed shaders/solid.frag.spv
var solidShader []byte

//go:embed shaders/texture.frag.spv
var textureShader []byte

//go:embed shaders/path.comp.spv
var pathShader []byte

type maskInfo struct {
	texture uint64
	bounds  image.Rectangle
}

func (info maskInfo) draw(color geometry.Color, clip geometry.Rect) gpu.Draw {
	return gpu.Draw{
		Rect:    rectFloats(geometry.Rect{X: float64(info.bounds.Min.X), Y: float64(info.bounds.Min.Y), Width: float64(info.bounds.Dx()), Height: float64(info.bounds.Dy())}),
		UV:      [4]float32{0, 0, 1, 1},
		Color:   colorFloats(color),
		Clip:    rectFloats(clip),
		Opacity: 1,
		Mode:    2,
		Texture: info.texture,
	}
}

type glyphPlacement struct {
	x      int
	y      int
	width  int
	height int
}

type glyphAtlas struct {
	key        string
	textureKey string
	size       int
	placements map[string]glyphPlacement
}

type renderCache struct {
	masks           map[string]maskInfo
	gradients       map[string]gpu.Draw
	imageKeys       map[uintptr]string
	imageRefs       map[uintptr]image.Image
	nextImageID     uint64
	atlas           *glyphAtlas
	pairs           map[string]uint64
	paths           map[string]uint64
	geometry        layoutinginterfaces.GeometryHandle
	geometryTexture uint64
}

func newRenderCache() *renderCache {
	return &renderCache{
		masks:     make(map[string]maskInfo),
		gradients: make(map[string]gpu.Draw),
		imageKeys: make(map[uintptr]string),
		imageRefs: make(map[uintptr]image.Image),
		pairs:     make(map[string]uint64),
		paths:     make(map[string]uint64),
	}
}

func hashBytes(hash uint64, data []byte) uint64 {
	for _, value := range data {
		hash ^= uint64(value)
		hash *= 1099511628211
	}
	return hash
}

func hashCoverage(hash uint64, coverage []float64) uint64 {
	for _, value := range coverage {
		bits := math.Float64bits(value)
		for shift := 0; shift < 64; shift += 8 {
			hash ^= (bits >> shift) & 0xff
			hash *= 1099511628211
		}
	}
	return hash
}

func (cache *renderCache) imageKey(source image.Image) string {
	value := reflect.ValueOf(source)
	pointer := uintptr(0)
	if value.IsValid() && value.Kind() == reflect.Ptr {
		pointer = value.Pointer()
	}
	if pointer != 0 {
		if key, ok := cache.imageKeys[pointer]; ok {
			return key
		}
	}
	cache.nextImageID++
	key := fmt.Sprintf("image:%d", cache.nextImageID)
	if pointer != 0 {
		cache.imageKeys[pointer] = key
		cache.imageRefs[pointer] = source
	}
	return key
}

func overlayKey(surface *software.Surface) string {
	return fmt.Sprintf("overlay:%dx%d:%x", surface.Width, surface.Height, hashBytes(1469598103934665603, surface.Pix))
}

func layerKey(surface *software.Surface) string {
	return fmt.Sprintf("layer:%dx%d:%x", surface.Width, surface.Height, hashBytes(1469598103934665603, surface.Pix))
}

func maskContentKey(mask *geometry.Mask) string {
	return fmt.Sprintf("fillmask:%dx%d:%x", mask.Width, mask.Height, hashCoverage(1469598103934665603, mask.Coverage))
}

func glyphContentKey(mask *image.Alpha) string {
	bounds := mask.Bounds()
	hash := hashBytes(1469598103934665603, []byte{byte(bounds.Min.X), byte(bounds.Min.Y), byte(bounds.Dx()), byte(bounds.Dy())})
	return fmt.Sprintf("glyph:%dx%d:%x", bounds.Dx(), bounds.Dy(), hashBytes(hash, mask.Pix))
}

// ErrUnavailable reports that the Vulkan backend was not compiled in.
var ErrUnavailable error

// Available reports whether the Vulkan backend is compiled in.
func Available() bool { return true }

// Backend constructs Vulkan renderers.
type Backend struct{}

// NewBackend returns the Vulkan backend.
func NewBackend() (interfaces.Backend, error) {
	return &Backend{}, nil
}

// Name returns the backend name.
func (backend *Backend) Name() string { return "vulkan" }

// NewRenderer returns a Vulkan renderer.
func (backend *Backend) NewRenderer() (interfaces.Renderer, error) {
	return &Renderer{fallback: software.NewRenderer()}, nil
}

// Renderer renders scenes with an offscreen Vulkan target and reads the
// result back into a CPU surface. Geometry and coverage masks are produced on
// the CPU; compositing, solid fills, gradients, images, and glyphs are drawn
// by the GPU.
type contextKey struct {
	width  int
	height int
}

type Renderer struct {
	mutex          sync.Mutex
	fallback       interfaces.Renderer
	contexts       map[contextKey]*gpu.Context
	caches         map[contextKey]*renderCache
	gpuErr         error
	fallbackLogged bool
	geometry       layoutinginterfaces.GeometryHandle
}

// Name returns the renderer name.
func (renderer *Renderer) Name() string { return "vulkan" }

// Render executes a scene and returns a read-back surface.
func (renderer *Renderer) Render(scene *interfaces.Scene) (interfaces.Surface, error) {
	if scene == nil {
		return software.NewSurface(0, 0), nil
	}
	renderer.mutex.Lock()
	if renderer.gpuErr != nil || !supportsGPU(scene) {
		if fallbackDisabled() {
			renderer.mutex.Unlock()
			if renderer.gpuErr != nil {
				return nil, fmt.Errorf("%w: %v", ErrSoftwareFallbackDisabled, renderer.gpuErr)
			}
			return nil, fmt.Errorf("%w: scene contains commands not yet accelerated", ErrSoftwareFallbackDisabled)
		}
		if !renderer.fallbackLogged {
			if renderer.gpuErr != nil {
				log.Printf("vulkan: GPU path disabled: %v", renderer.gpuErr)
			} else {
				log.Printf("vulkan: scene contains commands not yet accelerated; using software fallback")
			}
			renderer.fallbackLogged = true
		}
		renderer.mutex.Unlock()
		return renderer.fallback.Render(scene)
	}
	if renderer.contexts == nil {
		renderer.contexts = make(map[contextKey]*gpu.Context)
		renderer.caches = make(map[contextKey]*renderCache)
	}
	key := contextKey{width: scene.Width, height: scene.Height}
	context := renderer.contexts[key]
	cache := renderer.caches[key]
	if context == nil {
		created, err := gpu.New(scene.Width, scene.Height, vertexShader, solidShader, textureShader)
		if err != nil {
			renderer.gpuErr = err
			renderer.mutex.Unlock()
			if fallbackDisabled() {
				return nil, fmt.Errorf("%w: %v", ErrSoftwareFallbackDisabled, err)
			}
			return renderer.fallback.Render(scene)
		}
		if err := created.CreatePathPipeline(pathShader); err != nil {
			renderer.gpuErr = err
			renderer.mutex.Unlock()
			return nil, err
		}
		context = created
		renderer.contexts[key] = context
		cache = newRenderCache()
		renderer.caches[key] = cache
		log.Printf("vulkan: offscreen GPU target %dx%d ready", scene.Width, scene.Height)
	}
	if renderer.geometry != nil && cache.geometryTexture == 0 {
		texture, err := context.GeometryTexture(renderer.geometry.Buffer())
		if err != nil {
			renderer.mutex.Unlock()
			return nil, err
		}
		cache.geometry = renderer.geometry
		cache.geometryTexture = texture
	}
	if err := context.BeginLayers(); err != nil {
		renderer.mutex.Unlock()
		return nil, err
	}
	clear, draws, err := buildDraws(context, scene, cache)
	if err != nil {
		renderer.mutex.Unlock()
		if fallbackDisabled() {
			return nil, fmt.Errorf("%w: %v", ErrSoftwareFallbackDisabled, err)
		}
		return renderer.fallback.Render(scene)
	}
	output := make([]byte, scene.Width*scene.Height*4)
	if err := context.Render(clear, draws, output); err != nil {
		renderer.gpuErr = err
		log.Printf("vulkan: GPU render failed: %v", err)
		renderer.mutex.Unlock()
		if fallbackDisabled() {
			return nil, fmt.Errorf("%w: %v", ErrSoftwareFallbackDisabled, err)
		}
		return renderer.fallback.Render(scene)
	}
	surface := software.NewSurface(scene.Width, scene.Height)
	copy(surface.Pix, output)
	if context.TextureCount() > 2048 {
		context.Destroy()
		delete(renderer.contexts, key)
		delete(renderer.caches, key)
	}
	renderer.mutex.Unlock()
	return surface, nil
}

// SetGeometry binds backend-resident layout geometry for node draws.
func (renderer *Renderer) SetGeometry(handle layoutinginterfaces.GeometryHandle) error {
	renderer.mutex.Lock()
	defer renderer.mutex.Unlock()
	renderer.geometry = handle
	if handle == nil {
		return nil
	}
	for key, context := range renderer.contexts {
		texture, err := context.GeometryTexture(handle.Buffer())
		if err != nil {
			return err
		}
		if cache := renderer.caches[key]; cache != nil {
			cache.geometry = handle
			cache.geometryTexture = texture
		}
	}
	return nil
}

// AttachPresenter creates a Vulkan swapchain for the native window.
func (renderer *Renderer) AttachPresenter(display string, window uintptr, width, height int) error {
	renderer.mutex.Lock()
	defer renderer.mutex.Unlock()
	if renderer.contexts == nil {
		renderer.contexts = make(map[contextKey]*gpu.Context)
		renderer.caches = make(map[contextKey]*renderCache)
	}
	key := contextKey{width: width, height: height}
	context := renderer.contexts[key]
	if context == nil {
		created, err := gpu.New(width, height, vertexShader, solidShader, textureShader)
		if err != nil {
			return err
		}
		if err := created.CreatePathPipeline(pathShader); err != nil {
			return err
		}
		context = created
		renderer.contexts[key] = context
		renderer.caches[key] = newRenderCache()
	}
	return context.AttachSwapchain(display, window, width, height)
}

// PresentSurface presents a completed surface through the Vulkan swapchain.
func (renderer *Renderer) PresentSurface(surface interfaces.Surface, damage image.Rectangle) error {
	if surface == nil {
		return fmt.Errorf("vulkan: nil present surface")
	}
	pixelSurface, ok := surface.(interfaces.PixelSurface)
	if !ok {
		return fmt.Errorf("vulkan: surface is not pixel-backed")
	}
	width, height := surface.Size()
	pixels, stride := pixelSurface.Premultiplied()
	if stride != width*4 {
		compact := make([]byte, width*height*4)
		for row := 0; row < height; row++ {
			copy(compact[row*width*4:(row+1)*width*4], pixels[row*stride:row*stride+width*4])
		}
		pixels = compact
	}
	renderer.mutex.Lock()
	defer renderer.mutex.Unlock()
	context := renderer.contexts[contextKey{width: width, height: height}]
	if context == nil {
		return fmt.Errorf("vulkan: no swapchain context")
	}
	return context.PresentSurface(pixels, width, height)
}

// Close releases renderer resources.
func (renderer *Renderer) Close() error {
	renderer.mutex.Lock()
	defer renderer.mutex.Unlock()
	for _, context := range renderer.contexts {
		context.Destroy()
	}
	renderer.contexts = nil
	renderer.caches = nil
	if renderer.fallback != nil {
		return renderer.fallback.Close()
	}
	return nil
}

func supportsGPU(scene *interfaces.Scene) bool {
	for _, command := range scene.Commands {
		switch command.Kind {
		case interfaces.CommandClear,
			interfaces.CommandCompositeLayer,
			interfaces.CommandCompositeLayerMasked,
			interfaces.CommandFillRect,
			interfaces.CommandFillPath,
			interfaces.CommandStrokePath,
			interfaces.CommandFillMask,
			interfaces.CommandFillGradient,
			interfaces.CommandDrawImage,
			interfaces.CommandDrawGlyph,
			interfaces.CommandBlendPixel,
			interfaces.CommandSetClip,
			interfaces.CommandResetClip:
		default:
			return false
		}
	}
	return scene.Width > 0 && scene.Height > 0
}

func buildGlyphAtlas(context *gpu.Context, scene *interfaces.Scene, cache *renderCache) error {
	masks := make([]*image.Alpha, 0, len(scene.Commands))
	keys := make([]string, 0, len(scene.Commands))
	seen := make(map[string]bool)
	for _, command := range scene.Commands {
		if command.Kind != interfaces.CommandDrawGlyph || command.Glyph.Mask == nil {
			continue
		}
		key := glyphContentKey(command.Glyph.Mask)
		if seen[key] {
			continue
		}
		seen[key] = true
		keys = append(keys, key)
		masks = append(masks, command.Glyph.Mask)
	}
	if len(masks) == 0 {
		return nil
	}
	hash := uint64(1469598103934665603)
	for _, key := range keys {
		hash = hashBytes(hash, []byte(key))
	}
	key := fmt.Sprintf("%x", hash)
	if cache.atlas != nil && cache.atlas.key == key {
		return nil
	}
	area := 0
	for _, mask := range masks {
		bounds := mask.Bounds()
		area += (bounds.Dx() + 2) * (bounds.Dy() + 2)
	}
	size := 256
	for size < 2048 && size*size < area {
		size *= 2
	}
	if size*size < area {
		return fmt.Errorf("vulkan: glyph atlas exceeds %dx%d", size, size)
	}
	pixels := make([]byte, size*size)
	placements := make(map[string]glyphPlacement, len(masks))
	xPos := 1
	yPos := 1
	rowHeight := 0
	for index, mask := range masks {
		bounds := mask.Bounds()
		width := bounds.Dx()
		height := bounds.Dy()
		if xPos+width+1 >= size {
			xPos = 1
			yPos += rowHeight + 1
			rowHeight = 0
		}
		if yPos+height+1 >= size {
			return fmt.Errorf("vulkan: glyph atlas packing overflow")
		}
		for y := 0; y < height; y++ {
			copy(pixels[(yPos+y)*size+xPos:(yPos+y)*size+xPos+width], mask.Pix[y*mask.Stride:y*mask.Stride+width])
		}
		placements[keys[index]] = glyphPlacement{x: xPos, y: yPos, width: width, height: height}
		xPos += width + 1
		if height > rowHeight {
			rowHeight = height
		}
	}
	textureKey := "glyph-atlas:" + key
	if _, err := context.UploadTexture(textureKey, pixels, size, size, 0); err != nil {
		return err
	}
	cache.atlas = &glyphAtlas{key: key, textureKey: textureKey, size: size, placements: placements}
	return nil
}

func buildDraws(context *gpu.Context, scene *interfaces.Scene, cache *renderCache) ([4]float32, []gpu.Draw, error) {
	full := geometry.Rect{Width: float64(scene.Width), Height: float64(scene.Height)}
	clip := full
	clear := [4]float32{}
	draws := make([]gpu.Draw, 0, len(scene.Commands))
	whiteTexture, err := context.UploadTexture("solid-white", []byte{255, 255, 255, 255}, 1, 1, 1)
	if err != nil {
		return clear, nil, err
	}
	if err := buildGlyphAtlas(context, scene, cache); err != nil {
		return clear, nil, err
	}
	var overlay *software.Surface

	flushOverlay := func() error {
		if overlay == nil {
			return nil
		}
		handle, err := context.UploadTexture(overlayKey(overlay), overlay.Pix, overlay.Width, overlay.Height, 2)
		overlay = nil
		if err != nil {
			return err
		}
		draws = append(draws, gpu.Draw{
			Rect:    rectFloats(full),
			UV:      [4]float32{0, 0, 1, 1},
			Color:   [4]float32{1, 1, 1, 1},
			Clip:    rectFloats(clip),
			Opacity: 1,
			Mode:    3,
			Texture: handle,
		})
		return nil
	}

	for _, command := range scene.Commands {
		switch command.Kind {
		case interfaces.CommandClear:
			if len(draws) > 0 {
				return clear, nil, fmt.Errorf("vulkan: clear after drawings is unsupported")
			}
			clear = clearFloats(command.Color)
		case interfaces.CommandSetClip:
			if err := flushOverlay(); err != nil {
				return clear, nil, err
			}
			clip = command.Rect.Intersect(full)
		case interfaces.CommandResetClip:
			if err := flushOverlay(); err != nil {
				return clear, nil, err
			}
			clip = full
		case interfaces.CommandFillRect:
			if err := flushOverlay(); err != nil {
				return clear, nil, err
			}
			if command.UseNode && command.NodeIndex >= 0 && cache.geometry != nil && cache.geometryTexture != 0 {
				xField, yField, widthField, heightField := cache.geometry.FieldOffsets()
				draws = append(draws, gpu.Draw{
					Rect:    rectFloats(geometry.Rect{Width: float64(scene.Width), Height: float64(scene.Height)}),
					UV:      [4]float32{0, 0, 1, 1},
					Color:   colorFloats(command.Color),
					Clip:    rectFloats(clip),
					Opacity: 1,
					Mode:    6,
					Texture: cache.geometryTexture,
					Transform: [6]float32{
						float32(command.NodeIndex), float32(cache.geometry.Stride()),
						float32(xField), float32(yField), float32(widthField), float32(heightField),
					},
				})
				continue
			}
			rect := command.Rect.Intersect(clip)
			if rect.IsEmpty() {
				continue
			}
			draws = append(draws, gpu.Draw{
				Rect:    rectFloats(rect),
				UV:      [4]float32{0, 0, 1, 1},
				Color:   colorFloats(command.Color),
				Clip:    rectFloats(clip),
				Opacity: 1,
				Mode:    0,
				Texture: whiteTexture,
			})
		case interfaces.CommandFillPath:
			if err := flushOverlay(); err != nil {
				return clear, nil, err
			}
			key := pathKey(command.Path, command.FillRule, false, geometry.StrokeStyle{}, scene.Width, scene.Height)
			handle, ok := cache.paths[key]
			if !ok {
				rule := 0
				if command.FillRule == geometry.EvenOddFill {
					rule = 1
				}
				rasterized, err := context.RasterizePath(fillEdges(command.Path), scene.Width, scene.Height, rule)
				if err != nil {
					return clear, nil, err
				}
				handle = rasterized
				cache.paths[key] = handle
			}
			draws = append(draws, pathDraw(handle, command.Color, clip, scene.Width, scene.Height))
		case interfaces.CommandStrokePath:
			if err := flushOverlay(); err != nil {
				return clear, nil, err
			}
			key := pathKey(command.Path, geometry.NonZeroFill, true, command.Stroke, scene.Width, scene.Height)
			handle, ok := cache.paths[key]
			if !ok {
				rasterized, err := context.RasterizePath(strokeEdges(command.Path, command.Stroke), scene.Width, scene.Height, 0)
				if err != nil {
					return clear, nil, err
				}
				handle = rasterized
				cache.paths[key] = handle
			}
			draws = append(draws, pathDraw(handle, command.Color, clip, scene.Width, scene.Height))
		case interfaces.CommandFillMask:
			if err := flushOverlay(); err != nil {
				return clear, nil, err
			}
			draw, ok, err := maskDraw(context, command.Mask, command.Color, clip, maskContentKey(command.Mask))
			if err != nil {
				return clear, nil, err
			}
			if ok {
				draws = append(draws, draw)
			}
		case interfaces.CommandFillGradient:
			if err := flushOverlay(); err != nil {
				return clear, nil, err
			}
			gradientClip := command.Clip
			area := command.Rect.Intersect(gradientClip)
			key := gradientKey(command.Gradient, area) + gradientMaskKey(command.Mask)
			if cached, ok := cache.gradients[key]; ok {
				cached.Clip = rectFloats(gradientClip)
				draws = append(draws, cached)
				continue
			}
			draw, ok, err := gradientDraw(context, command, gradientClip, key)
			if err != nil {
				return clear, nil, err
			}
			if ok {
				cache.gradients[key] = draw
				draws = append(draws, draw)
			}
		case interfaces.CommandDrawImage:
			if err := flushOverlay(); err != nil {
				return clear, nil, err
			}
			draw, ok, err := imageDraw(context, command, command.Clip, cache)
			if err != nil {
				return clear, nil, err
			}
			if ok {
				draws = append(draws, draw)
			}
		case interfaces.CommandDrawGlyph:
			if err := flushOverlay(); err != nil {
				return clear, nil, err
			}
			draw, ok, err := glyphDraw(context, command, clip, cache)
			if err != nil {
				return clear, nil, err
			}
			if ok {
				draws = append(draws, draw)
			}
		case interfaces.CommandCompositeLayer, interfaces.CommandCompositeLayerMasked:
			if err := flushOverlay(); err != nil {
				return clear, nil, err
			}
			draw, ok, err := layerDraw(context, command, clip, scene.Width, scene.Height, cache)
			if err != nil {
				return clear, nil, err
			}
			if ok {
				draws = append(draws, draw)
			}
		case interfaces.CommandBlendPixel:
			if overlay == nil {
				overlay = software.NewSurface(scene.Width, scene.Height)
			}
			overlay.BlendPixel(command.X, command.Y, command.Color, command.Coverage)
		}
	}
	if err := flushOverlay(); err != nil {
		return clear, nil, err
	}
	return clear, draws, nil
}

func intersectDrawClip(current [4]float32, clip geometry.Rect) [4]float32 {
	minimumX := math.Max(float64(current[0]), clip.X)
	minimumY := math.Max(float64(current[1]), clip.Y)
	maximumX := math.Min(float64(current[0]+current[2]), clip.X+clip.Width)
	maximumY := math.Min(float64(current[1]+current[3]), clip.Y+clip.Height)
	if maximumX <= minimumX || maximumY <= minimumY {
		return [4]float32{}
	}
	return [4]float32{float32(minimumX), float32(minimumY), float32(maximumX - minimumX), float32(maximumY - minimumY)}
}

func composeDrawTransform(draw *gpu.Draw, outer geometry.Affine) {
	if outer == geometry.IdentityAffine() {
		return
	}
	if !draw.Transformed {
		draw.Transform = [6]float32{float32(outer.A), float32(outer.B), float32(outer.C), float32(outer.D), float32(outer.E), float32(outer.F)}
		draw.Transformed = true
		return
	}
	inner := geometry.Affine{
		A: float64(draw.Transform[0]),
		B: float64(draw.Transform[1]),
		C: float64(draw.Transform[2]),
		D: float64(draw.Transform[3]),
		E: float64(draw.Transform[4]),
		F: float64(draw.Transform[5]),
	}
	composed := outer.Multiply(inner)
	draw.Transform = [6]float32{float32(composed.A), float32(composed.B), float32(composed.C), float32(composed.D), float32(composed.E), float32(composed.F)}
}

func layerDraw(context *gpu.Context, command interfaces.Command, clip geometry.Rect, width, height int, cache *renderCache) (gpu.Draw, bool, error) {
	if command.Layer == nil || command.Opacity <= 0 {
		return gpu.Draw{}, false, nil
	}
	if command.Kind == interfaces.CommandCompositeLayer && command.Transform == (geometry.Matrix4{}) {
		return gpu.Draw{}, false, nil
	}
	layerScene := &interfaces.Scene{Width: width, Height: height, Commands: command.Layer.Commands}
	_, layerDraws, err := buildDraws(context, layerScene, cache)
	if err != nil {
		return gpu.Draw{}, false, err
	}
	layerHandle, err := context.RenderLayer([4]float32{}, layerDraws)
	if err != nil {
		return gpu.Draw{}, false, err
	}
	texture := layerHandle
	mode := 3
	if command.Kind == interfaces.CommandCompositeLayerMasked && command.Mask != nil {
		maskHandle, err := uploadLayerMask(context, command.Mask, width, height)
		if err != nil {
			return gpu.Draw{}, false, err
		}
		pairKey := fmt.Sprintf("%d:%d", layerHandle, maskHandle)
		if pair, ok := cache.pairs[pairKey]; ok {
			texture = pair
		} else {
			pair, err := context.PairTexture(layerHandle, maskHandle)
			if err != nil {
				return gpu.Draw{}, false, err
			}
			cache.pairs[pairKey] = pair
			texture = pair
		}
		mode = 5
	}
	draw := gpu.Draw{
		Rect:    rectFloats(geometry.Rect{Width: float64(width), Height: float64(height)}),
		UV:      [4]float32{0, 0, 1, 1},
		Color:   [4]float32{1, 1, 1, 1},
		Clip:    rectFloats(clip),
		Opacity: float32(command.Opacity),
		Mode:    mode,
		Texture: texture,
	}
	if command.Kind == interfaces.CommandCompositeLayer && !command.Transform.IsIdentity() {
		affine := command.Transform.Affine2D()
		draw.Transform = [6]float32{
			float32(affine.A), float32(affine.B), float32(affine.C),
			float32(affine.D), float32(affine.E), float32(affine.F),
		}
		draw.Transformed = true
	}
	return draw, true, nil
}

func uploadLayerMask(context *gpu.Context, mask *geometry.Mask, width, height int) (uint64, error) {
	if mask == nil {
		return 0, nil
	}
	pixels := make([]byte, width*height)
	for yPos := 0; yPos < height; yPos++ {
		for xPos := 0; xPos < width; xPos++ {
			coverage := mask.At(xPos, yPos)
			if coverage < 0 {
				coverage = 0
			}
			if coverage > 1 {
				coverage = 1
			}
			pixels[yPos*width+xPos] = uint8(math.Round(coverage * 255))
		}
	}
	key := fmt.Sprintf("layer-mask:%dx%d:%x", width, height, hashBytes(1469598103934665603, pixels))
	return context.UploadTexture(key, pixels, width, height, 0)
}

func messageCoverage(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}

func maskDraw(context *gpu.Context, mask *geometry.Mask, color geometry.Color, clip geometry.Rect, key string) (gpu.Draw, bool, error) {
	pixels, bounds, ok := maskPixels(mask)
	if !ok {
		return gpu.Draw{}, false, nil
	}
	handle, err := context.UploadTexture(key, pixels, bounds.Dx(), bounds.Dy(), 0)
	if err != nil {
		return gpu.Draw{}, false, err
	}
	return gpu.Draw{
		Rect:    rectFloats(geometry.Rect{X: float64(bounds.Min.X), Y: float64(bounds.Min.Y), Width: float64(bounds.Dx()), Height: float64(bounds.Dy())}),
		UV:      [4]float32{0, 0, 1, 1},
		Color:   colorFloats(color),
		Clip:    rectFloats(clip),
		Opacity: 1,
		Mode:    2,
		Texture: handle,
	}, true, nil
}

func imageDraw(context *gpu.Context, command interfaces.Command, clip geometry.Rect, cache *renderCache) (gpu.Draw, bool, error) {
	original := command.Rect
	destination := original.Intersect(clip)
	if destination.IsEmpty() || command.Image == nil || original.Width <= 0 || original.Height <= 0 {
		return gpu.Draw{}, false, nil
	}
	key := cache.imageKey(command.Image)
	if handle, ok := context.Texture(key); ok {
		return imageDrawWithTexture(handle, command, clip), true, nil
	}
	pixels, width, height, err := imagePixels(command.Image)
	if err != nil || width == 0 || height == 0 {
		return gpu.Draw{}, false, err
	}
	handle, err := context.UploadTexture(key, pixels, width, height, 1)
	if err != nil {
		return gpu.Draw{}, false, err
	}
	u0 := float32((destination.X - original.X) / original.Width)
	v0 := float32((destination.Y - original.Y) / original.Height)
	u1 := float32((destination.X + destination.Width - original.X) / original.Width)
	v1 := float32((destination.Y + destination.Height - original.Y) / original.Height)
	return gpu.Draw{
		Rect:    rectFloats(destination),
		UV:      [4]float32{u0, v0, u1, v1},
		Color:   [4]float32{1, 1, 1, 1},
		Clip:    rectFloats(clip),
		Opacity: float32(command.Opacity),
		Mode:    1,
		Texture: handle,
	}, true, nil
}

func imageDrawWithTexture(handle uint64, command interfaces.Command, clip geometry.Rect) gpu.Draw {
	original := command.Rect
	destination := original.Intersect(clip)
	u0 := float32((destination.X - original.X) / original.Width)
	v0 := float32((destination.Y - original.Y) / original.Height)
	u1 := float32((destination.X + destination.Width - original.X) / original.Width)
	v1 := float32((destination.Y + destination.Height - original.Y) / original.Height)
	return gpu.Draw{
		Rect:    rectFloats(destination),
		UV:      [4]float32{u0, v0, u1, v1},
		Color:   [4]float32{1, 1, 1, 1},
		Clip:    rectFloats(clip),
		Opacity: float32(command.Opacity),
		Mode:    1,
		Texture: handle,
	}
}

func glyphDraw(context *gpu.Context, command interfaces.Command, clip geometry.Rect, cache *renderCache) (gpu.Draw, bool, error) {
	mask := command.Glyph.Mask
	if mask == nil {
		return gpu.Draw{}, false, nil
	}
	bounds := mask.Bounds()
	destination := geometry.Rect{X: float64(command.Glyph.X + bounds.Min.X), Y: float64(command.Glyph.Y + bounds.Min.Y), Width: float64(bounds.Dx()), Height: float64(bounds.Dy())}
	if destination.Intersect(clip).IsEmpty() {
		return gpu.Draw{}, false, nil
	}
	key := glyphContentKey(mask)
	if cache != nil && cache.atlas != nil {
		if placement, ok := cache.atlas.placements[key]; ok {
			size := float32(cache.atlas.size)
			u0 := float32(placement.x) / size
			v0 := float32(placement.y) / size
			u1 := float32(placement.x+placement.width) / size
			v1 := float32(placement.y+placement.height) / size
			return gpu.Draw{
				Rect:    rectFloats(destination),
				UV:      [4]float32{u0, v0, u1, v1},
				Color:   colorFloats(command.Glyph.Color),
				Clip:    rectFloats(clip),
				Opacity: 1,
				Mode:    2,
				Texture: mustTexture(context, cache.atlas.textureKey),
			}, true, nil
		}
	}
	handle, err := context.UploadTexture(key, mask.Pix, bounds.Dx(), bounds.Dy(), 0)
	if err != nil {
		return gpu.Draw{}, false, err
	}
	return gpu.Draw{
		Rect:    rectFloats(destination),
		UV:      [4]float32{0, 0, 1, 1},
		Color:   colorFloats(command.Glyph.Color),
		Clip:    rectFloats(clip),
		Opacity: 1,
		Mode:    2,
		Texture: handle,
	}, true, nil
}

func mustTexture(context *gpu.Context, key string) uint64 {
	handle, _ := context.Texture(key)
	return handle
}

func gradientDraw(context *gpu.Context, command interfaces.Command, clip geometry.Rect, key string) (gpu.Draw, bool, error) {
	area := command.Rect.Intersect(clip)
	if area.IsEmpty() {
		return gpu.Draw{}, false, nil
	}
	minimumX := int(math.Max(0, math.Floor(area.X)))
	minimumY := int(math.Max(0, math.Floor(area.Y)))
	maximumX := int(math.Ceil(area.X + area.Width))
	maximumY := int(math.Ceil(area.Y + area.Height))
	maskWidth := maximumX - minimumX
	maskHeight := maximumY - minimumY
	if maskWidth <= 0 || maskHeight <= 0 {
		return gpu.Draw{}, false, nil
	}
	data := gradientData(command.Gradient)
	data[4] = float32(0)
	data[5] = 0
	data[6] = float32(minimumX)
	data[7] = float32(minimumY)
	data[8] = float32(maskWidth)
	data[9] = float32(maskHeight)
	masked := command.Mask != nil
	if masked {
		data[3] = 1
	} else {
		data[3] = 0
	}
	textureWidth := len(data)
	textureHeight := 1
	if masked && maskWidth > textureWidth {
		textureWidth = maskWidth
	}
	if masked {
		textureHeight = maskHeight + 1
	}
	data[4] = float32(textureWidth)
	data[5] = float32(textureHeight)
	pixels := make([]byte, textureWidth*textureHeight*4)
	copy(pixels, floatsToTextureBytes(data))
	if masked {
		for yPos := 0; yPos < maskHeight; yPos++ {
			for xPos := 0; xPos < maskWidth; xPos++ {
				coverage := command.Mask.At(minimumX+xPos, minimumY+yPos)
				if coverage < 0 {
					coverage = 0
				}
				if coverage > 1 {
					coverage = 1
				}
				offset := ((1+yPos)*textureWidth + xPos) * 4
				pixels[offset] = uint8(math.Round(coverage * 255))
			}
		}
	}
	handle, err := context.UploadTexture("gradient-data:"+key, pixels, textureWidth, textureHeight, 1)
	if err != nil {
		return gpu.Draw{}, false, err
	}
	return gpu.Draw{
		Rect:    rectFloats(area),
		UV:      [4]float32{float32(textureWidth), float32(textureHeight), float32(textureWidth), float32(textureHeight)},
		Color:   [4]float32{1, 1, 1, 1},
		Clip:    rectFloats(clip),
		Opacity: 1,
		Mode:    4,
		Texture: handle,
	}, true, nil
}

func floatsToTextureBytes(values []float32) []byte {
	if len(values) == 0 {
		return nil
	}
	return unsafe.Slice((*byte)(unsafe.Pointer(&values[0])), len(values)*4)
}

func gradientMaskKey(mask *geometry.Mask) string {
	if mask == nil {
		return ":none"
	}
	return fmt.Sprintf(":%x", hashCoverage(1469598103934665603, mask.Coverage))
}

func gradientData(gradient geometry.Gradient) []float32 {
	data := make([]float32, 10, 10+8+len(gradient.Stops)*5)
	data[0] = float32(gradient.Kind)
	if gradient.Repeating {
		data[1] = 1
	}
	data[2] = float32(len(gradient.Stops))
	data = append(data,
		float32(gradient.CenterX), float32(gradient.CenterY),
		float32(gradient.DirectionX), float32(gradient.DirectionY),
		float32(gradient.LineLength), float32(gradient.RadiusX),
		float32(gradient.RadiusY), float32(gradient.FromAngle))
	for _, stop := range gradient.Stops {
		data = append(data, float32(stop.Position),
			float32(stop.Color.Red), float32(stop.Color.Green),
			float32(stop.Color.Blue), float32(stop.Color.Alpha))
	}
	return data
}

func gradientKey(gradient geometry.Gradient, area geometry.Rect) string {
	return fmt.Sprintf("%v:%f:%f:%f:%f:%f:%f:%f:%d,%d,%d,%d", gradient.Stops, gradient.CenterX, gradient.CenterY, gradient.DirectionX, gradient.DirectionY, gradient.LineLength, gradient.RadiusX, gradient.RadiusY, int(area.X), int(area.Y), int(area.Width), int(area.Height))
}

func drawBounds(draw gpu.Draw) image.Rectangle {
	return image.Rect(int(draw.Rect[0]), int(draw.Rect[1]), int(draw.Rect[0]+draw.Rect[2]), int(draw.Rect[1]+draw.Rect[3]))
}

func maskPixels(mask *geometry.Mask) ([]byte, image.Rectangle, bool) {
	if mask == nil || mask.Width <= 0 || mask.Height <= 0 {
		return nil, image.Rectangle{}, false
	}
	minimumX := mask.Width
	minimumY := mask.Height
	maximumX := -1
	maximumY := -1
	for yPos := 0; yPos < mask.Height; yPos++ {
		for xPos := 0; xPos < mask.Width; xPos++ {
			if mask.Coverage[yPos*mask.Width+xPos] <= 0 {
				continue
			}
			if xPos < minimumX {
				minimumX = xPos
			}
			if yPos < minimumY {
				minimumY = yPos
			}
			if xPos > maximumX {
				maximumX = xPos
			}
			if yPos > maximumY {
				maximumY = yPos
			}
		}
	}
	if maximumX < minimumX || maximumY < minimumY {
		return nil, image.Rectangle{}, false
	}
	width := maximumX - minimumX + 1
	height := maximumY - minimumY + 1
	pixels := make([]byte, width*height)
	for yPos := 0; yPos < height; yPos++ {
		for xPos := 0; xPos < width; xPos++ {
			pixels[yPos*width+xPos] = uint8(math.Round(math.Min(1, mask.Coverage[(minimumY+yPos)*mask.Width+minimumX+xPos]) * 255))
		}
	}
	return pixels, image.Rect(minimumX, minimumY, maximumX+1, maximumY+1), true
}

func imagePixels(source image.Image) ([]byte, int, int, error) {
	bounds := source.Bounds()
	if converted, ok := source.(*image.NRGBA); ok && converted.Rect.Min.X == 0 && converted.Rect.Min.Y == 0 && converted.Stride == bounds.Dx()*4 {
		return converted.Pix, bounds.Dx(), bounds.Dy(), nil
	}
	converted := image.NewNRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	draw.Draw(converted, converted.Bounds(), source, bounds.Min, draw.Src)
	return converted.Pix, converted.Bounds().Dx(), converted.Bounds().Dy(), nil
}

func pathDraw(handle uint64, color geometry.Color, clip geometry.Rect, width, height int) gpu.Draw {
	return gpu.Draw{
		Rect:    [4]float32{0, 0, float32(width), float32(height)},
		UV:      [4]float32{0, 0, 1, 1},
		Color:   colorFloats(color),
		Clip:    rectFloats(clip),
		Opacity: 1,
		Mode:    2,
		Texture: handle,
	}
}

func appendEdge(edges []float32, start, end geometry.Point) []float32 {
	if start.X == end.X && start.Y == end.Y {
		return edges
	}
	winding := float32(1)
	if end.Y < start.Y {
		winding = -1
	}
	return append(edges, float32(start.X), float32(start.Y), float32(end.X), float32(end.Y), winding)
}

func fillEdges(path *geometry.Path) []float32 {
	if path == nil {
		return nil
	}
	var edges []float32
	for _, subpath := range path.Flatten(0.25) {
		points := subpath.Points
		if len(points) < 2 {
			continue
		}
		for index := 0; index+1 < len(points); index++ {
			edges = appendEdge(edges, points[index], points[index+1])
		}
		last := points[len(points)-1]
		first := points[0]
		edges = appendEdge(edges, last, first)
	}
	return edges
}

func appendPolygonEdges(edges []float32, points []geometry.Point) []float32 {
	if len(points) < 3 {
		return edges
	}
	for index := range points {
		edges = appendEdge(edges, points[index], points[(index+1)%len(points)])
	}
	return edges
}

func appendCircleEdges(edges []float32, center geometry.Point, radius float64) []float32 {
	const segments = 24
	if radius <= 0 {
		return edges
	}
	points := make([]geometry.Point, segments)
	for index := 0; index < segments; index++ {
		angle := 2 * math.Pi * float64(index) / segments
		points[index] = geometry.Point{X: center.X + radius*math.Cos(angle), Y: center.Y + radius*math.Sin(angle)}
	}
	return appendPolygonEdges(edges, points)
}

func strokeEdges(path *geometry.Path, style geometry.StrokeStyle) []float32 {
	if path == nil || style.Width <= 0 {
		return nil
	}
	half := style.Width / 2
	var edges []float32
	for _, subpath := range path.Flatten(0.25) {
		points := subpath.Points
		if len(points) == 0 {
			continue
		}
		if len(points) == 1 {
			if style.Cap == geometry.RoundCap || style.Cap == geometry.SquareCap {
				edges = appendCircleEdges(edges, points[0], half)
			}
			continue
		}
		segments := len(points) - 1
		if subpath.Closed {
			segments = len(points)
		}
		for index := 0; index < segments; index++ {
			start := points[index]
			end := points[(index+1)%len(points)]
			direction := end.Subtract(start)
			if direction.Length() == 0 {
				continue
			}
			normal := direction.Normalized().LeftNormal().Scaled(half)
			polygon := []geometry.Point{
				start.Add(normal), end.Add(normal), end.Subtract(normal), start.Subtract(normal),
			}
			edges = appendPolygonEdges(edges, polygon)
		}
		if !subpath.Closed {
			if style.Cap == geometry.RoundCap || style.Cap == geometry.SquareCap {
				edges = appendCircleEdges(edges, points[0], half)
				edges = appendCircleEdges(edges, points[len(points)-1], half)
			}
		} else if style.Join == geometry.RoundJoin {
			for _, point := range points {
				edges = appendCircleEdges(edges, point, half)
			}
		}
	}
	return edges
}

func pathKey(path *geometry.Path, rule geometry.FillRule, stroke bool, style geometry.StrokeStyle, width, height int) string {
	hash := uint64(1469598103934665603)
	mix := func(value float64) {
		bits := math.Float64bits(value)
		for shift := 0; shift < 64; shift += 8 {
			hash ^= (bits >> shift) & 0xff
			hash *= 1099511628211
		}
	}
	for _, subpath := range path.Flatten(0.25) {
		for _, point := range subpath.Points {
			mix(point.X)
			mix(point.Y)
		}
	}
	mix(float64(rule))
	mix(float64(width))
	mix(float64(height))
	if stroke {
		mix(style.Width)
		mix(float64(style.Cap))
		mix(float64(style.Join))
		mix(style.MiterLimit)
	}
	return fmt.Sprintf("path:%x:%t", hash, stroke)
}

func rectFloats(rect geometry.Rect) [4]float32 {
	return [4]float32{float32(rect.X), float32(rect.Y), float32(rect.Width), float32(rect.Height)}
}

func colorFloats(color geometry.Color) [4]float32 {
	return [4]float32{float32(color.Red) / 255, float32(color.Green) / 255, float32(color.Blue) / 255, float32(color.Alpha) / 255}
}

func clearFloats(color geometry.Color) [4]float32 {
	alpha := float32(color.Alpha) / 255
	return [4]float32{float32(color.Red) / 255 * alpha, float32(color.Green) / 255 * alpha, float32(color.Blue) / 255 * alpha, alpha}
}

var _ interfaces.Backend = (*Backend)(nil)
var _ interfaces.Renderer = (*Renderer)(nil)
