package windowing

import "context"

// EventType identifies a platform input or window event.
type EventType uint8

const (
	// EventKeyPress is a key-down event.
	EventKeyPress EventType = iota
	// EventKeyRelease is a key-up event.
	EventKeyRelease
	// EventMousePress is a pointer-button-down event.
	EventMousePress
	// EventMouseRelease is a pointer-button-up event.
	EventMouseRelease
	// EventMouseMove is a pointer-motion event.
	EventMouseMove
	// EventResize reports a viewport size change.
	EventResize
	// EventClose requests that the window close.
	EventClose
	// EventFocusIn reports keyboard focus entering the window.
	EventFocusIn
	// EventFocusOut reports keyboard focus leaving the window.
	EventFocusOut
	// EventExpose requests that the window content be repainted.
	EventExpose
)

// Modifiers records keyboard modifier state.
type Modifiers struct {
	Shift   bool
	Control bool
	Alt     bool
	Super   bool
}

// Event is one platform input or window event.
type Event struct {
	Type      EventType
	Key       string
	Text      string
	Modifiers Modifiers
	Button    int
	X         int
	Y         int
	Width     int
	Height    int
}

// InputWindow is implemented by windows that deliver input and clipboard
// events. Shells can type-assert a Window to InputWindow.
type InputWindow interface {
	Events() <-chan Event
	SetClipboard(text string) error
	GetClipboard(ctx context.Context) (string, error)
}
