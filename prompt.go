package main

import (
	"encoding/binary"
	"math/bits"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"
)

// ankFont is the 8x16 half-width PC-98 font from a 288768-byte font.rom.
// Index by character code. Bit 7 of each row is the leftmost pixel.
type ankFont [128][16]byte

// promptWatch notices when a disk has dropped back to the DOS prompt.
// The boot prompt is ignored until the game has held a screen that is not
// a prompt. A prompt that then stays up, or any prompt that stays up a
// long time, leaves the emulator so the door can show its menu again.
type promptWatch struct {
	font        *ankFont
	armed       bool
	plainSince  time.Time
	promptSince time.Time
	now         func() time.Time
}

const (
	promptArmAfter  = 6 * time.Second
	promptExitAfter = 1500 * time.Millisecond
	promptStuck     = 12 * time.Second
)

func openPromptWatch(bios string) *promptWatch {
	font, err := loadANK(bios)
	if err != nil {
		return nil
	}
	return &promptWatch{font: font}
}

func loadANK(bios string) (*ankFont, error) {
	entries, err := os.ReadDir(bios)
	if err != nil {
		return nil, err
	}
	var bmp, rom string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		switch strings.ToLower(e.Name()) {
		case "font.bmp":
			bmp = e.Name()
		case "font.rom":
			rom = e.Name()
		}
	}
	if bmp != "" {
		if g, err := ankFromBMP(filepath.Join(bios, bmp)); err == nil {
			return g, nil
		}
	}
	if rom == "" {
		return nil, os.ErrNotExist
	}
	return ankFromROM(filepath.Join(bios, rom))
}

// ankFromBMP reads the half-width glyphs from Neko Project's 2048x2048 font.
// Those glyphs are the top 16 rows, one character per byte, bits inverted.
func ankFromBMP(path string) (*ankFont, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	hdr := make([]byte, 14)
	if _, err := f.Read(hdr); err != nil {
		return nil, err
	}
	if hdr[0] != 'B' || hdr[1] != 'M' {
		return nil, os.ErrInvalid
	}
	off := binary.LittleEndian.Uint32(hdr[10:])
	const line = 2048 / 8
	const dataSize = line * 2048
	if _, err := f.Seek(int64(off), 0); err != nil {
		return nil, err
	}
	data := make([]byte, dataSize)
	if _, err := f.Read(data); err != nil {
		return nil, err
	}
	var g ankFont
	for ank := 0; ank < 128; ank++ {
		p := dataSize + ank
		for y := 0; y < 16; y++ {
			p -= line
			g[ank][y] = ^data[p]
		}
	}
	return &g, nil
}

func ankFromROM(path string) (*ankFont, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if st.Size() != 0x46800 {
		return nil, os.ErrInvalid
	}
	buf := make([]byte, 16*128)
	if _, err := f.ReadAt(buf, 0x0800); err != nil {
		return nil, err
	}
	var g ankFont
	for i := 0; i < 128; i++ {
		copy(g[i][:], buf[i*16:(i+1)*16])
	}
	return &g, nil
}

// step reports whether the door should leave the emulator.
func (w *promptWatch) step(raw []byte, width, height, pitch int, pix uint32) bool {
	if w == nil || w.font == nil || len(raw) == 0 || width < 8 || height < 16 {
		return false
	}
	now := time.Now()
	if w.now != nil {
		now = w.now()
	}
	if dosPrompt(w.font, raw, width, height, pitch, pix) {
		w.plainSince = time.Time{}
		if w.promptSince.IsZero() {
			w.promptSince = now
		}
		held := now.Sub(w.promptSince)
		if held >= promptStuck {
			return true
		}
		return w.armed && held >= promptExitAfter
	}
	w.promptSince = time.Time{}
	if w.plainSince.IsZero() {
		w.plainSince = now
	}
	if !w.armed && now.Sub(w.plainSince) >= promptArmAfter {
		w.armed = true
	}
	return false
}

