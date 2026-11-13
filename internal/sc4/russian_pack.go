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
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/ulikunitz/xz"
)

const (
	russianPackFile   = "russian.sc4pack"
	russianPackHeader = "SC4RU\x04"
	russianZIPHeader  = "SC4RU\x03"
	russianPackLimit  = 64 << 20
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

func russianPackCipher() (cipher.AEAD, error) {
	key, err := russianPackKey()
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	return cipher.NewGCMWithRandomNonce(block)
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
	aead, err := russianPackCipher()
	if err != nil {
		return err
	}

	work, err := os.MkdirTemp("", "sc4-russian-pack-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(work)
	rawArchive := filepath.Join(work, "resources.zip")
	err = publishFile(rawArchive, func(out io.Writer) error {
		return writeRussianArchive(dir, out)
	})
	if err != nil {
		return err
	}

	var data []byte
	if runtime.GOOS == "windows" {
		sevenZip, err := exec.LookPath("7z.exe")
		if err != nil {
			sevenZip = filepath.Join(os.Getenv("ProgramFiles"), "7-Zip", "7z.exe")
		}
		compressed := filepath.Join(work, "resources.xz")
		if output, err := exec.Command(sevenZip, "a", "-txz", "-mx=9", "-md=32m", "-mmt=1", compressed, rawArchive).CombinedOutput(); err != nil {
			return fmt.Errorf("compress Russian pack: %w: %s", err, output)
		}
		data, err = os.ReadFile(compressed)
		if err != nil {
			return err
		}
	} else {
		data, err = exec.Command("xz", "-9e", "-T1", "-c", rawArchive).Output()
		if err != nil {
			return fmt.Errorf("compress Russian pack: %w", err)
		}
	}
	header := []byte(russianPackHeader)
	pack := aead.Seal(bytes.Clone(header), nil, data, header)
	return publishFile(output, func(w io.Writer) error {
		_, err := w.Write(pack)
		return err
	})
}

func writeRussianArchive(dir string, out io.Writer) error {
	// Store the files together so XZ can find similarities across fonts.
	archive := zip.NewWriter(out)
	for _, name := range russianAssetNames() {
		raw, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return err
		}
		entry, err := archive.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Store})
		if err != nil {
			return err
		}
		if _, err := entry.Write(raw); err != nil {
			return err
		}
	}
	return archive.Close()
}

func unpackRussianAssets(pack, dir string) error {
	encoded, err := os.ReadFile(pack)
	if err != nil {
		return err
	}
	if len(encoded) < len(russianPackHeader) {
		return errors.New("unsupported Russian language pack")
	}
	header := encoded[:len(russianPackHeader)]
	switch string(header) {
	case russianPackHeader, russianZIPHeader:
	default:
		return errors.New("unsupported Russian language pack")
	}
	ciphertext := encoded[len(header):]
	aead, err := russianPackCipher()
	if err != nil {
		return err
	}
	decoded, err := aead.Open(nil, nil, ciphertext, header)
	if err != nil {
		return fmt.Errorf("decrypt Russian language pack: %w", err)
	}
	if string(header) == russianPackHeader {
		reader, err := (xz.ReaderConfig{SingleStream: true}).NewReader(bytes.NewReader(decoded))
		if err != nil {
			return err
		}
		decoded, err = io.ReadAll(io.LimitReader(reader, russianPackLimit+1))
		if err != nil {
			return err
		}
		if len(decoded) > russianPackLimit {
			return errors.New("Russian language pack exceeds the unpacked size limit")
		}
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
