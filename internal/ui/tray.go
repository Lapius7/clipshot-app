package ui

import (
	"fmt"
	"sync"

	"github.com/getlantern/systray"
)

// RunTray blocks running the system tray icon and menu. onUpload is invoked
// when the user clicks "Upload Image...", onSettings on "Settings",
// onCheckUpdate on "Check for Updates...", and onQuit on "Quit" (systray.Quit()
// is also called automatically afterward).
func RunTray(iconData []byte, version string, onUpload, onSettings, onCheckUpdate, onQuit func()) {
	systray.Run(func() {
		systray.SetIcon(iconData)
		systray.SetTooltip("ClipShot")

		mVersion := systray.AddMenuItem(fmt.Sprintf("ClipShot v%s", version), "")
		mVersion.Disable()
		systray.AddSeparator()
		mUpload := systray.AddMenuItem("Upload Image...", "Choose a file to upload")
		systray.AddSeparator()
		mSettings := systray.AddMenuItem("Settings", "Configure instance URL, API token, and hotkey")
		mCheckUpdate := systray.AddMenuItem("Check for Updates...", "Check GitHub for a newer release")
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
				case <-mCheckUpdate.ClickedCh:
					runExclusive(onCheckUpdate)
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
