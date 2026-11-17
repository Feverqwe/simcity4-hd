//go:build cgo

package macos

/*
#cgo LDFLAGS: -framework CoreFoundation
#include <CoreFoundation/CoreFoundation.h>
#include <stdlib.h>
*/
import "C"

import (
	"errors"
	"github.com/Feverqwe/simcity4-hd/internal/sc4/setup"
	"strconv"
	"unsafe"

	"github.com/Feverqwe/simcity4-hd/internal/sc4/exepatch/macho"
)

func macPreferenceString(value string) C.CFStringRef {
	utf8 := C.CString(value)
	defer C.free(unsafe.Pointer(utf8))
	return C.CFStringCreateWithCString(0, utf8, C.kCFStringEncodingUTF8)
}

func macPackedResolution(value C.CFPropertyListRef) int {
	if value == 0 || C.CFGetTypeID(value) != C.CFStringGetTypeID() {
		return 0
	}
	var text [16]C.char
	if C.CFStringGetCString(C.CFStringRef(value), &text[0], C.CFIndex(len(text)), C.kCFStringEncodingUTF8) == 0 {
		return 0
	}
	packed, _ := strconv.ParseUint(C.GoString(&text[0]), 10, 31)
	return int(packed)
}

func installedMacResolution() string {
	key, domain := macPreferenceString(macho.DisplayPreference), macPreferenceString(macSetupDisplayDomain)
	defer C.CFRelease(C.CFTypeRef(key))
	defer C.CFRelease(C.CFTypeRef(domain))
	value := C.CFPreferencesCopyAppValue(key, domain)
	if value != 0 {
		defer C.CFRelease(value)
	}
	return macResolutionLabel(macPackedResolution(value))
}

func publishMacResolution(requested int, publish func() error) error {
	if requested == 0 {
		return publish()
	}
	key, domain := macPreferenceString(macho.DisplayPreference), macPreferenceString(macSetupDisplayDomain)
	defer C.CFRelease(C.CFTypeRef(key))
	defer C.CFRelease(C.CFTypeRef(domain))
	previous := C.CFPreferencesCopyValue(key, domain, C.kCFPreferencesCurrentUser, C.kCFPreferencesAnyHost)
	if previous != 0 {
		defer C.CFRelease(previous)
	}
	effective := C.CFPreferencesCopyAppValue(key, domain)
	if effective != 0 {
		defer C.CFRelease(effective)
	}
	current := macPackedResolution(effective)
	next := macResolutionWithMode(requested, current)
	if next == current {
		return publish()
	}
	var value C.CFPropertyListRef
	if next != 0 {
		value = C.CFPropertyListRef(macPreferenceString(strconv.Itoa(next)))
		defer C.CFRelease(value)
	}
	write := func(value C.CFPropertyListRef) bool {
		C.CFPreferencesSetAppValue(key, value, domain)
		return C.CFPreferencesAppSynchronize(domain) != 0
	}
	var err error
	if !write(value) {
		err = setup.FieldError("resolution", errors.New("Could not save SimCity 4’s game resolution."))
	} else {
		err = publish()
	}
	if err != nil && !write(previous) {
		return errors.Join(err, errors.New("Could not restore SimCity 4’s previous game resolution."))
	}
	return err
}
