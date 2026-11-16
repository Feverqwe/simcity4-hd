package sc4

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"time"
)

const (
	modelURL  = "https://github.com/the-database/MangaJaNai/releases/download/3.0.0/IllustrationJaNai_V3detail.zip"
	modelSHA  = "27749962bc934779f867730cdb10617e3d7475313f920a7a966c56b75dbdbeda"
	modelName = "4x_IllustrationJaNai_V3detail_HAT_L_28k_bf16.safetensors"
)

func removeIfPresent(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func checkSHA(ctx context.Context, file, want string) error {
	f, err := os.Open(file)
	if err != nil {
		return err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, contextReader{ctx, f}); err != nil {
		return err
	}
	if fmt.Sprintf("%x", h.Sum(nil)) != want {
		return fmt.Errorf("unexpected SHA-256 for %s", filepath.Base(file))
	}
	return nil
}

func downloadSetupFile(ctx context.Context, output, url, digest string, limit int64, progress func(int)) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	client := http.Client{Timeout: 2 * time.Hour}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK || resp.ContentLength > limit {
		return fmt.Errorf("HTTP %s or unexpected file size", resp.Status)
	}
	return publishFile(output, func(w io.Writer) error {
		h := sha256.New()
		reader := io.LimitReader(resp.Body, limit+1)
		buf := make([]byte, 128<<10)
		var total int64
		last := -1
		w = io.MultiWriter(w, h)
		for {
			n, err := reader.Read(buf)
			total += int64(n)
			if total > limit {
				return errors.New("download exceeds the expected file size")
			}
			if _, writeErr := w.Write(buf[:n]); writeErr != nil {
				return writeErr
			}
			if progress != nil && resp.ContentLength > 0 {
				pct := int(total * 100 / resp.ContentLength)
				if pct != last {
					progress(pct)
					last = pct
				}
			}
			if err == io.EOF {
				break
			}
			if err != nil {
				return err
			}
		}
		if fmt.Sprintf("%x", h.Sum(nil)) != digest {
			return fmt.Errorf("unexpected SHA-256 for %s", filepath.Base(output))
		}
		return nil
	})
}

func extractOne(ctx context.Context, archive, name, output string, max uint64, want string) error {
	z, err := zip.OpenReader(archive)
	if err != nil {
		return err
	}
	defer z.Close()
	for _, entry := range z.File {
		if path.Base(entry.Name) != name || !entry.Mode().IsRegular() || entry.UncompressedSize64 > max {
			continue
		}
		return publishFile(output, func(w io.Writer) error {
			r, err := entry.Open()
			if err != nil {
				return err
			}
			defer r.Close()
			h := sha256.New()
			if _, err := io.Copy(io.MultiWriter(w, h), contextReader{ctx, r}); err != nil {
				return err
			}
			if want != "" && fmt.Sprintf("%x", h.Sum(nil)) != want {
				return fmt.Errorf("unexpected SHA-256 for %s", name)
			}
			return nil
		})
	}
	return fmt.Errorf("%s is missing from %s", name, filepath.Base(archive))
}
