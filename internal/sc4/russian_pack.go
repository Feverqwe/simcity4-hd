package sc4

import (
	"archive/zip"
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

const (
	russianPackFile   = "russian.sc4pack"
	russianPackHeader = "SC4RU\x02"
	// This key ships with Setup so it can read the pack offline.
	russianPackKey = "historical-key-removed0000000000"
)

const russianLocaleFile = "SimCityLocale.DAT"

func russianAssetNames() []string {
	names := make([]string, 1, 1+len(russianFontFiles))
	names[0] = russianLocaleFile
	for _, font := range russianFontFiles {
		names = append(names, russianFontSourcePath(font))
	}
	return names
}

func russianPackCipher() cipher.AEAD {
	block, err := aes.NewCipher([]byte(russianPackKey))
	if err != nil {
		panic(err)
	}
	aead, err := cipher.NewGCMWithRandomNonce(block)
	if err != nil {
		panic(err)
	}
	return aead
}

func runRussianPack(args []string) error {
	if len(args) == 3 {
		switch args[0] {
		case "pack":
			return packRussianAssets(args[1], args[2])
		case "unpack":
			return unpackRussianAssets(args[1], args[2])
		}
	}
	return errors.New("usage: russian-pack pack SOURCE_DIR OUTPUT_FILE or russian-pack unpack PACK NEW_OUTPUT_DIR")
}

func packRussianAssets(dir, output string) error {
	var data bytes.Buffer
	archive := zip.NewWriter(&data)
	for _, name := range russianAssetNames() {
		raw, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return err
		}
		entry, err := archive.Create(name)
		if err != nil {
			return err
		}
		if _, err := entry.Write(raw); err != nil {
			return err
		}
	}
	if err := archive.Close(); err != nil {
		return err
	}

	header := []byte(russianPackHeader)
	pack := russianPackCipher().Seal(bytes.Clone(header), nil, data.Bytes(), header)
	return publishFile(output, func(w io.Writer) error {
		_, err := w.Write(pack)
		return err
	})
}

func unpackRussianAssets(pack, dir string) error {
	encoded, err := os.ReadFile(pack)
	if err != nil {
		return err
	}
	header := []byte(russianPackHeader)
	ciphertext, found := bytes.CutPrefix(encoded, header)
	if !found {
		return errors.New("unsupported Russian language pack")
	}
	decoded, err := russianPackCipher().Open(nil, nil, ciphertext, header)
	if err != nil {
		return fmt.Errorf("decrypt Russian language pack: %w", err)
	}
	archive, err := zip.NewReader(bytes.NewReader(decoded), int64(len(decoded)))
	if err != nil {
		return err
	}
	names := russianAssetNames()
	assets := make([][]byte, len(names))
	for i, name := range names {
		assets[i], err = fs.ReadFile(archive, name)
		if err != nil {
			return err
		}
	}
	if err := os.Mkdir(dir, 0755); err != nil {
		return err
	}
	if err := os.Mkdir(filepath.Join(dir, "Fonts"), 0755); err != nil {
		_ = os.RemoveAll(dir)
		return err
	}
	for i, name := range names {
		if err := os.WriteFile(filepath.Join(dir, name), assets[i], 0644); err != nil {
			_ = os.RemoveAll(dir)
			return err
		}
	}
	return nil
}
