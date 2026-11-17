package setup

import (
	"os"
	"syscall"
	"unsafe"
)

var moveFileEx = syscall.NewLazyDLL("kernel32.dll").NewProc("MoveFileExW")

func MovePath(source, target string, replace bool) error {
	from, err := syscall.UTF16PtrFromString(source)
	if err != nil {
		return err
	}
	to, err := syscall.UTF16PtrFromString(target)
	if err != nil {
		return err
	}
	const writeThrough = 0x8
	const replaceExisting = 0x1
	flags := uintptr(writeThrough)
	if replace {
		flags |= replaceExisting
	}
	ok, _, err := moveFileEx.Call(uintptr(unsafe.Pointer(from)), uintptr(unsafe.Pointer(to)), flags)
	if ok == 0 {
		return &os.LinkError{Op: "move", Old: source, New: target, Err: err}
	}
	return nil
}

func publishTextureArchive(source, target string) error { return MovePath(source, target, false) }
