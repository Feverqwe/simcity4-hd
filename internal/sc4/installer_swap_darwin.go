//go:build cgo

package sc4

/*
#include <stdio.h>
#include <stdlib.h>
*/
import "C"

import (
	"os"
	"unsafe"
)

func moveNewMacTextureArchive(from, to string) error {
	return renameMacPath(from, to, C.RENAME_EXCL)
}

func renameMacPath(first, second string, flags C.uint) error {
	a, b := C.CString(first), C.CString(second)
	defer C.free(unsafe.Pointer(a))
	defer C.free(unsafe.Pointer(b))
	if result, err := C.renamex_np(a, b, flags); result != 0 {
		return &os.LinkError{Op: "rename", Old: first, New: second, Err: err}
	}
	return nil
}
