package main

import (
	"math/bits"
)

// encodeJXL writes a lossless 8-bit RGB JPEG XL codestream that libjxl and
// the vendored jxl-go decoder both accept. Width and height must be in
// 1..1024. The picture is stored as a palette plus per-pixel indexes, which
// suits the PC-9801's 16-color screen.
func encodeJXL(pix []byte, width, height int) ([]byte, error) {
	if width < 1 || height < 1 || width > 1024 || height > 1024 || len(pix) < width*height*4 {
		return nil, errJXLSize
	}
	pal, idx := paletteOf(pix, width, height)
	var b bitw
	writeImageHeader(&b, width, height)
	b.align()
	writeFrameHeader(&b)
	b.w(0, 1) // TOC is not permuted
	b.align()
	body := modularBody(pal, idx, width, height)
	lengths := []int{len(body), 0, 0, 0, 0}
	for _, n := range lengths {
		b.u32(uint32(n), []uint32{0, 1024, 17408, 4211712}, []int{10, 14, 22, 30})
	}
	b.align()
	b.buf = append(b.buf, body...)
	b.buf = append(b.buf, 0) // keep a zero-length TOC read off the end of the file
	return b.buf, nil
}

var errJXLSize = errSize{}

type errSize struct{}

func (errSize) Error() string { return "jpeg xl size must be between 1 and 1024 on each side" }

func paletteOf(pix []byte, w, h int) (pal [][3]byte, idx []byte) {
	seen := map[[3]byte]byte{}
	idx = make([]byte, w*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			o := (y*w + x) * 4
			c := [3]byte{pix[o], pix[o+1], pix[o+2]}
			id, ok := seen[c]
			if !ok {
				if len(pal) == 256 {
					id = 0
				} else {
					id = byte(len(pal))
					pal = append(pal, c)
					seen[c] = id
				}
			}
			idx[y*w+x] = id
		}
	}
	if len(pal) == 0 {
		pal = [][3]byte{{0, 0, 0}}
	}
	return pal, idx
}

func writeImageHeader(b *bitw, w, h int) {
	b.w(0x0AFF, 16) // FF 0A, least-significant bit first
	b.w(0, 1)       // dimensions are not stored in units of 8
	b.u32(uint32(h), []uint32{1, 1, 1, 1}, []int{9, 13, 18, 30})
	b.w(0, 3) // aspect ratio stored explicitly
	b.u32(uint32(w), []uint32{1, 1, 1, 1}, []int{9, 13, 18, 30})
	b.w(0, 1) // metadata is not all-default (XYB must be off)
	b.w(0, 1) // no extra header fields
	b.w(0, 1) // integer samples
	b.u32(8, []uint32{8, 10, 12, 1}, []int{0, 0, 0, 6})
	b.w(1, 1) // 16-bit modular buffers
	b.u32(0, []uint32{0, 1, 2, 1}, []int{0, 0, 4, 12})
	b.w(0, 1) // not XYB
	b.w(1, 1) // default sRGB colour encoding
	b.w(0, 2) // no extensions
	b.w(1, 1) // default upsampling matrices
}

func writeFrameHeader(b *bitw) {
	b.w(0, 1) // not all-default
	b.w(0, 2) // regular frame
	b.w(1, 1) // modular
	b.w(0, 2) // no flags
	b.w(0, 1) // no YCbCr
	b.w(0, 2) // no upsampling
	b.w(3, 2) // group size 1024, so a 640x400 frame is one group
	b.u32(2, []uint32{1, 2, 3, 4}, []int{0, 0, 0, 3})
	b.u32(0, []uint32{0, 1, 2, 3}, []int{0, 0, 0, 1})
	b.w(0, 2)                                         // downsample shift of the extra pass
	b.w(0, 1)                                         // no crop
	b.u32(0, []uint32{0, 1, 2, 3}, []int{0, 0, 0, 2}) // replace
	b.w(1, 1)                                         // last frame
	b.u32(0, []uint32{0, 0, 16, 48}, []int{0, 4, 5, 10})
	b.w(0, 1) // restoration filter is not default
	b.w(0, 1) // no Gabor
	b.w(0, 2) // no edge-preserving filter
	b.w(0, 2) // no restoration extensions
	b.w(0, 2) // no frame extensions
}

