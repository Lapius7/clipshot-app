package main

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/Lapius7/clipshot-app/internal/clipboard"
	"github.com/Lapius7/clipshot-app/internal/config"
	"github.com/Lapius7/clipshot-app/internal/credstore"
	"github.com/Lapius7/clipshot-app/internal/hotkey"
	"github.com/Lapius7/clipshot-app/internal/notify"
	"github.com/Lapius7/clipshot-app/internal/singleton"
	"github.com/Lapius7/clipshot-app/internal/ui"
	"github.com/Lapius7/clipshot-app/internal/updater"
	"github.com/Lapius7/clipshot-app/internal/uploader"
)

var version = "dev"

// showError logs the error and pops up a balloon notification. Balloon
// notifications alone proved unreliable to diagnose (see the v0.1.5/v0.1.6
// notify fixes), so every error shown to the user is now also recorded in
// clipshot.log.
func showError(message string) {
	log.Printf("error: %s", message)
	notify.ShowError(message)
}

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

// restartGuardPath returns the path of a marker file (next to the exe) that
// records the last self-update restart time, so a version-comparison bug
// can't spin the app into restarting forever with no visible symptom other
// than "nothing responds".
func restartGuardPath() string {
	exePath, err := os.Executable()
	if err != nil {
		return ""
	}
	return filepath.Join(filepath.Dir(exePath), ".clipshot-restart")
}

const minRestartInterval = 60 * time.Second

func recentlyRestarted() bool {
	p := restartGuardPath()
	if p == "" {
		return false
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return false
	}
	sec, err := strconv.ParseInt(string(data), 10, 64)
	if err != nil {
		return false
	}
	return time.Since(time.Unix(sec, 0)) < minRestartInterval
}

func markRestart() {
	p := restartGuardPath()
	if p == "" {
		return
	}
	_ = os.WriteFile(p, []byte(strconv.FormatInt(time.Now().Unix(), 10)), 0o600)
}

// checkForUpdate checks GitHub for a newer release and applies it. When
// silent is true (startup path), it only notifies on update/error, never on
// "already up to date", to avoid noise on every launch.
func checkForUpdate(silent bool) {
	log.Printf("checking for update (current version=%s)", version)

	if silent && recentlyRestarted() {
		log.Printf("skipping update check: restarted within the last %s", minRestartInterval)
		return
	}

	updated, err := updater.CheckAndUpdate()
	if err != nil {
		log.Printf("update check failed: %v", err)
		if !silent {
			notify.ShowError(fmt.Sprintf("Update check failed: %v", err))
		}
		return
	}
	if !updated {
		log.Printf("no update available")
		if !silent {
			notify.ShowInfo("You're up to date.")
		}
		return
	}

	log.Printf("update applied, restarting")
	notify.ShowInfo("Updated! Restarting...")
	markRestart()
	exePath, _ := os.Executable()
	os.StartProcess(exePath, os.Args, &os.ProcAttr{Dir: filepath.Dir(exePath)})
	os.Exit(0)
}

func main() {
	initLogFile()
	log.Printf("ClipShot v%s starting", version)

	if !singleton.Acquire() {
		log.Printf("another instance is already running, exiting")
		notify.ShowError("ClipShot is already running.")
		return
	}

	ui.ValidateHotkey = hotkey.Validate
	updater.SetVersion(version)

	checkForUpdate(true)

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
			showError(fmt.Sprintf("Hotkey registration failed (%s): %v -- if another ClipShot window/process is running, close it first", cfg.Hotkey, err))
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
				showError(fmt.Sprintf("File picker failed: %v", err))
			}
			return
		}
		go uploadFromFile(cfg, path)
	}

	onSettings := func() {
		ui.ShowSettings(cfg)
		startHotkey()
	}

	onCheckUpdate := func() {
		checkForUpdate(false)
	}

	onQuit := func() {
		if listener != nil {
			listener.Close()
		}
	}

	startHotkey()
	ui.RunTray(trayIcon, version, onUpload, onSettings, onCheckUpdate, onQuit)
}

func uploadFromClipboard(cfg *config.Config) {
	log.Printf("upload: starting from clipboard")
	notify.ShowInfo("Uploading...")

	data, err := clipboard.ReadImagePNG()
	if err != nil {
		showError(fmt.Sprintf("Clipboard read failed: %v", err))
		return
	}

	uploadAndNotify(cfg, "clipboard.png", "image/png", data)
}

func uploadFromFile(cfg *config.Config, path string) {
	log.Printf("upload: starting from file %q", path)
	notify.ShowInfo("Uploading...")

	data, err := os.ReadFile(path)
	if err != nil {
		showError(fmt.Sprintf("Read error: %v", err))
		return
	}

	contentType := http.DetectContentType(data)
	uploadAndNotify(cfg, filepath.Base(path), contentType, data)
}

func uploadAndNotify(cfg *config.Config, filename, contentType string, data []byte) {
	token, err := credstore.LoadToken(cfg.InstanceURL)
	if err != nil {
		showError("No API token - open Settings")
		return
	}

	client, err := uploader.New(cfg.InstanceURL, token)
	if err != nil {
		showError(fmt.Sprintf("Uploader init failed: %v", err))
		return
	}

	url, err := client.Upload(filename, contentType, data)
	if err != nil {
		showError(fmt.Sprintf("Upload failed: %v", err))
		return
	}
	log.Printf("upload: succeeded, url=%s", url)

	if err := clipboard.WriteText(url); err != nil {
		showError(fmt.Sprintf("Uploaded but clipboard write failed: %v", err))
		return
	}

	notify.ShowInfo("URL copied!")
}
