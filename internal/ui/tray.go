package ui

import (
	"sync"

	"github.com/getlantern/systray"
)

// RunTray blocks running the system tray icon and menu. onUpload is invoked
// when the user clicks "Upload Image...", onSettings on "Settings", and
// onQuit on "Quit" (systray.Quit() is also called automatically afterward).
func RunTray(iconData []byte, onUpload func(), onSettings func(), onQuit func()) {
	systray.Run(func() {
		systray.SetIcon(iconData)
		systray.SetTooltip("ClipShot")

		mUpload := systray.AddMenuItem("Upload Image...", "Choose a file to upload")
		systray.AddSeparator()
		mSettings := systray.AddMenuItem("Settings", "Configure instance URL, API token, and hotkey")
		systray.AddSeparator()
		mQuit := systray.AddMenuItem("Quit", "Exit ClipShot")

		// onUpload/onSettings open blocking native dialogs, so each click is
		// dispatched to its own goroutine. That keeps this select loop free
		// to keep handling menu clicks (including Quit) while a dialog is
		// open. busy guards against opening the same dialog twice at once.
		var busyMu sync.Mutex
		busy := false
		runExclusive := func(fn func()) {
			busyMu.Lock()
			if busy {
				busyMu.Unlock()
				return
			}
			busy = true
			busyMu.Unlock()

			go func() {
				defer func() {
					busyMu.Lock()
					busy = false
					busyMu.Unlock()
				}()
				fn()
			}()
		}

		go func() {
			for {
				select {
				case <-mUpload.ClickedCh:
					runExclusive(onUpload)
				case <-mSettings.ClickedCh:
					runExclusive(onSettings)
				case <-mQuit.ClickedCh:
					systray.Quit()
					return
				}
			}
		}()
	}, func() {
		if onQuit != nil {
			onQuit()
		}
	})
}
