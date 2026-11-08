// Package windowing defines the display and window abstraction used by the
// shell. Platform backends (headless, X11, and later Wayland) present rendering
// surfaces into native windows.
package windowing

import (
	"context"
	"image"

	"gonetsurf/rendering/interfaces"
)

// Display is a platform connection that can open one or more windows.
type Display interface {
	Open() (Window, error)
	EventLoop(ctx context.Context) error
	Close() error
}

// Window presents a rendering surface and receives platform events.
type Window interface {
	Surface() interfaces.Surface
	Present(damage image.Rectangle) error
	SetTitle(title string)
	Size() (width, height int)
	Scale() float64
}

// NativeWindow exposes a platform-native window handle for GPU presentation.
type NativeWindow interface {
	NativeWindowID() uintptr
	DisplayName() string
}

// SurfacePresenter is implemented by windows that can atomically composite a
// source surface into their backing store and present it. This avoids races
// between a resize replacing the backing surface and the shell blitting into
// the old one.
type SurfacePresenter interface {
	PresentSurface(source interfaces.Surface, damage image.Rectangle) error
}
