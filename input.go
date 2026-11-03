package main

import "strconv"

var errTimeout = timeoutError{}

type timeoutError struct{}

func (timeoutError) Error() string   { return "timeout" }
func (timeoutError) Timeout() bool   { return true }
func (timeoutError) Temporary() bool { return true }

// mouseSample is one SyncTERM pixel report, already converted to a
// 0-based point and the buttons held after that report.
type mouseSample struct {
	x, y, btn int
}

// pushKey turns a door key into the PC-98 BIOS buffer and the 8251 scan queue.
func (m *machine) pushKey(k int) {
	scan, ascii, shift, ok := pc98Key(k)
	if !ok {
		return
	}
	m.shift = shift
	m.keys = append(m.keys, int(uint16(scan)<<8|uint16(ascii)))
	m.kbd = append(m.kbd, scan, scan|0x80)
	m.kickKeyboard()
}

func (m *machine) readDOSKey() (byte, bool) {
	if m.ext != 0 {
		al := m.ext
		m.ext = 0
		return al, true
	}
	if len(m.keys) == 0 {
		return 0, false
	}
	ax := uint16(m.keys[0])
	m.keys = m.keys[1:]
	al := byte(ax)
	if al == 0 {
		m.ext = byte(ax >> 8)
	}
	return al, true
}

func (m *machine) kickKeyboard() {
	if len(m.cpu.ram) < 4 || m.inIRQ || m.cpu.flags&flagIF == 0 {
		return
	}
	off := m.cpu.read16(9 * 4)
	seg := m.cpu.read16(9*4 + 2)
	if seg == 0xE800 && off == 0 || len(m.kbd) == 0 {
		return
	}
	m.inIRQ = true
	m.irqSP = m.cpu.sp
	m.cpu.intr(9)
}

func pc98Key(k int) (scan, ascii, shift byte, ok bool) {
	if k&keyShift != 0 {
		shift |= 0x01
		k &^= keyShift
	}
	if k&keyCtrl != 0 {
		shift |= 0x10
		k &^= keyCtrl
	}
	switch k {
	case keyUp:
		return 0x3A, 0, shift, true
	case keyLeft:
		return 0x3B, 0, shift, true
	case keyRight:
		return 0x3C, 0, shift, true
	case keyDown:
		return 0x3D, 0, shift, true
	case keyPgUp:
		return 0x36, 0, shift, true
	case keyPgDn:
		return 0x37, 0, shift, true
	case keyHome:
		return 0x3E, 0, shift, true
	case keyEnd:
		return 0x3F, 0, shift, true
	case keyDel:
		return 0x39, 0, shift, true
	case keyBack:
		return 0x0E, 8, shift, true
	case keyEnter:
		return 0x1C, 13, shift, true
	case keyEsc:
		return 0x00, 0x1B, shift, true
	}
	if k >= 1 && k <= 26 {
		ch := byte('A' + k - 1)
		s, found := scanASCII(ch)
		return s, byte(k), shift | 0x10, found
	}
	if k < 32 || k > 126 {
		return 0, 0, shift, false
	}
	ch := byte(k)
	if ch >= 'a' && ch <= 'z' {
		ch -= 32
	} else if ch >= 'A' && ch <= 'Z' {
		shift |= 0x01
	}
	s, found := scanASCII(ch)
	if !found {
		s, found = scanASCII(byte(k))
	}
	return s, byte(k), shift, found
}

func scanASCII(ch byte) (byte, bool) {
	switch ch {
	case '1', '!':
		return 0x01, true
	case '2', '"':
		return 0x02, true
	case '3', '#':
		return 0x03, true
	case '4', '$':
		return 0x04, true
	case '5', '%':
		return 0x05, true
	case '6', '&':
		return 0x06, true
	case '7', '\'':
		return 0x07, true
	case '8', '(':
		return 0x08, true
	case '9', ')':
		return 0x09, true
	case '0':
		return 0x0A, true
	case '-', '=':
		return 0x0B, true
	case '^', '~':
		return 0x0C, true
	case '\\', '|':
		return 0x0D, true
	case '@', '`':
		return 0x1A, true
	case '[', '{':
		return 0x1B, true
	case ';', '+':
		return 0x26, true
	case ':', '*':
		return 0x27, true
	case ']', '}':
		return 0x28, true
	case ',', '<':
		return 0x30, true
	case '.', '>':
		return 0x31, true
	case '/', '?':
		return 0x32, true
	case ' ':
		return 0x34, true
	}
	const letters = "QWERTYUIOPASDFGHJKLZXCVBNM"
	const codes = "\x10\x11\x12\x13\x14\x15\x16\x17\x18\x19\x1d\x1e\x1f\x20\x21\x22\x23\x24\x25\x29\x2a\x2b\x2c\x2d\x2e\x2f"
	for i := 0; i < len(letters); i++ {
		if ch == letters[i] {
			return codes[i], true
		}
	}
	return 0, false
}

