//go:build windows

// Package hotkey registers a global hotkey via the Win32 RegisterHotKey API
// and delivers triggers on a channel.
package hotkey

import (
	"fmt"
	"strings"
	"syscall"
	"unsafe"
)

const (
	modAlt     = 0x0001
	modControl = 0x0002
	modShift   = 0x0004
	modWin     = 0x0008

	wmHotkey = 0x0312
)

var (
	user32              = syscall.NewLazyDLL("user32.dll")
	procRegisterHotKey  = user32.NewProc("RegisterHotKey")
	procUnregisterHotKey = user32.NewProc("UnregisterHotKey")
	procGetMessage      = user32.NewProc("GetMessageW")
)

type msg struct {
	Hwnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      struct{ X, Y int32 }
}

// Listener registers a single global hotkey and emits an event each time
// it is pressed. Call Close to unregister and stop the listener.
type Listener struct {
	id     int
	stopCh chan struct{}
}

// Register parses a combo like "Ctrl+Shift+U" and starts listening for it.
// Triggered presses are sent (non-blocking, dropping if unconsumed) to the
// returned channel.
func Register(combo string) (*Listener, <-chan struct{}, error) {
	mods, vk, err := parseCombo(combo)
	if err != nil {
		return nil, nil, err
	}

	const hotkeyID = 1
	ret, _, errno := procRegisterHotKey.Call(0, uintptr(hotkeyID), uintptr(mods), uintptr(vk))
	if ret == 0 {
		return nil, nil, fmt.Errorf("RegisterHotKey failed: %w", errno)
	}

	events := make(chan struct{}, 1)
	stopCh := make(chan struct{})

	go func() {
		var m msg
		for {
			select {
			case <-stopCh:
				return
			default:
			}
			ret, _, _ := procGetMessage.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
			if ret == 0 {
				return
			}
			if m.Message == wmHotkey && int(m.WParam) == hotkeyID {
				select {
				case events <- struct{}{}:
				default:
				}
			}
		}
	}()

	return &Listener{id: hotkeyID, stopCh: stopCh}, events, nil
}

func (l *Listener) Close() {
	close(l.stopCh)
	procUnregisterHotKey.Call(0, uintptr(l.id))
}

// Validate reports whether combo is a parseable hotkey string, without
// registering it. Used by the settings UI to reject bad input before saving.
func Validate(combo string) error {
	_, _, err := parseCombo(combo)
	return err
}

func parseCombo(combo string) (mods uint32, vk uint16, err error) {
	parts := strings.Split(combo, "+")
	if len(parts) == 0 {
		return 0, 0, fmt.Errorf("empty hotkey combo")
	}
	for _, p := range parts[:len(parts)-1] {
		switch p {
		case "Ctrl":
			mods |= modControl
		case "Alt":
			mods |= modAlt
		case "Shift":
			mods |= modShift
		case "Win":
			mods |= modWin
		default:
			return 0, 0, fmt.Errorf("unknown modifier: %s", p)
		}
	}
	key := parts[len(parts)-1]
	if len(key) != 1 {
		return 0, 0, fmt.Errorf("unsupported key (single character only): %s", key)
	}
	vk = uint16(key[0])
	return mods, vk, nil
}
