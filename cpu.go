package main

const (
	flagCF uint16 = 1 << 0
	flagPF uint16 = 1 << 2
	flagAF uint16 = 1 << 4
	flagZF uint16 = 1 << 6
	flagSF uint16 = 1 << 7
	flagIF uint16 = 1 << 9
	flagDF uint16 = 1 << 10
	flagOF uint16 = 1 << 11
)

const (
	svcNone = iota
	svcDisk
	svcKey
	svcMem
	svcEquip
	svcPause
	svcDOS
)

type cpu struct {
	ax, cx, dx, bx uint16
	sp, bp, si, di uint16
	cs, ds, es, ss uint16
	ip, flags      uint16
	seg            uint16
	hasSeg         bool
	rep            byte
	wide           bool
	ad32           bool
	rhi            [8]uint16
	ram            []byte
	halted         bool
	lastFault      string
	memWatch       func(addr uint32, old, val byte)
	inb            func(uint16) byte
	outb           func(uint16, byte)
	bios           func(byte) bool
	inw            func(uint16) uint16
	outw           func(uint16, uint16)
}

func (c *cpu) read8(a uint32) byte { return c.ram[a&0xFFFFF] }
func (c *cpu) read16(a uint32) uint16 {
	a &= 0xFFFFF
	return uint16(c.ram[a]) | uint16(c.ram[(a+1)&0xFFFFF])<<8
}
func (c *cpu) write8(a uint32, v byte) {
	a &= 0xFFFFF
	if c.memWatch != nil && c.ram[a] != v {
		c.memWatch(a, c.ram[a], v)
	}
	c.ram[a] = v
}
func (c *cpu) write16(a uint32, v uint16) {
	a &= 0xFFFFF
	c.write8(a, byte(v))
	c.write8((a+1)&0xFFFFF, byte(v>>8))
}

func (c *cpu) fetch8() byte {
	v := c.read8(uint32(c.cs)<<4 + uint32(c.ip))
	c.ip++
	return v
}
func (c *cpu) fetch16() uint16 {
	v := c.read16(uint32(c.cs)<<4 + uint32(c.ip))
	c.ip += 2
	return v
}

func (c *cpu) push(v uint16) {
	c.sp -= 2
	c.write16(uint32(c.ss)<<4+uint32(c.sp), v)
}
func (c *cpu) pop() uint16 {
	v := c.read16(uint32(c.ss)<<4 + uint32(c.sp))
	c.sp += 2
	return v
}

func (c *cpu) setFlag(bit uint16, on bool) {
	if on {
		c.flags |= bit
	} else {
		c.flags &^= bit
	}
}

func parity(b byte) bool {
	b ^= b >> 4
	b ^= b >> 2
	b ^= b >> 1
	return b&1 == 0
}

func (c *cpu) szp(v uint16, bits int) {
	c.flags &^= flagZF | flagSF | flagPF
	if bits == 8 {
		v &= 0xFF
	}
	if v == 0 {
		c.flags |= flagZF
	}
	sign := uint16(0x8000)
	if bits == 8 {
		sign = 0x80
	}
	if v&sign != 0 {
		c.flags |= flagSF
	}
	if parity(byte(v)) {
		c.flags |= flagPF
	}
}

func (c *cpu) add(a, b uint16, cin bool, bits int) uint16 {
	mask := uint16(0xFFFF)
	sign := uint16(0x8000)
	if bits == 8 {
		mask, sign = 0xFF, 0x80
		a, b = a&0xFF, b&0xFF
	}
	var carry uint32
	if cin {
		carry = 1
	}
	sum := uint32(a) + uint32(b) + carry
	r := uint16(sum) & mask
	c.flags &^= flagCF | flagOF | flagAF
	limit := uint32(0xFFFF)
	if bits == 8 {
		limit = 0xFF
	}
	if sum > limit {
		c.flags |= flagCF
	}
	if (a^r)&0x10 != 0 {
		c.flags |= flagAF
	}
	if (^(a ^ b))&(a^r)&sign != 0 {
		c.flags |= flagOF
	}
	c.szp(r, bits)
	return r
}

