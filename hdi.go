package main

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
)

type diskFile interface {
	ReadAt([]byte, int64) (int, error)
	WriteAt([]byte, int64) (int, error)
}

// hdiImage is an Anex86 hard-disk image: a 4096-byte header and then the sectors.
type hdiImage struct {
	f          diskFile
	Header     int64
	DataSize   int64
	SectorSize int
	Sectors    int
	Heads      int
	Cylinders  int
}

func openHDI(path string) (*hdiImage, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	img, err := bindHDI(f, st.Size())
	if err != nil {
		f.Close()
		return nil, err
	}
	return img, nil
}

func bindHDI(f diskFile, fileSize int64) (*hdiImage, error) {
	hdr := make([]byte, 32)
	if _, err := f.ReadAt(hdr, 0); err != nil {
		return nil, err
	}
	img, err := parseHDI(hdr, fileSize)
	if err != nil {
		return nil, err
	}
	img.f = f
	return img, nil
}

func parseHDI(hdr []byte, fileSize int64) (*hdiImage, error) {
	if len(hdr) < 32 {
		return nil, fmt.Errorf("hdi header is short")
	}
	u := func(off int) uint32 { return binary.LittleEndian.Uint32(hdr[off:]) }
	img := &hdiImage{
		Header:     int64(u(8)),
		DataSize:   int64(u(12)),
		SectorSize: int(u(16)),
		Sectors:    int(u(20)),
		Heads:      int(u(24)),
		Cylinders:  int(u(28)),
	}
	if img.Header != 4096 {
		return nil, fmt.Errorf("hdi header size %d", img.Header)
	}
	if img.SectorSize == 0 || img.Sectors == 0 || img.Heads == 0 || img.Cylinders == 0 {
		return nil, fmt.Errorf("hdi geometry is empty")
	}
	if fileSize != img.Header+img.DataSize {
		return nil, fmt.Errorf("hdi file is %d bytes, header describes %d", fileSize, img.Header+img.DataSize)
	}
	span := int64(img.SectorSize) * int64(img.Sectors) * int64(img.Heads) * int64(img.Cylinders)
	if span != img.DataSize {
		return nil, fmt.Errorf("hdi geometry covers %d bytes, header data size is %d", span, img.DataSize)
	}
	return img, nil
}

func (d *hdiImage) Close() error {
	if d.f == nil {
		return nil
	}
	if c, ok := d.f.(io.Closer); ok {
		err := c.Close()
		d.f = nil
		return err
	}
	d.f = nil
	return nil
}

// SectorCount is the number of addressable sectors.
func (d *hdiImage) SectorCount() int64 {
	return int64(d.Sectors) * int64(d.Heads) * int64(d.Cylinders)
}

// ReadSector reads one sector. n is zero-based.
func (d *hdiImage) ReadSector(n int64) ([]byte, error) {
	if n < 0 || n >= d.SectorCount() {
		return nil, fmt.Errorf("sector %d is outside the disk", n)
	}
	buf := make([]byte, d.SectorSize)
	off := d.Header + n*int64(d.SectorSize)
	if _, err := d.f.ReadAt(buf, off); err != nil {
		return nil, err
	}
	return buf, nil
}

// ReadAt reads the disk data area. off is relative to the first sector.
func (d *hdiImage) ReadAt(p []byte, off int64) (int, error) {
	if off < 0 || off > d.DataSize {
		return 0, fmt.Errorf("read at %d is outside the disk", off)
	}
	if off == d.DataSize {
		return 0, io.EOF
	}
	if int64(len(p)) > d.DataSize-off {
		p = p[:d.DataSize-off]
	}
	return d.f.ReadAt(p, d.Header+off)
}
