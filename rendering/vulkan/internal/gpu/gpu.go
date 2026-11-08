//go:build vulkan && cgo

// Package gpu is the cgo bridge to the minimal Vulkan offscreen renderer.
package gpu

/*
#cgo LDFLAGS: -lvulkan -lxcb
#include <stdlib.h>
#include "vk_helper.h"
*/
import "C"

import (
	"fmt"
	"unsafe"

	"gonetsurf/internal/vulkandevice"
)

// Draw is one textured or solid quad.
type Draw struct {
	Rect        [4]float32
	UV          [4]float32
	Color       [4]float32
	Clip        [4]float32
	Opacity     float32
	Mode        int
	Texture     uint64
	Transform   [6]float32
	Transformed bool
}

// Context owns the Vulkan device and offscreen target.
type Context struct {
	pointer  *C.vk_context
	Width    int
	Height   int
	textures map[string]uint64
}

// New creates a Vulkan offscreen context for the given shaders.
func New(width, height int, vertex, solid, texture []byte) (*Context, error) {
	if width <= 0 || height <= 0 {
		return nil, fmt.Errorf("vulkan: invalid target size %dx%d", width, height)
	}
	if len(vertex) == 0 || len(solid) == 0 || len(texture) == 0 {
		return nil, fmt.Errorf("vulkan: shader modules are empty")
	}
	device, err := vulkandevice.Shared()
	if err != nil {
		return nil, err
	}
	errorBuffer := make([]C.char, 512)
	pointer := C.vk_context_create(
		C.int(width),
		C.int(height),
		(*C.uint32_t)(unsafe.Pointer(&vertex[0])),
		C.size_t(len(vertex)),
		(*C.uint32_t)(unsafe.Pointer(&solid[0])),
		C.size_t(len(solid)),
		(*C.uint32_t)(unsafe.Pointer(&texture[0])),
		C.size_t(len(texture)),
		C.uintptr_t(device.Instance()),
		C.uintptr_t(device.PhysicalDevice()),
		C.uintptr_t(device.Device()),
		C.uintptr_t(device.Queue()),
		C.uint32_t(device.QueueFamily()),
		(*C.char)(unsafe.Pointer(&errorBuffer[0])),
		C.size_t(len(errorBuffer)),
	)
	if pointer == nil {
		return nil, fmt.Errorf("vulkan: %s", C.GoString((*C.char)(unsafe.Pointer(&errorBuffer[0]))))
	}
	return &Context{pointer: pointer, Width: width, Height: height, textures: make(map[string]uint64)}, nil
}

// UploadTexture uploads an RGBA (format 1) or alpha (format 0) texture and
// returns its cached handle. Format 2 is premultiplied RGBA.
func (context *Context) UploadTexture(key string, pixels []byte, width, height, format int) (uint64, error) {
	if handle, ok := context.textures[key]; ok {
		return handle, nil
	}
	if len(pixels) == 0 {
		return 0, fmt.Errorf("vulkan: empty texture")
	}
	errorBuffer := make([]C.char, 512)
	vulkandevice.LockSubmit()
	index := C.vk_context_create_texture(
		context.pointer,
		(*C.uint8_t)(unsafe.Pointer(&pixels[0])),
		C.int(width),
		C.int(height),
		C.int(textureFormat(format)),
		(*C.char)(unsafe.Pointer(&errorBuffer[0])),
		C.size_t(len(errorBuffer)),
	)
	vulkandevice.UnlockSubmit()
	if index == 0 {
		return 0, fmt.Errorf("vulkan: %s", C.GoString((*C.char)(unsafe.Pointer(&errorBuffer[0]))))
	}
	handle := uint64(index)
	context.textures[key] = handle
	return handle, nil
}

