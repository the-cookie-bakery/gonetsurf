//go:build vulkan && cgo

package vulkan

import (
	_ "embed"
	"fmt"
	"image"
	"image/draw"
	"log"
	"math"
	"reflect"
	"sync"

	"gonetsurf/rendering/geometry"
	"gonetsurf/rendering/interfaces"
	"gonetsurf/rendering/software"
	"gonetsurf/rendering/vulkan/internal/gpu"
)

//go:embed shaders/solid.vert.spv
var vertexShader []byte

//go:embed shaders/solid.frag.spv
var solidShader []byte

//go:embed shaders/texture.frag.spv
var textureShader []byte

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

type gradientInfo struct {
	texture uint64
	rect    [4]float32
}

func (info gradientInfo) draw(clip geometry.Rect) gpu.Draw {
	return gpu.Draw{
		Rect:    info.rect,
		UV:      [4]float32{0, 0, 1, 1},
		Color:   [4]float32{1, 1, 1, 1},
		Clip:    rectFloats(clip),
		Opacity: 1,
		Mode:    1,
		Texture: info.texture,
	}
}

type renderCache struct {
	masks       map[string]maskInfo
	gradients   map[string]gradientInfo
	imageKeys   map[uintptr]string
	imageRefs   map[uintptr]image.Image
	nextImageID uint64
}

