//go:build windows

// Package ui implements the system tray menu and a minimal native settings
// dialog (instance URL + API token), built directly on Win32 APIs so the
// whole app stays CGO-free and cross-compiles cleanly from non-Windows hosts.
package ui

import (
	"fmt"
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

// ShowSettings is a minimal settings flow: it reports the current values
// and prompts via sequential native input boxes. A richer dialog (edit
// controls in a single window) is a natural follow-up once the skeleton is
// validated; this keeps the app dependency-free for now.
func ShowSettings(cfg *config.Config) {
	url, err := promptInput("ClipShot Settings", "Instance URL (https://...)", cfg.InstanceURL)
	if err != nil || url == "" {
		return
	}
	if err := config.Validate(url); err != nil {
		messageBox("ClipShot", err.Error(), mbOK|mbIconErr)
		return
	}

	token, err := promptInput("ClipShot Settings", "API Token (leave blank to keep current)", "")
	if err != nil {
		return
	}

	hotkey, err := promptInput("ClipShot Settings", "Hotkey (e.g. Ctrl+Shift+U)", cfg.Hotkey)
	if err != nil || hotkey == "" {
		return
	}
	if ValidateHotkey != nil {
		if err := ValidateHotkey(hotkey); err != nil {
			messageBox("ClipShot", fmt.Sprintf("invalid hotkey: %v", err), mbOK|mbIconErr)
			return
		}
	}

	cfg.InstanceURL = url
	cfg.Hotkey = hotkey
	if err := config.Save(cfg); err != nil {
		messageBox("ClipShot", fmt.Sprintf("failed to save settings: %v", err), mbOK|mbIconErr)
		return
	}

	if token != "" {
		if err := credstore.SaveToken(url, token); err != nil {
			messageBox("ClipShot", fmt.Sprintf("failed to save token: %v", err), mbOK|mbIconErr)
			return
		}
	}

	messageBox("ClipShot", "Settings saved.", mbOK|mbIconInfo)
}
