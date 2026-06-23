//go:build !windows

package notify

import (
	"fmt"
	"time"

	"github.com/getlantern/systray"
)

const resetAfter = 4 * time.Second

func Show(message string) {
	fmt.Println(message)
	systray.SetTooltip(message)
	go func() {
		time.Sleep(resetAfter)
		systray.SetTooltip("ClipShot")
	}()
}
