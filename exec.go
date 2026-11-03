package main

func (c *cpu) exec(op byte) int {
	switch op {
	case 0x00, 0x01, 0x02, 0x03, 0x04, 0x05,
		0x08, 0x09, 0x0A, 0x0B, 0x0C, 0x0D,
		0x10, 0x11, 0x12, 0x13, 0x14, 0x15,
		0x18, 0x19, 0x1A, 0x1B, 0x1C, 0x1D,
		0x20, 0x21, 0x22, 0x23, 0x24, 0x25,
		0x28, 0x29, 0x2A, 0x2B, 0x2C, 0x2D,
		0x30, 0x31, 0x32, 0x33, 0x34, 0x35,
		0x38, 0x39, 0x3A, 0x3B, 0x3C, 0x3D:
		c.arith(op)
	case 0x06:
		c.push(c.es)
	case 0x07:
		c.es = c.pop()
	case 0x0E:
		c.push(c.cs)
	case 0x16:
		c.push(c.ss)
	case 0x17:
		c.ss = c.pop()
	case 0x1E:
		c.push(c.ds)
	case 0x1F:
		c.ds = c.pop()
	case 0x27, 0x2F, 0x37, 0x3F:
		c.adjust(op)
	case 0x40, 0x41, 0x42, 0x43, 0x44, 0x45, 0x46, 0x47:
		i := int(op & 7)
		cf := c.flags & flagCF
		if c.wide {
			c.set32(i, c.add32(c.reg32(i), 1, false))
		} else {
			c.set16(i, c.add(c.reg16(i), 1, false, 16))
		}
		c.setFlag(flagCF, cf != 0)
	case 0x48, 0x49, 0x4A, 0x4B, 0x4C, 0x4D, 0x4E, 0x4F:
		i := int(op & 7)
		cf := c.flags & flagCF
		if c.wide {
			c.set32(i, c.sub32(c.reg32(i), 1, false))
		} else {
			c.set16(i, c.sub(c.reg16(i), 1, false, 16))
		}
		c.setFlag(flagCF, cf != 0)
	case 0x50, 0x51, 0x52, 0x53, 0x54, 0x55, 0x56, 0x57:
		if c.wide {
			c.push32(c.reg32(int(op & 7)))
		} else {
			c.push(c.reg16(int(op & 7)))
		}
	case 0x58, 0x59, 0x5A, 0x5B, 0x5C, 0x5D, 0x5E, 0x5F:
		if c.wide {
			c.set32(int(op&7), c.pop32())
		} else {
			c.set16(int(op&7), c.pop())
		}
	case 0x60:
		sp := c.sp
		c.push(c.ax)
		c.push(c.cx)
		c.push(c.dx)
		c.push(c.bx)
		c.push(sp)
		c.push(c.bp)
		c.push(c.si)
		c.push(c.di)
	case 0x61:
		c.di = c.pop()
		c.si = c.pop()
		c.bp = c.pop()
		c.pop()
		c.bx = c.pop()
		c.dx = c.pop()
		c.cx = c.pop()
		c.ax = c.pop()
	case 0x68:
		if c.wide {
			c.push32(c.fetch32())
		} else {
			c.push(c.fetch16())
		}
	case 0x6A:
		v := uint32(int8(c.fetch8()))
		if c.wide {
			c.push32(v)
		} else {
			c.push(uint16(v))
		}
	case 0x69, 0x6B:
		reg, rm := c.parse()
		var imm uint32
		if op == 0x6B {
			imm = uint32(int32(int8(c.fetch8())))
		} else if c.wide {
			imm = c.fetch32()
		} else {
			imm = uint32(c.fetch16())
		}
		if c.wide {
			r := int64(int32(c.getWord(rm))) * int64(int32(imm))
			c.set32(reg, uint32(r))
		} else {
			c.set16(reg, c.mul(c.get16(rm), uint16(imm), true))
		}
	case 0x6C, 0x6D, 0x6E, 0x6F:
		c.stringIO(op)
	case 0x70, 0x71, 0x72, 0x73, 0x74, 0x75, 0x76, 0x77, 0x78, 0x79, 0x7A, 0x7B, 0x7C, 0x7D, 0x7E, 0x7F:
		rel := int8(c.fetch8())
		if c.cond(op & 0xF) {
			c.ip += uint16(rel)
		}
	case 0x80, 0x82:
		c.aluImm(8, false)
	case 0x81:
		c.aluImm(16, false)
	case 0x83:
		c.aluImm(16, true)
	case 0x84:
		reg, rm := c.parse()
		c.test(uint16(c.get8(rm)), uint16(c.reg8(reg)), 8)
	case 0x86, 0x87, 0x88, 0x89, 0x8A, 0x8B, 0x8C, 0x8D, 0x8E, 0x8F:
		c.move(op)
	case 0x85:
		reg, rm := c.parse()
		if c.wide {
			c.logic32(c.getWord(rm) & c.reg32(reg))
		} else {
			c.test(c.get16(rm), c.reg16(reg), 16)
		}
	case 0x90, 0x91, 0x92, 0x93, 0x94, 0x95, 0x96, 0x97:
		i := int(op & 7)
		v := c.reg16(i)
		c.set16(i, c.ax)
		c.ax = v
	case 0x98:
		if c.ax&0x80 != 0 {
			c.ax |= 0xFF00
		} else {
			c.ax &= 0x00FF
		}
	case 0x99:
		if c.ax&0x8000 != 0 {
			c.dx = 0xFFFF
		} else {
			c.dx = 0
		}
	case 0x9A:
		off, seg := c.fetch16(), c.fetch16()
		c.push(c.cs)
		c.push(c.ip)
		c.cs, c.ip = seg, off
	case 0x9C:
		c.push(c.flags | 2)
	case 0x9D:
		c.flags = c.pop() | 2
	case 0x9E:
		c.flags = (c.flags &^ 0xFF) | uint16(c.ax>>8) | 2
	case 0x9F:
		c.ax = (c.ax & 0x00FF) | ((c.flags | 2) << 8)
	case 0xA0:
		c.set8(0, c.read8(uint32(c.override())<<4+uint32(c.fetch16())))
	case 0xA1:
		off := c.moffs()
		if c.wide {
			c.set32(0, c.read32(uint32(c.override())<<4+off))
		} else {
			c.ax = c.read16(uint32(c.override())<<4 + off)
		}
	case 0xA2:
		c.write8(uint32(c.override())<<4+uint32(c.fetch16()), byte(c.ax))
	case 0xA3:
		off := c.moffs()
		if c.wide {
			c.write32(uint32(c.override())<<4+off, c.reg32(0))
		} else {
			c.write16(uint32(c.override())<<4+off, c.ax)
		}
	case 0xA4, 0xA5, 0xA6, 0xA7, 0xAA, 0xAB, 0xAC, 0xAD, 0xAE, 0xAF:
		c.stringOp(op)
	case 0xA8:
		c.test(uint16(c.ax), uint16(c.fetch8()), 8)
	case 0xA9:
		if c.wide {
			c.logic32(c.reg32(0) & c.fetch32())
		} else {
			c.test(c.ax, c.fetch16(), 16)
		}
	case 0xB0, 0xB1, 0xB2, 0xB3, 0xB4, 0xB5, 0xB6, 0xB7:
		c.set8(int(op&7), c.fetch8())
	case 0xB8, 0xB9, 0xBA, 0xBB, 0xBC, 0xBD, 0xBE, 0xBF:
		if c.wide {
			c.set32(int(op&7), c.fetch32())
		} else {
			c.set16(int(op&7), c.fetch16())
		}
	case 0xC0:
		c.shift(8, int(c.fetch8()))
	case 0xC1:
		c.shift(16, int(c.fetch8()))
	case 0xC2:
		n := c.fetch16()
		c.ip = c.pop()
		c.sp += n
	case 0xC3:
		c.ip = c.pop()
	case 0xC4, 0xC5:
		reg, rm := c.parse()
		if rm.reg {
			return c.fault(op)
		}
		c.set16(reg, c.read16(rm.addr))
		if op == 0xC4 {
			c.es = c.read16(rm.addr + 2)
		} else {
			c.ds = c.read16(rm.addr + 2)
		}
	case 0xC6:
		_, rm := c.parse()
		c.put8(rm, c.fetch8())
	case 0xC7:
		_, rm := c.parse()
		if c.wide {
			c.putWord(rm, c.fetch32())
		} else {
			c.put16(rm, c.fetch16())
		}
	case 0xC8:
		c.enter(c.fetch16(), c.fetch8())
	case 0xC9:
		c.sp = c.bp
		c.bp = c.pop()
	case 0xCA:
		n := c.fetch16()
		c.ip = c.pop()
		c.cs = c.pop()
		c.sp += n
	case 0xCB:
		c.ip = c.pop()
		c.cs = c.pop()
	case 0xCC:
		c.intr(3)
	case 0xCD:
		n := c.fetch8()
		if c.bios != nil && c.bios(n) {
			break
		}
		c.intr(n)
	case 0xCF:
		c.iret()
	case 0xD0:
		c.shift(8, 1)
	case 0xD1:
		c.shift(16, 1)
	case 0xD2:
		c.shift(8, int(c.cx&31))
	case 0xD3:
		c.shift(16, int(c.cx&31))
	case 0xD7:
		c.set8(0, c.read8(uint32(c.ds)<<4+uint32(c.bx)+uint32(c.ax&0xFF)))
	case 0xD8, 0xD9, 0xDA, 0xDB, 0xDC, 0xDD, 0xDE, 0xDF:
		c.parse()
	case 0xCE:
		if c.flags&flagOF != 0 {
			c.intr(4)
		}
	case 0xE0, 0xE1, 0xE2:
		c.cx--
		rel := int8(c.fetch8())
		take := c.cx != 0
		if op == 0xE0 {
			take = take && c.flags&flagZF == 0
		} else if op == 0xE1 {
			take = take && c.flags&flagZF != 0
		}
		if take {
			c.ip += uint16(rel)
		}
	case 0xE3:
		rel := int8(c.fetch8())
		if c.cx == 0 {
			c.ip += uint16(rel)
		}
	case 0xE4:
		if c.inb != nil {
			c.set8(0, c.inb(uint16(c.fetch8())))
		} else {
			c.fetch8()
			c.set8(0, 0xFF)
		}
	case 0xE5:
		port := uint16(c.fetch8())
		if c.inw != nil {
			c.ax = c.inw(port)
		} else {
			c.ax = 0xFFFF
		}
	case 0xE6:
		v := byte(c.ax)
		port := uint16(c.fetch8())
		if c.outb != nil {
			c.outb(port, v)
		}
	case 0xE7:
		port := uint16(c.fetch8())
		if c.outw != nil {
			c.outw(port, c.ax)
		}
	case 0xE8:
		rel := int16(c.fetch16())
		c.push(c.ip)
		c.ip += uint16(rel)
	case 0xE9:
		rel := int16(c.fetch16())
		c.ip += uint16(rel)
	case 0xEA:
		off, seg := c.fetch16(), c.fetch16()
		c.cs, c.ip = seg, off
	case 0xEB:
		c.ip += uint16(int8(c.fetch8()))
	case 0xEC:
		if c.inb != nil {
			c.set8(0, c.inb(c.dx))
		} else {
			c.set8(0, 0xFF)
		}
	case 0xED:
		if c.inw != nil {
			c.ax = c.inw(c.dx)
		} else {
			c.ax = 0xFFFF
		}
	case 0xEE:
		if c.outb != nil {
			c.outb(c.dx, byte(c.ax))
		}
	case 0xEF:
		if c.outw != nil {
			c.outw(c.dx, c.ax)
		}
	case 0xF4:
		lin := uint32(c.cs)<<4 + uint32(c.ip-1)
		switch lin {
		case biosDisk:
			return svcDisk
		case biosKey:
			return svcKey
		case biosMem:
			return svcMem
		case biosEquip:
			return svcEquip
		case biosDOS:
			return svcDOS
		default:
			return svcPause
		}
	case 0xF5:
		c.flags ^= flagCF
	case 0xF6:
		c.groupF6(8)
	case 0xF7:
		c.groupF6(16)
	case 0xF8:
		c.flags &^= flagCF
	case 0xF9:
		c.flags |= flagCF
	case 0xFA:
		c.flags &^= flagIF
	case 0xFB:
		c.flags |= flagIF
	case 0xFC:
		c.flags &^= flagDF
	case 0xFD:
		c.flags |= flagDF
	case 0xFE:
		reg, rm := c.parse()
		cf := c.flags & flagCF
		switch reg {
		case 0:
			c.put8(rm, byte(c.add(uint16(c.get8(rm)), 1, false, 8)))
		case 1:
			c.put8(rm, byte(c.sub(uint16(c.get8(rm)), 1, false, 8)))
		default:
			return c.fault(op)
		}
		c.setFlag(flagCF, cf != 0)
	case 0xFF:
		return c.groupFF()
	default:
		c.lastFault = "unimplemented opcode"
		return c.fault(op)
	}
	return svcNone
}

