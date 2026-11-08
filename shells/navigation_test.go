package shells

import (
	"context"
	"testing"
	"time"

	"gonetsurf/windowing"
	"gonetsurf/windowing/headless"
)

func TestRunRendersAfterAddressNavigation(t *testing.T) {
	display := headless.New(200, 150, "navigation test")
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
	time.Sleep(200 * time.Millisecond)
	window.Inject(keyEvent("l", "", windowing.Modifiers{Control: true}))
	window.Inject(keyEvent("", `data:text/html,<body style="margin:0"><div style="width:100px;height:100px;background:rgb(255,0,0)"></div></body>`, windowing.Modifiers{}))
	window.Inject(keyEvent("Enter", "", windowing.Modifiers{}))
	time.Sleep(300 * time.Millisecond)
	window.Inject(windowing.Event{Type: windowing.EventClose})
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Run did not stop")
	}
	surface := asSoftwareSurface(t, window.Surface())
	offset := 80*surface.Stride + 20*4
	if surface.Pix[offset] < 250 || surface.Pix[offset+1] > 5 || surface.Pix[offset+2] > 5 {
		t.Fatalf("content was not rendered after navigation: %v", surface.Pix[offset:offset+4])
	}
}

func TestRunSyncsShellSizeFromWindow(t *testing.T) {
	display := headless.New(200, 150, "size test")
	opened, err := display.Open()
	if err != nil {
		t.Fatal(err)
	}
	window := opened.(*headless.Window)
	shell := New(100, 100, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- shell.Run(ctx, window) }()
	time.Sleep(250 * time.Millisecond)
	window.Inject(windowing.Event{Type: windowing.EventClose})
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Run did not stop")
	}
	if shell.Width != 200 || shell.Height != 150 {
		t.Fatalf("shell size = %dx%d, want 200x150", shell.Width, shell.Height)
	}
}
