//go:build windows

package ui

import (
	"fmt"
	"sync"
	"syscall"
	"unsafe"

	"github.com/Lapius7/clipshot-app/internal/config"
	"github.com/Lapius7/clipshot-app/internal/credstore"
)

var procMessageBoxW = user32.NewProc("MessageBoxW")

const (
	mbOK       = 0x0
	mbIconInfo = 0x40
	mbIconErr  = 0x10
)

func messageBox(title, text string, flags uint32) {
	t, _ := syscall.UTF16PtrFromString(title)
	m, _ := syscall.UTF16PtrFromString(text)
	procMessageBoxW.Call(0, uintptr(unsafe.Pointer(m)), uintptr(unsafe.Pointer(t)), uintptr(flags))
}

var ValidateHotkey func(combo string) error

type settingsResult struct {
	url    string
	token  string
	hotkey string
	ok     bool
}

const (
	idSettingsEditURL    = 200
	idSettingsEditToken  = 201
	idSettingsEditHotkey = 202
	idSettingsOK         = 210
	idSettingsCancel     = 211
)

var (
	settingsMu   sync.Mutex
	settingsData settingsResult
	settingsHwnd [3]uintptr
)

func ShowSettings(cfg *config.Config) {
	res := showSettingsDialog(cfg)
	if !res.ok {
		return
	}

	if err := config.Validate(res.url); err != nil {
		messageBox("ClipShot", err.Error(), mbOK|mbIconErr)
		return
	}

	if ValidateHotkey != nil {
		if err := ValidateHotkey(res.hotkey); err != nil {
			messageBox("ClipShot", fmt.Sprintf("invalid hotkey: %v", err), mbOK|mbIconErr)
			return
		}
	}

	cfg.InstanceURL = res.url
	cfg.Hotkey = res.hotkey
	if err := config.Save(cfg); err != nil {
		messageBox("ClipShot", fmt.Sprintf("failed to save settings: %v", err), mbOK|mbIconErr)
		return
	}

	if res.token != "" {
		if err := credstore.SaveToken(res.url, res.token); err != nil {
			messageBox("ClipShot", fmt.Sprintf("failed to save token: %v", err), mbOK|mbIconErr)
			return
		}
	}

	messageBox("ClipShot", "Settings saved.", mbOK|mbIconInfo)
}

