package dbpf

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
)

const HeaderSize = 96
const IndexEntrySize = 20

var Little = binary.LittleEndian

type ID struct {
	Kind, Group, Instance uint32
}

func (id ID) String() string {
	return fmt.Sprintf("%08x:%08x:%08x", id.Kind, id.Group, id.Instance)
}

type Record struct {
	ID           ID
	Offset, Size uint32
}

type Archive struct {
	file    *os.File
	Records []Record
}

func Open(path string) (*Archive, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	records, err := readIndex(file)
	if err != nil {
		file.Close()
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &Archive{file, records}, nil
}

func readIndex(file *os.File) ([]Record, error) {
	var header [HeaderSize]byte
	if _, err := io.ReadFull(file, header[:]); err != nil {
		return nil, err
	}
	if string(header[:4]) != "DBPF" || Little.Uint32(header[4:]) != 1 ||
		Little.Uint32(header[8:]) != 0 || Little.Uint32(header[32:]) != 7 {
		return nil, errors.New("expected a DBPF 1.0 archive with an index version of 7")
	}
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	count := Little.Uint32(header[36:])
	offset := Little.Uint32(header[40:])
	size := Little.Uint32(header[44:])
	if uint64(count)*IndexEntrySize != uint64(size) ||
		uint64(offset)+uint64(size) > uint64(info.Size()) ||
		(size != 0 && offset < HeaderSize) {
		return nil, errors.New("invalid DBPF index extent")
	}
	reader := io.NewSectionReader(file, int64(offset), int64(size))
	records := make([]Record, 0, min(count, 4096))
	var row [IndexEntrySize]byte
	for range count {
		if _, err := io.ReadFull(reader, row[:]); err != nil {
			return nil, err
		}
		r := Record{
			ID{Little.Uint32(row[:]), Little.Uint32(row[4:]), Little.Uint32(row[8:])},
			Little.Uint32(row[12:]), Little.Uint32(row[16:]),
		}
		if uint64(r.Offset)+uint64(r.Size) > uint64(info.Size()) ||
			(r.Size != 0 && r.Offset < HeaderSize) {
			return nil, fmt.Errorf("%s: invalid DBPF resource extent", r.ID)
		}
		records = append(records, r)
	}
	return records, nil
}

func (a *Archive) Close() error { return a.file.Close() }
func (a *Archive) Name() string { return a.file.Name() }
func (a *Archive) ReadAt(data []byte, offset int64) (int, error) {
	return a.file.ReadAt(data, offset)
}

// ReadRaw returns the bytes stored in the archive, including any QFS encoding.
func (a *Archive) ReadRaw(r Record) ([]byte, error) {
	data := make([]byte, r.Size)
	if _, err := a.ReadAt(data, int64(r.Offset)); err != nil {
		return nil, err
	}
	return data, nil
}

func (a *Archive) Read(r Record) ([]byte, error) {
	data, err := a.ReadRaw(r)
	if err != nil {
		return nil, err
	}
	if isQFS(data) {
		return decodeQFS(data)
	}
	return data, nil
}

// VerifyCompression checks QFS records without reading the bodies of raw records.
func (a *Archive) VerifyCompression(r Record) error {
	var header [6]byte
	if r.Size < uint32(len(header)) {
		return nil
	}
	if _, err := a.ReadAt(header[:], int64(r.Offset)); err != nil {
		return err
	}
	if !isQFS(header[:]) {
		return nil
	}
	_, err := a.Read(r)
	return err
}

func isQFS(data []byte) bool {
	if len(data) < 6 || data[4] != 0x10 || data[5] != 0xfb {
		return false
	}
	// The length in a raw FSH or S3D header can start with the QFS marker.
	switch string(data[:4]) {
	case "SHPI", "3DMD":
		return false
	}
	return true
}

type Entry struct {
	ID   ID
	Data []byte
}

// Write keeps the caller's resource order so generated DATs can match
// the existing builder byte for byte.
func Write(w io.Writer, entries []Entry) error {
	indexSize := uint64(len(entries)) * IndexEntrySize
	offset := uint64(HeaderSize)
	for _, entry := range entries {
		offset += uint64(len(entry.Data))
	}
	if offset+indexSize > math.MaxUint32 {
		return errors.New("DBPF archive exceeds the 32-bit size limit")
	}
	var header [HeaderSize]byte
	copy(header[:], "DBPF")
	Little.PutUint32(header[4:], 1)
	Little.PutUint32(header[32:], 7)
	Little.PutUint32(header[36:], uint32(len(entries)))
	Little.PutUint32(header[40:], uint32(offset))
	Little.PutUint32(header[44:], uint32(indexSize))
	if _, err := w.Write(header[:]); err != nil {
		return err
	}
	for _, entry := range entries {
		if _, err := w.Write(entry.Data); err != nil {
			return err
		}
	}
	offset = HeaderSize
	var row [IndexEntrySize]byte
	for _, entry := range entries {
		Little.PutUint32(row[:], entry.ID.Kind)
		Little.PutUint32(row[4:], entry.ID.Group)
		Little.PutUint32(row[8:], entry.ID.Instance)
		Little.PutUint32(row[12:], uint32(offset))
		Little.PutUint32(row[16:], uint32(len(entry.Data)))
		if _, err := w.Write(row[:]); err != nil {
			return err
		}
		offset += uint64(len(entry.Data))
	}
	return nil
}
