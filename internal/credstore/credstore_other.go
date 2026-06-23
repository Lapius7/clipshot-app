//go:build !windows

package credstore

import "errors"

var errUnsupported = errors.New("credstore is only supported on windows")

func SaveToken(instanceURL, token string) error    { return errUnsupported }
func LoadToken(instanceURL string) (string, error) { return "", errUnsupported }
func DeleteToken(instanceURL string) error          { return errUnsupported }