// dosPrompt reports whether the last line of text is a DOS command prompt.
func dosPrompt(font *ankFont, raw []byte, width, height, pitch int, pix uint32) bool {
	cols := width / 8
	rows := height / 16
	if cols < 16 || rows < 8 {
		return false
	}
	if cols > 80 {
		cols = 80
	}
	view := frameView{raw: raw, width: width, height: height, pitch: pitch, pix: pix}
	last := ""
	for row := 0; row < rows; row++ {
		line := readRow(font, view, row, cols)
		if strings.TrimRight(line, " ") != "" {
			last = line
		}
	}
	return promptRow(last)
}

func readRow(font *ankFont, view frameView, row, cols int) string {
	var b strings.Builder
	b.Grow(cols)
	for col := 0; col < cols; col++ {
		b.WriteByte(font.classify(view.cell(col*8, row*16)))
	}
	return b.String()
}

func (f *ankFont) classify(cell [16]byte) byte {
	ink := 0
	for _, b := range cell {
		ink += bits.OnesCount8(b)
	}
	if ink < 4 {
		return ' '
	}
	best := 255
	var bestC byte
	for c := byte(0x20); c < 0x7f; c++ {
		d := 0
		for i := 0; i < 16; i++ {
			d += bits.OnesCount8(cell[i] ^ f[c][i])
		}
		if d < best {
			best = d
			bestC = c
		}
	}
	if best > 16 {
		return '?'
	}
	return bestC
}

func promptRow(line string) bool {
	line = strings.TrimRight(line, " ")
	if line == "" {
		return false
	}
	// The cursor sits in the cell after the prompt and may not be a glyph.
	if i := strings.LastIndex(line, ">"); i >= 0 && i < len(line)-1 {
		tail := line[i+1:]
		if len(tail) == 1 {
			line = line[:i+1]
		}
	}
	if !strings.HasSuffix(line, ">") {
		return false
	}
	head := line[:len(line)-1]
	if head == "" {
		return false
	}
	r := rune(head[0])
	if r < 'A' || r > 'Z' {
		if r < 'a' || r > 'z' {
			return false
		}
		head = string(unicode.ToUpper(r)) + head[1:]
	}
	if len(head) == 1 {
		return true
	}
	if len(head) < 3 || head[1] != ':' || head[2] != '\\' {
		return false
	}
	for i := 3; i < len(head); i++ {
		c := head[i]
		switch {
		case c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z', c >= '0' && c <= '9':
		case c == '\\', c == '.', c == '_', c == '-', c == '$', c == '~':
		default:
			return false
		}
	}
	return true
}

type frameView struct {
	raw           []byte
	width, height int
	pitch         int
	pix           uint32
}

func (f frameView) on(x, y int) bool {
	if x < 0 || y < 0 || x >= f.width || y >= f.height {
		return false
	}
	var r, g, b int
	switch f.pix {
	case 1:
		o := y*f.pitch + x*4
		if o+2 >= len(f.raw) {
			return false
		}
		b, g, r = int(f.raw[o]), int(f.raw[o+1]), int(f.raw[o+2])
	default:
		o := y*f.pitch + x*2
		if o+1 >= len(f.raw) {
			return false
		}
		v := uint16(f.raw[o]) | uint16(f.raw[o+1])<<8
		if f.pix == 2 {
			r = int((v>>11)&31) * 255 / 31
			g = int((v>>5)&63) * 255 / 63
			b = int(v&31) * 255 / 31
		} else {
			r = int((v>>10)&31) * 255 / 31
			g = int((v>>5)&31) * 255 / 31
			b = int(v&31) * 255 / 31
		}
	}
	return r > 80 || g > 80 || b > 80
}

func (f frameView) cell(x0, y0 int) (bits [16]byte) {
	for y := 0; y < 16; y++ {
		var b byte
		for x := 0; x < 8; x++ {
			if f.on(x0+x, y0+y) {
				b |= 0x80 >> x
			}
		}
		bits[y] = b
	}
	return bits
}