func (c *cpu) sub(a, b uint16, cin bool, bits int) uint16 {
	mask := uint16(0xFFFF)
	sign := uint16(0x8000)
	if bits == 8 {
		mask, sign = 0xFF, 0x80
		a, b = a&0xFF, b&0xFF
	}
	var borrow uint32
	if cin {
		borrow = 1
	}
	need := uint32(b) + borrow
	r := uint16(uint32(a)-need) & mask
	c.flags &^= flagCF | flagOF | flagAF
	if uint32(a) < need {
		c.flags |= flagCF
	}
	if (a^b^r)&0x10 != 0 {
		c.flags |= flagAF
	}
	if (a^b)&(a^r)&sign != 0 {
		c.flags |= flagOF
	}
	c.szp(r, bits)
	return r
}

func (c *cpu) logic(v uint16, bits int) uint16 {
	c.flags &^= flagCF | flagOF | flagAF
	c.szp(v, bits)
	return v
}

func (c *cpu) reg16(i int) uint16 {
	switch i & 7 {
	case 0:
		return c.ax
	case 1:
		return c.cx
	case 2:
		return c.dx
	case 3:
		return c.bx
	case 4:
		return c.sp
	case 5:
		return c.bp
	case 6:
		return c.si
	default:
		return c.di
	}
}

func (c *cpu) set16(i int, v uint16) {
	switch i & 7 {
	case 0:
		c.ax = v
	case 1:
		c.cx = v
	case 2:
		c.dx = v
	case 3:
		c.bx = v
	case 4:
		c.sp = v
	case 5:
		c.bp = v
	case 6:
		c.si = v
	default:
		c.di = v
	}
}

func (c *cpu) reg8(i int) byte {
	switch i & 7 {
	case 0:
		return byte(c.ax)
	case 1:
		return byte(c.cx)
	case 2:
		return byte(c.dx)
	case 3:
		return byte(c.bx)
	case 4:
		return byte(c.ax >> 8)
	case 5:
		return byte(c.cx >> 8)
	case 6:
		return byte(c.dx >> 8)
	default:
		return byte(c.bx >> 8)
	}
}

func (c *cpu) set8(i int, v byte) {
	switch i & 7 {
	case 0:
		c.ax = c.ax&0xFF00 | uint16(v)
	case 1:
		c.cx = c.cx&0xFF00 | uint16(v)
	case 2:
		c.dx = c.dx&0xFF00 | uint16(v)
	case 3:
		c.bx = c.bx&0xFF00 | uint16(v)
	case 4:
		c.ax = uint16(v)<<8 | c.ax&0xFF
	case 5:
		c.cx = uint16(v)<<8 | c.cx&0xFF
	case 6:
		c.dx = uint16(v)<<8 | c.dx&0xFF
	default:
		c.bx = uint16(v)<<8 | c.bx&0xFF
	}
}

func (c *cpu) override() uint16 {
	if c.hasSeg {
		return c.seg
	}
	return c.ds
}

func (c *cpu) ea(modrm byte) (addr uint32, reg bool) {
	mod, rm := modrm>>6, modrm&7
	if mod == 3 {
		return uint32(rm), true
	}
	var off uint16
	seg := c.ds
	switch rm {
	case 0:
		off = c.bx + c.si
	case 1:
		off = c.bx + c.di
	case 2:
		off = c.bp + c.si
		seg = c.ss
	case 3:
		off = c.bp + c.di
		seg = c.ss
	case 4:
		off = c.si
	case 5:
		off = c.di
	case 6:
		if mod == 0 {
			off = c.fetch16()
			break
		}
		off = c.bp
		seg = c.ss
	default:
		off = c.bx
	}
	if mod == 1 {
		off += uint16(int8(c.fetch8()))
	} else if mod == 2 {
		off += c.fetch16()
	}
	if c.hasSeg {
		seg = c.seg
	}
	return uint32(seg)<<4 + uint32(off), false
}

