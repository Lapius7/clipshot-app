//go:build windows

// Package clipboard reads image bytes (as PNG) from the Windows clipboard
// and writes plain text (the resulting URL) back to it.
package clipboard

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"syscall"
	"unsafe"
)

const (
	cfBitmap   = 2
	cfDIB      = 8
	cfUnicodeText = 13
)

var (
	user32             = syscall.NewLazyDLL("user32.dll")
	procOpenClipboard  = user32.NewProc("OpenClipboard")
	procCloseClipboard = user32.NewProc("CloseClipboard")
	procGetClipData    = user32.NewProc("GetClipboardData")
	procSetClipData    = user32.NewProc("SetClipboardData")
	procEmptyClipboard = user32.NewProc("EmptyClipboard")
	procIsClipFormatAvail = user32.NewProc("IsClipboardFormatAvailable")

	kernel32          = syscall.NewLazyDLL("kernel32.dll")
	procGlobalAlloc   = kernel32.NewProc("GlobalAlloc")
	procGlobalLock    = kernel32.NewProc("GlobalLock")
	procGlobalUnlock  = kernel32.NewProc("GlobalUnlock")
	procGlobalSize    = kernel32.NewProc("GlobalSize")
)

var ErrNoImage = errors.New("clipboard does not contain an image")

type bitmapInfoHeader struct {
	Size          uint32
	Width         int32
	Height        int32
	Planes        uint16
	BitCount      uint16
	Compression   uint32
	SizeImage     uint32
	XPelsPerMeter int32
	YPelsPerMeter int32
	ClrUsed       uint32
	ClrImportant  uint32
}

// ReadImagePNG reads the current clipboard contents as a CF_DIB bitmap and
// re-encodes it as PNG bytes, since clipshot-server only accepts a fixed
// set of standard image formats.
func ReadImagePNG() ([]byte, error) {
	if r, _, _ := procIsClipFormatAvail.Call(uintptr(cfDIB)); r == 0 {
		return nil, ErrNoImage
	}

	if r, _, _ := procOpenClipboard.Call(0); r == 0 {
		return nil, fmt.Errorf("OpenClipboard failed")
	}
	defer procCloseClipboard.Call()

	h, _, _ := procGetClipData.Call(uintptr(cfDIB))
	if h == 0 {
		return nil, ErrNoImage
	}

	size, _, _ := procGlobalSize.Call(h)
	ptr, _, _ := procGlobalLock.Call(h)
	if ptr == 0 {
		return nil, fmt.Errorf("GlobalLock failed")
	}
	defer procGlobalUnlock.Call(h)

	// ptr is a GlobalLock'd Win32 memory address, not a Go-managed value;
	// go vet's unsafe.Pointer check flags this pattern but it is the
	// standard way to bridge syscall uintptr results into Go slices.
	raw := unsafe.Slice((*byte)(unsafe.Pointer(ptr)), int(size))
	img, err := decodeDIB(raw)
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, fmt.Errorf("encode png: %w", err)
	}
	return buf.Bytes(), nil
}

func decodeDIB(data []byte) (image.Image, error) {
	if len(data) < 40 {
		return nil, fmt.Errorf("dib data too small")
	}
	var hdr bitmapInfoHeader
	hdr.Size = bytesToU32(data[0:4])
	hdr.Width = int32(bytesToU32(data[4:8]))
	hdr.Height = int32(bytesToU32(data[8:12]))
	hdr.BitCount = uint16(bytesToU32(data[14:16]))

	if hdr.BitCount != 32 && hdr.BitCount != 24 {
		return nil, fmt.Errorf("unsupported clipboard bitmap depth: %d bits", hdr.BitCount)
	}

	width := int(hdr.Width)
	height := int(hdr.Height)
	if height < 0 {
		height = -height
	}

	pixelOffset := int(hdr.Size)
	bytesPerPixel := int(hdr.BitCount) / 8
	rowSize := ((width*bytesPerPixel + 3) / 4) * 4

	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		srcY := height - 1 - y // DIB rows are bottom-up
		rowStart := pixelOffset + srcY*rowSize
		if rowStart+rowSize > len(data) {
			return nil, fmt.Errorf("dib data truncated")
		}
		for x := 0; x < width; x++ {
			i := rowStart + x*bytesPerPixel
			b := data[i]
			g := data[i+1]
			r := data[i+2]
			a := byte(255)
			if bytesPerPixel == 4 {
				a = data[i+3]
				if a == 0 {
					a = 255 // many DIBs leave alpha as 0 but are fully opaque
				}
			}
			img.SetRGBA(x, y, color.RGBA{R: r, G: g, B: b, A: a})
		}
	}
	return img, nil
}

func bytesToU32(b []byte) uint32 {
	return uint32(b[0]) | uint32(b[1])<<8 | uint32(b[2])<<16 | uint32(b[3])<<24
}

// WriteText puts plain text (the uploaded image URL) onto the clipboard.
func WriteText(text string) error {
	if r, _, _ := procOpenClipboard.Call(0); r == 0 {
		return fmt.Errorf("OpenClipboard failed")
	}
	defer procCloseClipboard.Call()

	procEmptyClipboard.Call()

	utf16, err := syscall.UTF16FromString(text)
	if err != nil {
		return err
	}
	size := len(utf16) * 2

	const gmemMoveable = 0x0002
	h, _, _ := procGlobalAlloc.Call(uintptr(gmemMoveable), uintptr(size))
	if h == 0 {
		return fmt.Errorf("GlobalAlloc failed")
	}
	ptr, _, _ := procGlobalLock.Call(h)
	if ptr == 0 {
		return fmt.Errorf("GlobalLock failed")
	}
	// Same GlobalLock/uintptr bridging pattern as in decodeDIB above.
	dst := unsafe.Slice((*uint16)(unsafe.Pointer(ptr)), len(utf16))
	copy(dst, utf16)
	procGlobalUnlock.Call(h)

	if r, _, _ := procSetClipData.Call(uintptr(cfUnicodeText), h); r == 0 {
		return fmt.Errorf("SetClipboardData failed")
	}
	return nil
}
