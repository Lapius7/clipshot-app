//go:build windows

// Package notify shows feedback via Windows balloon notifications. The
// previous implementation only changed the tray icon's tooltip text, which
// is invisible unless the user happens to hover over the icon. Balloon
// notifications pop up on their own, so failures (e.g. upload errors) are
// impossible to miss.
package notify

import (
	"sync"
	"syscall"
	"unsafe"
)

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	shell32  = syscall.NewLazyDLL("shell32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")

	procRegisterClassExW = user32.NewProc("RegisterClassExW")
	procCreateWindowExW  = user32.NewProc("CreateWindowExW")
	procDefWindowProcW   = user32.NewProc("DefWindowProcW")
	procShellNotifyIconW = shell32.NewProc("Shell_NotifyIconW")
	procGetModuleHandleW = kernel32.NewProc("GetModuleHandleW")
)

const (
	nimAdd    = 0x00000000
	nimModify = 0x00000001

	nifInfo  = 0x00000010
	nifState = 0x00000008

	nisHidden = 0x00000001

	niifInfo  = 0x00000001
	niifError = 0x00000003
)

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

// notifyIconDataW mirrors NOTIFYICONDATAW (the subset of fields this
// package needs); szTip/szInfo/szInfoTitle are fixed-size per the Win32 ABI.
type notifyIconDataW struct {
	cbSize           uint32
	hWnd             uintptr
	uID              uint32
	uFlags           uint32
	uCallbackMessage uint32
	hIcon            syscall.Handle
	szTip            [128]uint16
	dwState          uint32
	dwStateMask      uint32
	szInfo           [256]uint16
	uTimeoutOrVer    uint32
	szInfoTitle      [64]uint16
	dwInfoFlags      uint32
	guidItem         [16]byte
	hBalloonIcon     syscall.Handle
}

var (
	initOnce sync.Once
	hwnd     uintptr
)

func toUTF16Array(dst []uint16, s string) {
	u, err := syscall.UTF16FromString(s)
	if err != nil {
		return
	}
	n := len(u)
	if n > len(dst) {
		n = len(dst)
	}
	copy(dst[:n], u[:n])
}

func defWndProc(hwnd uintptr, msg uint32, wp, lp uintptr) uintptr {
	r, _, _ := procDefWindowProcW.Call(hwnd, uintptr(msg), wp, lp)
	return r
}

// ensureWindow lazily creates a hidden message-only-style window used solely
// as the owner for the notify icon; Shell_NotifyIconW requires a window
// handle even though this icon is never shown (NIS_HIDDEN).
func ensureWindow() uintptr {
	initOnce.Do(func() {
		className, _ := syscall.UTF16PtrFromString("ClipShotNotify")
		hInstance, _, _ := procGetModuleHandleW.Call(0)

		wc := wndClassExW{
			cbSize:        uint32(unsafe.Sizeof(wndClassExW{})),
			lpfnWndProc:   syscall.NewCallback(defWndProc),
			hInstance:     syscall.Handle(hInstance),
			lpszClassName: className,
		}
		procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))

		title, _ := syscall.UTF16PtrFromString("ClipShotNotify")
		h, _, _ := procCreateWindowExW.Call(
			0, uintptr(unsafe.Pointer(className)), uintptr(unsafe.Pointer(title)),
			0, 0, 0, 1, 1, 0, 0, hInstance, 0,
		)
		hwnd = h

		var nid notifyIconDataW
		nid.cbSize = uint32(unsafe.Sizeof(nid))
		nid.hWnd = hwnd
		nid.uID = 1
		nid.uFlags = nifState
		nid.dwState = nisHidden
		nid.dwStateMask = nisHidden
		toUTF16Array(nid.szTip[:], "ClipShot")
		procShellNotifyIconW.Call(nimAdd, uintptr(unsafe.Pointer(&nid)))
	})
	return hwnd
}

func balloon(title, msg string, flags uint32) {
	h := ensureWindow()
	if h == 0 {
		return
	}

	var nid notifyIconDataW
	nid.cbSize = uint32(unsafe.Sizeof(nid))
	nid.hWnd = h
	nid.uID = 1
	nid.uFlags = nifInfo | nifState
	nid.dwState = nisHidden
	nid.dwStateMask = nisHidden
	nid.dwInfoFlags = flags
	toUTF16Array(nid.szInfoTitle[:], title)
	toUTF16Array(nid.szInfo[:], msg)

	procShellNotifyIconW.Call(nimModify, uintptr(unsafe.Pointer(&nid)))
}

// ShowInfo pops up an informational balloon notification (upload progress,
// success, etc).
func ShowInfo(message string) {
	balloon("ClipShot", message, niifInfo)
}

// ShowError pops up an error balloon notification. Unlike ShowInfo, Windows
// renders this with an error icon and it stays distinguishable in
// Notification history.
func ShowError(message string) {
	balloon("ClipShot", message, niifError)
}
