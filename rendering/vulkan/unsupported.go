//go:build !(vulkan && cgo)

package vulkan

import (
	"errors"

	"gonetsurf/rendering/interfaces"
)

// ErrUnavailable reports that the Vulkan backend was not compiled in.
var ErrUnavailable = errors.New("vulkan backend unavailable: build with -tags vulkan and cgo enabled")

// Available reports whether the Vulkan backend is compiled in.
func Available() bool { return false }

// NewBackend returns the Vulkan backend when it is compiled in.
func NewBackend() (interfaces.Backend, error) {
	return nil, ErrUnavailable
}