// Render draws the scene and copies premultiplied RGBA8 pixels into output.
func (context *Context) Render(clear [4]float32, draws []Draw, output []byte) error {
	required := context.Width * context.Height * 4
	if len(output) < required {
		return fmt.Errorf("vulkan: output buffer is too small")
	}
	cDraws := make([]C.vk_draw, len(draws))
	for index, draw := range draws {
		for component := 0; component < 4; component++ {
			cDraws[index].rect[component] = C.float(draw.Rect[component])
			cDraws[index].uv[component] = C.float(draw.UV[component])
			cDraws[index].color[component] = C.float(draw.Color[component])
			cDraws[index].clip[component] = C.float(draw.Clip[component])
		}
		cDraws[index].opacity = C.float(draw.Opacity)
		cDraws[index].mode = C.int(draw.Mode)
		cDraws[index].texture = C.uint64_t(draw.Texture)
		for component := 0; component < 6; component++ {
			cDraws[index].transform[component] = C.float(draw.Transform[component])
		}
		if draw.Transformed {
			cDraws[index].transformed = 1
		}
	}
	var drawPointer *C.vk_draw
	if len(cDraws) > 0 {
		drawPointer = &cDraws[0]
	}
	errorBuffer := make([]C.char, 512)
	vulkandevice.LockSubmit()
	result := C.vk_context_render(
		context.pointer,
		(*C.float)(unsafe.Pointer(&clear[0])),
		drawPointer,
		C.int(len(cDraws)),
		(*C.uint8_t)(unsafe.Pointer(&output[0])),
		(*C.char)(unsafe.Pointer(&errorBuffer[0])),
		C.size_t(len(errorBuffer)),
	)
	vulkandevice.UnlockSubmit()
	if result == 0 {
		return fmt.Errorf("vulkan: %s", C.GoString((*C.char)(unsafe.Pointer(&errorBuffer[0]))))
	}
	return nil
}

// CreatePathPipeline compiles the path coverage compute shader.
func (context *Context) CreatePathPipeline(spirv []byte) error {
	if context == nil || context.pointer == nil || len(spirv) == 0 {
		return fmt.Errorf("vulkan: invalid path pipeline")
	}
	errorBuffer := make([]C.char, 512)
	if C.vk_context_create_path_pipeline(context.pointer,
		(*C.uint32_t)(unsafe.Pointer(&spirv[0])), C.size_t(len(spirv)),
		(*C.char)(unsafe.Pointer(&errorBuffer[0])), C.size_t(len(errorBuffer))) == 0 {
		return fmt.Errorf("vulkan: %s", C.GoString((*C.char)(unsafe.Pointer(&errorBuffer[0]))))
	}
	return nil
}

// RasterizePath computes path coverage on the GPU and returns its texture handle.
func (context *Context) RasterizePath(edges []float32, width, height, rule int) (uint64, error) {
	if context == nil || context.pointer == nil || len(edges) < 5 || width <= 0 || height <= 0 {
		return 0, nil
	}
	errorBuffer := make([]C.char, 512)
	vulkandevice.LockSubmit()
	index := C.vk_context_rasterize_path(context.pointer,
		(*C.float)(unsafe.Pointer(&edges[0])), C.int(len(edges)/5),
		C.int(width), C.int(height), C.int(rule),
		(*C.char)(unsafe.Pointer(&errorBuffer[0])), C.size_t(len(errorBuffer)))
	vulkandevice.UnlockSubmit()
	if index == 0 {
		return 0, fmt.Errorf("vulkan: %s", C.GoString((*C.char)(unsafe.Pointer(&errorBuffer[0]))))
	}
	return uint64(index), nil
}

// BeginLayers resets the per-frame offscreen layer target pool.
func (context *Context) BeginLayers() error {
	if context == nil || context.pointer == nil {
		return fmt.Errorf("vulkan: no context")
	}
	errorBuffer := make([]C.char, 512)
	if C.vk_context_begin_layers(context.pointer,
		(*C.char)(unsafe.Pointer(&errorBuffer[0])), C.size_t(len(errorBuffer))) == 0 {
		return fmt.Errorf("vulkan: %s", C.GoString((*C.char)(unsafe.Pointer(&errorBuffer[0]))))
	}
	return nil
}

// RenderLayer renders draws into an offscreen layer and returns its texture handle.
func (context *Context) RenderLayer(clear [4]float32, draws []Draw) (uint64, error) {
	if context == nil || context.pointer == nil {
		return 0, fmt.Errorf("vulkan: no context")
	}
	cDraws := makeCDraws(draws)
	var drawPointer *C.vk_draw
	if len(cDraws) > 0 {
		drawPointer = &cDraws[0]
	}
	errorBuffer := make([]C.char, 512)
	vulkandevice.LockSubmit()
	index := C.vk_context_render_layer(context.pointer, C.int(context.Width), C.int(context.Height),
		(*C.float)(unsafe.Pointer(&clear[0])), drawPointer, C.int(len(cDraws)),
		(*C.char)(unsafe.Pointer(&errorBuffer[0])), C.size_t(len(errorBuffer)))
	vulkandevice.UnlockSubmit()
	if index == 0 {
		return 0, fmt.Errorf("vulkan: %s", C.GoString((*C.char)(unsafe.Pointer(&errorBuffer[0]))))
	}
	return uint64(index), nil
}

