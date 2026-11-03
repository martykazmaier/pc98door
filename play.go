package main

import (
	"fmt"
	"io"
	"time"
)

const (
	biosBase  = 0xE8000
	biosDisk  = biosBase + 0x10
	biosKey   = biosBase + 0x20
	biosMem   = biosBase + 0x30
	biosEquip = biosBase + 0x40
	biosDOS   = biosBase + 0x50
)

type machine struct {
	cpu       cpu
	disk      *hdiImage
	font      []byte
	reads     int
	keys      []int
	psp       uint16
	drive     byte
	dtaSeg    uint16
	dtaOff    uint16
	row       int
	col       int
	con       bool
	files     []*dosFile
	nextSeg   uint16
	blocks    map[uint16]uint16
	vol       *volume
	cwd       string
	shell     bool
	vsync     int
	inIRQ     bool
	irqSP     uint16
	tickOn    bool
	tickSeg   uint16
	tickOff   uint16
	tickEvery int
	dosLog    []string
	child     *execChild
	uptime    int
	path      string
	ownDOS    bool
	kbd       []byte
	shift     byte
	ext       byte
	holdKey   bool
	mouseC    byte
	mouseMode byte
	mouseHeld bool
	mouseLive bool
	mouseSeen bool
	mouseHook bool
	mousePend uint16
	mouseMask uint16
	mouseSeg  uint16
	mouseOff  uint16
	mx, my    int
	mbtn      int
	mdx, mdy  int
	mickeyX   int
	mickeyY   int
	mouseLX   int8
	mouseLY   int8
	xmin      int
	xmax      int
	ymin      int
	ymax      int
}

func bootMachine(disk *hdiImage) *machine {
	m := &machine{disk: disk}
	m.cpu.ram = make([]byte, 1<<20)
	m.cpu.flags = 2 | flagIF
	m.cpu.cs = 0x1FE0
	m.cpu.ds = 0
	m.cpu.es = 0x1FE0
	m.cpu.ss = 0x1000
	m.cpu.sp = 0x800
	m.cpu.ram[0x055C] = 0x01
	m.cpu.ram[0x055D] = 0x01
	m.cpu.ram[0x584] = 0x80
	buf := make([]byte, 8192)
	_, _ = disk.ReadAt(buf, 0)
	copy(m.cpu.ram[0x1FE00:], buf)
	for i := 0; i < 256; i++ {
		m.cpu.write16(uint32(i*4), 0)
		m.cpu.write16(uint32(i*4+2), 0xE800)
	}
	m.cpu.ram[biosBase] = 0xCF
	m.cpu.ram[biosDisk] = 0xF4
	m.cpu.ram[biosKey] = 0xF4
	m.cpu.ram[biosMem] = 0xF4
	m.cpu.ram[biosEquip] = 0xF4
	m.cpu.write16(0x1B*4, 0x10)
	m.cpu.write16(0x1B*4+2, 0xE800)
	m.cpu.write16(0x18*4, 0x20)
	m.cpu.write16(0x18*4+2, 0xE800)
	m.cpu.write16(0x12*4, 0x30)
	m.cpu.write16(0x12*4+2, 0xE800)
	m.cpu.write16(0x11*4, 0x40)
	m.cpu.write16(0x11*4+2, 0xE800)
	m.xmin, m.xmax = 0, 639
	m.ymin, m.ymax = 0, 399
	m.mouseC = 0x10
	m.cpu.inb = m.inPort
	m.cpu.outb = m.outPort
	m.cpu.bios = m.biosCall
	m.cpu.inw = func(port uint16) uint16 { return uint16(m.cpu.inb(port)) | 0xFF00 }
	m.cpu.outw = func(uint16, uint16) {}
	return m
}

// biosCall answers firmware services the guest reaches with INT.
// INT 1Ch AH=02 arms the interval timer with a routine at CS:BX.
func (m *machine) biosCall(n byte) bool {
	if n == 0x33 {
		return m.mouseBIOS()
	}
	if n != 0x1C || byte(m.cpu.ax>>8) != 0x02 {
		return false
	}
	m.tickOn = true
	m.tickSeg = m.cpu.cs
	m.tickOff = m.cpu.bx
	m.tickEvery = int(m.cpu.cx)
	if m.tickEvery < 1 {
		m.tickEvery = 1
	}
	return true
}

