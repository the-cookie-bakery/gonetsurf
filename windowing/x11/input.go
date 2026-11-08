package x11

import (
	"encoding/binary"
	"fmt"
	"io"

	"gonetsurf/windowing"
)

const (
	eventKeyPressEvent      = 2
	eventKeyReleaseEvent    = 3
	eventButtonPressEvent   = 4
	eventButtonReleaseEvent = 5
	eventMotionNotify       = 6
	eventFocusIn            = 9
	eventFocusOut           = 10
)

func (display *Display) loadKeyboardMapping() error {
	if display.setup == nil {
		return fmt.Errorf("x11: setup is not loaded")
	}
	first := display.setup.minKeycode
	count := display.setup.maxKeycode - display.setup.minKeycode + 1
	if count == 0 {
		return nil
	}
	sequence, err := display.sendRequest(getKeyboardMappingRequest(first, count))
	if err != nil {
		return err
	}
	reply, err := display.waitForReplyData(sequence)
	if err != nil {
		return err
	}
	keysymsPerCode := int(reply[1])
	if keysymsPerCode == 0 {
		return nil
	}
	data := reply[32:]
	if len(data) < keysymsPerCode*int(count)*4 {
		return fmt.Errorf("x11: truncated keyboard mapping")
	}
	keymap := make(map[byte][]uint32, count)
	for index := 0; index < int(count); index++ {
		keysyms := make([]uint32, keysymsPerCode)
		for symbolIndex := 0; symbolIndex < keysymsPerCode; symbolIndex++ {
			offset := (index*keysymsPerCode + symbolIndex) * 4
			keysyms[symbolIndex] = binary.LittleEndian.Uint32(data[offset : offset+4])
		}
		keymap[first+byte(index)] = keysyms
	}
	display.keymap = keymap
	return nil
}

func (display *Display) waitForReplyData(sequence uint16) ([]byte, error) {
	packet := make([]byte, 32)
	for {
		if _, err := io.ReadFull(display.reader, packet); err != nil {
			return nil, err
		}
		switch packet[0] {
		case 0:
			return nil, fmt.Errorf("x11: request failed with error code %d (major %d minor %d)", packet[1], packet[10], binary.LittleEndian.Uint16(packet[8:10]))
		case 1:
			if binary.LittleEndian.Uint16(packet[2:4]) == sequence {
				extraLength := int(binary.LittleEndian.Uint32(packet[4:8])) * 4
				if extraLength <= 0 {
					return packet, nil
				}
				extra := make([]byte, extraLength)
				if _, err := io.ReadFull(display.reader, extra); err != nil {
					return nil, err
				}
				return append(packet, extra...), nil
			}
			display.pendingPackets = append(display.pendingPackets, append([]byte(nil), packet...))
		default:
			display.pendingPackets = append(display.pendingPackets, append([]byte(nil), packet...))
		}
	}
}

func (window *Window) sendEvent(event windowing.Event) {
	if window.events == nil {
		return
	}
	select {
	case window.events <- event:
	default:
	}
}

// Events returns the window event channel.
func (window *Window) Events() <-chan windowing.Event {
	return window.events
}

func (window *Window) handleKeyEvent(packet []byte, pressed bool) {
	keycode := packet[1]
	state := binary.LittleEndian.Uint16(packet[28:30])
	var keysym uint32
	if keysyms := window.display.keymap[keycode]; len(keysyms) > 0 {
		index := 0
		if state&1 != 0 && len(keysyms) > 1 {
			index = 1
		}
		keysym = keysyms[index]
	}
	key, text := keysymValues(keysym)
	eventType := windowing.EventKeyRelease
	if pressed {
		eventType = windowing.EventKeyPress
	}
	window.sendEvent(windowing.Event{
		Type:      eventType,
		Key:       key,
		Text:      text,
		Modifiers: modifiersFromState(state),
	})
}

func (window *Window) handlePointerEvent(packet []byte, eventType windowing.EventType) {
	state := binary.LittleEndian.Uint16(packet[28:30])
	window.sendEvent(windowing.Event{
		Type:      eventType,
		Button:    int(packet[1]),
		X:         int(int16(binary.LittleEndian.Uint16(packet[24:26]))),
		Y:         int(int16(binary.LittleEndian.Uint16(packet[26:28]))),
		Modifiers: modifiersFromState(state),
	})
}

func modifiersFromState(state uint16) windowing.Modifiers {
	return windowing.Modifiers{
		Shift:   state&1 != 0,
		Control: state&4 != 0,
		Alt:     state&8 != 0,
		Super:   state&64 != 0,
	}
}

func keysymValues(keysym uint32) (string, string) {
	if keysym == 0 {
		return "", ""
	}
	if keysym >= 0x01000000 {
		character := rune(keysym & 0x00ffffff)
		return string(character), string(character)
	}
	if (keysym >= 0x20 && keysym <= 0x7e) || (keysym >= 0xa0 && keysym <= 0xff) {
		character := rune(keysym)
		return string(character), string(character)
	}
	names := map[uint32]string{
		0xff08: "BackSpace", 0xff09: "Tab", 0xff0d: "Enter", 0xff1b: "Escape",
		0xff50: "Home", 0xff51: "Left", 0xff52: "Up", 0xff53: "Right", 0xff54: "Down",
		0xff55: "Page_Up", 0xff56: "Page_Down", 0xff57: "End", 0xffff: "Delete",
		0xffe1: "Shift_L", 0xffe2: "Shift_R", 0xffe3: "Control_L", 0xffe4: "Control_R",
		0xffe9: "Alt_L", 0xffea: "Alt_R", 0xffeb: "Super_L", 0xffec: "Super_R",
	}
	if name, ok := names[keysym]; ok {
		return name, ""
	}
	if keysym >= 0xffbe && keysym <= 0xffc9 {
		return fmt.Sprintf("F%d", keysym-0xffbe+1), ""
	}
	return fmt.Sprintf("KeySym-%#x", keysym), ""
}

// Inject enqueues an event for tests and automation drivers.
func (window *Window) Inject(event windowing.Event) {
	window.sendEvent(event)
}