// PairTexture builds a descriptor pairing a layer texture with a mask texture.
func (context *Context) PairTexture(first, second uint64) (uint64, error) {
	if context == nil || context.pointer == nil {
		return 0, fmt.Errorf("vulkan: no context")
	}
	errorBuffer := make([]C.char, 512)
	index := C.vk_context_pair_texture(context.pointer, C.int(first), C.int(second),
		(*C.char)(unsafe.Pointer(&errorBuffer[0])), C.size_t(len(errorBuffer)))
	if index == 0 {
		return 0, fmt.Errorf("vulkan: %s", C.GoString((*C.char)(unsafe.Pointer(&errorBuffer[0]))))
	}
	return uint64(index), nil
}

func makeCDraws(draws []Draw) []C.vk_draw {
	cDraws := make([]C.vk_draw, len(draws))
	for index, draw := range draws {
		for component := 0; component < 4; component++ {
			cDraws[index].rect[component] = C.float(draw.Rect[component])
			cDraws[index].uv[component] = C.float(draw.UV[component])
			cDraws[index].color[component] = C.float(draw.Color[component])
			cDraws[index].clip[component] = C.float(draw.Clip[component])
		}
		cDraws[index].opacity = C.float(draw.Opacity)
		cDraws[index].mode = C.int(draw.Mode)
		cDraws[index].texture = C.uint64_t(draw.Texture)
		for component := 0; component < 6; component++ {
			cDraws[index].transform[component] = C.float(draw.Transform[component])
		}
		if draw.Transformed {
			cDraws[index].transformed = 1
		}
	}
	return cDraws
}

// GeometryTexture binds a layout node buffer for GPU node draws.
func (context *Context) GeometryTexture(buffer uintptr) (uint64, error) {
	if context == nil || context.pointer == nil || buffer == 0 {
		return 0, fmt.Errorf("vulkan: invalid geometry buffer")
	}
	errorBuffer := make([]C.char, 512)
	index := C.vk_context_geometry_texture(context.pointer, C.uintptr_t(buffer),
		(*C.char)(unsafe.Pointer(&errorBuffer[0])), C.size_t(len(errorBuffer)))
	if index == 0 {
		return 0, fmt.Errorf("vulkan: %s", C.GoString((*C.char)(unsafe.Pointer(&errorBuffer[0]))))
	}
	return uint64(index), nil
}

// AttachSwapchain creates an XCB Vulkan swapchain for a native window.
func (context *Context) AttachSwapchain(display string, window uintptr, width, height int) error {
	if context == nil || context.pointer == nil || window == 0 {
		return fmt.Errorf("vulkan: invalid swapchain target")
	}
	errorBuffer := make([]C.char, 512)
	displayCString := C.CString(display)
	defer C.free(unsafe.Pointer(displayCString))
	if C.vk_context_attach_swapchain(context.pointer, displayCString, C.uintptr_t(window),
		C.int(width), C.int(height),
		(*C.char)(unsafe.Pointer(&errorBuffer[0])), C.size_t(len(errorBuffer))) == 0 {
		return fmt.Errorf("vulkan: %s", C.GoString((*C.char)(unsafe.Pointer(&errorBuffer[0]))))
	}
	return nil
}

// PresentSurface copies premultiplied RGBA8 pixels into the swapchain and presents.
func (context *Context) PresentSurface(pixels []byte, width, height int) error {
	if context == nil || context.pointer == nil || len(pixels) < width*height*4 {
		return fmt.Errorf("vulkan: invalid present surface")
	}
	errorBuffer := make([]C.char, 512)
	vulkandevice.LockSubmit()
	result := C.vk_context_present_surface(context.pointer,
		(*C.uint8_t)(unsafe.Pointer(&pixels[0])), C.int(width), C.int(height),
		(*C.char)(unsafe.Pointer(&errorBuffer[0])), C.size_t(len(errorBuffer)))
	vulkandevice.UnlockSubmit()
	if result == 0 {
		return fmt.Errorf("vulkan: %s", C.GoString((*C.char)(unsafe.Pointer(&errorBuffer[0]))))
	}
	return nil
}

// Destroy releases the device and all uploaded textures.
func (context *Context) Destroy() {
	if context == nil || context.pointer == nil {
		return
	}
	C.vk_context_destroy(context.pointer)
	context.pointer = nil
	context.textures = nil
}

func textureFormat(format int) int {
	switch format {
	case 1:
		return 1
	case 2:
		return 1
	default:
		return 0
	}
}

// TextureCount reports the number of cached textures.
func (context *Context) TextureCount() int {
	if context == nil || context.textures == nil {
		return 0
	}
	return len(context.textures)
}

// Texture returns a cached texture handle.
func (context *Context) Texture(key string) (uint64, bool) {
	if context == nil || context.textures == nil {
		return 0, false
	}
	handle, ok := context.textures[key]
	return handle, ok
}
