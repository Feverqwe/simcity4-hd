//go:build !cgo

package sc4

import "errors"

func russianPackKey() ([32]byte, error) {
	return [32]byte{}, errors.New("Russian language packs require a builder compiled with CGO_ENABLED=1 and a C compiler")
}
