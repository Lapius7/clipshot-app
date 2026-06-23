//go:build windows

package ui

import (
	"errors"
	"runtime"
	"syscall"
	"unsafe"
)

var (
	comdlg32            = syscall.NewLazyDLL("comdlg32.dll")
	procGetOpenFileName = comdlg32.NewProc("GetOpenFileNameW")
)

const (
	ofnFileMustExist = 0x00001000
	ofnPathMustExist = 0x00000800
	ofnExplorer      = 0x00080000
)

type openFileNameW struct {
	lStructSize       uint32
	hwndOwner         uintptr
	hInstance         uintptr
	lpstrFilter       *uint16
	lpstrCustomFilter *uint16
	nMaxCustFilter    uint32
	nFilterIndex      uint32
	lpstrFile         *uint16
	nMaxFile          uint32
	lpstrFileTitle    *uint16
	nMaxFileTitle     uint32
	lpstrInitialDir   *uint16
	lpstrTitle        *uint16
	flags             uint32
	nFileOffset       uint16
	nFileExtension    uint16
	lpstrDefExt       *uint16
	lCustData         uintptr
	lpfnHook          uintptr
	lpTemplateName    *uint16
	pvReserved        uintptr
	dwReserved        uint32
	flagsEx           uint32
}

var ErrNoFileSelected = errors.New("no file selected")

// PickImageFile shows the native "Open File" dialog filtered to common
// image types and returns the chosen path. GetOpenFileNameW runs its own
// modal message loop on the calling thread, so the call is pinned to a
// dedicated locked OS thread to keep it isolated from systray's message
// pump and from the Go scheduler moving the calling goroutine mid-call.
func PickImageFile() (string, error) {
	type result struct {
		path string
		err  error
	}
	resCh := make(chan result, 1)

	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()

		const maxPath = 32768
		fileBuf := make([]uint16, maxPath)

		filter, _ := syscall.UTF16PtrFromString(
			"Image files\x00*.png;*.jpg;*.jpeg;*.gif;*.webp\x00All files\x00*.*\x00\x00",
		)
		title, _ := syscall.UTF16PtrFromString("Select an image to upload")

		ofn := openFileNameW{
			lpstrFilter:  filter,
			lpstrFile:    &fileBuf[0],
			nMaxFile:     uint32(len(fileBuf)),
			lpstrTitle:   title,
			flags:        ofnFileMustExist | ofnPathMustExist | ofnExplorer,
			nFilterIndex: 1,
		}
		ofn.lStructSize = uint32(unsafe.Sizeof(ofn))

		ret, _, _ := procGetOpenFileName.Call(uintptr(unsafe.Pointer(&ofn)))
		if ret == 0 {
			resCh <- result{"", ErrNoFileSelected}
			return
		}
		resCh <- result{syscall.UTF16ToString(fileBuf), nil}
	}()

	res := <-resCh
	return res.path, res.err
}
