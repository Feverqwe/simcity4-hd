package game

import (
	"errors"
	"strings"
	"syscall"
	"unicode"
	"unsafe"
)

var shortPathProc = syscall.NewLazyDLL("kernel32.dll").NewProc("GetShortPathNameW")

func ProfilePath(path string) (string, error) {
	if !strings.ContainsFunc(path, unicode.IsSpace) {
		return path, nil
	}
	input, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return "", err
	}
	buf := make([]uint16, 32768)
	n, _, _ := shortPathProc.Call(uintptr(unsafe.Pointer(input)), uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	if n != 0 && n < uintptr(len(buf)) {
		short := syscall.UTF16ToString(buf[:n])
		if !strings.ContainsFunc(short, unicode.IsSpace) {
			return short, nil
		}
	}
	return "", errors.New("SimCity 4 needs a profile path without spaces. Choose a profile folder whose full path has no spaces.")
}