func (m *machine) service(svc int) {
	switch svc {
	case svcDisk:
		m.diskBIOS()
	case svcKey:
		m.keyBIOS()
	case svcMem:
		m.cpu.ax = 640
		m.cpu.iret()
	case svcEquip:
		m.cpu.ax = 0x0021
		m.cpu.iret()
	case svcDOS:
		if m.ownDOS {
			m.dosRegs()
		} else {
			m.dosCall()
		}
	}
}

func (m *machine) diskBIOS() {
	ah := byte(m.cpu.ax >> 8)
	cf := false
	switch ah {
	case 0x84:
		m.cpu.bx = uint16(m.disk.SectorSize)
		m.cpu.cx = uint16(m.disk.Cylinders)
		m.cpu.dx = uint16(m.disk.Heads)<<8 | uint16(m.disk.Sectors)
		m.cpu.ax = m.cpu.ax & 0x00FF
	case 0x06, 0x05:
		cf = m.transfer(ah == 0x05)
	case 0x07, 0x04, 0x03, 0x01, 0x8E, 0x0E, 0x00, 0x0A, 0x0F, 0x14, 0x24:
		m.cpu.ax = m.cpu.ax & 0x00FF
		if ah == 0x14 {
			// BL is the device type. 00h is a hard disk. 84h is a
			// 5-inch MO and sends the loader down the SCSI path.
			m.cpu.bx = 0
		}
	default:
		cf = true
		m.cpu.ax = 0x4000 | uint16(ah)<<8
	}
	m.cpu.setCarryStacked(cf)
	m.cpu.iret()
}

func (m *machine) transfer(write bool) bool {
	count := int(m.cpu.bx)
	if count == 0 {
		return false
	}
	lba, ok := m.sectorIndex()
	if !ok {
		m.cpu.ax = 0x4000 | m.cpu.ax&0xFF00
		return true
	}
	off := lba * int64(m.disk.SectorSize)
	buf := make([]byte, count)
	phys := uint32(m.cpu.es)<<4 + uint32(m.cpu.bp)
	if write {
		for i := 0; i < count; i++ {
			buf[i] = m.cpu.read8(phys + uint32(i))
		}
		if _, err := m.disk.f.WriteAt(buf, m.disk.Header+off); err != nil {
			m.cpu.ax = 0x4000 | m.cpu.ax&0xFF00
			return true
		}
	} else {
		if _, err := m.disk.ReadAt(buf, off); err != nil {
			m.cpu.ax = 0x4000 | m.cpu.ax&0xFF00
			return true
		}
		for i := 0; i < count; i++ {
			m.cpu.write8(phys+uint32(i), buf[i])
		}
		m.reads++
	}
	m.cpu.ax = m.cpu.ax & 0x00FF
	return false
}

// sectorIndex decodes the PC-98 INT 1Bh address. AL 0xh and 2xh are LBA
// (CX is the low word, DX the high word). AL 8xh and Axh are CHS.
func (m *machine) sectorIndex() (int64, bool) {
	switch byte(m.cpu.ax) & 0xF0 {
	case 0x00, 0x20:
		return int64(m.cpu.cx) | int64(m.cpu.dx)<<16, true
	default:
		cyl := int(m.cpu.cx)
		head := int(m.cpu.dx >> 8)
		sec := int(m.cpu.dx & 0xFF)
		if cyl < 0 || head < 0 || head >= m.disk.Heads || sec < 0 || sec >= m.disk.Sectors {
			return 0, false
		}
		return int64((cyl*m.disk.Heads+head)*m.disk.Sectors + sec), true
	}
}

