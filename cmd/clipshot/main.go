package main

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/Lapius7/clipshot-app/internal/clipboard"
	"github.com/Lapius7/clipshot-app/internal/config"
	"github.com/Lapius7/clipshot-app/internal/credstore"
	"github.com/Lapius7/clipshot-app/internal/hotkey"
	"github.com/Lapius7/clipshot-app/internal/notify"
	"github.com/Lapius7/clipshot-app/internal/ui"
	"github.com/Lapius7/clipshot-app/internal/updater"
	"github.com/Lapius7/clipshot-app/internal/uploader"
)

var version = "dev"

// initLogFile redirects the standard logger to clipshot.log next to the
// executable. The app is built with -H windowsgui (no console), so without
// this, log.Printf output (including notify's Shell_NotifyIconW failures)
// went nowhere and silent failures were undebuggable.
func initLogFile() {
	exePath, err := os.Executable()
	if err != nil {
		return
	}
	f, err := os.Create(filepath.Join(filepath.Dir(exePath), "clipshot.log"))
	if err != nil {
		return
	}
	log.SetOutput(f)
}

func main() {
	initLogFile()
	ui.ValidateHotkey = hotkey.Validate
	updater.SetVersion(version)

	if updated, err := updater.CheckAndUpdate(); err != nil {
		log.Printf("update check failed: %v", err)
	} else if updated {
		notify.ShowInfo("Updated! Restarting...")
		exePath, _ := os.Executable()
		os.StartProcess(exePath, os.Args, &os.ProcAttr{Dir: filepath.Dir(exePath)})
		return
	}

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config error: %v", err)
	}

	var listener *hotkey.Listener

	startHotkey := func() {
		if listener != nil {
			listener.Close()
			listener = nil
		}
		if cfg.InstanceURL == "" {
			return
		}
		l, ch, err := hotkey.Register(cfg.Hotkey)
		if err != nil {
			log.Printf("failed to register hotkey %q: %v", cfg.Hotkey, err)
			return
		}
		listener = l
		go func() {
			for range ch {
				go uploadFromClipboard(cfg)
			}
		}()
	}

	onUpload := func() {
		path, err := ui.PickImageFile()
		if err != nil {
			if !errors.Is(err, ui.ErrNoFileSelected) {
				notify.ShowError(err.Error())
			}
			return
		}
		go uploadFromFile(cfg, path)
	}

	onSettings := func() {
		ui.ShowSettings(cfg)
		startHotkey()
	}

	onQuit := func() {
		if listener != nil {
			listener.Close()
		}
	}

	startHotkey()
	ui.RunTray(trayIcon, onUpload, onSettings, onQuit)
}

func uploadFromClipboard(cfg *config.Config) {
	notify.ShowInfo("Uploading...")

	data, err := clipboard.ReadImagePNG()
	if err != nil {
		notify.ShowError(err.Error())
		return
	}

	uploadAndNotify(cfg, "clipboard.png", "image/png", data)
}

func uploadFromFile(cfg *config.Config, path string) {
	notify.ShowInfo("Uploading...")

	data, err := os.ReadFile(path)
	if err != nil {
		notify.ShowError(fmt.Sprintf("Read error: %v", err))
		return
	}

	contentType := http.DetectContentType(data)
	uploadAndNotify(cfg, filepath.Base(path), contentType, data)
}

func uploadAndNotify(cfg *config.Config, filename, contentType string, data []byte) {
	token, err := credstore.LoadToken(cfg.InstanceURL)
	if err != nil {
		notify.ShowError("No API token - open Settings")
		return
	}

	client, err := uploader.New(cfg.InstanceURL, token)
	if err != nil {
		notify.ShowError(err.Error())
		return
	}

	url, err := client.Upload(filename, contentType, data)
	if err != nil {
		notify.ShowError(fmt.Sprintf("Upload failed: %v", err))
		return
	}

	if err := clipboard.WriteText(url); err != nil {
		notify.ShowError("Uploaded but clipboard failed")
		return
	}

	notify.ShowInfo("URL copied!")
}