func newRenderCache() *renderCache {
	return &renderCache{
		masks:     make(map[string]maskInfo),
		gradients: make(map[string]gradientInfo),
		imageKeys: make(map[uintptr]string),
		imageRefs: make(map[uintptr]image.Image),
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
			return renderer.fallback.Render(scene)
		}
		context = created
		renderer.contexts[key] = context
		cache = newRenderCache()
		renderer.caches[key] = cache
		log.Printf("vulkan: offscreen GPU target %dx%d ready", scene.Width, scene.Height)
	}
	clear, draws, err := buildDraws(context, scene, cache)
	if err != nil {
		renderer.mutex.Unlock()
		return renderer.fallback.Render(scene)
	}
	output := make([]byte, scene.Width*scene.Height*4)
	if err := context.Render(clear, draws, output); err != nil {
		renderer.gpuErr = err
		log.Printf("vulkan: GPU render failed: %v", err)
		renderer.mutex.Unlock()
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

func buildDraws(context *gpu.Context, scene *interfaces.Scene, cache *renderCache) ([4]float32, []gpu.Draw, error) {
	full := geometry.Rect{Width: float64(scene.Width), Height: float64(scene.Height)}
	clip := full
	clear := [4]float32{}
	draws := make([]gpu.Draw, 0, len(scene.Commands))
	whiteTexture, err := context.UploadTexture("solid-white", []byte{255, 255, 255, 255}, 1, 1, 1)
	if err != nil {
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
			if info, ok := cache.masks[key]; ok {
				draws = append(draws, info.draw(command.Color, clip))
				continue
			}
			mask := geometry.RasterizeFill(command.Path, scene.Width, scene.Height, command.FillRule)
			draw, ok, err := maskDraw(context, mask, command.Color, clip, key)
			if err != nil {
				return clear, nil, err
			}
			if ok {
				cache.masks[key] = maskInfo{texture: draw.Texture, bounds: drawBounds(draw)}
				draws = append(draws, draw)
			}
		case interfaces.CommandStrokePath:
			if err := flushOverlay(); err != nil {
				return clear, nil, err
			}
			key := pathKey(command.Path, geometry.NonZeroFill, true, command.Stroke, scene.Width, scene.Height)
			if info, ok := cache.masks[key]; ok {
				draws = append(draws, info.draw(command.Color, clip))
				continue
			}
			mask := geometry.RasterizeStroke(command.Path, scene.Width, scene.Height, command.Stroke)
			draw, ok, err := maskDraw(context, mask, command.Color, clip, key)
			if err != nil {
				return clear, nil, err
			}
			if ok {
				cache.masks[key] = maskInfo{texture: draw.Texture, bounds: drawBounds(draw)}
				draws = append(draws, draw)
			}
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
			area := command.Rect.Intersect(clip)
			key := gradientKey(command.Gradient, area)
			if info, ok := cache.gradients[key]; ok {
				draws = append(draws, info.draw(clip))
				continue
			}
			draw, ok, err := gradientDraw(context, command, clip, key)
			if err != nil {
				return clear, nil, err
			}
			if ok {
				cache.gradients[key] = gradientInfo{texture: draw.Texture, rect: draw.Rect}
				draws = append(draws, draw)
			}
		case interfaces.CommandDrawImage:
			if err := flushOverlay(); err != nil {
				return clear, nil, err
			}
			draw, ok, err := imageDraw(context, command, clip, cache)
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
			draw, ok, err := glyphDraw(context, command, clip)
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
			draw, ok, err := layerDraw(context, command, clip, scene.Width, scene.Height)
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

func layerDraw(context *gpu.Context, command interfaces.Command, clip geometry.Rect, width, height int) (gpu.Draw, bool, error) {
	if command.Layer == nil || command.Opacity <= 0 {
		return gpu.Draw{}, false, nil
	}
	if command.Kind == interfaces.CommandCompositeLayer && !command.Transform.IsIdentity() {
		return gpu.Draw{}, false, fmt.Errorf("vulkan: transformed layers are not supported yet")
	}
	layer := software.NewSurface(width, height)
	software.Execute(&interfaces.Scene{Width: width, Height: height, Commands: command.Layer.Commands}, layer)
	if command.Kind == interfaces.CommandCompositeLayerMasked && command.Mask != nil {
		for yPos := 0; yPos < height; yPos++ {
			for xPos := 0; xPos < width; xPos++ {
				coverage := messageCoverage(command.Mask.At(xPos, yPos))
				offset := yPos*layer.Stride + xPos*4
				layer.Pix[offset] = uint8(float64(layer.Pix[offset]) * coverage)
				layer.Pix[offset+1] = uint8(float64(layer.Pix[offset+1]) * coverage)
				layer.Pix[offset+2] = uint8(float64(layer.Pix[offset+2]) * coverage)
				layer.Pix[offset+3] = uint8(float64(layer.Pix[offset+3]) * coverage)
			}
		}
	}
	opacity := command.Opacity
	if opacity < 1 {
		for offset := 0; offset < len(layer.Pix); offset++ {
			layer.Pix[offset] = uint8(float64(layer.Pix[offset]) * opacity)
		}
	}
	handle, err := context.UploadTexture(layerKey(layer), layer.Pix, width, height, 2)
	if err != nil {
		return gpu.Draw{}, false, err
	}
	return gpu.Draw{
		Rect:    rectFloats(geometry.Rect{Width: float64(width), Height: float64(height)}),
		UV:      [4]float32{0, 0, 1, 1},
		Color:   [4]float32{1, 1, 1, 1},
		Clip:    rectFloats(clip),
		Opacity: 1,
		Mode:    3,
		Texture: handle,
	}, true, nil
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

func glyphDraw(context *gpu.Context, command interfaces.Command, clip geometry.Rect) (gpu.Draw, bool, error) {
	mask := command.Glyph.Mask
	if mask == nil {
		return gpu.Draw{}, false, nil
	}
	bounds := mask.Bounds()
	destination := geometry.Rect{X: float64(command.Glyph.X + bounds.Min.X), Y: float64(command.Glyph.Y + bounds.Min.Y), Width: float64(bounds.Dx()), Height: float64(bounds.Dy())}
	if destination.Intersect(clip).IsEmpty() {
		return gpu.Draw{}, false, nil
	}
	handle, err := context.UploadTexture(glyphContentKey(mask), mask.Pix, bounds.Dx(), bounds.Dy(), 0)
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

func gradientDraw(context *gpu.Context, command interfaces.Command, clip geometry.Rect, key string) (gpu.Draw, bool, error) {
	area := command.Rect.Intersect(clip)
	if area.IsEmpty() {
		return gpu.Draw{}, false, nil
	}
	minimumX := int(math.Max(0, math.Floor(area.X)))
	minimumY := int(math.Max(0, math.Floor(area.Y)))
	maximumX := int(math.Ceil(area.X + area.Width))
	maximumY := int(math.Ceil(area.Y + area.Height))
	width := maximumX - minimumX
	height := maximumY - minimumY
	if width <= 0 || height <= 0 {
		return gpu.Draw{}, false, nil
	}
	picture := image.NewNRGBA(image.Rect(0, 0, width, height))
	for yPos := 0; yPos < height; yPos++ {
		for xPos := 0; xPos < width; xPos++ {
			coverage := 1.0
			if command.Mask != nil {
				coverage = command.Mask.At(minimumX+xPos, minimumY+yPos)
			}
			color := command.Gradient.ColorAt(float64(minimumX+xPos)+0.5, float64(minimumY+yPos)+0.5)
			alpha := float64(color.Alpha) / 255 * coverage
			offset := yPos*picture.Stride + xPos*4
			picture.Pix[offset] = color.Red
			picture.Pix[offset+1] = color.Green
			picture.Pix[offset+2] = color.Blue
			picture.Pix[offset+3] = uint8(math.Round(alpha * 255))
		}
	}
	handle, err := context.UploadTexture("gradient:"+key, picture.Pix, width, height, 1)
	if err != nil {
		return gpu.Draw{}, false, err
	}
	return gpu.Draw{
		Rect:    [4]float32{float32(minimumX), float32(minimumY), float32(width), float32(height)},
		UV:      [4]float32{0, 0, 1, 1},
		Color:   [4]float32{1, 1, 1, 1},
		Clip:    rectFloats(clip),
		Opacity: 1,
		Mode:    1,
		Texture: handle,
	}, true, nil
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
