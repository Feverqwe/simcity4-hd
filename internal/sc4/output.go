package sc4

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// publishFile exposes a complete file and refuses to replace an existing path.
func publishFile(output string, write func(io.Writer) error) error {
	parent := filepath.Dir(output)
	if err := os.MkdirAll(parent, 0755); err != nil {
		return err
	}
	temp, err := os.CreateTemp(parent, ".sc4-hd-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name())

	writeErr := write(temp)
	closeErr := temp.Close()
	if err := errors.Join(writeErr, closeErr); err != nil {
		return err
	}
	if err := os.Link(temp.Name(), output); errors.Is(err, os.ErrExist) {
		return fmt.Errorf("%s already exists", output)
	} else {
		return err
	}
}

type contextReader struct {
	ctx context.Context
	io.Reader
}

func (r contextReader) Read(b []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.Reader.Read(b)
}
