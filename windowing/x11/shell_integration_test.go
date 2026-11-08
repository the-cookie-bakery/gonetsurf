package x11

import (
	"context"
	"os"
	"testing"
	"time"

	"gonetsurf/shells"
	"gonetsurf/windowing"
)

func TestShellNavigationPresentsContent(t *testing.T) {
	if os.Getenv("DISPLAY") == "" {
		t.Skip("DISPLAY is not set")
	}
	const width = 320
	const height = 240
	display := New(Options{Width: width, Height: height, Title: "gonetsurf navigation test"})
	opened, err := display.Open()
	if err != nil {
		t.Fatal(err)
	}
	defer display.Close()
	window := opened.(*Window)
	shell := shells.New(width, height, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- shell.Run(ctx, window) }()
	time.Sleep(250 * time.Millisecond)
	window.Inject(windowing.Event{Type: windowing.EventKeyPress, Key: "l", Modifiers: windowing.Modifiers{Control: true}})
	window.Inject(windowing.Event{Type: windowing.EventKeyPress, Text: `data:text/html,<body style="margin:0"><div style="width:100px;height:100px;background:rgb(255,0,0)"></div></body>`})
	window.Inject(windowing.Event{Type: windowing.EventKeyPress, Key: "Enter"})
	time.Sleep(700 * time.Millisecond)
	window.mutex.Lock()
	surface := window.surface
	window.mutex.Unlock()
	if surface == nil {
		t.Fatal("window has no surface")
	}
	offset := 80*surface.Stride + 20*4
	if surface.Pix[offset] < 250 || surface.Pix[offset+1] > 5 || surface.Pix[offset+2] > 5 {
		t.Fatalf("content pixel = %v", surface.Pix[offset:offset+4])
	}
	window.Inject(windowing.Event{Type: windowing.EventClose})
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("shell did not stop")
	}
}