type operand struct {
	reg  bool
	n    int
	addr uint32
	off  uint16
}

func (c *cpu) parse() (reg int, op operand) {
	b := c.fetch8()
	if c.ad32 {
		return c.parse32(b)
	}
	reg = int((b >> 3) & 7)
	mod, rm := b>>6, b&7
	if mod == 3 {
		return reg, operand{reg: true, n: int(rm)}
	}
	var off uint16
	seg := c.ds
	switch rm {
	case 0:
		off = c.bx + c.si
	case 1:
		off = c.bx + c.di
	case 2:
		off = c.bp + c.si
		seg = c.ss
	case 3:
		off = c.bp + c.di
		seg = c.ss
	case 4:
		off = c.si
	case 5:
		off = c.di
	case 6:
		if mod == 0 {
			off = c.fetch16()
			break
		}
		off = c.bp
		seg = c.ss
	default:
		off = c.bx
	}
	if mod == 1 {
		off += uint16(int8(c.fetch8()))
	} else if mod == 2 {
		off += c.fetch16()
	}
	if c.hasSeg {
		seg = c.seg
	}
	return reg, operand{addr: uint32(seg)<<4 + uint32(off), off: off}
}

func (c *cpu) get8(op operand) byte {
	if op.reg {
		return c.reg8(op.n)
	}
	return c.read8(op.addr)
}
func (c *cpu) put8(op operand, v byte) {
	if op.reg {
		c.set8(op.n, v)
		return
	}
	c.write8(op.addr, v)
}
func (c *cpu) get16(op operand) uint16 {
	if op.reg {
		return c.reg16(op.n)
	}
	return c.read16(op.addr)
}
func (c *cpu) put16(op operand, v uint16) {
	if op.reg {
		c.set16(op.n, v)
		return
	}
	c.write16(op.addr, v)
}

func (c *cpu) intr(n byte) {
	if n == 0x21 {
		c.repairDOS()
	}
	c.push(c.flags)
	c.flags &^= flagIF | 0x100
	c.push(c.cs)
	c.push(c.ip)
	c.ip = c.read16(uint32(n) * 4)
	c.cs = c.read16(uint32(n)*4 + 2)
}

// repairDOS points the DOS interrupt chain at the real service entry.
// The image ships with that chain aimed at the shutdown routine, so the
// first call that would launch a program halts. The service entry is the
// standard CMP AH,0F8h / CMP AH,6Ch dispatcher in the same segment.
func (c *cpu) repairDOS() {
	seg := c.read16(0x21*4 + 2)
	off := c.read16(0x21 * 4)
	h := (uint32(seg)<<4 + uint32(off)) & 0xFFFFF
	if h > 0xFFFFF-16 || !c.dosStub(h) {
		return
	}
	disp := uint16(c.ram[h+14]) | uint16(c.ram[h+15])<<8
	ptr := (uint32(seg)<<4 + uint32(disp)) & 0xFFFFF
	destOff := c.read16(ptr)
	destSeg := c.read16((ptr + 2) & 0xFFFFF)
	dest := (uint32(destSeg)<<4 + uint32(destOff)) & 0xFFFFF
	if !c.shutdownEntry(dest) {
		return
	}
	entry, ok := c.dosEntry(destSeg)
	if !ok {
		return
	}
	c.write16(ptr, entry)
}

func (c *cpu) dosStub(h uint32) bool {
	b := c.ram
	return b[h] == 0x2E && b[h+1] == 0x80 && b[h+2] == 0x3E && b[h+5] == 0x00 &&
		b[h+6] == 0x74 && b[h+7] == 0x03 && b[h+8] == 0xE8 &&
		b[h+11] == 0x2E && b[h+12] == 0xFF && b[h+13] == 0x2E
}

func (c *cpu) shutdownEntry(dest uint32) bool {
	if dest > 0xFFFFF-0x60 || c.ram[dest] != 0xFA || c.ram[dest+1] != 0x8C || c.ram[dest+2] != 0xC8 {
		return false
	}
	out, hlt := false, false
	for i := uint32(0); i < 0x60; i++ {
		if c.ram[dest+i] == 0xE6 && c.ram[dest+i+1] == 0x37 {
			out = true
		}
		if out && c.ram[dest+i] == 0xF4 {
			hlt = true
		}
	}
	return hlt
}

