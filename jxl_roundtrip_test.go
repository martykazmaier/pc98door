package main

import (
	"bytes"
	"image"
	"os"
	"strings"
	"testing"

	jxl "github.com/kpfaulkner/jxl-go"
)

func wrapJXL(raw []byte) []byte {
	size := uint32(len(raw) + 8)
	b := []byte{
		0x00, 0x00, 0x00, 0x0C, 'J', 'X', 'L', ' ', 0x0D, 0x0A, 0x87, 0x0A,
		byte(size >> 24), byte(size >> 16), byte(size >> 8), byte(size),
		'j', 'x', 'l', 'c',
	}
	return append(b, raw...)
}

func TestJXLRoundTrip(t *testing.T) {
	const w, h = 32, 16
	pix := make([]byte, w*h*4)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			o := (y*w + x) * 4
			if x < 8 {
				pix[o], pix[o+1], pix[o+2] = 0xAA, 0, 0
			} else if x < 16 {
				pix[o], pix[o+1], pix[o+2] = 0, 0, 0xAA
			} else {
				pix[o+3] = 255
				continue
			}
			pix[o+3] = 255
		}
	}
	raw, err := encodeJXL(pix, w, h)
	if err != nil {
		t.Fatal(err)
	}
	stream := wrapJXL(raw)
	img, err := jxl.Decode(bytes.NewReader(stream))
	if err != nil {
		t.Fatalf("decode: %v (stream %d bytes)", err, len(stream))
	}
	if img.Bounds().Dx() != w || img.Bounds().Dy() != h {
		t.Fatalf("size %v", img.Bounds())
	}
	c := img.At(1, 1)
	r, g, b, _ := c.RGBA()
	t.Logf("pixel 1,1 = %d %d %d (%T)", r>>8, g>>8, b>>8, img)
	_ = image.NRGBA{}
	if r>>8 < 0x80 || g>>8 > 0x20 || b>>8 > 0x20 {
		t.Fatalf("red pixel decoded as %d,%d,%d", r>>8, g>>8, b>>8)
	}
}

func TestAegeanFrameDecodes(t *testing.T) {
	const path = `\\bbs\ele\doors\pc98\roms\Aegean Kai No Shizuku [T-En by Svipur v1.0].hdi`
	d, err := openHDI(path)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	font, err := os.ReadFile(`\\bbs\ele\doors\pc98\bios\font.rom`)
	if err != nil {
		t.Fatal(err)
	}
	m := bootMachine(d)
	m.font = font
	for i := 0; i < 5000000 && !m.cpu.halted; i++ {
		svc := m.step()
		if svc == svcPause || (svc == svcKey && byte(m.cpu.ax>>8) == 0 && len(m.keys) == 0) {
			break
		}
		if svc != svcNone {
			m.service(svc)
		}
	}
	pix := m.render()
	raw, err := encodeJXL(pix, 640, 400)
	if err != nil {
		t.Fatal(err)
	}
	img, err := jxl.Decode(bytes.NewReader(wrapJXL(raw)))
	if err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	for y := 0; y < 400; y += 16 {
		for x := 0; x < 640; x += 8 {
			r, g, bl, _ := img.At(x+3, y+4).RGBA()
			if r|g|bl > 0x2000 {
				b.WriteByte('#')
			} else {
				b.WriteByte(' ')
			}
		}
		b.WriteByte('\n')
	}
	t.Logf("\n%s", b.String())
	if !strings.Contains(b.String(), "####") {
		t.Fatal("decoded frame has no text")
	}
}

func TestJXLScreenRoundTrip(t *testing.T) {
	const w, h = 640, 400
	pix := make([]byte, w*h*4)
	for i := 0; i < len(pix); i += 4 {
		pix[i+3] = 255
	}
	// A white glyph-sized block where the DOS banner sits.
	for y := 16; y < 32; y++ {
		for x := 8; x < 80; x++ {
			o := (y*w + x) * 4
			pix[o], pix[o+1], pix[o+2] = 0xAA, 0xAA, 0xAA
		}
	}
	raw, err := encodeJXL(pix, w, h)
	if err != nil {
		t.Fatal(err)
	}
	img, err := jxl.Decode(bytes.NewReader(wrapJXL(raw)))
	if err != nil {
		t.Fatalf("decode %d-byte screen: %v", len(raw), err)
	}
	r, g, b, _ := img.At(20, 20).RGBA()
	if r>>8 < 0x80 || g>>8 < 0x80 || b>>8 < 0x80 {
		t.Fatalf("banner pixel = %d,%d,%d", r>>8, g>>8, b>>8)
	}
	r, g, b, _ = img.At(0, 0).RGBA()
	if r>>8 != 0 || g>>8 != 0 || b>>8 != 0 {
		t.Fatalf("origin pixel = %d,%d,%d", r>>8, g>>8, b>>8)
	}
}
