// Package updater checks for new releases on GitHub and self-updates the binary.
package updater

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	repoOwner = "Lapius7"
	repoName  = "clipshot-app"
	apiURL    = "https://api.github.com/repos/" + repoOwner + "/" + repoName + "/releases/latest"
)

type githubRelease struct {
	TagName string `json:"tag_name"`
	Assets  []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

var currentVersion = "dev"

// SetVersion sets the version string for comparison.
func SetVersion(v string) {
	currentVersion = v
}

// CheckAndUpdate checks for a new release and updates if available.
// Returns (updated, error).
func CheckAndUpdate() (bool, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(apiURL)
	if err != nil {
		return false, fmt.Errorf("check update: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false, nil
	}

	var release githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return false, fmt.Errorf("parse release: %w", err)
	}

	latest := strings.TrimPrefix(release.TagName, "v")
	if latest == currentVersion || latest == "" {
		return false, nil
	}

	assetName := "clipshot.exe"
	if runtime.GOOS == "darwin" {
		assetName = "clipshot-macos"
	} else if runtime.GOOS == "linux" {
		assetName = "clipshot-linux"
	}

	var downloadURL string
	for _, a := range release.Assets {
		if a.Name == assetName {
			downloadURL = a.BrowserDownloadURL
			break
		}
	}
	if downloadURL == "" {
		return false, nil
	}

	exePath, err := os.Executable()
	if err != nil {
		return false, fmt.Errorf("get executable path: %w", err)
	}
	exePath, err = filepath.EvalSymlinks(exePath)
	if err != nil {
		return false, fmt.Errorf("resolve executable path: %w", err)
	}

	tmpPath := exePath + ".tmp"
	if err := downloadFile(tmpPath, downloadURL); err != nil {
		os.Remove(tmpPath)
		return false, fmt.Errorf("download update: %w", err)
	}

	backupPath := exePath + ".bak"
	os.Remove(backupPath)
	os.Rename(exePath, backupPath)

	if err := os.Rename(tmpPath, exePath); err != nil {
		os.Rename(backupPath, exePath)
		os.Remove(tmpPath)
		return false, fmt.Errorf("apply update: %w", err)
	}

	return true, nil
}

func downloadFile(dst, url string) error {
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed: status %d", resp.StatusCode)
	}

	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = io.Copy(f, resp.Body)
	return err
}