func (c *cpu) arith(op byte) {
	kind := op >> 3
	form := op & 7
	if c.wide && (form == 1 || form == 3 || form == 5) {
		c.arith32(kind, form)
		return
	}
	var a, b uint16
	bits := 16
	var rm operand
	reg := 0
	switch form {
	case 0, 1:
		reg, rm = c.parse()
		if form == 0 {
			bits = 8
			a, b = uint16(c.get8(rm)), uint16(c.reg8(reg))
		} else {
			a, b = c.get16(rm), c.reg16(reg)
		}
	case 2, 3:
		reg, rm = c.parse()
		if form == 2 {
			bits = 8
			a, b = uint16(c.reg8(reg)), uint16(c.get8(rm))
		} else {
			a, b = c.reg16(reg), c.get16(rm)
		}
	case 4:
		bits = 8
		a, b = uint16(byte(c.ax)), uint16(c.fetch8())
	default:
		a, b = c.ax, c.fetch16()
	}
	cin := c.flags&flagCF != 0
	var r uint16
	switch kind {
	case 0:
		r = c.add(a, b, false, bits)
	case 1:
		r = c.logic(a|b, bits)
	case 2:
		r = c.add(a, b, cin, bits)
	case 3:
		r = c.sub(a, b, cin, bits)
	case 4:
		r = c.logic(a&b, bits)
	case 5:
		r = c.sub(a, b, false, bits)
	case 6:
		r = c.logic(a^b, bits)
	default:
		c.sub(a, b, false, bits)
		return
	}
	switch form {
	case 0:
		c.put8(rm, byte(r))
	case 1:
		c.put16(rm, r)
	case 2:
		c.set8(reg, byte(r))
	case 3:
		c.set16(reg, r)
	case 4:
		c.set8(0, byte(r))
	default:
		c.ax = r
	}
}

