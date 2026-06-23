package ui

import (
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

		go func() {
			for {
				select {
				case <-mUpload.ClickedCh:
					onUpload()
				case <-mSettings.ClickedCh:
					onSettings()
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