func modularBody(pal [][3]byte, idx []byte, w, h int) []byte {
	var b bitw
	b.w(1, 1) // default quantizer
	b.w(0, 1) // no global tree
	b.w(0, 1) // local tree
	b.w(1, 1) // default weighted predictor
	b.u32(1, []uint32{0, 1, 2, 18}, []int{0, 0, 4, 8})
	b.w(1, 2) // palette transform
	b.u32(0, []uint32{0, 8, 72, 1096}, []int{3, 6, 10, 13})
	b.u32(3, []uint32{1, 3, 4, 1}, []int{0, 0, 0, 13})
	b.u32(uint32(len(pal)), []uint32{0, 256, 1280, 5376}, []int{8, 10, 12, 16})
	b.u32(0, []uint32{0, 1, 257, 1281}, []int{0, 8, 10, 16})
	b.w(0, 4) // delta predictor unused
	writeTreeEntropy(&b)
	writePixelEntropy(&b, pixelTokens(pal, idx, w, h))
	b.align()
	return b.buf
}

// writeTreeEntropy is a two-cluster prefix stream whose only symbols are the
// constants the MA tree needs: a leaf, the "west" predictor, and a multiplier of 1.
func writeTreeEntropy(b *bitw) {
	b.w(0, 1) // no LZ77
	b.w(1, 1) // simple clustering
	b.w(1, 2) // one bit per distribution
	b.w(0, 1) // dist 0
	b.w(0, 1) // dist 1: property symbol 0 → leaf
	b.w(1, 1) // dist 2: predictor
	b.w(0, 1)
	b.w(0, 1)
	b.w(0, 1)
	b.w(1, 1) // prefix codes
	b.w(15, 4)
	b.w(15, 4) // split exponent covers the whole alphabet
	b.w(0, 1)  // cluster 0 alphabet size 1 (symbol 0)
	b.w(1, 1)  // cluster 1 alphabet size 2
	b.w(0, 4)
	b.w(1, 2) // simple prefix
	b.w(0, 2) // one symbol
	b.w(1, 1) // that symbol is 1, the west predictor
}

func writePixelEntropy(b *bitw, tokens []token) {
	b.w(0, 1) // no LZ77
	b.w(1, 1) // prefix codes
	b.w(4, 4) // hybrid split
	b.w(0, 3) // msb in token
	b.w(0, 3) // lsb in token
	b.w(1, 1) // explicit alphabet size
	b.w(4, 4) // 1 + (1<<4) + 4 = 21 symbols
	b.w(4, 4)
	lengths := pixelLengths()
	writePrefix(b, lengths)
	codes := codesFor(lengths)
	for _, t := range tokens {
		c := codes[t.tok]
		b.w(uint64(bits.Reverse32(c.code)), c.n)
		if t.extra > 0 {
			b.w(t.bits, t.extra)
		}
	}
}

func pixelLengths() []int {
	l := make([]int, 21)
	l[0] = 2
	for _, t := range []int{1, 2, 3, 4} {
		l[t] = 4
	}
	for t := 5; t <= 20; t++ {
		l[t] = 5
	}
	return l
}

type token struct {
	tok   int
	extra int
	bits  uint64
}

func pixelTokens(pal [][3]byte, idx []byte, w, h int) []token {
	var out []token
	// Palette meta channel: height is the component count, width is the colour count.
	prev := make([]int, len(pal))
	for c := 0; c < 3; c++ {
		west := 0
		if c > 0 {
			west = prev[0]
		}
		for i := 0; i < len(pal); i++ {
			if i == 0 && c > 0 {
				west = prev[0]
			}
			v := int(pal[i][c])
			out = append(out, hybrid(packSigned(v-west)))
			prev[i] = v
			if i+1 < len(pal) {
				west = v
			}
		}
	}
	// The decoder's "west" of the first pixel on a row is the first pixel of the previous row.
	row0 := 0
	for y := 0; y < h; y++ {
		west := 0
		if y > 0 {
			west = row0
		}
		row0 = int(idx[y*w])
		for x := 0; x < w; x++ {
			v := int(idx[y*w+x])
			out = append(out, hybrid(packSigned(v-west)))
			west = v
		}
	}
	return out
}

