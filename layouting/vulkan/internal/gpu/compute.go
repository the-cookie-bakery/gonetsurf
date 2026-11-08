//go:build vulkan && cgo

// Package gpu is the cgo bridge to the Vulkan compute layout passes.
package gpu

/*
#cgo LDFLAGS: -lvulkan
#include "vk_compute.h"
*/
import "C"

import (
	"fmt"
	"unsafe"

	"gonetsurf/internal/vulkandevice"
)

const MaxBuffers = 16

// Context owns the Vulkan device, descriptor set, and storage buffers used by
// the layout passes.
type Context struct {
	pointer *C.vk_compute_context
	buffers int
	push    int
}

// Pipeline is a compiled compute shader.
type Pipeline struct {
	pointer *C.vk_compute_pipeline
	context *Context
}

// New creates a compute context with the given number of storage buffers and
// push-constant size.
func New(bufferCount, pushSize int) (*Context, error) {
	if bufferCount <= 0 || bufferCount > MaxBuffers {
		return nil, fmt.Errorf("vulkan: invalid buffer count %d", bufferCount)
	}
	device, err := vulkandevice.Shared()
	if err != nil {
		return nil, err
	}
	errorBuffer := make([]C.char, 512)
	pointer := C.vk_compute_create(C.int(bufferCount), C.uint32_t(pushSize),
		C.uintptr_t(device.Instance()), C.uintptr_t(device.PhysicalDevice()),
		C.uintptr_t(device.Device()), C.uintptr_t(device.Queue()),
		C.uint32_t(device.QueueFamily()),
		(*C.char)(unsafe.Pointer(&errorBuffer[0])), C.size_t(len(errorBuffer)))
	if pointer == nil {
		return nil, fmt.Errorf("vulkan: %s", C.GoString((*C.char)(unsafe.Pointer(&errorBuffer[0]))))
	}
	return &Context{pointer: pointer, buffers: bufferCount, push: pushSize}, nil
}

// CreateBuffer allocates a storage buffer at index.
func (context *Context) CreateBuffer(index, size int) error {
	if context == nil || context.pointer == nil {
		return fmt.Errorf("vulkan: no context")
	}
	if index < 0 || index >= context.buffers {
		return fmt.Errorf("vulkan: invalid buffer index %d", index)
	}
	errorBuffer := make([]C.char, 512)
	result := C.vk_compute_buffer(context.pointer, C.int(index), C.size_t(size),
		(*C.char)(unsafe.Pointer(&errorBuffer[0])), C.size_t(len(errorBuffer)))
	if result == 0 {
		return fmt.Errorf("vulkan: %s", C.GoString((*C.char)(unsafe.Pointer(&errorBuffer[0]))))
	}
	return nil
}

// Upload copies data into a storage buffer.
func (context *Context) Upload(index int, data []byte) error {
	if context == nil || context.pointer == nil || len(data) == 0 {
		return nil
	}
	if C.vk_compute_upload(context.pointer, C.int(index),
		unsafe.Pointer(&data[0]), C.size_t(len(data))) == 0 {
		return fmt.Errorf("vulkan: failed to upload buffer %d", index)
	}
	return nil
}

// UploadRange copies data into a range of a storage buffer.
func (context *Context) UploadRange(index, offset int, data []byte) error {
	if context == nil || context.pointer == nil || len(data) == 0 {
		return nil
	}
	if offset < 0 {
		return fmt.Errorf("vulkan: invalid upload offset %d", offset)
	}
	if C.vk_compute_upload_range(context.pointer, C.int(index), C.size_t(offset),
		unsafe.Pointer(&data[0]), C.size_t(len(data))) == 0 {
		return fmt.Errorf("vulkan: failed to upload buffer %d range", index)
	}
	return nil
}

// Readback copies a storage buffer into data.
func (context *Context) Readback(index int, data []byte) error {
	if context == nil || context.pointer == nil || len(data) == 0 {
		return nil
	}
	if C.vk_compute_readback(context.pointer, C.int(index),
		unsafe.Pointer(&data[0]), C.size_t(len(data))) == 0 {
		return fmt.Errorf("vulkan: failed to read buffer %d", index)
	}
	return nil
}

// BufferHandle returns the raw VkBuffer handle at index.
func (context *Context) BufferHandle(index int) uintptr {
	if context == nil || context.pointer == nil {
		return 0
	}
	return uintptr(C.vk_compute_buffer_handle(context.pointer, C.int(index)))
}

