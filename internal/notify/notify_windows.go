//go:build windows

// Package notify shows feedback via Windows balloon notifications. The
// previous implementation only changed the tray icon's tooltip text, which
// is invisible unless the user happens to hover over the icon. Balloon
// notifications pop up on their own, so failures (e.g. upload errors) are
// impossible to miss.
package notify

import (
	"log"
	"runtime"
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

type balloonRequest struct {
	title string
	msg   string
	flags uint32
}

// requests is drained by a single dedicated, OS-thread-locked goroutine
// (see init/run below). Shell_NotifyIconW and the window that owns the
// notify icon must stay on the same OS thread for the lifetime of the
// process; previously the window was created lazily on whatever
// short-lived goroutine happened to call balloon() first (e.g. the
// goroutine running uploadFromClipboard), so once that goroutine
// returned, the owning thread's identity became unstable and later
// Shell_NotifyIconW calls silently failed.
var requests = make(chan balloonRequest, 8)

func init() {
	go run()
}

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

func run() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	className, _ := syscall.UTF16PtrFromString("ClipShotNotify")
	hInstance, _, _ := procGetModuleHandleW.Call(0)

	wc := wndClassExW{
		cbSize:        uint32(unsafe.Sizeof(wndClassExW{})),
		lpfnWndProc:   syscall.NewCallback(defWndProc),
		hInstance:     syscall.Handle(hInstance),
		lpszClassName: className,
	}
	if ret, _, err := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc))); ret == 0 {
		log.Printf("notify: RegisterClassExW failed: %v", err)
		return
	}

	title, _ := syscall.UTF16PtrFromString("ClipShotNotify")
	hwnd, _, err := procCreateWindowExW.Call(
		0, uintptr(unsafe.Pointer(className)), uintptr(unsafe.Pointer(title)),
		0, 0, 0, 1, 1, 0, 0, hInstance, 0,
	)
	if hwnd == 0 {
		log.Printf("notify: CreateWindowExW failed: %v", err)
		return
	}

	var nid notifyIconDataW
	nid.cbSize = uint32(unsafe.Sizeof(nid))
	nid.hWnd = hwnd
	nid.uID = 1
	nid.uFlags = nifState
	nid.dwState = nisHidden
	nid.dwStateMask = nisHidden
	toUTF16Array(nid.szTip[:], "ClipShot")
	if ret, _, err := procShellNotifyIconW.Call(nimAdd, uintptr(unsafe.Pointer(&nid))); ret == 0 {
		log.Printf("notify: Shell_NotifyIconW(NIM_ADD) failed: %v", err)
	}

	for req := range requests {
		var b notifyIconDataW
		b.cbSize = uint32(unsafe.Sizeof(b))
		b.hWnd = hwnd
		b.uID = 1
		b.uFlags = nifInfo | nifState
		b.dwState = nisHidden
		b.dwStateMask = nisHidden
		b.dwInfoFlags = req.flags
		toUTF16Array(b.szInfoTitle[:], req.title)
		toUTF16Array(b.szInfo[:], req.msg)

		if ret, _, err := procShellNotifyIconW.Call(nimModify, uintptr(unsafe.Pointer(&b))); ret == 0 {
			log.Printf("notify: Shell_NotifyIconW(NIM_MODIFY) failed: %v", err)
		}
	}
}

func send(title, msg string, flags uint32) {
	select {
	case requests <- balloonRequest{title, msg, flags}:
	default:
		log.Printf("notify: request queue full, dropping: %s", msg)
	}
}

// ShowInfo pops up an informational balloon notification (upload progress,
// success, etc).
func ShowInfo(message string) {
	send("ClipShot", message, niifInfo)
}

// ShowError pops up an error balloon notification. Unlike ShowInfo, Windows
// renders this with an error icon and it stays distinguishable in
// Notification history.
func ShowError(message string) {
	send("ClipShot", message, niifError)
}
