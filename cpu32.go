package main

func (c *cpu) reg32(i int) uint32 {
	return uint32(c.rhi[i&7])<<16 | uint32(c.reg16(i))
}

func (c *cpu) set32(i int, v uint32) {
	c.rhi[i&7] = uint16(v >> 16)
	c.set16(i, uint16(v))
}

func (c *cpu) read32(a uint32) uint32 {
	return uint32(c.read16(a)) | uint32(c.read16(a+2))<<16
}

func (c *cpu) write32(a uint32, v uint32) {
	c.write16(a, uint16(v))
	c.write16(a+2, uint16(v>>16))
}

func (c *cpu) fetch32() uint32 {
	lo := uint32(c.fetch16())
	return lo | uint32(c.fetch16())<<16
}

func (c *cpu) push32(v uint32) {
	c.sp -= 4
	c.write32(uint32(c.ss)<<4+uint32(c.sp), v)
}

func (c *cpu) pop32() uint32 {
	v := c.read32(uint32(c.ss)<<4 + uint32(c.sp))
	c.sp += 4
	return v
}

func (c *cpu) getWord(op operand) uint32 {
	if c.wide {
		if op.reg {
			return c.reg32(op.n)
		}
		return c.read32(op.addr)
	}
	return uint32(c.get16(op))
}

func (c *cpu) putWord(op operand, v uint32) {
	if c.wide {
		if op.reg {
			c.set32(op.n, v)
			return
		}
		c.write32(op.addr, v)
		return
	}
	c.put16(op, uint16(v))
}

func (c *cpu) szp32(v uint32) {
	c.flags &^= flagZF | flagSF | flagPF
	if v == 0 {
		c.flags |= flagZF
	}
	if v&0x80000000 != 0 {
		c.flags |= flagSF
	}
	if parity(byte(v)) {
		c.flags |= flagPF
	}
}

func (c *cpu) add32(a, b uint32, cin bool) uint32 {
	var carry uint64
	if cin {
		carry = 1
	}
	sum := uint64(a) + uint64(b) + carry
	r := uint32(sum)
	c.flags &^= flagCF | flagOF | flagAF
	if sum > 0xFFFFFFFF {
		c.flags |= flagCF
	}
	if (a^r)&0x10 != 0 {
		c.flags |= flagAF
	}
	if (^(a ^ b))&(a^r)&0x80000000 != 0 {
		c.flags |= flagOF
	}
	c.szp32(r)
	return r
}

func (c *cpu) sub32(a, b uint32, cin bool) uint32 {
	var borrow uint64
	if cin {
		borrow = 1
	}
	need := uint64(b) + borrow
	r := uint32(uint64(a) - need)
	c.flags &^= flagCF | flagOF | flagAF
	if uint64(a) < need {
		c.flags |= flagCF
	}
	if (a^b^r)&0x10 != 0 {
		c.flags |= flagAF
	}
	if (a^b)&(a^r)&0x80000000 != 0 {
		c.flags |= flagOF
	}
	c.szp32(r)
	return r
}

func (c *cpu) logic32(v uint32) uint32 {
	c.flags &^= flagCF | flagOF | flagAF
	c.szp32(v)
	return v
}

func (c *cpu) arith32(kind, form byte) {
	var a, b uint32
	var rm operand
	reg := 0
	switch form {
	case 1:
		reg, rm = c.parse()
		a, b = c.getWord(rm), c.reg32(reg)
	case 3:
		reg, rm = c.parse()
		a, b = c.reg32(reg), c.getWord(rm)
	default:
		a, b = c.reg32(0), c.fetch32()
	}
	cin := c.flags&flagCF != 0
	var r uint32
	store := true
	switch kind {
	case 0:
		r = c.add32(a, b, false)
	case 1:
		r = c.logic32(a | b)
	case 2:
		r = c.add32(a, b, cin)
	case 3:
		r = c.sub32(a, b, cin)
	case 4:
		r = c.logic32(a & b)
	case 5:
		r = c.sub32(a, b, false)
	case 6:
		r = c.logic32(a ^ b)
	default:
		c.sub32(a, b, false)
		store = false
	}
	if !store {
		return
	}
	switch form {
	case 1:
		c.putWord(rm, r)
	case 3:
		c.set32(reg, r)
	default:
		c.set32(0, r)
	}
}

func (c *cpu) aluImm32(sign bool) {
	reg, rm := c.parse()
	var imm uint32
	if sign {
		imm = uint32(int32(int8(c.fetch8())))
	} else {
		imm = c.fetch32()
	}
	a := c.getWord(rm)
	cin := c.flags&flagCF != 0
	var r uint32
	store := true
	switch reg {
	case 0:
		r = c.add32(a, imm, false)
	case 1:
		r = c.logic32(a | imm)
	case 2:
		r = c.add32(a, imm, cin)
	case 3:
		r = c.sub32(a, imm, cin)
	case 4:
		r = c.logic32(a & imm)
	case 5:
		r = c.sub32(a, imm, false)
	case 6:
		r = c.logic32(a ^ imm)
	default:
		c.sub32(a, imm, false)
		store = false
	}
	if store {
		c.putWord(rm, r)
	}
}

