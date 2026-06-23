//go:build windows

// Package singleton prevents more than one ClipShot instance from running
// at once. Two instances fight over the same global hotkey and the same
// tray notify icon, so the second one mostly just causes confusing
// silent failures (e.g. "hotkey already registered") instead of behaving
// like a fresh launch.
package singleton

import (
	"syscall"
	"unsafe"
)

var (
	kernel32         = syscall.NewLazyDLL("kernel32.dll")
	procCreateMutexW = kernel32.NewProc("CreateMutexW")
)

const errorAlreadyExists = 183

// Acquire tries to claim a process-wide lock. It returns true if this is
// the only running instance, false if another instance already holds the
// lock. The lock is held for the lifetime of the process (released
// automatically when it exits).
func Acquire() bool {
	name, _ := syscall.UTF16PtrFromString("ClipShotSingleInstanceMutex")
	_, _, errno := procCreateMutexW.Call(0, 1, uintptr(unsafe.Pointer(name)))
	return errno != syscall.Errno(errorAlreadyExists)
}