func packSigned(s int) int {
	if s >= 0 {
		return s * 2
	}
	return -s*2 - 1
}

func hybrid(v int) token {
	const splitExp = 4
	split := 1 << splitExp
	if v < split {
		return token{tok: v}
	}
	n := bits.Len(uint(v)) - 1
	return token{tok: n + split - splitExp, extra: n, bits: uint64(v) & ((1 << n) - 1)}
}

var codeLenOrder = []int{1, 2, 3, 4, 0, 5, 17, 6, 16, 7, 8, 9, 10, 11, 12, 13, 14, 15}

// level-0 code for a level-1 code length, least-significant bit first.
var level0 = [][]uint64{
	0: {0, 0},
	1: {1, 1, 1, 0},
	2: {1, 1, 0},
	3: {0, 1},
	4: {1, 0},
	5: {1, 1, 1, 1},
}

func writePrefix(b *bitw, lengths []int) {
	b.w(0, 2) // complex prefix, no skipped lengths
	meta := make([]int, 18)
	meta[2], meta[4], meta[5] = 2, 2, 1
	total := 0
	for i := 0; i < 18 && total < 32; i++ {
		ln := meta[codeLenOrder[i]]
		for _, bit := range level0[ln] {
			b.w(bit, 1)
		}
		if ln != 0 {
			total += 32 >> ln
		}
	}
	codes := codesFor(meta)
	total = 0
	for i := 0; i < len(lengths) && total < 32768; i++ {
		c := codes[lengths[i]]
		b.w(uint64(bits.Reverse32(c.code)), c.n)
		total += 32768 >> lengths[i]
	}
}

type hcode struct {
	code uint32
	n    int
}

// codesFor reproduces the canonical codes the decoder builds from a length table.
func codesFor(lengths []int) []hcode {
	counts := make([]int, len(lengths)+1)
	for _, l := range lengths {
		counts[l]++
	}
	for i := 1; i < len(counts); i++ {
		counts[i] += counts[i-1]
	}
	scrLen := make([]int, len(lengths))
	scrSym := make([]int, len(lengths))
	for i := len(lengths) - 1; i >= 0; i-- {
		counts[lengths[i]]--
		at := counts[lengths[i]]
		scrLen[at] = lengths[i]
		scrSym[at] = i
	}
	out := make([]hcode, len(lengths))
	var code uint32
	for i := 0; i < len(lengths); i++ {
		if scrLen[i] == 0 {
			continue
		}
		out[scrSym[i]] = hcode{code, scrLen[i]}
		code += 1 << (32 - scrLen[i])
	}
	return out
}

type bitw struct {
	buf []byte
	acc uint64
	n   int
}

func (b *bitw) w(v uint64, n int) {
	if n <= 0 {
		return
	}
	if n < 64 {
		v &= (uint64(1) << n) - 1
	}
	b.acc |= v << b.n
	b.n += n
	for b.n >= 8 {
		b.buf = append(b.buf, byte(b.acc))
		b.acc >>= 8
		b.n -= 8
	}
}

func (b *bitw) align() {
	if b.n > 0 {
		b.buf = append(b.buf, byte(b.acc))
		b.acc = 0
		b.n = 0
	}
}

func (b *bitw) u32(v uint32, c []uint32, u []int) {
	for i := 0; i < 4; i++ {
		span := uint64(1) << u[i]
		if uint64(v) >= uint64(c[i]) && uint64(v) < uint64(c[i])+span {
			b.w(uint64(i), 2)
			b.w(uint64(v)-uint64(c[i]), u[i])
			return
		}
	}
	panic("u32 value does not fit")
}
