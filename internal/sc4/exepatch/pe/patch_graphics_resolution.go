package pe

import (
	"encoding/binary"

	"github.com/Feverqwe/simcity4-hd/internal/sc4/exepatch/x86"
	"github.com/Feverqwe/simcity4-hd/internal/sc4/patchsrc"
	"github.com/Feverqwe/simcity4-hd/internal/sc4/uiids"
)

func appendGraphicsResolutionPatches(stub []byte, patches []x86.Patch, base int) ([]byte, []x86.Patch) {
	const (
		imageBase   = 0x400000
		initSite    = 0x391d57
		resetSite   = 0x392eb1
		acceptSite  = 0x392c14
		messageSite = 0x392f7a
	)

	for len(stub)%4 != 0 {
		stub = append(stub, 0)
	}
	data := imageBase + base + len(stub)
	// The supported Steam EXE imports these entry points. Resolve the additional
	// Win32 APIs through them so that the existing PE import table stays intact.
	for _, field := range []uint32{0xa800f0, 0xa800cc, 0xa80810, 0x603580, 0x414250, uiids.HdUITextGroup, uiids.DisplayTextFirst + 3} {
		stub = binary.LittleEndian.AppendUint32(stub, field)
	}
	for _, field := range []string{
		"kernel32.dll", "user32.dll", "SC4GraphicsOptions.dll", "GetModuleFileNameW",
		"GetPrivateProfileStringW", "GetPrivateProfileIntW", "WritePrivateProfileStringW",
		"GetFileAttributesW", "GetTempFileNameW", "CopyFileW", "MoveFileExW", "DeleteFileW",
		"GetActiveWindow", "MonitorFromWindow", "GetMonitorInfoW", "EnumDisplaySettingsW",
	} {
		stub = append(stub, field...)
		stub = append(stub, 0)
	}
	if len(stub)%2 != 0 {
		stub = append(stub, 0)
	}
	for _, field := range []string{
		"SC4GraphicsOptions.ini", "GraphicsOptions", "WindowMode", "WindowWidth", "WindowHeight",
		"Borderless", "BorderlessFullScreen", "Windowed", "FullScreen", "sc4",
	} {
		for _, c := range field {
			stub = binary.LittleEndian.AppendUint16(stub, uint16(c))
		}
		stub = binary.LittleEndian.AppendUint16(stub, 0)
	}

	entry := base + len(stub)
	stub = append(stub, patchsrc.GraphicsResolutionCode...)
	// The helper receives the graphics dialog, operation, and read-only data.
	addCall := func(body []byte, operation byte, dialogOpcode ...byte) []byte {
		body = binary.LittleEndian.AppendUint32(append(body, 0x68), uint32(data))
		body = append(body, 0x6a, operation)
		body = append(body, dialogOpcode...)
		body = append(body, x86.RelativeInstruction(0xe8, base+len(stub)+len(body), entry)...)
		return append(body, 0x83, 0xc4, 0x0c)
	}
	var patch x86.Patch
	initOriginal := x86.HexBytes("8b106a016a018bc8")
	init := addCall(x86.HexBytes("608d46fc"), 0, 0x50) // The constructor's ESI is &dialog->host.
	init = append(append(init, 0x61), initOriginal...)
	stub, patch = x86.AppendStub(stub, base, initSite, initOriginal, init)
	patches = append(patches, patch)

	resetOriginal := x86.HexBytes("8bcee808d1ffff")
	reset := append(x86.HexBytes("8bce"), x86.RelativeInstruction(0xe8, base+len(stub)+2, 0x38ffc0)...)
	reset = addCall(append(reset, 0x60), 1, 0x56)
	stub, patch = x86.AppendStub(stub, base, resetSite, resetOriginal, append(reset, 0x61))
	patches = append(patches, patch)

	acceptOriginal := x86.HexBytes("8b068bceff501c")
	accept := addCall([]byte{0x60}, 2, 0x56)
	accept = append(accept, x86.HexBytes("8944241c6183f8027505")...)
	accept = append(accept, x86.RelativeInstruction(0xe9, base+len(stub)+len(accept), 0x392ebf)...)
	accept = append(accept, 0x50) // Preserve the successful-save flag across native Apply.
	accept = append(accept, acceptOriginal...)
	accept = append(accept, x86.HexBytes("5883f8017504c6461a01")...)
	stub, patch = x86.AppendStub(stub, base, acceptSite, acceptOriginal, accept)
	patches = append(patches, patch)

	// The combo's selection notification arrives after its selected index changes.
	// Preserve the stock listener for every other event.
	messageOriginal := x86.HexBytes("8b4c24108b1132c0")
	message := x86.HexBytes("8b4c24108339037524817904551271e8751b81790804444ae7751260")
	message = addCall(message, 3, 0x56)
	message = append(message, 0x61)
	message = append(message, messageOriginal...)
	stub, patch = x86.AppendStub(stub, base, messageSite, messageOriginal, message)
	return stub, append(patches, patch)
}
