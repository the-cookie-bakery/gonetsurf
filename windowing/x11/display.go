// Package x11 implements a pure-Go X11 display backend. It connects to a local
// or remote X server, creates a window, and uploads rendering surfaces with
// PutImage. Input, clipboard, and shared-memory uploads are later Phase 8 work.
package x11

import (
	"bufio"
	"context"
	"encoding/binary"
	"fmt"
	"image"
	"io"
	"net"
	"os"
	"sync"
	"time"

	"gonetsurf/rendering/geometry"
	"gonetsurf/rendering/interfaces"
	"gonetsurf/rendering/software"
	"gonetsurf/windowing"
)

// Options configures an X11 display.
type Options struct {
	Display string
	Width   int
	Height  int
	Title   string
}

// Display is an X11 connection.
type Display struct {
	options Options

	writeMutex     sync.Mutex
	stateMutex     sync.Mutex
	conn           net.Conn
	reader         *bufio.Reader
	setup          *setupInfo
	screen         screenInfo
	visual         visualInfo
	bitsPerPixel   int
	sequence       uint16
	nextID         uint32
	atoms          map[string]uint32
	keymap         map[byte][]uint32
	pendingPackets [][]byte

	shmMutex     sync.Mutex
	shmSupported bool
	shmMajor     byte
	shm          *shmSegment

	window *Window
}

// New returns an unopened X11 display.
func New(options Options) *Display {
	return &Display{options: options}
}

// Open connects to the X server and creates the window.
func (display *Display) Open() (windowing.Window, error) {
	display.stateMutex.Lock()
	defer display.stateMutex.Unlock()
	if display.window != nil {
		return display.window, nil
	}

	displayValue := display.options.Display
	if displayValue == "" {
		displayValue = os.Getenv("DISPLAY")
	}
	name, err := parseDisplayName(displayValue)
	if err != nil {
		return nil, err
	}
	hostname, _ := os.Hostname()
	authName, authData := authorityFor(name, hostname)
	connection, err := dialDisplay(name)
	if err != nil {
		return nil, err
	}
	reader := bufio.NewReader(connection)
	if _, err := connection.Write(setupRequest(authName, authData)); err != nil {
		connection.Close()
		return nil, err
	}
	setup, err := readSetup(reader)
	if err != nil {
		connection.Close()
		return nil, err
	}
	if len(setup.screens) == 0 {
		connection.Close()
		return nil, fmt.Errorf("x11: server reports no screens")
	}
	screenIndex := name.screen
	if screenIndex >= len(setup.screens) {
		screenIndex = 0
	}
	screen := setup.screens[screenIndex]
	visual, bitsPerPixel, err := setup.selectVisual(screen)
	if err != nil {
		connection.Close()
		return nil, err
	}

	display.conn = connection
	display.reader = reader
	display.setup = setup
	display.screen = screen
	display.visual = visual
	display.bitsPerPixel = bitsPerPixel
	display.nextID = 1
	display.atoms = make(map[string]uint32)

	for _, atomName := range []string{"_NET_WM_NAME", "UTF8_STRING", "WM_PROTOCOLS", "WM_DELETE_WINDOW", "CLIPBOARD", "TARGETS", "TEXT", "_NETSURF_CLIPBOARD"} {
		atom, err := display.internAtom(atomName)
		if err != nil {
			connection.Close()
			return nil, err
		}
		display.atoms[atomName] = atom
	}

	width := display.options.Width
	if width <= 0 {
		width = int(screen.width)
	}
	height := display.options.Height
	if height <= 0 {
		height = int(screen.height)
	}
	if width > 65535 {
		width = 65535
	}
	if height > 65535 {
		height = 65535
	}
	windowID := display.allocateID()
	gcID := display.allocateID()

	const valueMaskBackground = 0x2
	const valueMaskEvent = 0x800
	eventMask := uint32(eventMaskKeyPress | eventMaskKeyRelease | eventMaskButtonPress | eventMaskButtonRelease | eventMaskPointerMotion | eventMaskButtonMotion | eventMaskExposure | eventMaskStructureNotify | eventMaskPropertyChange | eventMaskFocusChange)
	createRequest := createWindowRequest(
		visual.depth,
		windowID,
		screen.root,
		uint16(width),
		uint16(height),
		visual.id,
		valueMaskBackground|valueMaskEvent,
		[]uint32{screen.whitePixel, eventMask},
	)
	if _, err := display.sendRequest(createRequest); err != nil {
		connection.Close()
		return nil, err
	}
	gcRequest := createGCRequest(gcID, windowID, 0x4, []uint32{screen.blackPixel})
	if _, err := display.sendRequest(gcRequest); err != nil {
		connection.Close()
		return nil, err
	}

	if err := display.loadKeyboardMapping(); err != nil {
		connection.Close()
		return nil, err
	}
	display.querySHM()

	window := &Window{
		display:          display,
		id:               windowID,
		gc:               gcID,
		width:            width,
		height:           height,
		title:            display.options.Title,
		surface:          newWindowSurface(width, height),
		events:           make(chan windowing.Event, 64),
		clipboardPending: make(map[uint32]chan string),
	}
	window.setTitle(display.options.Title)
	window.setProtocols()
	display.enableSHM(width, height)
	if _, err := display.sendRequest(mapWindowRequest(windowID)); err != nil {
		connection.Close()
		return nil, err
	}
	display.window = window
	return window, nil
}

