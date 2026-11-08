// Package headless provides an in-memory display backend for tests and
// off-screen rendering. It has no platform dependencies.
package headless

import (
	"context"
	"image"

	"gonetsurf/rendering/interfaces"
	"gonetsurf/rendering/software"
	"gonetsurf/windowing"
)

// Display is an in-memory display.
type Display struct {
	width  int
	height int
	title  string
	window *Window
}

// New returns a headless display with the given viewport size.
func New(width, height int, title string) *Display {
	return &Display{width: width, height: height, title: title}
}

// Open creates the in-memory window.
func (display *Display) Open() (windowing.Window, error) {
	if display.window == nil {
		display.window = &Window{
			width:     display.width,
			height:    display.height,
			title:     display.title,
			surface:   software.NewSurface(display.width, display.height),
			scale:     1,
			events:    make(chan windowing.Event, 64),
			clipboard: "",
		}
	}
	return display.window, nil
}

// EventLoop blocks until the context is cancelled.
func (display *Display) EventLoop(ctx context.Context) error {
	<-ctx.Done()
	return ctx.Err()
}

// Close releases the window.
func (display *Display) Close() error {
	display.window = nil
	return nil
}

// Window is an in-memory window backed by a rendering surface.
type Window struct {
	width     int
	height    int
	title     string
	surface   *software.Surface
	scale     float64
	events    chan windowing.Event
	clipboard string
}

// Surface returns the window's rendering surface.
func (window *Window) Surface() interfaces.Surface {
	return window.surface
}

// Present accepts a damage rectangle. The surface is already the backing store.
func (window *Window) Present(image.Rectangle) error {
	return nil
}

// SetTitle changes the window title.
func (window *Window) SetTitle(title string) {
	window.title = title
}

// Size returns the current window size.
func (window *Window) Size() (width, height int) {
	return window.width, window.height
}

// Scale returns the window scale factor.
func (window *Window) Scale() float64 {
	return window.scale
}

// Title returns the current window title.
func (window *Window) Title() string {
	return window.title
}

// Events returns the window event channel.
func (window *Window) Events() <-chan windowing.Event {
	return window.events
}

// Inject enqueues an event for tests or drivers.
func (window *Window) Inject(event windowing.Event) {
	if window.events == nil {
		return
	}
	select {
	case window.events <- event:
	default:
	}
}

// SetClipboard stores clipboard text locally.
func (window *Window) SetClipboard(text string) error {
	window.clipboard = text
	return nil
}

// GetClipboard returns the local clipboard text.
func (window *Window) GetClipboard(ctx context.Context) (string, error) {
	return window.clipboard, nil
}
