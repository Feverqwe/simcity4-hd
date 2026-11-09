package sc4

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
)

const headerSize = 96
const indexEntrySize = 20

var little = binary.LittleEndian

type resourceID struct {
	kind, group, instance uint32
}

func (id resourceID) String() string {
	return fmt.Sprintf("%08x:%08x:%08x", id.kind, id.group, id.instance)
}

type record struct {
	id           resourceID
	offset, size uint32
}

type archive struct {
	file    *os.File
	records []record
}

func openArchive(path string) (*archive, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	records, err := readIndex(file)
	if err != nil {
		file.Close()
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &archive{file, records}, nil
}

func readIndex(file *os.File) ([]record, error) {
	var header [headerSize]byte
	if _, err := io.ReadFull(file, header[:]); err != nil {
		return nil, err
	}
	if string(header[:4]) != "DBPF" || little.Uint32(header[4:]) != 1 ||
		little.Uint32(header[8:]) != 0 || little.Uint32(header[32:]) != 7 {
		return nil, errors.New("expected a DBPF 1.0 archive with an index version of 7")
	}
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	count := little.Uint32(header[36:])
	offset := little.Uint32(header[40:])
	size := little.Uint32(header[44:])
	if uint64(count)*indexEntrySize != uint64(size) ||
		uint64(offset)+uint64(size) > uint64(info.Size()) ||
		(size != 0 && offset < headerSize) {
		return nil, errors.New("invalid DBPF index extent")
	}
	reader := io.NewSectionReader(file, int64(offset), int64(size))
	records := make([]record, 0, min(count, 4096))
	var row [indexEntrySize]byte
	for range count {
		if _, err := io.ReadFull(reader, row[:]); err != nil {
			return nil, err
		}
		r := record{
			resourceID{little.Uint32(row[:]), little.Uint32(row[4:]), little.Uint32(row[8:])},
			little.Uint32(row[12:]), little.Uint32(row[16:]),
		}
		if uint64(r.offset)+uint64(r.size) > uint64(info.Size()) ||
			(r.size != 0 && r.offset < headerSize) {
			return nil, fmt.Errorf("%s: invalid DBPF resource extent", r.id)
		}
		records = append(records, r)
	}
	return records, nil
}

func (a *archive) read(r record) ([]byte, error) {
	data := make([]byte, r.size)
	if _, err := a.file.ReadAt(data, int64(r.offset)); err != nil {
		return nil, err
	}
	if len(data) >= 6 && data[4] == 0x10 && data[5] == 0xfb {
		return decodeQFS(data)
	}
	return data, nil
}

type resource struct {
	id   resourceID
	data []byte
}

// writeArchive keeps the caller's resource order so generated DATs can match
// the existing builder byte for byte.
func writeArchive(w io.Writer, entries []resource) error {
	indexSize := uint64(len(entries)) * indexEntrySize
	offset := uint64(headerSize)
	for _, entry := range entries {
		offset += uint64(len(entry.data))
	}
	if offset+indexSize > math.MaxUint32 {
		return errors.New("DBPF archive exceeds the 32-bit size limit")
	}
	var header [headerSize]byte
	copy(header[:], "DBPF")
	little.PutUint32(header[4:], 1)
	little.PutUint32(header[32:], 7)
	little.PutUint32(header[36:], uint32(len(entries)))
	little.PutUint32(header[40:], uint32(offset))
	little.PutUint32(header[44:], uint32(indexSize))
	if _, err := w.Write(header[:]); err != nil {
		return err
	}
	for _, entry := range entries {
		if _, err := w.Write(entry.data); err != nil {
			return err
		}
	}
	offset = headerSize
	var row [indexEntrySize]byte
	for _, entry := range entries {
		little.PutUint32(row[:], entry.id.kind)
		little.PutUint32(row[4:], entry.id.group)
		little.PutUint32(row[8:], entry.id.instance)
		little.PutUint32(row[12:], uint32(offset))
		little.PutUint32(row[16:], uint32(len(entry.data)))
		if _, err := w.Write(row[:]); err != nil {
			return err
		}
		offset += uint64(len(entry.data))
	}
	return nil
}