func (c *cpu) parse32(b byte) (reg int, op operand) {
	reg = int((b >> 3) & 7)
	mod, rm := b>>6, b&7
	if mod == 3 {
		return reg, operand{reg: true, n: int(rm)}
	}
	var off uint32
	seg := c.ds
	if rm == 4 {
		sib := c.fetch8()
		scale := sib >> 6
		idx := (sib >> 3) & 7
		base := sib & 7
		if idx != 4 {
			off = c.reg32(int(idx)) << scale
		}
		if mod == 0 && base == 5 {
			off += c.fetch32()
		} else {
			if base == 4 || base == 5 {
				seg = c.ss
			}
			off += c.reg32(int(base))
			if mod == 1 {
				off += uint32(int8(c.fetch8()))
			} else if mod == 2 {
				off += c.fetch32()
			}
		}
	} else if mod == 0 && rm == 5 {
		off = c.fetch32()
	} else {
		if rm == 5 {
			seg = c.ss
		}
		off = c.reg32(int(rm))
		if mod == 1 {
			off += uint32(int8(c.fetch8()))
		} else if mod == 2 {
			off += c.fetch32()
		}
	}
	if c.hasSeg {
		seg = c.seg
	}
	off16 := uint16(off)
	return reg, operand{addr: uint32(seg)<<4 + uint32(off16), off: off16}
}

func (c *cpu) op0F() int {
	op := c.fetch8()
	switch {
	case op >= 0x80 && op <= 0x8F:
		rel := int16(c.fetch16())
		if c.wide {
			c.ip -= 2
			rel32 := int32(c.fetch32())
			if c.cond(op & 0xF) {
				c.ip = uint16(int32(c.ip) + rel32)
			}
			return svcNone
		}
		if c.cond(op & 0xF) {
			c.ip += uint16(rel)
		}
		return svcNone
	case op >= 0x90 && op <= 0x9F:
		_, rm := c.parse()
		v := byte(0)
		if c.cond(op & 0xF) {
			v = 1
		}
		c.put8(rm, v)
		return svcNone
	case op == 0xB6 || op == 0xBE:
		reg, rm := c.parse()
		v := uint32(c.get8(rm))
		if op == 0xBE && v&0x80 != 0 {
			v |= 0xFFFFFF00
		}
		if c.wide {
			c.set32(reg, v)
		} else {
			c.set16(reg, uint16(v))
		}
		return svcNone
	case op == 0xB7 || op == 0xBF:
		reg, rm := c.parse()
		v := uint32(c.get16(rm))
		if op == 0xBF && v&0x8000 != 0 {
			v |= 0xFFFF0000
		}
		if c.wide {
			c.set32(reg, v)
		} else {
			c.set16(reg, uint16(v))
		}
		return svcNone
	case op == 0xAF:
		reg, rm := c.parse()
		if c.wide {
			r := int64(int32(c.reg32(reg))) * int64(int32(c.getWord(rm)))
			c.set32(reg, uint32(r))
			c.flags &^= flagCF | flagOF
			if int64(int32(r)) != r {
				c.flags |= flagCF | flagOF
			}
		} else {
			c.set16(reg, c.mul(c.reg16(reg), c.get16(rm), true))
		}
		return svcNone
	case op == 0x00 || op == 0x01:
		reg, rm := c.parse()
		if reg <= 1 || (op == 0x01 && reg == 4) {
			if rm.reg {
				c.set16(rm.n, 0)
			} else if reg == 0 && op == 0x01 {
				for i := uint32(0); i < 6; i++ {
					c.write8(rm.addr+i, 0)
				}
			} else {
				c.write16(rm.addr, 0)
			}
		}
		return svcNone
	case op == 0x1F || op == 0x0D || op == 0x18:
		c.parse()
		return svcNone
	case op == 0x31:
		c.set32(0, 0)
		c.set32(2, 0)
		return svcNone
	case op == 0xA2:
		c.set32(0, 0)
		c.set32(1, 0)
		c.set32(2, 0)
		c.set32(3, 0)
		return svcNone
	case op == 0xA0 || op == 0xA8:
		if c.wide {
			c.push32(0)
		} else {
			c.push(0)
		}
		return svcNone
	case op == 0xA1 || op == 0xA9:
		if c.wide {
			c.pop32()
		} else {
			c.pop()
		}
		return svcNone
	default:
		c.lastFault = "unimplemented opcode 0F" + hexByte(op)
		return c.fault(op)
	}
}

func (c *cpu) moffs() uint32 {
	if c.ad32 {
		return uint32(uint16(c.fetch32()))
	}
	return uint32(c.fetch16())
}

func (c *cpu) indexAddr(which int) uint32 {
	seg := c.ds
	if which == 7 {
		seg = c.es
	}
	if c.hasSeg && which == 6 {
		seg = c.seg
	}
	var off uint16
	if which == 6 {
		off = c.si
	} else {
		off = c.di
	}
	return uint32(seg)<<4 + uint32(off)
}
