package shells

import (
	"context"
	"testing"
	"time"

	"gonetsurf/windowing"
	"gonetsurf/windowing/headless"
)

func keyEvent(key, text string, modifiers windowing.Modifiers) windowing.Event {
	return windowing.Event{Type: windowing.EventKeyPress, Key: key, Text: text, Modifiers: modifiers}
}

func TestShellRenderAndInput(t *testing.T) {
	shell := New(400, 300, nil)
	rendered, renderErr := shell.Render()
	if renderErr != nil {
		t.Fatal(renderErr)
	}
	surface := asSoftwareSurface(t, rendered)
	if surface.Width != 400 || surface.Height != 300 {
		t.Fatalf("surface = %dx%d", surface.Width, surface.Height)
	}
	if pixelAt := surface.Pix[0]; pixelAt == 0 {
		t.Fatal("chrome was not rendered")
	}
	shell.HandleEvent(context.Background(), nil, keyEvent("l", "", windowing.Modifiers{Control: true}))
	if !shell.OmniboxFocused {
		t.Fatal("Ctrl+L did not focus the omnibox")
	}
	shell.HandleEvent(context.Background(), nil, keyEvent("a", "a", windowing.Modifiers{}))
	shell.HandleEvent(context.Background(), nil, keyEvent("b", "b", windowing.Modifiers{}))
	if shell.OmniboxText != "ab" || shell.Caret != 2 {
		t.Errorf("omnibox = %q caret %d", shell.OmniboxText, shell.Caret)
	}
	shell.HandleEvent(context.Background(), nil, keyEvent("BackSpace", "", windowing.Modifiers{}))
	if shell.OmniboxText != "a" || shell.Caret != 1 {
		t.Errorf("after backspace = %q caret %d", shell.OmniboxText, shell.Caret)
	}
	shell.HandleEvent(context.Background(), nil, keyEvent("Escape", "", windowing.Modifiers{}))
	if shell.OmniboxFocused {
		t.Error("Escape did not blur the omnibox")
	}
}

func TestShellTabs(t *testing.T) {
	shell := New(400, 300, nil)
	shell.HandleEvent(context.Background(), nil, keyEvent("t", "", windowing.Modifiers{Control: true}))
	if len(shell.Browser.Tabs) != 2 || shell.Browser.Active != 1 {
		t.Fatalf("tabs = %d active %d", len(shell.Browser.Tabs), shell.Browser.Active)
	}
	shell.HandleEvent(context.Background(), nil, keyEvent("Tab", "", windowing.Modifiers{Control: true}))
	if shell.Browser.Active != 0 {
		t.Errorf("Ctrl+Tab active = %d", shell.Browser.Active)
	}
	shell.HandleEvent(context.Background(), nil, keyEvent("w", "", windowing.Modifiers{Control: true}))
	if len(shell.Browser.Tabs) != 1 {
		t.Errorf("tabs after close = %d", len(shell.Browser.Tabs))
	}
}

func TestShellRunHandlesInputAndClose(t *testing.T) {
	display := headless.New(200, 150, "shell test")
	opened, err := display.Open()
	if err != nil {
		t.Fatal(err)
	}
	window := opened.(*headless.Window)
	shell := New(200, 150, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- shell.Run(ctx, window) }()
	time.Sleep(20 * time.Millisecond)
	window.Inject(keyEvent("l", "", windowing.Modifiers{Control: true}))
	window.Inject(keyEvent("x", "x", windowing.Modifiers{}))
	time.Sleep(40 * time.Millisecond)
	window.Inject(windowing.Event{Type: windowing.EventClose})
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Run = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not stop on close")
	}
	if shell.OmniboxText != "x" {
		t.Errorf("omnibox = %q", shell.OmniboxText)
	}
}