// EventLoop reads X11 events until the window is closed or ctx is cancelled.
func (display *Display) EventLoop(ctx context.Context) error {
	display.stateMutex.Lock()
	window := display.window
	connection := display.conn
	reader := display.reader
	display.stateMutex.Unlock()
	if window == nil || connection == nil {
		return fmt.Errorf("x11: display is not open")
	}
	packet := make([]byte, 32)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		for len(display.pendingPackets) > 0 {
			pending := display.pendingPackets[0]
			display.pendingPackets = display.pendingPackets[1:]
			closeRequested, err := window.handleEvent(pending)
			if err != nil {
				return err
			}
			if closeRequested {
				return nil
			}
		}
		connection.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
		if _, err := reader.Peek(32); err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			return err
		}
		if _, err := io.ReadFull(reader, packet); err != nil {
			return err
		}
		closeRequested, err := window.handleEvent(packet)
		if err != nil {
			return err
		}
		if closeRequested {
			return nil
		}
	}
}

// Close destroys the window and closes the connection.
func (display *Display) Close() error {
	display.stateMutex.Lock()
	defer display.stateMutex.Unlock()
	if display.window != nil {
		display.disableSHM()
		display.sendRequest(destroyWindowRequest(display.window.id))
		display.sendRequest(freeGCRequest(display.window.gc))
		display.window = nil
	}
	if display.conn != nil {
		err := display.conn.Close()
		display.conn = nil
		display.reader = nil
		return err
	}
	return nil
}

func dialDisplay(name displayName) (net.Conn, error) {
	if name.isUnix {
		connection, err := net.Dial("unix", name.address())
		if err == nil {
			return connection, nil
		}
		return net.Dial("unix", "@"+name.address())
	}
	return net.Dial("tcp", name.address())
}

func (display *Display) sendRequest(request []byte) (uint16, error) {
	display.writeMutex.Lock()
	defer display.writeMutex.Unlock()
	display.sequence++
	_, err := display.conn.Write(request)
	return display.sequence, err
}

func (display *Display) waitForReply(sequence uint16) ([]byte, error) {
	packet := make([]byte, 32)
	for {
		if _, err := io.ReadFull(display.reader, packet); err != nil {
			return nil, err
		}
		switch packet[0] {
		case 0:
			return nil, fmt.Errorf("x11: request failed with error code %d", packet[1])
		case 1:
			if binary.LittleEndian.Uint16(packet[2:4]) == sequence {
				return packet, nil
			}
		default:
			// Ignore events received while waiting for a reply.
		}
	}
}

func (display *Display) internAtom(name string) (uint32, error) {
	sequence, err := display.sendRequest(internAtomRequest(name))
	if err != nil {
		return 0, err
	}
	reply, err := display.waitForReply(sequence)
	if err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint32(reply[8:12]), nil
}

func (display *Display) allocateID() uint32 {
	identifier := display.setup.resourceBase | display.nextID
	display.nextID++
	return identifier
}

// Window is an X11 window.
type Window struct {
	gpuOwned bool
	display  *Display
	id       uint32
	gc       uint32

	mutex         sync.Mutex
	width         int
	height        int
	title         string
	surface       *software.Surface
	resizeHandler func(width, height int)

	events chan windowing.Event

	clipboardMutex   sync.Mutex
	clipboardText    string
	clipboardOwned   bool
	clipboardPending map[uint32]chan string
}