func (c *cpu) aluImm(bits int, sign bool) {
	if bits == 16 && c.wide {
		c.aluImm32(sign)
		return
	}
	reg, rm := c.parse()
	var imm uint16
	if bits == 8 {
		imm = uint16(c.fetch8())
	} else if sign {
		imm = uint16(int8(c.fetch8()))
	} else {
		imm = c.fetch16()
	}
	var a uint16
	if bits == 8 {
		a = uint16(c.get8(rm))
	} else {
		a = c.get16(rm)
	}
	cin := c.flags&flagCF != 0
	var r uint16
	store := true
	switch reg {
	case 0:
		r = c.add(a, imm, false, bits)
	case 1:
		r = c.logic(a|imm, bits)
	case 2:
		r = c.add(a, imm, cin, bits)
	case 3:
		r = c.sub(a, imm, cin, bits)
	case 4:
		r = c.logic(a&imm, bits)
	case 5:
		r = c.sub(a, imm, false, bits)
	case 6:
		r = c.logic(a^imm, bits)
	default:
		if bits == 8 {
			imm &= 0xFF
		}
		c.sub(a, imm, false, bits)
		store = false
	}
	if !store {
		return
	}
	if bits == 8 {
		c.put8(rm, byte(r))
	} else {
		c.put16(rm, r)
	}
}

