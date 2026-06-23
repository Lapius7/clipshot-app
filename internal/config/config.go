// Package config manages clipshot-app's non-secret settings, persisted as
// JSON under %APPDATA%\ClipShot\config.json. The API token itself is never
// written here -- see internal/credstore.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Config struct {
	InstanceURL string `json:"instance_url"`
	Hotkey      string `json:"hotkey"`
}

var ErrInsecureURL = errors.New("instance url must start with https://")

func Validate(instanceURL string) error {
	if !strings.HasPrefix(instanceURL, "https://") {
		return ErrInsecureURL
	}
	return nil
}

func dir() (string, error) {
	appData := os.Getenv("APPDATA")
	if appData == "" {
		return "", fmt.Errorf("APPDATA environment variable not set")
	}
	return filepath.Join(appData, "ClipShot"), nil
}

func path() (string, error) {
	d, err := dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "config.json"), nil
}

func Load() (*Config, error) {
	p, err := path()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(p)
	if errors.Is(err, os.ErrNotExist) {
		return &Config{Hotkey: "Ctrl+Shift+U"}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	var c Config
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	if c.Hotkey == "" {
		c.Hotkey = "Ctrl+Shift+U"
	}
	return &c, nil
}

func Save(c *Config) error {
	if err := Validate(c.InstanceURL); err != nil {
		return err
	}
	d, err := dir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(d, 0o700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	p, err := path()
	if err != nil {
		return err
	}
	if err := os.WriteFile(p, data, 0o600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return nil
}
