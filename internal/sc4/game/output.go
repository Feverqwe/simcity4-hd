package game

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// publishFile exposes a complete file and refuses to replace an existing path.
func PublishFile(output string, write func(io.Writer) error) error {
	return PublishStagedFile(output, write, os.Link)
}

func PublishStagedFile(output string, write func(io.Writer) error, publish func(from, to string) error) error {
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
	if err := publish(temp.Name(), output); errors.Is(err, os.ErrExist) {
		return fmt.Errorf("%s already exists", output)
	} else {
		return err
	}
}

func WithContext(ctx context.Context, reader io.Reader) io.Reader {
	return contextReader{ctx, reader}
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
