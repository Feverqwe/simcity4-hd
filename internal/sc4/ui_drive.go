package sc4

func driveGaugeEntries(stock uiStock, art uiArtwork) ([]resource, error) {
	// Frame counts come from the stock Free Drive exemplars. The gauge
	// selects a single frame from each horizontal strip.
	atlases := [...]struct {
		instance uint32
		frames   int
	}{
		{0xebcbb93f, 59}, {0xcbcb6e9f, 55}, {0xcbe99a5e, 60},
		{0x0c0729aa, 60}, {0x0c0729ab, 15}, {0xcbcba947, 37},
		{0xcbcba948, 55}, {0xcbcba949, 55}, {0xcbcba950, 55}, {0xcbcba952, 55},
		{0xabec941c, 60}, {0xac0da30b, 25}, {0xac0da30c, 59},
		{0xac0da30d, 9}, {0xac0da30e, 37}, {0x0beb3dbf, 16},
	}
	entries := make([]resource, 0, len(atlases))
	for _, atlas := range atlases {
		entry, err := runtimeArtwork(stock, art, atlas.instance, atlas.frames, true)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

func appendDriveGaugePatches(stub []byte, patches []instructionPatch, base int) ([]byte, []instructionPatch) {
	// Record the density for the upcoming SetBitmap. The temporary resource
	// at ESP+0x60 stores the group used by the successful loader.
	loaded := hexBytes("8b4c24108b442464")
	body := append([]byte(nil), loaded...)
	body = append(body, hexBytes("80a107010000bf817c246c")...)
	body = little.AppendUint32(body, runtimeArtworkGroup)
	body = append(body, hexBytes("750780890701000040")...)
	var density instructionPatch
	stub, density = appendStub(stub, base, 0x164752, loaded, body)
	patches = append(patches, density)

	// Paint receives the window interface four bytes into the gauge object.
	// An HD frame already matches screen density. For a stock fallback use
	// the existing doubled destination, with the same origin and source crop.
	original := hexBytes("897c241c895c2420")
	body = hexBytes("f686030100004075108d143f8954241c8d141b89542420eb08")
	body = append(body, original...)
	var paint instructionPatch
	stub, paint = appendStub(stub, base, 0x3628c4, original, body)
	return stub, append(patches, paint)
}
