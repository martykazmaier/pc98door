package main

import "testing"

func TestPC98KeysReachBIOSAndUART(t *testing.T) {
	m := &machine{}
	m.cpu.ram = make([]byte, 1<<20)
	m.cpu.flags = 2 | flagIF
	m.cpu.write16(9*4+2, 0xE800)
	m.pushKey('A')
	m.pushKey(keyLeft)
	if len(m.keys) != 2 || m.keys[0] != 0x1D41 || m.keys[1] != 0x3B00 {
		t.Fatalf("bios queue %#x", m.keys)
	}
	if m.inPort(0x43)&0x02 == 0 {
		t.Fatal("keyboard status did not show a byte")
	}
	if got := m.inPort(0x41); got != 0x1D {
		t.Fatalf("make %02X", got)
	}
	if got := m.inPort(0x41); got != 0x9D {
		t.Fatalf("break %02X", got)
	}
	m.cpu.ss, m.cpu.sp = 0x1000, 0x100
	m.cpu.ax = 0
	m.keyBIOS()
	if m.cpu.ax != 0x1D41 {
		t.Fatalf("int 18h AX %04X", m.cpu.ax)
	}
	if mapSeq('[', "", 'C') != keyRight || mapSeq('[', "", 'D') != keyLeft {
		t.Fatal("cursor left and right were dropped")
	}
	if mapSeq('[', "1;5", 'A') != keyUp|keyCtrl {
		t.Fatal("ctrl+up was dropped")
	}
}

func TestBusMouseAndINT33(t *testing.T) {
	m := &machine{xmin: 0, xmax: 639, ymin: 0, ymax: 399}
	m.applyMouse(0, 0, 0)
	m.applyMouse(16, 32, 1)
	m.mouseControl(0x93)
	m.mouseControl(0x0F)
	if got := m.inPort(0x7FD9); got != 0x60 {
		t.Fatalf("X low %02X", got)
	}
	m.mouseControl(0x0B)
	if got := m.inPort(0x7FD9); got != 0x61 {
		t.Fatalf("X high %02X", got)
	}
	m.mouseControl(0x0A)
	m.mouseControl(0x0D)
	if got := m.inPort(0x7FD9); got != 0x60 {
		t.Fatalf("Y low %02X", got)
	}
	m.mouseControl(0x0B)
	if got := m.inPort(0x7FD9); got != 0x62 {
		t.Fatalf("Y high %02X", got)
	}
	m.cpu.ax = 3
	if !m.mouseBIOS() || m.cpu.bx != 1 || m.cpu.cx != 16 || m.cpu.dx != 32 {
		t.Fatalf("int 33h bx=%04X cx=%04X dx=%04X", m.cpu.bx, m.cpu.cx, m.cpu.dx)
	}
}
