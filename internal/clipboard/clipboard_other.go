//go:build !windows

package clipboard

import "errors"

var errUnsupported = errors.New("clipboard is only supported on windows")
var ErrNoImage = errUnsupported

func ReadImagePNG() ([]byte, error) { return nil, errUnsupported }
func WriteText(text string) error   { return errUnsupported }