func (c *cpu) test(a, b uint16, bits int) {
	c.logic(a&b, bits)
}

func (c *cpu) move(op byte) {
	reg, rm := c.parse()
	switch op {
	case 0x86:
		v := c.get8(rm)
		c.put8(rm, c.reg8(reg))
		c.set8(reg, v)
	case 0x87:
		if c.wide {
			v := c.getWord(rm)
			c.putWord(rm, c.reg32(reg))
			c.set32(reg, v)
			break
		}
		v := c.get16(rm)
		c.put16(rm, c.reg16(reg))
		c.set16(reg, v)
	case 0x88:
		c.put8(rm, c.reg8(reg))
	case 0x89:
		if c.wide {
			c.putWord(rm, c.reg32(reg))
		} else {
			c.put16(rm, c.reg16(reg))
		}
	case 0x8A:
		c.set8(reg, c.get8(rm))
	case 0x8B:
		if c.wide {
			c.set32(reg, c.getWord(rm))
		} else {
			c.set16(reg, c.get16(rm))
		}
	case 0x8C:
		sreg := []uint16{c.es, c.cs, c.ss, c.ds}[reg&3]
		c.put16(rm, sreg)
	case 0x8D:
		if rm.reg {
			c.fault(op)
			return
		}
		c.set16(reg, rm.off)
	case 0x8E:
		v := c.get16(rm)
		switch reg & 3 {
		case 0:
			c.es = v
		case 2:
			c.ss = v
		case 3:
			c.ds = v
		}
	case 0x8F:
		if c.wide {
			c.putWord(rm, c.pop32())
		} else {
			c.put16(rm, c.pop())
		}
	}
}

