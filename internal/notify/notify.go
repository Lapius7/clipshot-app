// Package notify shows brief feedback by temporarily changing the system
// tray icon's tooltip text.
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

func Show(message string) {
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