func newWindowSurface(width, height int) *software.Surface {
	surface := software.NewSurface(width, height)
	surface.Clear(geometry.NewColor(255, 255, 255))
	return surface
}

// Surface returns the window's rendering surface.
func (window *Window) Surface() interfaces.Surface {
	window.mutex.Lock()
	defer window.mutex.Unlock()
	return window.surface
}

// Present uploads the surface to the X server, using MIT-SHM when available.
func (window *Window) Present(damage image.Rectangle) error {
	window.mutex.Lock()
	surface := window.surface
	window.mutex.Unlock()
	return window.presentSurface(surface, damage)
}

// PresentSurface composites source into the window backing store and presents
// it atomically with respect to window resizes.
func (window *Window) PresentSurface(source interfaces.Surface, damage image.Rectangle) error {
	if source == nil {
		return nil
	}
	window.mutex.Lock()
	if window.surface == nil {
		window.surface = software.NewSurface(window.width, window.height)
	}
	window.surface.Blit(0, 0, source, geometry.Copy)
	surface := window.surface
	window.mutex.Unlock()
	return window.presentSurface(surface, damage)
}

func (window *Window) presentSurface(surface *software.Surface, damage image.Rectangle) error {
	if surface == nil || surface.Width <= 0 || surface.Height <= 0 {
		return nil
	}
	width := surface.Width
	height := surface.Height
	converted := surface.Image()
	startRow := 0
	endRow := height
	if !damage.Empty() {
		clamped := damage.Intersect(image.Rect(0, 0, width, height))
		if !clamped.Empty() {
			startRow = clamped.Min.Y
			endRow = clamped.Max.Y
		}
	}
	if startRow < 0 {
		startRow = 0
	}
	if endRow > height {
		endRow = height
	}
	if startRow >= endRow {
		return nil
	}
	if window.presentSHM(converted, width, height, startRow, endRow) {
		return nil
	}

	maxRequestBytes := int(window.display.setup.maxRequestLength) * 4
	if maxRequestBytes <= 0 {
		maxRequestBytes = 65535 * 4
	}
	payloadLimit := maxRequestBytes - 24
	stride := width * 4
	if stride <= 0 || payloadLimit < stride {
		return fmt.Errorf("x11: window is too wide to upload")
	}
	rowsPerRequest := payloadLimit / stride
	if rowsPerRequest < 1 {
		rowsPerRequest = 1
	}
	masks := [3]uint32{window.display.visual.redMask, window.display.visual.greenMask, window.display.visual.blueMask}
	for bandStart := startRow; bandStart < endRow; bandStart += rowsPerRequest {
		bandEnd := bandStart + rowsPerRequest
		if bandEnd > endRow {
			bandEnd = endRow
		}
		bandHeight := bandEnd - bandStart
		data := make([]byte, bandHeight*stride)
		encodeNRGBABand(converted, width, bandStart, bandEnd, window.display.setup.imageByteOrder, masks, data)
		request := putImageRequest(
			window.id,
			window.gc,
			uint16(width),
			uint16(bandHeight),
			0,
			int16(bandStart),
			window.display.visual.depth,
			data,
		)
		if _, err := window.display.sendRequest(request); err != nil {
			return err
		}
	}
	return nil
}

// SetResizeHandler installs a callback invoked after the window is resized.
// The callback runs on the event-loop goroutine.
func (window *Window) SetResizeHandler(handler func(width, height int)) {
	window.mutex.Lock()
	window.resizeHandler = handler
	window.mutex.Unlock()
}

// SetTitle changes the window title.
func (window *Window) SetTitle(title string) {
	window.mutex.Lock()
	window.title = title
	window.mutex.Unlock()
	window.setTitle(title)
}

func (window *Window) setTitle(title string) {
	if title == "" {
		return
	}
	display := window.display
	utf8Atom := display.atoms["UTF8_STRING"]
	if netWMName := display.atoms["_NET_WM_NAME"]; netWMName != 0 {
		display.sendRequest(changePropertyRequest(window.id, netWMName, utf8Atom, 8, []byte(title)))
	}
	display.sendRequest(changePropertyRequest(window.id, atomWMName, atomString, 8, []byte(title)))
}