// NewPipeline compiles a SPIR-V compute shader.
func (context *Context) NewPipeline(spirv []byte) (*Pipeline, error) {
	if context == nil || context.pointer == nil {
		return nil, fmt.Errorf("vulkan: no context")
	}
	if len(spirv) == 0 {
		return nil, fmt.Errorf("vulkan: empty shader")
	}
	errorBuffer := make([]C.char, 512)
	pointer := C.vk_compute_pipeline_create(context.pointer,
		(*C.uint32_t)(unsafe.Pointer(&spirv[0])), C.size_t(len(spirv)),
		(*C.char)(unsafe.Pointer(&errorBuffer[0])), C.size_t(len(errorBuffer)))
	if pointer == nil {
		return nil, fmt.Errorf("vulkan: %s", C.GoString((*C.char)(unsafe.Pointer(&errorBuffer[0]))))
	}
	return &Pipeline{pointer: pointer, context: context}, nil
}

// Begin starts a batch of compute dispatches submitted in one operation.
func (context *Context) Begin() error {
	if context == nil || context.pointer == nil {
		return fmt.Errorf("vulkan: no context")
	}
	errorBuffer := make([]C.char, 512)
	if C.vk_compute_begin(context.pointer,
		(*C.char)(unsafe.Pointer(&errorBuffer[0])), C.size_t(len(errorBuffer))) == 0 {
		return fmt.Errorf("vulkan: %s", C.GoString((*C.char)(unsafe.Pointer(&errorBuffer[0]))))
	}
	return nil
}

// Record appends a dispatch to the current batch without submitting it.
func (context *Context) Record(pipeline *Pipeline, groupsX, groupsY int, push []byte) error {
	if context == nil || context.pointer == nil || pipeline == nil || pipeline.pointer == nil {
		return fmt.Errorf("vulkan: invalid dispatch")
	}
	if groupsX <= 0 || groupsY <= 0 {
		return nil
	}
	if len(push) > context.push {
		return fmt.Errorf("vulkan: push constant buffer too small")
	}
	var pointer unsafe.Pointer
	if len(push) > 0 {
		pointer = unsafe.Pointer(&push[0])
	}
	errorBuffer := make([]C.char, 512)
	if C.vk_compute_record(context.pointer, pipeline.pointer,
		C.uint32_t(groupsX), C.uint32_t(groupsY), pointer, C.size_t(len(push)),
		(*C.char)(unsafe.Pointer(&errorBuffer[0])), C.size_t(len(errorBuffer))) == 0 {
		return fmt.Errorf("vulkan: %s", C.GoString((*C.char)(unsafe.Pointer(&errorBuffer[0]))))
	}
	return nil
}

// End submits the current batch and waits for completion.
func (context *Context) End() error {
	if context == nil || context.pointer == nil {
		return fmt.Errorf("vulkan: no context")
	}
	errorBuffer := make([]C.char, 512)
	vulkandevice.LockSubmit()
	result := C.vk_compute_end(context.pointer,
		(*C.char)(unsafe.Pointer(&errorBuffer[0])), C.size_t(len(errorBuffer)))
	vulkandevice.UnlockSubmit()
	if result == 0 {
		return fmt.Errorf("vulkan: %s", C.GoString((*C.char)(unsafe.Pointer(&errorBuffer[0]))))
	}
	return nil
}

// Dispatch runs a pipeline over groupsX by groupsY workgroups.
func (context *Context) Dispatch(pipeline *Pipeline, groupsX, groupsY int, push []byte) error {
	if err := context.Begin(); err != nil {
		return err
	}
	if err := context.Record(pipeline, groupsX, groupsY, push); err != nil {
		return err
	}
	return context.End()
}

// Destroy releases the context and all pipelines created from it.
func (context *Context) Destroy() {
	if context == nil || context.pointer == nil {
		return
	}
	C.vk_compute_destroy(context.pointer)
	context.pointer = nil
}

// Destroy releases a pipeline.
func (pipeline *Pipeline) Destroy() {
	if pipeline == nil || pipeline.pointer == nil || pipeline.context == nil {
		return
	}
	C.vk_compute_pipeline_destroy(pipeline.context.pointer, pipeline.pointer)
	pipeline.pointer = nil
}