func (c *cpu) mul(a, b uint16, signed bool) uint16 {
	var r int32
	if signed {
		r = int32(int16(a)) * int32(int16(b))
	} else {
		r = int32(uint32(a) * uint32(b))
	}
	c.flags &^= flagCF | flagOF
	if uint16(r) != uint16(uint32(r)&0xFFFF) || (signed && int32(int16(r)) != r) {
		c.flags |= flagCF | flagOF
	}
	if !signed && uint32(r) != uint32(uint16(r)) {
		c.flags |= flagCF | flagOF
	}
	c.szp(uint16(r), 16)
	return uint16(r)
}

func (c *cpu) groupF6(bits int) {
	reg, rm := c.parse()
	switch reg {
	case 0, 1:
		var imm uint32
		if bits == 8 {
			imm = uint32(c.fetch8())
		} else if c.wide {
			imm = c.fetch32()
		} else {
			imm = uint32(c.fetch16())
		}
		if bits == 8 {
			c.test(uint16(c.get8(rm)), uint16(imm), 8)
		} else if c.wide {
			c.logic32(c.getWord(rm) & imm)
		} else {
			c.test(c.get16(rm), uint16(imm), 16)
		}
	case 2:
		if bits == 8 {
			c.put8(rm, ^c.get8(rm))
		} else if c.wide {
			c.putWord(rm, ^c.getWord(rm))
		} else {
			c.put16(rm, ^c.get16(rm))
		}
	case 3:
		if bits == 8 {
			v := c.get8(rm)
			c.put8(rm, byte(-int8(v)))
			c.sub(0, uint16(v), false, 8)
		} else if c.wide {
			v := c.getWord(rm)
			c.putWord(rm, uint32(-int32(v)))
			c.sub32(0, v, false)
		} else {
			v := c.get16(rm)
			c.put16(rm, uint16(-int16(v)))
			c.sub(0, v, false, 16)
		}
	case 4, 5:
		c.multiply(rm, bits, reg == 5)
	case 6, 7:
		c.divide(rm, bits, reg == 7)
	}
}