func (window *Window) setProtocols() {
	display := window.display
	protocolAtom := display.atoms["WM_PROTOCOLS"]
	deleteAtom := display.atoms["WM_DELETE_WINDOW"]
	if protocolAtom == 0 || deleteAtom == 0 {
		return
	}
	data := make([]byte, 4)
	binary.LittleEndian.PutUint32(data, deleteAtom)
	display.sendRequest(changePropertyRequest(window.id, protocolAtom, 4, 32, data))
}

// SetGPUOwned disables software presentation for Expose events when the
// window content is presented by a GPU swapchain.
func (window *Window) SetGPUOwned(owned bool) {
	if window == nil {
		return
	}
	window.mutex.Lock()
	window.gpuOwned = owned
	window.mutex.Unlock()
}

// NativeWindowID returns the X11 window identifier.
func (window *Window) NativeWindowID() uintptr {
	if window == nil {
		return 0
	}
	return uintptr(window.id)
}

// DisplayName returns the X display name used for this window.
func (window *Window) DisplayName() string {
	if window == nil || window.display == nil {
		return os.Getenv("DISPLAY")
	}
	if window.display.options.Display != "" {
		return window.display.options.Display
	}
	return os.Getenv("DISPLAY")
}

// Size returns the current window size.
func (window *Window) Size() (width, height int) {
	window.mutex.Lock()
	defer window.mutex.Unlock()
	return window.width, window.height
}

// Scale returns the window scale factor.
func (window *Window) Scale() float64 {
	return 1
}

func (window *Window) handleEvent(packet []byte) (bool, error) {
	eventType := packet[0] & 0x7f
	switch eventType {
	case 0:
		return false, nil
	case eventKeyPressEvent:
		window.handleKeyEvent(packet, true)
	case eventKeyReleaseEvent:
		window.handleKeyEvent(packet, false)
	case eventButtonPressEvent:
		window.handlePointerEvent(packet, windowing.EventMousePress)
	case eventButtonReleaseEvent:
		window.handlePointerEvent(packet, windowing.EventMouseRelease)
	case eventMotionNotify:
		window.handlePointerEvent(packet, windowing.EventMouseMove)
	case eventFocusIn:
		window.sendEvent(windowing.Event{Type: windowing.EventFocusIn})
	case eventFocusOut:
		window.sendEvent(windowing.Event{Type: windowing.EventFocusOut})
	case eventSelectionClear:
		window.handleSelectionClear(packet)
	case eventSelectionRequest:
		window.handleSelectionRequest(packet)
	case eventSelectionNotify:
		window.handleSelectionNotify(packet)
	case eventExpose:
		window.mutex.Lock()
		width := window.width
		height := window.height
		gpuOwned := window.gpuOwned
		window.mutex.Unlock()
		if !gpuOwned {
			if err := window.Present(image.Rect(0, 0, width, height)); err != nil {
				return false, err
			}
		}
		window.sendEvent(windowing.Event{Type: windowing.EventExpose})
	case eventConfigureNotify:
		width := int(binary.LittleEndian.Uint16(packet[20:22]))
		height := int(binary.LittleEndian.Uint16(packet[22:24]))
		if width <= 0 || height <= 0 {
			return false, nil
		}
		window.mutex.Lock()
		resized := width != window.width || height != window.height
		handler := window.resizeHandler
		if resized {
			window.width = width
			window.height = height
			window.surface = newWindowSurface(width, height)
			window.display.resizeSHM(width, height)
		}
		window.mutex.Unlock()
		if resized {
			window.sendEvent(windowing.Event{Type: windowing.EventResize, Width: width, Height: height})
		}
		if resized && handler != nil {
			handler(width, height)
		}
	case eventClientMessage:
		messageType := binary.LittleEndian.Uint32(packet[8:12])
		protocolAtom := window.display.atoms["WM_PROTOCOLS"]
		deleteAtom := window.display.atoms["WM_DELETE_WINDOW"]
		if messageType == protocolAtom && binary.LittleEndian.Uint32(packet[16:20]) == deleteAtom {
			window.sendEvent(windowing.Event{Type: windowing.EventClose})
			return true, nil
		}
	case eventDestroyNotify:
		if binary.LittleEndian.Uint32(packet[4:8]) == window.id {
			window.sendEvent(windowing.Event{Type: windowing.EventClose})
			return true, nil
		}
	case eventGeneric:
		return false, nil
	}
	return false, nil
}
