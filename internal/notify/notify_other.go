//go:build !windows

// Package notify shows brief feedback by temporarily changing the system
// tray icon's tooltip text. This is a non-Windows fallback so the package
// still cross-compiles; ClipShot itself only ships for Windows, where
// notify_windows.go's balloon notifications are used instead.
package notify

import (
	"sync"
	"time"

	"github.com/getlantern/systray"
)

const resetAfter = 4 * time.Second

var (
	mu      sync.Mutex
	timer   *time.Timer
	lastMsg string
)

func show(message string) {
	mu.Lock()
	defer mu.Unlock()

	lastMsg = message
	systray.SetTooltip(message)

	if timer != nil {
		timer.Stop()
	}
	timer = time.AfterFunc(resetAfter, func() {
		mu.Lock()
		defer mu.Unlock()
		if lastMsg == message {
			systray.SetTooltip("ClipShot")
		}
	})
}

func ShowInfo(message string)  { show(message) }
func ShowError(message string) { show(message) }
