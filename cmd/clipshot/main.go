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
	"github.com/Lapius7/clipshot-app/internal/uploader"
)

func main() {
	ui.ValidateHotkey = hotkey.Validate

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config error: %v", err)
	}

	var listener *hotkey.Listener
	var events <-chan struct{}

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
		events = ch
		go handleHotkeyEvents(cfg, events)
	}

	onUpload := func() {
		if err := uploadFromFileDialog(cfg); err != nil {
			if !errors.Is(err, ui.ErrNoFileSelected) {
				notify.Show(fmt.Sprintf("ClipShot: %v", err))
			}
			return
		}
		notify.Show("ClipShot: URL copied to clipboard")
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

func handleHotkeyEvents(cfg *config.Config, events <-chan struct{}) {
	for range events {
		data, err := clipboard.ReadImagePNG()
		if err != nil {
			notify.Show(fmt.Sprintf("ClipShot: %v", err))
			continue
		}
		if err := uploadAndCopyURL(cfg, "clipboard.png", "image/png", data); err != nil {
			notify.Show(fmt.Sprintf("ClipShot: %v", err))
			continue
		}
		notify.Show("ClipShot: URL copied to clipboard")
	}
}

func uploadFromFileDialog(cfg *config.Config) error {
	path, err := ui.PickImageFile()
	if err != nil {
		return err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	contentType := http.DetectContentType(data)
	return uploadAndCopyURL(cfg, filepath.Base(path), contentType, data)
}

// uploadAndCopyURL loads the configured token, uploads data to the
// configured instance, and writes the resulting URL to the clipboard. It is
// shared by both the hotkey (clipboard image) and tray menu (file dialog)
// upload paths.
func uploadAndCopyURL(cfg *config.Config, filename, contentType string, data []byte) error {
	token, err := credstore.LoadToken(cfg.InstanceURL)
	if err != nil {
		return fmt.Errorf("no API token configured: %w", err)
	}

	client, err := uploader.New(cfg.InstanceURL, token)
	if err != nil {
		return err
	}

	url, err := client.Upload(filename, contentType, data)
	if err != nil {
		return err
	}

	if err := clipboard.WriteText(url); err != nil {
		return fmt.Errorf("upload succeeded but failed to copy url: %w", err)
	}
	return nil
}
