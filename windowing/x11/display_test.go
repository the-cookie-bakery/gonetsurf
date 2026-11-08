package x11

import (
	"context"
	"encoding/binary"
	"image"
	"os"
	"testing"
	"time"

	"gonetsurf/rendering/geometry"
	"gonetsurf/rendering/software"
)

func TestOpenDisplay(t *testing.T) {
	if os.Getenv("DISPLAY") == "" {
		t.Skip("DISPLAY is not set")
	}
	display := New(Options{Width: 64, Height: 48, Title: "gonetsurf x11 test"})
	window, err := display.Open()
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	surface := window.Surface()
	surface.Clear(geometry.NewColor(255, 0, 0))
	if err := window.Present(image.Rect(0, 0, 64, 48)); err != nil {
		t.Fatalf("Present: %v", err)
	}
	if width, height := window.Size(); width != 64 || height != 48 {
		t.Errorf("Size = %dx%d", width, height)
	}
	if err := display.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
}

func TestOpenInvalidDisplay(t *testing.T) {
	display := New(Options{Display: ":99", Width: 32, Height: 32})
	if _, err := display.Open(); err == nil {
		display.Close()
		t.Fatal("expected an error for a missing display")
	}
}

func TestConfigureNotifyResize(t *testing.T) {
	window := &Window{
		display: &Display{atoms: map[string]uint32{}},
		id:      1,
		width:   64,
		height:  48,
		surface: software.NewSurface(64, 48),
	}
	calledWidth := 0
	calledHeight := 0
	window.SetResizeHandler(func(width, height int) {
		calledWidth = width
		calledHeight = height
	})
	packet := make([]byte, 32)
	packet[0] = eventConfigureNotify
	binary.LittleEndian.PutUint16(packet[20:22], 100)
	binary.LittleEndian.PutUint16(packet[22:24], 80)
	if _, err := window.handleEvent(packet); err != nil {
		t.Fatal(err)
	}
	if calledWidth != 100 || calledHeight != 80 {
		t.Errorf("resize handler = %dx%d", calledWidth, calledHeight)
	}
	if width, height := window.Size(); width != 100 || height != 80 {
		t.Errorf("window size = %dx%d", width, height)
	}
	if width, height := window.Surface().Size(); width != 100 || height != 80 {
		t.Errorf("surface size = %dx%d", width, height)
	}
}

func TestClientMessageClose(t *testing.T) {
	window := &Window{
		display: &Display{atoms: map[string]uint32{"WM_PROTOCOLS": 10, "WM_DELETE_WINDOW": 11}},
		id:      1,
		width:   10,
		height:  10,
		surface: software.NewSurface(10, 10),
	}
	packet := make([]byte, 32)
	packet[0] = eventClientMessage
	binary.LittleEndian.PutUint32(packet[8:12], 10)
	binary.LittleEndian.PutUint32(packet[16:20], 11)
	closeRequested, err := window.handleEvent(packet)
	if err != nil {
		t.Fatal(err)
	}
	if !closeRequested {
		t.Error("expected the delete message to close the window")
	}
}

