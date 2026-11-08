// Package backends selects a rendering backend by name.
package backends

import (
	"fmt"

	"gonetsurf/rendering/interfaces"
	"gonetsurf/rendering/software"
	"gonetsurf/rendering/vulkan"
)

// Names lists the selectable backend names.
var Names = []string{"auto", "software", "vulkan"}

// Select returns the backend for name. "auto" and "" select software.
func Select(name string) (interfaces.Backend, error) {
	switch name {
	case "", "auto", "software":
		return software.NewBackend(), nil
	case "vulkan":
		return vulkan.NewBackend()
	default:
		return nil, fmt.Errorf("unknown renderer %q (want auto, software, or vulkan)", name)
	}
}
