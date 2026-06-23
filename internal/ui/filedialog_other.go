//go:build !windows

package ui

import "errors"

var ErrNoFileSelected = errors.New("no file selected")

func PickImageFile() (string, error) { return "", errors.New("file dialog is only supported on windows") }