func showSettingsDialog(cfg *config.Config) settingsResult {
	className, _ := syscall.UTF16PtrFromString("ClipShotSettingsDlg")
	hInstance, _, _ := procGetModuleHandleW.Call(0)

	wndProc := syscall.NewCallback(settingsWndProc)

	wc := wndClassExW{
		cbSize:        uint32(unsafe.Sizeof(wndClassExW{})),
		style:         3,
		lpfnWndProc:   wndProc,
		hInstance:     syscall.Handle(hInstance),
		hbrBackground: 16,
		lpszClassName: className,
	}
	procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))

	titleW, _ := syscall.UTF16PtrFromString("ClipShot Settings")
	hwnd, _, _ := procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(titleW)),
		uintptr(wsOverlappedWindow|wsVisible),
		0x80000000, 0x80000000, 460, 280,
		0, 0, hInstance, 0,
	)

	editClass, _ := syscall.UTF16PtrFromString("EDIT")
	staticClass, _ := syscall.UTF16PtrFromString("STATIC")
	buttonClass, _ := syscall.UTF16PtrFromString("BUTTON")
	groupBoxClass, _ := syscall.UTF16PtrFromString("BUTTON")
	okText, _ := syscall.UTF16PtrFromString("OK")
	cancelText, _ := syscall.UTF16PtrFromString("Cancel")

	// Connection group
	connGroup, _ := syscall.UTF16PtrFromString("Connection")
	procCreateWindowExW.Call(0x00000004, uintptr(unsafe.Pointer(groupBoxClass)), uintptr(unsafe.Pointer(connGroup)),
		uintptr(wsChild|wsVisible|0x00000007), 10, 5, 430, 105, hwnd, 0, hInstance, 0)

	urlLabel, _ := syscall.UTF16PtrFromString("Server URL:")
	procCreateWindowExW.Call(0, uintptr(unsafe.Pointer(staticClass)), uintptr(unsafe.Pointer(urlLabel)),
		uintptr(wsChild|wsVisible), 25, 25, 100, 18, hwnd, 0, hInstance, 0)
	urlDefault, _ := syscall.UTF16PtrFromString(cfg.InstanceURL)
	urlEdit, _, _ := procCreateWindowExW.Call(0x200, uintptr(unsafe.Pointer(editClass)), uintptr(unsafe.Pointer(urlDefault)),
		uintptr(wsChild|wsVisible|wsTabStop|0x800000|0x00800000), 125, 22, 300, 22, hwnd, uintptr(idSettingsEditURL), hInstance, 0)
	settingsHwnd[0] = urlEdit

	tokenLabel, _ := syscall.UTF16PtrFromString("API Token:")
	procCreateWindowExW.Call(0, uintptr(unsafe.Pointer(staticClass)), uintptr(unsafe.Pointer(tokenLabel)),
		uintptr(wsChild|wsVisible), 25, 55, 100, 18, hwnd, 0, hInstance, 0)
	tokenEdit, _, _ := procCreateWindowExW.Call(0x200, uintptr(unsafe.Pointer(editClass)), 0,
		uintptr(wsChild|wsVisible|wsTabStop|0x800000|0x00800000), 125, 52, 300, 22, hwnd, uintptr(idSettingsEditToken), hInstance, 0)
	settingsHwnd[1] = tokenEdit

	tokenHint, _ := syscall.UTF16PtrFromString("(leave blank to keep current)")
	procCreateWindowExW.Call(0, uintptr(unsafe.Pointer(staticClass)), uintptr(unsafe.Pointer(tokenHint)),
		uintptr(wsChild|wsVisible), 125, 76, 300, 16, hwnd, 0, hInstance, 0)

	// Hotkey group
	hkGroup, _ := syscall.UTF16PtrFromString("Hotkey")
	procCreateWindowExW.Call(0x00000004, uintptr(unsafe.Pointer(groupBoxClass)), uintptr(unsafe.Pointer(hkGroup)),
		uintptr(wsChild|wsVisible|0x00000007), 10, 115, 430, 65, hwnd, 0, hInstance, 0)

	hotkeyLabel, _ := syscall.UTF16PtrFromString("Shortcut:")
	procCreateWindowExW.Call(0, uintptr(unsafe.Pointer(staticClass)), uintptr(unsafe.Pointer(hotkeyLabel)),
		uintptr(wsChild|wsVisible), 25, 138, 100, 18, hwnd, 0, hInstance, 0)
	hotkeyDefault, _ := syscall.UTF16PtrFromString(cfg.Hotkey)
	hotkeyEdit, _, _ := procCreateWindowExW.Call(0x200, uintptr(unsafe.Pointer(editClass)), uintptr(unsafe.Pointer(hotkeyDefault)),
		uintptr(wsChild|wsVisible|wsTabStop|0x800000|0x00800000), 125, 135, 150, 22, hwnd, uintptr(idSettingsEditHotkey), hInstance, 0)
	settingsHwnd[2] = hotkeyEdit

	hotkeyHint, _ := syscall.UTF16PtrFromString("e.g. Ctrl+Shift+U")
	procCreateWindowExW.Call(0, uintptr(unsafe.Pointer(staticClass)), uintptr(unsafe.Pointer(hotkeyHint)),
		uintptr(wsChild|wsVisible), 285, 138, 140, 18, hwnd, 0, hInstance, 0)

	// Buttons
	procCreateWindowExW.Call(0, uintptr(unsafe.Pointer(buttonClass)), uintptr(unsafe.Pointer(okText)),
		uintptr(wsChild|wsVisible|wsTabStop|0x00000001), 260, 195, 80, 28, hwnd, uintptr(idSettingsOK), hInstance, 0)
	procCreateWindowExW.Call(0, uintptr(unsafe.Pointer(buttonClass)), uintptr(unsafe.Pointer(cancelText)),
		uintptr(wsChild|wsVisible|wsTabStop|0x00000001), 350, 195, 80, 28, hwnd, uintptr(idSettingsCancel), hInstance, 0)

	procSetFocus.Call(urlEdit)

	settingsMu.Lock()
	settingsData = settingsResult{}
	settingsMu.Unlock()

	var m msgStruct
	for {
		ret, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&m)), hwnd, 0, 0)
		if ret == 0 || ret == 0xFFFFFFFF {
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}

	settingsMu.Lock()
	defer settingsMu.Unlock()
	return settingsData
}

type msgStruct struct {
	Hwnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      struct{ X, Y int32 }
}

func settingsWndProc(hwnd uintptr, msg uint32, wParam, lParam uintptr) uintptr {
	switch msg {
	case wmCommand:
		id := wParam & 0xFFFF
		switch id {
		case idSettingsOK:
			buf := make([]uint16, 1024)
			settingsMu.Lock()
			for i, h := range settingsHwnd {
				procGetWindowTextW.Call(h, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
				val := syscall.UTF16ToString(buf)
				switch i {
				case 0:
					settingsData.url = val
				case 1:
					settingsData.token = val
				case 2:
					settingsData.hotkey = val
				}
			}
			settingsData.ok = true
			settingsMu.Unlock()
			procDestroyWindow.Call(hwnd)
			return 0
		case idSettingsCancel:
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
