//go:build windows

package ui

import (
	"fmt"
	"runtime"
	"sync"
	"syscall"
	"unsafe"

	"github.com/Lapius7/clipshot-app/internal/config"
	"github.com/Lapius7/clipshot-app/internal/credstore"
)

var (
	procMessageBoxW          = user32.NewProc("MessageBoxW")
	procSendMessageW         = user32.NewProc("SendMessageW")
	procSystemParametersInfo = user32.NewProc("SystemParametersInfoW")
	gdi32                    = syscall.NewLazyDLL("gdi32.dll")
	procCreateFontIndirectW  = gdi32.NewProc("CreateFontIndirectW")
)

const wmSetFont = 0x0030

// uiFont is the system message-box font (matches what Explorer/most modern
// apps use), applied to every control instead of the legacy default GUI
// font that made the old dialog look dated.
var uiFont uintptr

// nonClientMetrics mirrors NONCLIENTMETRICSW, trimmed to the fields needed
// to pull lfMessageFont out via SystemParametersInfo(SPI_GETNONCLIENTMETRICS).
type nonClientMetrics struct {
	cbSize          uint32
	borderWidth     int32
	scrollWidth     int32
	scrollHeight    int32
	captionWidth    int32
	captionHeight   int32
	captionFont     logFont
	smCaptionWidth  int32
	smCaptionHeight int32
	smCaptionFont   logFont
	menuWidth       int32
	menuHeight      int32
	menuFont        logFont
	statusFont      logFont
	messageFont     logFont
}

type logFont struct {
	height         int32
	width          int32
	escapement     int32
	orientation    int32
	weight         int32
	italic         byte
	underline      byte
	strikeOut      byte
	charSet        byte
	outPrecision   byte
	clipPrecision  byte
	quality        byte
	pitchAndFamily byte
	faceName       [32]uint16
}

const spiGetNonClientMetrics = 0x0029

func loadUIFont() uintptr {
	if uiFont != 0 {
		return uiFont
	}
	var ncm nonClientMetrics
	ncm.cbSize = uint32(unsafe.Sizeof(ncm))
	ret, _, _ := procSystemParametersInfo.Call(spiGetNonClientMetrics, uintptr(ncm.cbSize), uintptr(unsafe.Pointer(&ncm)), 0)
	if ret == 0 {
		return 0
	}
	hFont, _, _ := procCreateFontIndirectW.Call(uintptr(unsafe.Pointer(&ncm.messageFont)))
	uiFont = hFont
	return uiFont
}

func applyFont(hwnd uintptr) {
	font := loadUIFont()
	if font == 0 {
		return
	}
	procSendMessageW.Call(hwnd, wmSetFont, font, 1)
}

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
	// Show whatever token is currently stored so the user can verify it was
	// saved correctly, instead of always starting from a blank field --
	// that made it impossible to confirm a re-issued token had actually
	// been applied without going through a failed upload first.
	currentToken, _ := credstore.LoadToken(cfg.InstanceURL)

	// The dialog creates a window and runs its own GetMessageW loop, both of
	// which must stay pinned to one OS thread for the duration of the
	// dialog's life. Running this on a dedicated, locked OS thread keeps it
	// isolated from systray's own message pump and from Go's scheduler
	// migrating the goroutine mid-loop, which previously caused the dialog
	// to hang intermittently.
	resCh := make(chan settingsResult, 1)
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		resCh <- showSettingsDialog(cfg, currentToken)
	}()
	res := <-resCh

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

func showSettingsDialog(cfg *config.Config, currentToken string) settingsResult {
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

	const (
		winWidth  = 520
		winHeight = 340
	)

	titleW, _ := syscall.UTF16PtrFromString("ClipShot Settings")
	hwnd, _, _ := procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(titleW)),
		uintptr(wsOverlappedWindow&^0x00020000|wsVisible), // drop WS_MAXIMIZEBOX, dialog isn't resizable
		0x80000000, 0x80000000, winWidth, winHeight,
		0, 0, hInstance, 0,
	)

	editClass, _ := syscall.UTF16PtrFromString("EDIT")
	staticClass, _ := syscall.UTF16PtrFromString("STATIC")
	buttonClass, _ := syscall.UTF16PtrFromString("BUTTON")
	groupBoxClass, _ := syscall.UTF16PtrFromString("BUTTON")

	const (
		marginX = 16
		fieldX  = 130
		fieldW  = 350
		labelW  = 100
		groupW  = winWidth - 2*marginX - 14
	)

	all := make([]uintptr, 0, 16)
	create := func(class *uint16, text string, style uint32, x, y, w, h int32, id uintptr) uintptr {
		var textPtr uintptr
		if text != "" {
			p, _ := syscall.UTF16PtrFromString(text)
			textPtr = uintptr(unsafe.Pointer(p))
		}
		h2, _, _ := procCreateWindowExW.Call(0, uintptr(unsafe.Pointer(class)), textPtr,
			uintptr(style), uintptr(x), uintptr(y), uintptr(w), uintptr(h), hwnd, id, hInstance, 0)
		all = append(all, h2)
		return h2
	}

	// Connection group
	create(groupBoxClass, "Connection", wsChild|wsVisible|0x00000007, marginX, 12, groupW, 110, 0)

	create(staticClass, "Server URL:", wsChild|wsVisible, marginX+16, 38, labelW, 18, 0)
	urlEdit := create(editClass, cfg.InstanceURL, wsChild|wsVisible|wsTabStop|0x00800000, fieldX, 35, fieldW, 22, idSettingsEditURL)
	settingsHwnd[0] = urlEdit

	create(staticClass, "API Token:", wsChild|wsVisible, marginX+16, 68, labelW, 18, 0)
	tokenEdit := create(editClass, currentToken, wsChild|wsVisible|wsTabStop|0x00800000, fieldX, 65, fieldW, 22, idSettingsEditToken)
	settingsHwnd[1] = tokenEdit

	create(staticClass, "(shown in plain text -- this PC only)", wsChild|wsVisible, fieldX, 92, fieldW, 16, 0)

	// Hotkey group
	create(groupBoxClass, "Hotkey", wsChild|wsVisible|0x00000007, marginX, 134, groupW, 70, 0)

	create(staticClass, "Shortcut:", wsChild|wsVisible, marginX+16, 162, labelW, 18, 0)
	hotkeyEdit := create(editClass, cfg.Hotkey, wsChild|wsVisible|wsTabStop|0x00800000, fieldX, 159, 160, 22, idSettingsEditHotkey)
	settingsHwnd[2] = hotkeyEdit

	create(staticClass, "e.g. Ctrl+Shift+U", wsChild|wsVisible, fieldX+172, 162, 180, 18, 0)

	// Buttons (bottom-right)
	btnW, btnH := int32(88), int32(30)
	create(buttonClass, "OK", wsChild|wsVisible|wsTabStop|0x00000001, winWidth-2*btnW-32, winHeight-66, btnW, btnH, idSettingsOK)
	create(buttonClass, "Cancel", wsChild|wsVisible|wsTabStop|0x00000001, winWidth-btnW-16, winHeight-66, btnW, btnH, idSettingsCancel)

	for _, h := range all {
		applyFont(h)
	}

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
