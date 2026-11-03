package main

import "encoding/base64"

// jxlTerm is a door session whose client can draw a JPEG XL blob.
type jxlTerm interface {
	JXL() bool
}

// pc98RGB is the 16-color digital palette. Index bits are B, R, G, I.
var pc98RGB = [16][3]byte{
	{0x00, 0x00, 0x00},
	{0x00, 0x00, 0xAA},
	{0xAA, 0x00, 0x00},
	{0xAA, 0x00, 0xAA},
	{0x00, 0xAA, 0x00},
	{0x00, 0xAA, 0xAA},
	{0xAA, 0xAA, 0x00},
	{0xAA, 0xAA, 0xAA},
	{0x55, 0x55, 0x55},
	{0x55, 0x55, 0xFF},
	{0xFF, 0x55, 0x55},
	{0xFF, 0x55, 0xFF},
	{0x55, 0xFF, 0x55},
	{0x55, 0xFF, 0xFF},
	{0xFF, 0xFF, 0x55},
	{0xFF, 0xFF, 0xFF},
}

func jxlQuery() []byte {
	return []byte("\x1b_SyncTERM:Q;JXL\x1b\\")
}

func (m *machine) jxlBlob() ([]byte, error) {
	return jxlDraw(m.render(), 640, 400)
}

func jxlDraw(pix []byte, w, h int) ([]byte, error) {
	stream, err := encodeJXL(pix, w, h)
	if err != nil {
		return nil, err
	}
	enc := base64.StdEncoding
	dst := make([]byte, enc.EncodedLen(len(stream)))
	enc.Encode(dst, stream)
	b := make([]byte, 0, len(dst)+48)
	b = append(b, "\x1b_SyncTERM:C;DrawJXLBlob;DX=0;DY=0;ZX=1;ZY=1;"...)
	b = append(b, dst...)
	b = append(b, "\x1b\\"...)
	return b, nil
}

func (m *machine) pictureSum() uint64 {
	h := uint64(14695981039346656037)
	regions := [][2]int{
		{0xA0000, 160 * 25},
		{0xA8000, 80 * 400},
		{0xB0000, 80 * 400},
		{0xB8000, 80 * 400},
		{0xE0000, 80 * 400},
	}
	for _, r := range regions {
		for _, b := range m.cpu.ram[r[0] : r[0]+r[1]] {
			h ^= uint64(b)
			h *= 1099511628211
		}
	}
	return h
}

func (m *machine) render() []byte {
	pix := make([]byte, 640*400*4)
	ram := m.cpu.ram
	for y := 0; y < 400; y++ {
		row := y >> 4
		for x := 0; x < 640; x++ {
			bit := byte(0x80 >> (x & 7))
			o := y*80 + x>>3
			idx := 0
			if ram[0xA8000+o]&bit != 0 {
				idx |= 1
			}
			if ram[0xB0000+o]&bit != 0 {
				idx |= 2
			}
			if ram[0xB8000+o]&bit != 0 {
				idx |= 4
			}
			if ram[0xE0000+o]&bit != 0 {
				idx |= 8
			}
			col := x >> 3
			cell := 0xA0000 + row*160 + col*2
			ch := ram[cell]
			attr := ram[cell+1]
			if ch != 0 && ch != ' ' {
				on := m.glyphRow(ch, y&15)&bit != 0
				if attr&0x04 != 0 {
					on = !on
				}
				if on {
					idx = textColor(attr)
				}
			}
			p := (y*640 + x) * 4
			pix[p] = pc98RGB[idx][0]
			pix[p+1] = pc98RGB[idx][1]
			pix[p+2] = pc98RGB[idx][2]
			pix[p+3] = 255
		}
	}
	return pix
}

// textColor maps a PC-98 attribute. Bits 5–7 are GRB and bit 4 is intensity.
// A zero color is the normal white used by DOS, not black.
func textColor(attr byte) int {
	c := int((attr >> 5) & 7)
	if attr&0x10 != 0 {
		c |= 8
	}
	if c&7 == 0 {
		c |= 7
	}
	return c
}

func (m *machine) glyphRow(ch byte, row int) byte {
	if m.font == nil || row < 0 || row > 15 {
		return 0
	}
	// 8×16 ANK follows the 256 8×8 glyphs.
	off := 0x800 + int(ch)*16 + row
	if off < len(m.font) {
		return m.font[off]
	}
	if row&1 == 1 {
		return 0
	}
	off = int(ch)*8 + row/2
	if off < len(m.font) {
		return m.font[off]
	}
	return 0
}
