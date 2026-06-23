//go:build !windows

package hotkey

import "errors"

var errUnsupported = errors.New("hotkey is only supported on windows")

type Listener struct{}

func Register(combo string) (*Listener, <-chan struct{}, error) { return nil, nil, errUnsupported }
func (l *Listener) Close()                                      {}
func Validate(combo string) error                                { return errUnsupported }
