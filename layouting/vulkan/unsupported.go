//go:build !(vulkan && cgo)

package vulkan

import (
	"errors"

	"gonetsurf/layouting/interfaces"
)

// ErrUnavailable reports that the Vulkan layout backend was not compiled in.
var ErrUnavailable = errors.New("vulkan layout backend unavailable: build with -tags vulkan and cgo enabled")

// Available reports whether the Vulkan layout backend is compiled in.
func Available() bool { return false }

// NewBackend returns the Vulkan layout backend when it is compiled in.
func NewBackend() (interfaces.Backend, error) {
	return nil, ErrUnavailable
}