func TestClipboardAcrossWindows(t *testing.T) {
	if os.Getenv("DISPLAY") == "" {
		t.Skip("DISPLAY is not set")
	}
	first := New(Options{Width: 100, Height: 80, Title: "gonetsurf clipboard owner"})
	second := New(Options{Width: 100, Height: 80, Title: "gonetsurf clipboard reader"})
	if _, err := first.Open(); err != nil {
		t.Fatalf("open first: %v", err)
	}
	defer first.Close()
	if _, err := second.Open(); err != nil {
		t.Fatalf("open second: %v", err)
	}
	defer second.Close()
	firstWindow := first.window
	secondWindow := second.window

	firstContext, firstCancel := context.WithCancel(context.Background())
	secondContext, secondCancel := context.WithCancel(context.Background())
	firstDone := make(chan error, 1)
	secondDone := make(chan error, 1)
	go func() { firstDone <- first.EventLoop(firstContext) }()
	go func() { secondDone <- second.EventLoop(secondContext) }()
	time.Sleep(50 * time.Millisecond)

	const clipboardText = "gonetsurf clipboard text"
	if err := firstWindow.SetClipboard(clipboardText); err != nil {
		t.Fatalf("SetClipboard: %v", err)
	}
	if local, err := firstWindow.GetClipboard(context.Background()); err != nil || local != clipboardText {
		t.Errorf("local clipboard = (%q,%v)", local, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	text, err := secondWindow.GetClipboard(ctx)
	if err != nil {
		t.Fatalf("GetClipboard: %v", err)
	}
	if text != clipboardText {
		t.Errorf("remote clipboard = %q, want %q", text, clipboardText)
	}

	firstCancel()
	secondCancel()
	select {
	case <-firstDone:
	case <-time.After(2 * time.Second):
		t.Error("first event loop did not stop")
	}
	select {
	case <-secondDone:
	case <-time.After(2 * time.Second):
		t.Error("second event loop did not stop")
	}
}

func TestMITSHMUpload(t *testing.T) {
	if os.Getenv("DISPLAY") == "" {
		t.Skip("DISPLAY is not set")
	}
	display := New(Options{Width: 32, Height: 32, Title: "gonetsurf shm test"})
	opened, err := display.Open()
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	window := opened.(*Window)
	defer display.Close()
	if !display.shmSupported {
		t.Skip("server does not advertise MIT-SHM")
	}
	if display.shm == nil {
		t.Skip("shared-memory segment could not be created")
	}
	surface := window.Surface()
	time.Sleep(250 * time.Millisecond)
	surface.Clear(geometry.NewColor(255, 0, 0))
	if err := window.Present(image.Rect(0, 0, 32, 32)); err != nil {
		t.Fatalf("Present: %v", err)
	}
	time.Sleep(100 * time.Millisecond)
	sequence, err := display.sendRequest(getImageRequest(window.id, 0, 0, 8, 8))
	if err != nil {
		t.Fatal(err)
	}
	reply, err := display.waitForReplyData(sequence)
	if err != nil {
		t.Fatal(err)
	}
	if len(reply) < 32+8*8*4 {
		t.Fatalf("short get-image reply: %d", len(reply))
	}
	pixel := reply[32+5*32+5*4:]
	red, green, blue := pixel[2], pixel[1], pixel[0]
	if red < 200 || green > 50 || blue > 50 {
		t.Errorf("SHM pixel = (%d,%d,%d)", red, green, blue)
	}
}

func TestDamagePresentation(t *testing.T) {
	if os.Getenv("DISPLAY") == "" {
		t.Skip("DISPLAY is not set")
	}
	display := New(Options{Width: 48, Height: 48, Title: "gonetsurf damage test"})
	opened, err := display.Open()
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	window := opened.(*Window)
	defer display.Close()
	surface := window.Surface()
	time.Sleep(250 * time.Millisecond)
	surface.Clear(geometry.NewColor(255, 0, 0))
	if err := window.Present(image.Rect(0, 0, 48, 48)); err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond)
	surface.Clear(geometry.NewColor(0, 0, 255))
	if err := window.Present(image.Rect(0, 0, 16, 16)); err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)
	sequence, err := display.sendRequest(getImageRequest(window.id, 0, 0, 32, 32))
	if err != nil {
		t.Fatal(err)
	}
	reply, err := display.waitForReplyData(sequence)
	if err != nil {
		t.Fatal(err)
	}
	if len(reply) < 32+32*32*4 {
		t.Fatalf("short get-image reply: %d", len(reply))
	}
	bluePixel := reply[32+5*32*4+5*4:]
	redPixel := reply[32+20*32*4+20*4:]
	if bluePixel[0] < 200 || bluePixel[2] > 50 {
		t.Errorf("damaged pixel = %v, want blue", bluePixel[:4])
	}
	if redPixel[2] < 200 || redPixel[0] > 50 {
		t.Errorf("preserved pixel = %v, want red", redPixel[:4])
	}
}