func (c *cpu) multiply(rm operand, bits int, signed bool) {
	if bits == 8 {
		a := c.ax & 0xFF
		var b uint16
		if signed {
			b = uint16(int8(c.get8(rm)))
			a = uint16(int8(byte(a)))
		} else {
			b = uint16(c.get8(rm))
		}
		r := c.mul(a, b, signed)
		c.ax = r
		return
	}
	a, b := c.ax, c.get16(rm)
	var hi, lo uint16
	if signed {
		r := int32(int16(a)) * int32(int16(b))
		lo = uint16(r)
		hi = uint16(uint32(r) >> 16)
		c.flags &^= flagCF | flagOF
		if int32(int16(lo)) != r {
			c.flags |= flagCF | flagOF
		}
	} else {
		r := uint32(a) * uint32(b)
		lo = uint16(r)
		hi = uint16(r >> 16)
		c.flags &^= flagCF | flagOF
		if hi != 0 {
			c.flags |= flagCF | flagOF
		}
	}
	c.ax, c.dx = lo, hi
}

func (c *cpu) divide(rm operand, bits int, signed bool) {
	if bits == 8 {
		d := uint16(c.get8(rm))
		if d == 0 {
			c.intr(0)
			return
		}
		q := c.ax / d
		if q > 0xFF {
			c.intr(0)
			return
		}
		c.ax = (c.ax%d)<<8 | q
		return
	}
	n := uint32(c.dx)<<16 | uint32(c.ax)
	d := uint32(c.get16(rm))
	if d == 0 {
		c.intr(0)
		return
	}
	q := n / d
	if q > 0xFFFF {
		c.intr(0)
		return
	}
	c.ax = uint16(q)
	c.dx = uint16(n % d)
	_ = signed
}

func (c *cpu) shift(bits, count int) {
	reg, rm := c.parse()
	count &= 31
	if count == 0 {
		return
	}
	if bits == 16 && c.wide {
		bits = 32
	}
	var v uint32
	mask := uint32(0xFFFF)
	sign := uint32(0x8000)
	if bits == 8 {
		v = uint32(c.get8(rm))
		mask, sign = 0xFF, 0x80
	} else if bits == 32 {
		v = c.getWord(rm)
		mask, sign = 0xFFFFFFFF, 0x80000000
	} else {
		v = uint32(c.get16(rm))
	}
	c.flags &^= flagCF | flagOF
	switch reg {
	case 0, 1: // ROL ROR simplified as ROL/ROR
		for i := 0; i < count; i++ {
			if reg == 0 {
				bit := (v & sign) >> (bits - 1)
				v = ((v << 1) | bit) & mask
				if bit != 0 {
					c.flags |= flagCF
				} else {
					c.flags &^= flagCF
				}
			} else {
				bit := v & 1
				v = (v >> 1) | (bit << (bits - 1))
				if bit != 0 {
					c.flags |= flagCF
				} else {
					c.flags &^= flagCF
				}
			}
		}
	case 2, 3:
		for i := 0; i < count; i++ {
			cin := uint32(0)
			if c.flags&flagCF != 0 {
				cin = 1
			}
			if reg == 2 {
				c.setFlag(flagCF, v&sign != 0)
				v = ((v << 1) | cin) & mask
			} else {
				c.setFlag(flagCF, v&1 != 0)
				top := uint32(0)
				if cin != 0 {
					top = sign
				}
				v = (v >> 1) | top
			}
		}
	case 4, 6:
		if v&(sign>>uint(count-1)) != 0 {
			c.flags |= flagCF
		}
		v = (v << uint(count)) & mask
	case 5:
		if v&(1<<uint(count-1)) != 0 {
			c.flags |= flagCF
		}
		v >>= uint(count)
	case 7:
		bit := v & sign
		if v&(1<<uint(count-1)) != 0 {
			c.flags |= flagCF
		}
		v >>= uint(count)
		if bit != 0 {
			v |= mask << (bits - count) & mask
			fill := mask &^ ((uint32(1) << uint(bits-count)) - 1)
			v = (v & ((1 << uint(bits-count)) - 1)) | (fill & mask)
		}
	}
	if bits == 8 {
		c.put8(rm, byte(v))
		c.szp(uint16(v), 8)
	} else if bits == 32 {
		c.putWord(rm, v)
		c.szp32(v)
	} else {
		c.put16(rm, uint16(v))
		c.szp(uint16(v), 16)
	}
}