func (m *machine) inPort(port uint16) byte {
	switch port {
	case 0x41:
		if len(m.kbd) == 0 {
			return 0
		}
		b := m.kbd[0]
		m.kbd = m.kbd[1:]
		return b
	case 0x43:
		st := byte(0x05)
		if len(m.kbd) > 0 {
			st |= 0x02
		}
		return st
	case 0x7FD9:
		return m.mousePortA()
	case 0x7FDB:
		return 0
	case 0x7FDD:
		return m.mouseC
	case 0x60, 0xA0:
		return 0x24
	default:
		return 0xFF
	}
}

func (m *machine) outPort(port uint16, v byte) {
	switch port {
	case 0x7FDD:
		m.mouseWriteC(v, 0xF0)
	case 0x7FDF:
		m.mouseControl(v)
	}
}

// mouseControl accepts the 8255 mode byte and the port C bit set/reset command.
func (m *machine) mouseControl(v byte) {
	if v&0x80 != 0 {
		m.mouseMode = v
		m.mouseC = 0x10
		m.mouseLive = false
		m.mouseHeld = false
		return
	}
	if m.mouseMode == 0 || m.mouseMode&0x08 != 0 {
		return
	}
	bit := (v >> 1) & 7
	var data byte
	if v&1 != 0 {
		data = 1 << bit
	}
	m.mouseWriteC(data, 1<<bit)
}

func (m *machine) mouseWriteC(data, mask byte) {
	mask &= 0xF0
	prev := m.mouseC
	m.mouseC = (m.mouseC &^ mask) | (data & mask)
	if prev&0x80 == 0 && m.mouseC&0x80 != 0 {
		m.mouseLX = int8(m.mdx)
		m.mouseLY = int8(m.mdy)
		m.mdx, m.mdy = 0, 0
		m.mouseHeld = true
	}
	if m.mouseC&0x80 == 0 {
		m.mouseHeld = false
	}
	m.mouseLive = m.mouseMode != 0 && m.mouseC&0x10 == 0
}

func (m *machine) mousePortA() byte {
	r := byte(0xE0)
	if m.mbtn&1 != 0 {
		r &^= 0x80
	}
	if m.mbtn&2 != 0 {
		r &^= 0x20
	}
	if m.mbtn&4 != 0 {
		r &^= 0x40
	}
	x, y := int8(m.mdx), int8(m.mdy)
	if m.mouseHeld {
		x, y = m.mouseLX, m.mouseLY
	}
	var nib byte
	switch (m.mouseC >> 5) & 3 {
	case 0:
		nib = byte(x) & 0x0F
	case 1:
		nib = byte(uint8(x)>>4) & 0x0F
	case 2:
		nib = byte(y) & 0x0F
	default:
		nib = byte(uint8(y)>>4) & 0x0F
	}
	return r | nib
}

func (m *machine) applyMouse(x, y, btn int) {
	if x < m.xmin {
		x = m.xmin
	}
	if x > m.xmax {
		x = m.xmax
	}
	if y < m.ymin {
		y = m.ymin
	}
	if y > m.ymax {
		y = m.ymax
	}
	if !m.mouseSeen {
		m.mouseSeen = true
		m.mx, m.my, m.mbtn = x, y, btn
		return
	}
	dx, dy := x-m.mx, y-m.my
	m.mdx += dx
	m.mdy += dy
	m.mickeyX += dx
	m.mickeyY += dy
	ev := 0
	if dx != 0 || dy != 0 {
		ev |= 1
	}
	if btn&1 != 0 && m.mbtn&1 == 0 {
		ev |= 1 << 1
	}
	if btn&1 == 0 && m.mbtn&1 != 0 {
		ev |= 1 << 2
	}
	if btn&2 != 0 && m.mbtn&2 == 0 {
		ev |= 1 << 3
	}
	if btn&2 == 0 && m.mbtn&2 != 0 {
		ev |= 1 << 4
	}
	if btn&4 != 0 && m.mbtn&4 == 0 {
		ev |= 1 << 5
	}
	if btn&4 == 0 && m.mbtn&4 != 0 {
		ev |= 1 << 6
	}
	m.mx, m.my, m.mbtn = x, y, btn
	if m.mouseHook && uint16(ev)&m.mouseMask != 0 {
		m.mousePend = uint16(ev) & m.mouseMask
	}
}

func (m *machine) drainMouse(keys keyboard) {
	src, ok := keys.(interface{ TakeMouse() []mouseSample })
	if !ok {
		return
	}
	for _, s := range src.TakeMouse() {
		m.applyMouse(s.x, s.y, s.btn)
	}
}