// dosDispatch reports the near call that leaves the DOS service dispatcher.
// The 36h prefix was already consumed, so the four bytes before IP are
// FF 16 EA 05, a call through the saved BX slot at SS:05EA.
func (c *cpu) dosDispatch(rm operand) bool {
	if rm.reg || !c.hasSeg || c.seg != c.ss || rm.off != 0x05EA || c.ip < 4 {
		return false
	}
	at := uint32(c.cs)<<4 + uint32(c.ip-4)
	return c.read8(at) == 0xFF && c.read8(at+1) == 0x16 && c.read16(at+2) == 0x05EA
}

func (c *cpu) dosEntry(seg uint16) (uint16, bool) {
	base := uint32(seg) << 4
	for i := uint32(0); i < 0xFFF0; i++ {
		p := (base + i) & 0xFFFFF
		if p > 0xFFFFF-12 {
			continue
		}
		b := c.ram
		if b[p] == 0x80 && b[p+1] == 0xFC && b[p+2] == 0xF8 && b[p+3] == 0x72 && b[p+4] == 0x03 && b[p+5] == 0xE9 &&
			b[p+8] == 0xFA && b[p+9] == 0x80 && b[p+10] == 0xFC && b[p+11] == 0x6C {
			return uint16(i), true
		}
	}
	return 0, false
}

func (c *cpu) iret() {
	c.ip = c.pop()
	c.cs = c.pop()
	c.flags = c.pop() | 2
}

func (c *cpu) setCarryStacked(on bool) {
	a := uint32(c.ss)<<4 + uint32(c.sp) + 4
	f := c.read16(a)
	if on {
		f |= flagCF
	} else {
		f &^= flagCF
	}
	c.write16(a, f|2)
}

func (c *cpu) cond(cc byte) bool {
	cf := c.flags&flagCF != 0
	zf := c.flags&flagZF != 0
	sf := c.flags&flagSF != 0
	of := c.flags&flagOF != 0
	pf := c.flags&flagPF != 0
	switch cc & 0xF {
	case 0:
		return of
	case 1:
		return !of
	case 2:
		return cf
	case 3:
		return !cf
	case 4:
		return zf
	case 5:
		return !zf
	case 6:
		return cf || zf
	case 7:
		return !cf && !zf
	case 8:
		return sf
	case 9:
		return !sf
	case 0xA:
		return pf
	case 0xB:
		return !pf
	case 0xC:
		return sf != of
	case 0xD:
		return sf == of
	case 0xE:
		return zf || sf != of
	default:
		return !zf && sf == of
	}
}

func (c *cpu) step() int {
	c.hasSeg = false
	c.rep = 0
	c.wide = false
	c.ad32 = false
	for {
		op := c.fetch8()
		switch op {
		case 0x26:
			c.seg, c.hasSeg = c.es, true
		case 0x2E:
			c.seg, c.hasSeg = c.cs, true
		case 0x36:
			c.seg, c.hasSeg = c.ss, true
		case 0x3E:
			c.seg, c.hasSeg = c.ds, true
		case 0xF0:
		case 0xF2:
			c.rep = 1
		case 0xF3:
			c.rep = 2
		case 0x66:
			c.wide = true
		case 0x67:
			c.ad32 = true
		case 0x0F:
			return c.op0F()
		default:
			return c.exec(op)
		}
	}
}

func (c *cpu) fault(op byte) int {
	if c.lastFault == "" || c.lastFault == "unimplemented opcode" {
		c.lastFault = "unimplemented opcode " + hexByte(op)
	}
	c.halted = true
	c.ip--
	return svcNone
}

func hexByte(op byte) string {
	const h = "0123456789ABCDEF"
	return string([]byte{h[op>>4], h[op&15]})
}