func (c *cpu) enter(size uint16, level byte) {
	if c.wide {
		c.push32(c.reg32(5))
		frame := c.reg32(4)
		if level > 0 {
			for level--; level > 0; level-- {
				bp := c.reg32(5) - 4
				c.set32(5, bp)
				c.push32(c.read32(uint32(c.ss)<<4 + (bp & 0xFFFF)))
			}
			c.push32(frame)
		}
		c.set32(5, frame)
		c.set32(4, frame-uint32(size))
		return
	}
	c.push(c.bp)
	frame := c.sp
	if level > 0 {
		for level--; level > 0; level-- {
			c.bp -= 2
			c.push(c.read16(uint32(c.ss)<<4 + uint32(c.bp)))
		}
		c.push(frame)
	}
	c.bp = frame
	c.sp = frame - size
}

func (c *cpu) groupFF() int {
	reg, rm := c.parse()
	switch reg {
	case 0, 1:
		cf := c.flags & flagCF
		if c.wide {
			if reg == 0 {
				c.putWord(rm, c.add32(c.getWord(rm), 1, false))
			} else {
				c.putWord(rm, c.sub32(c.getWord(rm), 1, false))
			}
		} else if reg == 0 {
			c.put16(rm, c.add(c.get16(rm), 1, false, 16))
		} else {
			c.put16(rm, c.sub(c.get16(rm), 1, false, 16))
		}
		c.setFlag(flagCF, cf != 0)
	case 2:
		// MS-DOS reaches its service routines through this one near call.
		// The image's pointer table is still the startup code, so follow
		// the call into the service routine the door provides instead.
		if !c.wide && c.dosDispatch(rm) {
			return svcDOS
		}
		if c.wide {
			c.push32(uint32(c.ip))
			c.ip = uint16(c.getWord(rm))
		} else {
			c.push(c.ip)
			c.ip = c.get16(rm)
		}
	case 3:
		if rm.reg {
			return c.fault(0xFF)
		}
		off := c.read16(rm.addr)
		seg := c.read16(rm.addr + 2)
		c.push(c.cs)
		c.push(c.ip)
		c.cs, c.ip = seg, off
	case 4:
		c.ip = c.get16(rm)
	case 5:
		if rm.reg {
			return c.fault(0xFF)
		}
		c.ip = c.read16(rm.addr)
		c.cs = c.read16(rm.addr + 2)
	case 6:
		if c.wide {
			c.push32(c.getWord(rm))
		} else {
			c.push(c.get16(rm))
		}
	default:
		return c.fault(0xFF)
	}
	return svcNone
}

func (c *cpu) stringIO(op byte) {
	for {
		n := uint16(1)
		switch op {
		case 0x6C:
			v := byte(0xFF)
			if c.inb != nil {
				v = c.inb(c.dx)
			}
			c.write8(uint32(c.es)<<4+uint32(c.di), v)
			c.stepDI(1)
		case 0x6D:
			if c.wide {
				var v uint32
				if c.inw != nil {
					v = uint32(c.inw(c.dx)) | uint32(c.inw(c.dx))<<16
				}
				c.write32(uint32(c.es)<<4+uint32(c.di), v)
				n = 4
			} else {
				var v uint16
				if c.inw != nil {
					v = c.inw(c.dx)
				}
				c.write16(uint32(c.es)<<4+uint32(c.di), v)
				n = 2
			}
			c.stepDI(n)
		case 0x6E:
			if c.outb != nil {
				c.outb(c.dx, c.read8(uint32(c.override())<<4+uint32(c.si)))
			}
			c.stepSI(1)
		case 0x6F:
			if c.wide {
				if c.outw != nil {
					v := c.read32(uint32(c.override())<<4 + uint32(c.si))
					c.outw(c.dx, uint16(v))
					c.outw(c.dx, uint16(v>>16))
				}
				n = 4
			} else if c.outw != nil {
				c.outw(c.dx, c.read16(uint32(c.override())<<4+uint32(c.si)))
				n = 2
			} else {
				n = 2
			}
			c.stepSI(n)
		}
		if c.rep == 0 {
			return
		}
		c.cx--
		if c.cx == 0 {
			return
		}
	}
}

