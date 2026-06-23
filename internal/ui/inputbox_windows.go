//go:build windows

package ui

import (
	"errors"
	"sync"
	"syscall"
	"unsafe"
)

var errCancelled = errors.New("input cancelled")

var (
	procCreateWindowExW  = user32.NewProc("CreateWindowExW")
	procDestroyWindow    = user32.NewProc("DestroyWindow")
	procShowWindow       = user32.NewProc("ShowWindow")
	procGetWindowTextW   = user32.NewProc("GetWindowTextW")
	procDefWindowProcW   = user32.NewProc("DefWindowProcW")
	procRegisterClassExW = user32.NewProc("RegisterClassExW")
	procGetMessageW      = user32.NewProc("GetMessageW")
	procTranslateMessage = user32.NewProc("TranslateMessage")
	procDispatchMessageW = user32.NewProc("DispatchMessageW")
	procPostQuitMessage  = user32.NewProc("PostQuitMessage")
	procSetFocus         = user32.NewProc("SetFocus")

)

var (
	modkernel32          = syscall.NewLazyDLL("kernel32.dll")
	procGetModuleHandleW = modkernel32.NewProc("GetModuleHandleW")
)

const (
	wsOverlappedWindow = 0x00CF0000
	wsVisible          = 0x10000000
	wsChild            = 0x40000000
	wsTabStop          = 0x00010000
	wsBorder           = 0x00800000
	esLeft             = 0x0000
	bsPushButton       = 0x00000000

	wmDestroy  = 0x0002
	wmCommand  = 0x0111
	wmClose    = 0x0010

	idEdit   = 100
	idOK     = 101
	idCancel = 102
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

var (
	inputResultMu sync.Mutex
	inputResult   string
	inputOK       bool
	inputHwndEdit uintptr
)

// promptInput shows a small native modal-style window with a single text
// field, OK and Cancel buttons. Returns the entered text and whether OK was
// pressed (false on Cancel/close).
func promptInput(title, label, defaultValue string) (string, error) {
	className, _ := syscall.UTF16PtrFromString("ClipShotInputBox")
	hInstance, _, _ := procGetModuleHandleW.Call(0)

	wndProc := syscall.NewCallback(inputBoxWndProc)

	wc := wndClassExW{
		cbSize:        uint32(unsafe.Sizeof(wndClassExW{})),
		lpfnWndProc:   wndProc,
		hInstance:     syscall.Handle(hInstance),
		lpszClassName: className,
	}
	procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))

	titleW, _ := syscall.UTF16PtrFromString(title)
	hwnd, _, _ := procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(titleW)),
		uintptr(wsOverlappedWindow|wsVisible),
		0x80000000, 0x80000000, 420, 160, // CW_USEDEFAULT x/y
		0, 0, hInstance, 0,
	)

	labelW, _ := syscall.UTF16PtrFromString(label)
	editClass, _ := syscall.UTF16PtrFromString("EDIT")
	staticClass, _ := syscall.UTF16PtrFromString("STATIC")
	buttonClass, _ := syscall.UTF16PtrFromString("BUTTON")
	defaultValueW, _ := syscall.UTF16PtrFromString(defaultValue)
	okText, _ := syscall.UTF16PtrFromString("OK")
	cancelText, _ := syscall.UTF16PtrFromString("Cancel")

	procCreateWindowExW.Call(0, uintptr(unsafe.Pointer(staticClass)), uintptr(unsafe.Pointer(labelW)),
		uintptr(wsChild|wsVisible|esLeft), 10, 10, 380, 20, hwnd, 0, hInstance, 0)

	editHwnd, _, _ := procCreateWindowExW.Call(uintptr(wsBorder), uintptr(unsafe.Pointer(editClass)), uintptr(unsafe.Pointer(defaultValueW)),
		uintptr(wsChild|wsVisible|wsTabStop|wsBorder), 10, 35, 380, 24, hwnd, uintptr(idEdit), hInstance, 0)
	inputHwndEdit = editHwnd

	procCreateWindowExW.Call(0, uintptr(unsafe.Pointer(buttonClass)), uintptr(unsafe.Pointer(okText)),
		uintptr(wsChild|wsVisible|wsTabStop|bsPushButton), 220, 75, 80, 28, hwnd, uintptr(idOK), hInstance, 0)
	procCreateWindowExW.Call(0, uintptr(unsafe.Pointer(buttonClass)), uintptr(unsafe.Pointer(cancelText)),
		uintptr(wsChild|wsVisible|wsTabStop|bsPushButton), 310, 75, 80, 28, hwnd, uintptr(idCancel), hInstance, 0)

	procSetFocus.Call(editHwnd)

	inputResultMu.Lock()
	inputResult = ""
	inputOK = false
	inputResultMu.Unlock()

	var m struct {
		Hwnd    uintptr
		Message uint32
		WParam  uintptr
		LParam  uintptr
		Time    uint32
		Pt      struct{ X, Y int32 }
	}
	for {
		ret, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if ret == 0 {
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}

	inputResultMu.Lock()
	defer inputResultMu.Unlock()
	if !inputOK {
		return "", errCancelled
	}
	return inputResult, nil
}

func inputBoxWndProc(hwnd uintptr, msg uint32, wParam, lParam uintptr) uintptr {
	switch msg {
	case wmCommand:
		id := wParam & 0xFFFF
		switch id {
		case idOK:
			buf := make([]uint16, 1024)
			procGetWindowTextW.Call(inputHwndEdit, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
			inputResultMu.Lock()
			inputResult = syscall.UTF16ToString(buf)
			inputOK = true
			inputResultMu.Unlock()
			procDestroyWindow.Call(hwnd)
			return 0
		case idCancel:
			procDestroyWindow.Call(hwnd)
			return 0
		}
	case wmClose:
		procDestroyWindow.Call(hwnd)
		return 0
	case wmDestroy:
		procPostQuitMessage.Call(0)
		return 0
	}
	ret, _, _ := procDefWindowProcW.Call(hwnd, uintptr(msg), wParam, lParam)
	return ret
}
