//go:build windows

// Package ui implements the system tray menu and a native settings dialog
// built directly on Win32 APIs so the whole app stays CGO-free and
// cross-compiles cleanly from non-Windows hosts.
package ui

import (
	"fmt"
	"sync"
	"syscall"
	"unsafe"

	"github.com/Lapius7/clipshot-app/internal/config"
	"github.com/Lapius7/clipshot-app/internal/credstore"
)

var procMessageBox = user32.NewProc("MessageBoxW")

const (
	mbOK       = 0x0
	mbIconInfo = 0x40
	mbIconErr  = 0x10
)

func messageBox(title, text string, flags uint32) {
	t, _ := syscall.UTF16PtrFromString(title)
	m, _ := syscall.UTF16PtrFromString(text)
	procMessageBox.Call(0, uintptr(unsafe.Pointer(m)), uintptr(unsafe.Pointer(t)), uintptr(flags))
}

// ValidateHotkey is provided by the hotkey package's combo parser; declared
// here as a function value to avoid an import cycle (internal/hotkey does
// not depend on internal/ui).
var ValidateHotkey func(combo string) error

// settingsResult holds the values returned from the settings dialog.
type settingsResult struct {
	url     string
	token   string
	hotkey  string
	ok      bool
}

const (
	idSettingsEditURL    = 100
	idSettingsEditToken  = 101
	idSettingsEditHotkey = 102
	idSettingsOK         = 110
	idSettingsCancel     = 111
)

var (
	settingsMu   sync.Mutex
	settingsData settingsResult
	settingsHwnd [3]uintptr // URL, Token, Hotkey edit handles
)

// ShowSettings displays a single dialog with all three configuration fields
// (Instance URL, API Token, Hotkey) and saves them on OK.
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
	className, _ := syscall.UTF16PtrFromString("ClipShotSettings")
	hInstance, _, _ := procGetModuleHandleW.Call(0)

	wndProc := syscall.NewCallback(settingsWndProc)

	wc := wndClassExW{
		cbSize:        uint32(unsafe.Sizeof(wndClassExW{})),
		lpfnWndProc:   wndProc,
		hInstance:     syscall.Handle(hInstance),
		lpszClassName: className,
	}
	procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))

	titleW, _ := syscall.UTF16PtrFromString("ClipShot Settings")
	hwnd, _, _ := procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(titleW)),
		uintptr(wsOverlappedWindow|wsVisible),
		0x80000000, 0x80000000, 440, 250,
		0, 0, hInstance, 0,
	)

	editClass, _ := syscall.UTF16PtrFromString("EDIT")
	staticClass, _ := syscall.UTF16PtrFromString("STATIC")
	buttonClass, _ := syscall.UTF16PtrFromString("BUTTON")
	okText, _ := syscall.UTF16PtrFromString("OK")
	cancelText, _ := syscall.UTF16PtrFromString("Cancel")

	// Instance URL
	urlLabel, _ := syscall.UTF16PtrFromString("Instance URL (https://...):")
	procCreateWindowExW.Call(0, uintptr(unsafe.Pointer(staticClass)), uintptr(unsafe.Pointer(urlLabel)),
		uintptr(wsChild|wsVisible|esLeft), 10, 10, 400, 20, hwnd, 0, hInstance, 0)
	urlDefault, _ := syscall.UTF16PtrFromString(cfg.InstanceURL)
	urlEdit, _, _ := procCreateWindowExW.Call(uintptr(wsBorder), uintptr(unsafe.Pointer(editClass)), uintptr(unsafe.Pointer(urlDefault)),
		uintptr(wsChild|wsVisible|wsTabStop|wsBorder), 10, 32, 400, 24, hwnd, uintptr(idSettingsEditURL), hInstance, 0)
	settingsHwnd[0] = urlEdit

	// API Token
	tokenLabel, _ := syscall.UTF16PtrFromString("API Token (leave blank to keep current):")
	procCreateWindowExW.Call(0, uintptr(unsafe.Pointer(staticClass)), uintptr(unsafe.Pointer(tokenLabel)),
		uintptr(wsChild|wsVisible|esLeft), 10, 66, 400, 20, hwnd, 0, hInstance, 0)
	tokenEdit, _, _ := procCreateWindowExW.Call(uintptr(wsBorder), uintptr(unsafe.Pointer(editClass)), 0,
		uintptr(wsChild|wsVisible|wsTabStop|wsBorder), 10, 88, 400, 24, hwnd, uintptr(idSettingsEditToken), hInstance, 0)
	settingsHwnd[1] = tokenEdit

	// Hotkey
	hotkeyLabel, _ := syscall.UTF16PtrFromString("Hotkey (e.g. Ctrl+Shift+U):")
	procCreateWindowExW.Call(0, uintptr(unsafe.Pointer(staticClass)), uintptr(unsafe.Pointer(hotkeyLabel)),
		uintptr(wsChild|wsVisible|esLeft), 10, 122, 400, 20, hwnd, 0, hInstance, 0)
	hotkeyDefault, _ := syscall.UTF16PtrFromString(cfg.Hotkey)
	hotkeyEdit, _, _ := procCreateWindowExW.Call(uintptr(wsBorder), uintptr(unsafe.Pointer(editClass)), uintptr(unsafe.Pointer(hotkeyDefault)),
		uintptr(wsChild|wsVisible|wsTabStop|wsBorder), 10, 144, 400, 24, hwnd, uintptr(idSettingsEditHotkey), hInstance, 0)
	settingsHwnd[2] = hotkeyEdit

	// Buttons
	procCreateWindowExW.Call(0, uintptr(unsafe.Pointer(buttonClass)), uintptr(unsafe.Pointer(okText)),
		uintptr(wsChild|wsVisible|wsTabStop|bsPushButton), 230, 185, 80, 28, hwnd, uintptr(idSettingsOK), hInstance, 0)
	procCreateWindowExW.Call(0, uintptr(unsafe.Pointer(buttonClass)), uintptr(unsafe.Pointer(cancelText)),
		uintptr(wsChild|wsVisible|wsTabStop|bsPushButton), 330, 185, 80, 28, hwnd, uintptr(idSettingsCancel), hInstance, 0)

	procSetFocus.Call(urlEdit)

	settingsMu.Lock()
	settingsData = settingsResult{}
	settingsMu.Unlock()

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

	settingsMu.Lock()
	defer settingsMu.Unlock()
	return settingsData
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
