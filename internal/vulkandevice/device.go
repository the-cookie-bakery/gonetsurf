//go:build vulkan && cgo

// Package vulkandevice owns the single Vulkan instance, physical device, and
// device shared by the compute layout bridge and the graphics rendering bridge.
package vulkandevice

/*
#cgo LDFLAGS: -lvulkan
#include "vk_shared.h"
*/
import "C"

import (
	"fmt"
	"sync"
	"unsafe"
)

// Device exposes the shared Vulkan handles as raw pointers.
type Device struct {
	instance    uintptr
	physical    uintptr
	handle      uintptr
	queue       uintptr
	queueFamily uint32
}

var (
	once       sync.Once
	shared     *Device
	sharedErr  error
	submitLock sync.Mutex
)

// LockSubmit serializes command submissions to the shared queue.
func LockSubmit() { submitLock.Lock() }

// UnlockSubmit releases the shared queue submission lock.
func UnlockSubmit() { submitLock.Unlock() }

// Shared returns the process-wide Vulkan device, creating it on first use.
func Shared() (*Device, error) {
	once.Do(func() {
		errorBuffer := make([]C.char, 512)
		if C.vk_shared_init((*C.char)(unsafe.Pointer(&errorBuffer[0])), C.size_t(len(errorBuffer))) == 0 {
			sharedErr = fmt.Errorf("vulkan: %s", C.GoString((*C.char)(unsafe.Pointer(&errorBuffer[0]))))
			return
		}
		shared = &Device{
			instance:    uintptr(C.vk_shared_instance()),
			physical:    uintptr(C.vk_shared_physical()),
			handle:      uintptr(C.vk_shared_device()),
			queue:       uintptr(C.vk_shared_queue()),
			queueFamily: uint32(C.vk_shared_queue_family()),
		}
	})
	return shared, sharedErr
}

// Instance returns the VkInstance handle.
func (device *Device) Instance() uintptr { return device.instance }

// PhysicalDevice returns the VkPhysicalDevice handle.
func (device *Device) PhysicalDevice() uintptr { return device.physical }

// Device returns the VkDevice handle.
func (device *Device) Device() uintptr { return device.handle }

// Queue returns the VkQueue handle.
func (device *Device) Queue() uintptr { return device.queue }

// QueueFamily returns the queue family index.
func (device *Device) QueueFamily() uint32 { return device.queueFamily }
