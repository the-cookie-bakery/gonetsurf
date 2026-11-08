//go:build !(vulkan && cgo)

// Package vulkandevice owns the shared Vulkan device. It is unavailable in
// builds without the vulkan build tag and cgo.
package vulkandevice

import "errors"

// ErrUnavailable reports that the Vulkan shared device is not compiled in.
var ErrUnavailable = errors.New("vulkan shared device unavailable: build with -tags vulkan and cgo enabled")

// Device exposes the shared Vulkan handles.
type Device struct{}

// Shared returns the shared device when the Vulkan build is available.
func Shared() (*Device, error) { return nil, ErrUnavailable }

// LockSubmit is a no-op in builds without Vulkan.
func LockSubmit() {}

// UnlockSubmit is a no-op in builds without Vulkan.
func UnlockSubmit() {}

// Instance returns the instance handle.
func (device *Device) Instance() uintptr { return 0 }

// PhysicalDevice returns the physical device handle.
func (device *Device) PhysicalDevice() uintptr { return 0 }

// Device returns the device handle.
func (device *Device) Device() uintptr { return 0 }

// Queue returns the queue handle.
func (device *Device) Queue() uintptr { return 0 }

// QueueFamily returns the queue family index.
func (device *Device) QueueFamily() uint32 { return 0 }
