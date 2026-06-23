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

func main() {
	ui.ValidateHotkey = hotkey.Validate
	updater.SetVersion(version)

	if updated, err := updater.CheckAndUpdate(); err != nil {
		log.Printf("update check failed: %v", err)
	} else if updated {
		notify.Show("ClipShot updated! Restarting...")
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
				go doUpload(cfg, "clipboard.png", func() ([]byte, string, error) {
					data, err := clipboard.ReadImagePNG()
					return data, "image/png", err
				})
			}
		}()
	}

	onUpload := func() {
		path, err := ui.PickImageFile()
		if err != nil {
			if !errors.Is(err, ui.ErrNoFileSelected) {
				notify.Show(fmt.Sprintf("ClipShot: %v", err))
			}
			return
		}
		go doUpload(cfg, filepath.Base(path), func() ([]byte, string, error) {
			data, err := os.ReadFile(path)
			return data, http.DetectContentType(data), err
		})
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

func doUpload(cfg *config.Config, filename string, loadData func() ([]byte, string, error)) {
	notify.Show("ClipShot: Uploading...")

	data, contentType, err := loadData()
	if err != nil {
		notify.Show(fmt.Sprintf("ClipShot: %v", err))
		return
	}

	token, err := credstore.LoadToken(cfg.InstanceURL)
	if err != nil {
		notify.Show("ClipShot: No API token configured")
		return
	}

	client, err := uploader.New(cfg.InstanceURL, token)
	if err != nil {
		notify.Show(fmt.Sprintf("ClipShot: %v", err))
		return
	}

	url, err := client.Upload(filename, contentType, data)
	if err != nil {
		notify.Show(fmt.Sprintf("ClipShot: Upload failed: %v", err))
		return
	}

	if err := clipboard.WriteText(url); err != nil {
		notify.Show(fmt.Sprintf("ClipShot: Upload OK but clipboard failed: %v", err))
		return
	}

	notify.Show("ClipShot: URL copied!")
}