func (c *cpu) stringOp(op byte) {
	for {
		switch op {
		case 0xA4:
			c.write8(uint32(c.es)<<4+uint32(c.di), c.read8(uint32(c.override())<<4+uint32(c.si)))
			c.stepIndex(1)
		case 0xA5:
			if c.wide {
				c.write32(c.indexAddr(7), c.read32(c.indexAddr(6)))
				c.stepIndex(4)
			} else {
				c.write16(uint32(c.es)<<4+uint32(c.di), c.read16(uint32(c.override())<<4+uint32(c.si)))
				c.stepIndex(2)
			}
		case 0xA6:
			a := c.read8(uint32(c.override())<<4 + uint32(c.si))
			b := c.read8(uint32(c.es)<<4 + uint32(c.di))
			c.sub(uint16(a), uint16(b), false, 8)
			c.stepIndex(1)
		case 0xA7:
			if c.wide {
				c.sub32(c.read32(c.indexAddr(6)), c.read32(c.indexAddr(7)), false)
				c.stepIndex(4)
			} else {
				a := c.read16(uint32(c.override())<<4 + uint32(c.si))
				b := c.read16(uint32(c.es)<<4 + uint32(c.di))
				c.sub(a, b, false, 16)
				c.stepIndex(2)
			}
		case 0xAA:
			c.write8(uint32(c.es)<<4+uint32(c.di), byte(c.ax))
			c.stepDI(1)
		case 0xAB:
			if c.wide {
				c.write32(c.indexAddr(7), c.reg32(0))
				c.stepDI(4)
			} else {
				c.write16(uint32(c.es)<<4+uint32(c.di), c.ax)
				c.stepDI(2)
			}
		case 0xAC:
			c.set8(0, c.read8(uint32(c.override())<<4+uint32(c.si)))
			c.stepSI(1)
		case 0xAD:
			if c.wide {
				c.set32(0, c.read32(c.indexAddr(6)))
				c.stepSI(4)
			} else {
				c.ax = c.read16(uint32(c.override())<<4 + uint32(c.si))
				c.stepSI(2)
			}
		case 0xAE:
			c.sub(uint16(byte(c.ax)), uint16(c.read8(uint32(c.es)<<4+uint32(c.di))), false, 8)
			c.stepDI(1)
		case 0xAF:
			if c.wide {
				c.sub32(c.reg32(0), c.read32(c.indexAddr(7)), false)
				c.stepDI(4)
			} else {
				c.sub(c.ax, c.read16(uint32(c.es)<<4+uint32(c.di)), false, 16)
				c.stepDI(2)
			}
		}
		if c.rep == 0 {
			return
		}
		c.cx--
		if c.cx == 0 {
			return
		}
		if (op == 0xA6 || op == 0xA7 || op == 0xAE || op == 0xAF) &&
			((c.rep == 2 && c.flags&flagZF == 0) || (c.rep == 1 && c.flags&flagZF != 0)) {
			return
		}
	}
}

func (c *cpu) stepIndex(n uint16) {
	c.stepSI(n)
	c.stepDI(n)
}
func (c *cpu) stepSI(n uint16) {
	if c.flags&flagDF != 0 {
		c.si -= n
	} else {
		c.si += n
	}
}
func (c *cpu) stepDI(n uint16) {
	if c.flags&flagDF != 0 {
		c.di -= n
	} else {
		c.di += n
	}
}

func (c *cpu) adjust(op byte) {
	_ = op
}
