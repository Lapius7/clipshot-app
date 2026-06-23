//go:build windows

package notify

import (
	"syscall"
	"unsafe"
	"time"
)

var (
	modShell32        = syscall.NewLazyDLL("shell32.dll")
	procShellNotifyIconW = modShell32.NewProc("Shell_NotifyIconW")
	procCreateWindowExW  = user32.NewProc("CreateWindowExW")
	procDestroyWindow    = user32.NewProc("DestroyWindow")
)

const (
	nimAdd           = 0x00000000
	nimDelete        = 0x00000002
	nimModify        = 0x00000001
	nifInfo          = 0x00000010
	nifIcon          = 0x00000002
	nifMessage       = 0x00000001
	niifNone         = 0x00000000
	niifInfo         = 0x00000001
	niifWarning      = 0x00000002
	niifError        = 0x00000003
	wmUser           = 0x0400
)

var user32 = syscall.NewLazyDLL("user32.dll")

type notifyIconDataW struct {
	CbSize           uint32
	HWnd             uintptr
	UID              uint32
	UFlags           uint32
	UCallbackMessage uint32
	HIcon            uintptr
	SzTip            [128]uint16
	DwState          uint32
	DwStateMask      uint32
	SzInfo           [256]uint16
	UVersion         uint32
	SzInfoTitle      [64]uint16
	DwInfoFlags      uint32
	GuidItem         [16]byte
	HBalloonIcon     uintptr
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

var (
	notifyHwnd    uintptr
	notifyIconID  uint32 = 1
	notifyCreated bool
)

func ensureNotifyHwnd() uintptr {
	if notifyCreated {
		return notifyHwnd
	}

	className, _ := syscall.UTF16PtrFromString("ClipShotNotify")
	hInstance, _, _ := procGetModuleHandleW.Call(0)

	wc := wndClassExW{
		cbSize:        uint32(unsafe.Sizeof(wndClassExW{})),
		lpfnWndProc:   syscall.NewCallback(notifyWndProc),
		hInstance:     syscall.Handle(hInstance),
		lpszClassName: className,
	}
	procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))

	title, _ := syscall.UTF16PtrFromString("ClipShot")
	hwnd, _, _ := procCreateWindowExW.Call(
		0, uintptr(unsafe.Pointer(className)), uintptr(unsafe.Pointer(title)),
		0, 0, 0, 0, 0, 0, 0, hInstance, 0,
	)
	notifyHwnd = hwnd
	notifyCreated = true
	return hwnd
}

var procRegisterClassExW = user32.NewProc("RegisterClassExW")
var procGetModuleHandleW = syscall.NewLazyDLL("kernel32.dll").NewProc("GetModuleHandleW")

func notifyWndProc(hwnd uintptr, msg uint32, wp, lp uintptr) uintptr {
	r, _, _ := procDefWindowProcW.Call(hwnd, uintptr(msg), wp, lp)
	return r
}

var procDefWindowProcW = user32.NewProc("DefWindowProcW")

func Show(message string) {
	go func() {
		hwnd := ensureNotifyHwnd()

		nid := notifyIconDataW{
			CbSize:           uint32(unsafe.Sizeof(notifyIconDataW{})),
			HWnd:             hwnd,
			UID:              notifyIconID,
			UFlags:           nifInfo,
			UVersion:         3,
			DwInfoFlags:      niifInfo,
		}

		title, _ := syscall.UTF16PtrFromString("ClipShot")
		copy(nid.SzInfoTitle[:], utf16PtrToSlice(title))

		msgW, _ := syscall.UTF16PtrFromString(message)
		copy(nid.SzInfo[:], utf16PtrToSlice(msgW))

		procShellNotifyIconW.Call(nimModify, uintptr(unsafe.Pointer(&nid)))

		time.Sleep(5 * time.Second)

		nid.UFlags = nifInfo
		empty, _ := syscall.UTF16PtrFromString("")
		copy(nid.SzInfo[:], utf16PtrToSlice(empty))
		procShellNotifyIconW.Call(nimModify, uintptr(unsafe.Pointer(&nid)))
	}()
}

func utf16PtrToSlice(p *uint16) []uint16 {
	var s []uint16
	for i := 0; i < 256; i++ {
		v := *(*uint16)(unsafe.Pointer(uintptr(unsafe.Pointer(p)) + uintptr(i*2)))
		if v == 0 {
			break
		}
		s = append(s, v)
	}
	return s
}