func (m *machine) mouseBIOS() bool {
	switch m.cpu.ax {
	case 0x0000, 0x0021:
		m.mouseSeen = false
		m.mx, m.my = 0, 0
		m.mdx, m.mdy = 0, 0
		m.mickeyX, m.mickeyY = 0, 0
		m.mbtn = 0
		m.mouseHook = false
		m.mousePend = 0
		m.xmin, m.xmax = 0, 639
		m.ymin, m.ymax = 0, 399
		m.cpu.ax = 0xFFFF
		m.cpu.bx = 3
	case 0x0001, 0x0002:
	case 0x0003:
		m.cpu.bx = uint16(m.mbtn)
		m.cpu.cx = uint16(m.mx)
		m.cpu.dx = uint16(m.my)
	case 0x0004:
		m.mx = int(m.cpu.cx)
		m.my = int(m.cpu.dx)
		m.mouseSeen = true
	case 0x0007:
		m.xmin, m.xmax = int(m.cpu.cx), int(m.cpu.dx)
	case 0x0008:
		m.ymin, m.ymax = int(m.cpu.cx), int(m.cpu.dx)
	case 0x000B:
		m.cpu.cx = uint16(int16(m.mickeyX))
		m.cpu.dx = uint16(int16(m.mickeyY))
		m.mickeyX, m.mickeyY = 0, 0
	case 0x000C:
		m.mouseMask = m.cpu.cx
		m.mouseSeg = m.cpu.es
		m.mouseOff = m.cpu.dx
		m.mouseHook = m.cpu.cx != 0
	default:
		return false
	}
	return true
}

func (m *machine) fireMouseHook() {
	m.cpu.ax = m.mousePend
	m.cpu.bx = uint16(m.mbtn)
	m.cpu.cx = uint16(m.mx)
	m.cpu.dx = uint16(m.my)
	m.cpu.si = uint16(int16(m.mickeyX))
	m.cpu.di = uint16(int16(m.mickeyY))
	m.mousePend = 0
	m.inIRQ = true
	m.irqSP = m.cpu.sp
	m.cpu.push(m.cpu.cs)
	m.cpu.push(m.cpu.ip)
	m.cpu.cs = m.mouseSeg
	m.cpu.ip = m.mouseOff
}

// parseSGR reads a SyncTERM SGR-pixel report. Coordinates are 1-based.
// Button bits in the returned mask are left=1, right=2, middle=4.
func parseSGR(params string, final byte, held int) (x, y, btn int, ok bool) {
	if len(params) == 0 || params[0] != '<' || (final != 'M' && final != 'm') {
		return 0, 0, held, false
	}
	parts := splitSemi(params[1:])
	if len(parts) < 3 {
		return 0, 0, held, false
	}
	pb, err1 := strconv.Atoi(parts[0])
	px, err2 := strconv.Atoi(parts[1])
	py, err3 := strconv.Atoi(parts[2])
	if err1 != nil || err2 != nil || err3 != nil {
		return 0, 0, held, false
	}
	x, y = px-1, py-1
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}
	button := pb & 3
	if pb&64 != 0 {
		return x, y, held, true
	}
	if button > 2 {
		return x, y, held, true
	}
	bit := []int{1, 4, 2}[button]
	if final == 'm' && pb&32 == 0 {
		held &^= bit
	} else if pb&32 == 0 {
		held |= bit
	}
	return x, y, held, true
}

func splitSemi(s string) []string {
	var out []string
	start := 0
	for i := 0; i <= len(s); i++ {
		if i == len(s) || s[i] == ';' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	return out
}

func mapByte(b byte, lastCR bool) (int, bool) {
	switch b {
	case 13:
		return keyEnter, true
	case 10:
		if lastCR {
			return keyNone, false
		}
		return keyEnter, false
	case 127, 8:
		return keyBack, false
	case 0:
		return keyNone, false
	default:
		return int(b), false
	}
}

func mapSeq(intro byte, params string, final byte) int {
	_ = intro
	mod := 0
	if i := lastSemi(params); i >= 0 {
		if n, err := strconv.Atoi(params[i+1:]); err == nil {
			mod = csiMod(n)
			params = params[:i]
		}
	}
	switch final {
	case 'A':
		return keyUp | mod
	case 'B':
		return keyDown | mod
	case 'C':
		return keyRight | mod
	case 'D':
		return keyLeft | mod
	case 'H':
		return keyHome | mod
	case 'F', 'K':
		return keyEnd | mod
	case 'V':
		return keyPgUp | mod
	case 'U':
		return keyPgDn | mod
	case '~':
		n := params
		switch n {
		case "1", "7":
			return keyHome | mod
		case "4", "8":
			return keyEnd | mod
		case "5":
			return keyPgUp | mod
		case "6":
			return keyPgDn | mod
		case "3":
			return keyDel | mod
		case "2":
			return keyHome | mod
		}
	}
	return keyNone
}

func lastSemi(s string) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == ';' {
			return i
		}
	}
	return -1
}

func csiMod(n int) int {
	if n <= 1 {
		return 0
	}
	n--
	var m int
	if n&1 != 0 {
		m |= keyShift
	}
	if n&4 != 0 {
		m |= keyCtrl
	}
	return m
}
