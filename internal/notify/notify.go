// Package notify shows brief feedback by temporarily changing the system
// tray icon's tooltip text, avoiding a separate native notification
// mechanism (and its own hwnd/icon-id bookkeeping).
package notify

import (
	"time"

	"github.com/getlantern/systray"
)

const resetAfter = 4 * time.Second

// Show sets the tray tooltip to message for a short duration, then restores
// the idle tooltip. Safe to call from any goroutine.
func Show(message string) {
	systray.SetTooltip(message)
	go func() {
		time.Sleep(resetAfter)
		systray.SetTooltip("ClipShot")
	}()
}
