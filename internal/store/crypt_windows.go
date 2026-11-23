//go:build windows

package store

import (
	"syscall"
	"unsafe"
)

// Passwords are encrypted with Windows DPAPI: only the same Windows user on
// the same machine can decrypt them. The Subsonic token auth needs the real
// password on every request, so hashing it is not an option.

var (
	crypt32    = syscall.NewLazyDLL("crypt32.dll")
	kernel32   = syscall.NewLazyDLL("kernel32.dll")
	pProtect   = crypt32.NewProc("CryptProtectData")
	pUnprotect = crypt32.NewProc("CryptUnprotectData")
	pLocalFree = kernel32.NewProc("LocalFree")
)

type blob struct {
	n uint32
	p *byte
}

func inBlob(b []byte) blob {
	if len(b) == 0 {
		return blob{}
	}
	return blob{n: uint32(len(b)), p: &b[0]}
}

// takeBlob copies the output blob into Go memory and frees the system buffer.
func takeBlob(b blob) []byte {
	if b.p == nil {
		return nil
	}
	defer pLocalFree.Call(uintptr(unsafe.Pointer(b.p)))
	return append([]byte(nil), unsafe.Slice(b.p, b.n)...)
}

const uiForbidden = 0x1 // CRYPTPROTECT_UI_FORBIDDEN

func protect(plain []byte) ([]byte, error) {
	in := inBlob(plain)
	var out blob
	r, _, err := pProtect.Call(uintptr(unsafe.Pointer(&in)), 0, 0, 0, 0, uiForbidden, uintptr(unsafe.Pointer(&out)))
	if r == 0 {
		return nil, err
	}
	return takeBlob(out), nil
}

func unprotect(enc []byte) ([]byte, error) {
	in := inBlob(enc)
	var out blob
	r, _, err := pUnprotect.Call(uintptr(unsafe.Pointer(&in)), 0, 0, 0, 0, uiForbidden, uintptr(unsafe.Pointer(&out)))
	if r == 0 {
		return nil, err
	}
	return takeBlob(out), nil
}
