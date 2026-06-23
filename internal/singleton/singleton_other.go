//go:build !windows

package singleton

// Acquire always succeeds on non-Windows; ClipShot only ships for Windows.
func Acquire() bool { return true }
