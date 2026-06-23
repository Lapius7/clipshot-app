//go:build windows

package hotkey

import (
	"fmt"
	"runtime"
	"strings"
	"syscall"
	"unsafe"
)

const (
	modAlt     = 0x0001
	modControl = 0x0002
	modShift   = 0x0004
	modWin     = 0x0008

	wmHotkey  = 0x0312
	wmQuit    = 0x0012
)

var (
	user32              = syscall.NewLazyDLL("user32.dll")
	procRegisterHotKey  = user32.NewProc("RegisterHotKey")
	procUnregisterHotKey = user32.NewProc("UnregisterHotKey")
	procGetMessageW     = user32.NewProc("GetMessageW")
	procPostMessageW    = user32.NewProc("PostMessageW")
)

type msg struct {
	Hwnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      struct{ X, Y int32 }
}

type Listener struct {
	id     int
	hwnd   uintptr
	stopCh chan struct{}
	done   chan struct{}
}

func Register(combo string) (*Listener, <-chan struct{}, error) {
	mods, vk, err := parseCombo(combo)
	if err != nil {
		return nil, nil, err
	}

	events := make(chan struct{}, 1)
	stopCh := make(chan struct{})
	done := make(chan struct{})
	l := &Listener{id: 1, stopCh: stopCh, done: done}

	ready := make(chan error, 1)

	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		defer close(done)

		className, _ := syscall.UTF16PtrFromString("ClipShotHotkey")
		hInstance, _ := getModuleHandle()

		wc := wndClassExW{
			cbSize:      uint32(unsafe.Sizeof(wndClassExW{})),
			lpfnWndProc: syscall.NewCallback(defWndProc),
			hInstance:     syscall.Handle(hInstance),
			lpszClassName: className,
		}
		procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))

		title, _ := syscall.UTF16PtrFromString("ClipShot")
		hwnd, _, _ := procCreateWindowExW.Call(
			0, uintptr(unsafe.Pointer(className)), uintptr(unsafe.Pointer(title)),
			0, 0, 0, 1, 1, 0, 0, hInstance, 0,
		)
		if hwnd == 0 {
			ready <- fmt.Errorf("CreateWindowEx failed")
			return
		}
		l.hwnd = hwnd

		ret, _, errno := procRegisterHotKey.Call(hwnd, uintptr(l.id), uintptr(mods), uintptr(vk))
		if ret == 0 {
			ready <- fmt.Errorf("RegisterHotKey failed: %v", errno)
			return
		}
		ready <- nil

		var m msg
		for {
			ret, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&m)), hwnd, 0, 0)
			if ret == 0 || ret == 0xFFFFFFFF {
				return
			}
			if m.Message == wmHotkey && int(m.WParam) == l.id {
				select {
				case events <- struct{}{}:
				default:
				}
			}
		}
	}()

	if err := <-ready; err != nil {
		<-done
		return nil, nil, err
	}

	return l, events, nil
}

func defWndProc(hwnd uintptr, msg uint32, wp, lp uintptr) uintptr {
	ret, _, _ := procDefWindowProcW.Call(hwnd, uintptr(msg), wp, lp)
	return ret
}

var (
	procRegisterClassExW = user32.NewProc("RegisterClassExW")
	procCreateWindowExW  = user32.NewProc("CreateWindowExW")
	procDefWindowProcW   = user32.NewProc("DefWindowProcW")
	modkernel32          = syscall.NewLazyDLL("kernel32.dll")
	procGetModuleHandleW = modkernel32.NewProc("GetModuleHandleW")
)

func getModuleHandle() (uintptr, error) {
	h, _, err := procGetModuleHandleW.Call(0)
	if h == 0 {
		return 0, fmt.Errorf("GetModuleHandle failed: %v", err)
	}
	return h, nil
}

type wndClassExW struct {
	cbSize        uint32
	style         uint32
	lpfnWndProc   uintptr
	cbClsExtra    int32
	cbWndExtra    int32
	hInstance     syscall.Handle
	hIcon         syscall.Handle
	hCursor       syscall.Handle
	hbrBackground syscall.Handle
	lpszMenuName  *uint16
	lpszClassName *uint16
	hIconSm       syscall.Handle
}

func (l *Listener) Close() {
	close(l.stopCh)
	if l.hwnd != 0 {
		procPostMessageW.Call(l.hwnd, wmQuit, 0, 0)
	}
	<-l.done
}

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