func (m *machine) keyBIOS() {
	ah := byte(m.cpu.ax >> 8)
	switch ah {
	case 0x00:
		if len(m.keys) == 0 {
			return
		}
		k := m.keys[0]
		m.keys = m.keys[1:]
		m.cpu.ax = uint16(k)
		m.cpu.iret()
		return
	case 0x01:
		f := m.cpu.read16(uint32(m.cpu.ss)<<4 + uint32(m.cpu.sp) + 4)
		if len(m.keys) == 0 {
			f |= flagZF
		} else {
			f &^= flagZF
			m.cpu.ax = uint16(m.keys[0])
		}
		m.cpu.write16(uint32(m.cpu.ss)<<4+uint32(m.cpu.sp)+4, f|2)
	case 0x02:
		m.cpu.ax = uint16(m.shift)
	default:
	}
	m.cpu.iret()
}

func (m *machine) textPage() string {
	var b []byte
	b = append(b, "\x1b[0m\x1b[H"...)
	for row := 0; row < 25; row++ {
		for col := 0; col < 80; col++ {
			ch := m.cpu.ram[0xA0000+row*160+col*2]
			if ch < 32 || ch == 127 {
				ch = ' '
			}
			b = append(b, ch)
		}
		b = append(b, '\r', '\n')
	}
	return string(b)
}

func play(w io.Writer, keys keyboard, disk *hdiImage, font []byte, title string) error {
	m := bootMachine(disk)
	m.font = font
	useJXL := false
	if j, ok := w.(jxlTerm); ok && j.JXL() {
		useJXL = true
	}
	if _, err := w.Write([]byte("\x1b[0m\x1b[2J\x1b[H")); err != nil {
		return err
	}
	if useJXL {
		if _, err := w.Write(jxlQuery()); err != nil {
			return err
		}
		if _, err := w.Write([]byte("\x1b[?1003h\x1b[?1006h\x1b[?1016h")); err != nil {
			return err
		}
		defer func() { _, _ = w.Write([]byte("\x1b[?1003l\x1b[?1006l\x1b[?1016l")) }()
		_, _, _ = keys.WaitKey(150 * time.Millisecond)
	} else if _, err := w.Write([]byte(utf8ToCP437(title) + "\r\n")); err != nil {
		return err
	}
	sum := ^uint64(0)
	next := time.Time{}
	var waiting, paused bool
	for {
		m.drainMouse(keys)
		if !waiting && !paused && !m.cpu.halted {
			deadline := time.Now().Add(30 * time.Millisecond)
			for n := 0; n < 200000 && !m.cpu.halted && !waiting; n++ {
				if n&8191 == 0 && time.Now().After(deadline) {
					break
				}
				svc := m.step()
				if svc == svcPause {
					paused = true
					break
				}
				if svc != svcNone {
					m.service(svc)
				}
				if m.holdKey || (svc == svcKey && byte(m.cpu.ax>>8) == 0 && len(m.keys) == 0) {
					waiting = true
					break
				}
			}
		}
		if time.Now().After(next) {
			if err := m.publish(w, useJXL, &sum); err != nil {
				return err
			}
			next = time.Now().Add(100 * time.Millisecond)
		}
		wait := time.Duration(0)
		if waiting || paused || m.cpu.halted {
			wait = -1
		}
		k, ok, err := keys.WaitKey(wait)
		if err != nil {
			return err
		}
		m.drainMouse(keys)
		if !ok {
			continue
		}
		if k == keyCtrlQ {
			return nil
		}
		m.pushKey(k)
		if paused {
			paused = false
			continue
		}
		if waiting {
			waiting = false
			if !m.holdKey {
				m.keyBIOS()
			}
		}
	}
}

func (m *machine) publish(w io.Writer, useJXL bool, sum *uint64) error {
	if useJXL {
		h := m.pictureSum()
		if h == *sum {
			return nil
		}
		blob, err := m.jxlBlob()
		if err != nil {
			return err
		}
		if _, err := w.Write(blob); err != nil {
			return err
		}
		*sum = h
		return nil
	}
	page := m.textPage()
	if _, err := w.Write([]byte(page)); err != nil {
		return err
	}
	if m.cpu.lastFault != "" {
		fmt.Fprintf(w, "\r\n%s\r\n", m.cpu.lastFault)
	}
	return nil
}
