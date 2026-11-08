// Package backends selects a layout backend by name.
package backends

import (
	"fmt"

	"gonetsurf/layouting/interfaces"
	"gonetsurf/layouting/software"
	"gonetsurf/layouting/vulkan"
)

// Names lists the selectable layout backend names.
var Names = []string{"auto", "software", "vulkan"}

// Select returns the layout backend for name. "auto" and "" select software.
func Select(name string) (interfaces.Backend, error) {
	switch name {
	case "", "auto", "software":
		return software.NewBackend(), nil
	case "vulkan":
		return vulkan.NewBackend()
	default:
		return nil, fmt.Errorf("unknown layout backend %q (want auto, software, or vulkan)", name)
	}
}
