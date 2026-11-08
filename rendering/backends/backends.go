// Package backends selects a rendering backend by name.
package backends

import (
	"fmt"

	layoutbackends "gonetsurf/layouting/backends"
	layoutinterfaces "gonetsurf/layouting/interfaces"
	"gonetsurf/rendering/interfaces"
	"gonetsurf/rendering/software"
	"gonetsurf/rendering/vulkan"
)

// Names lists the selectable backend names.
var Names = []string{"auto", "software", "vulkan"}

// Select returns the backend for name. "auto" and "" select software.
func Select(name string) (interfaces.Backend, error) {
	switch name {
	case "", "auto":
		if backend, err := vulkan.NewBackend(); err == nil {
			return backend, nil
		}
		return software.NewBackend(), nil
	case "software":
		return software.NewBackend(), nil
	case "vulkan":
		return vulkan.NewBackend()
	default:
		return nil, fmt.Errorf("unknown renderer %q (want auto, software, or vulkan)", name)
	}
}

// Pipeline groups the rendering and layout backends selected for one run.
type Pipeline struct {
	Renderer interfaces.Backend
	Layout   layoutinterfaces.Backend
}

// SelectPipeline selects the rendering backend for name and the matching
// layout backend. The layout backend follows the renderer name so that
// --renderer=vulkan also uses the Vulkan layout pipeline.
func SelectPipeline(name string) (Pipeline, error) {
	renderer, err := Select(name)
	if err != nil {
		return Pipeline{}, err
	}
	layoutName := name
	switch name {
	case "", "auto":
		layoutName = "software"
	}
	layout, err := layoutbackends.Select(layoutName)
	if err != nil {
		return Pipeline{}, err
	}
	return Pipeline{Renderer: renderer, Layout: layout}, nil
}
