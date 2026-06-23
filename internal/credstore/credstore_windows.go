//go:build windows

// Package credstore stores the API token in the Windows Credential Manager
// (DPAPI-backed) so it never touches disk as plaintext. The target name is
// scoped per instance URL so multiple instances can be configured without
// collision.
package credstore

import (
	"fmt"
	"syscall"
	"unsafe"
)

const credTypeGeneric = 1
const credPersistLocalMachine = 2

type credential struct {
	Flags              uint32
	Type               uint32
	TargetName         *uint16
	Comment            *uint16
	LastWritten        syscall.Filetime
	CredentialBlobSize uint32
	CredentialBlob     *byte
	Persist            uint32
	AttributeCount     uint32
	Attributes         uintptr
	TargetAlias        *uint16
	UserName           *uint16
}

var (
	modAdvapi32     = syscall.NewLazyDLL("advapi32.dll")
	procCredWrite   = modAdvapi32.NewProc("CredWriteW")
	procCredRead    = modAdvapi32.NewProc("CredReadW")
	procCredFree    = modAdvapi32.NewProc("CredFree")
	procCredDelete  = modAdvapi32.NewProc("CredDeleteW")
)

func targetName(instanceURL string) string {
	return "ClipShot:" + instanceURL
}

func SaveToken(instanceURL, token string) error {
	target, err := syscall.UTF16PtrFromString(targetName(instanceURL))
	if err != nil {
		return err
	}
	blob := []byte(token)

	cred := credential{
		Type:               credTypeGeneric,
		TargetName:         target,
		CredentialBlobSize: uint32(len(blob)),
		CredentialBlob:     &blob[0],
		Persist:            credPersistLocalMachine,
	}

	ret, _, err := procCredWrite.Call(uintptr(unsafe.Pointer(&cred)), 0)
	if ret == 0 {
		return fmt.Errorf("CredWriteW failed: %w", err)
	}
	return nil
}

func LoadToken(instanceURL string) (string, error) {
	target, err := syscall.UTF16PtrFromString(targetName(instanceURL))
	if err != nil {
		return "", err
	}

	var credPtr *credential
	ret, _, err := procCredRead.Call(
		uintptr(unsafe.Pointer(target)),
		uintptr(credTypeGeneric),
		0,
		uintptr(unsafe.Pointer(&credPtr)),
	)
	if ret == 0 {
		return "", fmt.Errorf("CredReadW failed: %w", err)
	}
	defer procCredFree.Call(uintptr(unsafe.Pointer(credPtr)))

	blob := unsafe.Slice(credPtr.CredentialBlob, credPtr.CredentialBlobSize)
	return string(blob), nil
}

func DeleteToken(instanceURL string) error {
	target, err := syscall.UTF16PtrFromString(targetName(instanceURL))
	if err != nil {
		return err
	}
	ret, _, err := procCredDelete.Call(uintptr(unsafe.Pointer(target)), uintptr(credTypeGeneric), 0)
	if ret == 0 {
		return fmt.Errorf("CredDeleteW failed: %w", err)
	}
	return nil
}
