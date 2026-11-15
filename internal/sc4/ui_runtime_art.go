package sc4

import "fmt"

const runtimeArtworkGroup = 0xe74a40cc
const tooltipFrame = 0x14416190

func runtimeChromeEntries(stock uiStock, art uiArtwork) ([]resource, error) {
	instances := [...]uint32{
		tooltipFrame,
		0x14315e60, 0x14315e61, 0x14315e62, // Viewport alert border states.
	}
	entries := make([]resource, 0, len(instances))
	for _, instance := range instances {
		entry, err := runtimeArtwork(stock, art, instance, 1, false)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

func runtimeArtwork(stock uiStock, art uiArtwork, instance uint32, frames int, keepAlpha bool) (resource, error) {
	source := resourceID{uiImage, catalogFrameGroup, instance}
	target := resourceID{uiImage, runtimeArtworkGroup, instance}
	if _, exists := stock.index[target]; exists {
		return resource{}, fmt.Errorf("runtime image key collision: %s", target)
	}
	original, err := stock.read(source)
	if err != nil {
		return resource{}, err
	}
	data, err := art.upscale(layoutAtlas{source: source, frames: frames, project: true}, original)
	if err != nil {
		return resource{}, fmt.Errorf("runtime image %s: %w", source, err)
	}
	if keepAlpha {
		data, err = copyScreenAlpha(original, data)
		if err != nil {
			return resource{}, fmt.Errorf("runtime image alpha %s: %w", source, err)
		}
	}
	return resource{target, data}, nil
}

func copyScreenAlpha(original, enlarged []byte) ([]byte, error) {
	source, err := decodeRGBA(original)
	if err != nil {
		return nil, err
	}
	result, err := decodeRGBA(enlarged)
	if err != nil {
		return nil, err
	}
	if result.Rect.Dx() != 2*source.Rect.Dx() || result.Rect.Dy() != 2*source.Rect.Dy() {
		return nil, fmt.Errorf("invalid screen-density dimensions")
	}
	// Keep holes and partially transparent pixels in the original gauge masks.
	for y := range result.Rect.Dy() {
		for x := range result.Rect.Dx() {
			result.Pix[result.PixOffset(x, y)+3] = source.Pix[source.PixOffset(x/2, y/2)+3]
		}
	}
	return encodeRGBA(result)
}

func appendRuntimeArtPatches(stub []byte, patches []instructionPatch, base int) ([]byte, []instructionPatch) {
	const (
		loadByKey   = 0x202b00
		loadByParts = 0x202b70
		closeBitmap = 0x202be0
	)
	// Both overloads construct a temporary bitmap resource. Try the HD
	// group, then destroy the failed temporary before a stock retry.
	partsEntry := base + len(stub)
	stub = append(stub, hexBytes("538bd9")...)                   // save EBX and retain the resource
	stub = append(stub, hexBytes("ff742418ff742418ff742418")...) // both options and instance
	stub = little.AppendUint32(append(stub, 0x68), runtimeArtworkGroup)
	stub = append(stub, hexBytes("ff742418")...) // type
	stub = append(stub, relativeInstruction(0xe8, base+len(stub), loadByParts)...)
	stub = append(stub, hexBytes("837b040074045bc214008bcb")...)
	stub = append(stub, relativeInstruction(0xe8, base+len(stub), closeBitmap)...)
	stub = append(stub, hexBytes("8bcb5b")...)
	stub = append(stub, relativeInstruction(0xe9, base+len(stub), loadByParts)...)

	keyEntry := base + len(stub)
	stub = append(stub, hexBytes("53568bd98b74240c")...) // retain the resource and the caller's local key
	stub = little.AppendUint32(append(stub, hexBytes("c74604")...), runtimeArtworkGroup)
	stub = append(stub, hexBytes("ff742414ff742414ff742414")...)
	stub = append(stub, relativeInstruction(0xe8, base+len(stub), loadByKey)...)
	stub = little.AppendUint32(append(stub, hexBytes("c74604")...), catalogFrameGroup)
	stub = append(stub, hexBytes("837b040074055e5bc20c008bcb")...)
	stub = append(stub, relativeInstruction(0xe8, base+len(stub), closeBitmap)...)
	stub = append(stub, hexBytes("8bcb5e5b")...)
	stub = append(stub, relativeInstruction(0xe9, base+len(stub), loadByKey)...)

	for _, call := range []struct{ site, stock, hd int }{
		{0x16474d, loadByParts, partsEntry}, // Free Drive gauge.
		{0x3e8b35, loadByParts, partsEntry}, // Viewport alert border.
		{0x029653, loadByKey, keyEntry},     // Callout used in the tutorial.
		{0x3e7d95, loadByKey, keyEntry},     // Hover tooltip.
		{0x3eede0, loadByKey, keyEntry},     // In-city callout.
	} {
		patches = append(patches, instructionPatch{
			call.site, relativeInstruction(0xe8, call.site, call.stock),
			relativeInstruction(0xe8, call.site, call.hd),
		})
	}
	return stub, patches
}
