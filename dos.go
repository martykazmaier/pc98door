package main

import (
	"fmt"
	"strings"
)

// dosFile is an open file in the DOS high-level handler. Handles 0, 1, and 2
// are the console. A program's own files start at 5.
type dosFile struct {
	data []byte
	pos  int
}

// execChild is a program started by INT 21h AH=4Bh whose return still belongs
// to the caller. Queue is the rest of a batch file, one line at a time.
type execChild struct {
	prev                *execChild
	name                string
	psp                 uint16
	dosCS, epiIP, dosSS uint16
	frameSS, frameSP    uint16
	seg, paras          uint16
	queue               []string
	steps               int
	resident            bool
}

func (m *machine) step() int {
	if m.child != nil {
		m.child.steps++
	}
	// PC-98 vertical blank is INT 0Ah. The game waits on the counter that
	// handler advances, so the door delivers the tick between instructions.
	if m.inIRQ && m.cpu.sp >= m.irqSP {
		m.inIRQ = false
	}
	if !m.inIRQ && m.mousePend != 0 && m.cpu.flags&flagIF != 0 {
		m.fireMouseHook()
	}
	if m.child != nil && !m.inIRQ && m.cpu.flags&flagIF != 0 {
		m.vsync++
		every := 1000
		if m.tickOn && m.tickEvery > 0 {
			every = 400
		}
		if m.vsync >= every {
			m.vsync = 0
			m.inIRQ = true
			m.irqSP = m.cpu.sp
			if m.tickOn {
				m.cpu.push(m.cpu.flags)
				m.cpu.flags &^= flagIF
				m.cpu.push(m.cpu.cs)
				m.cpu.push(m.cpu.ip)
				m.cpu.cs = m.tickSeg
				m.cpu.ip = m.tickOff
			} else {
				m.cpu.intr(0x08)
			}
		}
	}
	svc := m.cpu.step()
	if m.cpu.halted && m.cpu.lastFault == "instruction needs a 386" && m.child != nil && m.child.steps < 64 {
		m.note("skip 386 " + m.child.name)
		m.finishChild(0, true, false)
	}
	if !m.shell && m.child == nil && !m.cpu.halted {
		m.uptime++
		if m.uptime > 400000 && m.uptime%1000 == 0 && m.bannerUp() {
			m.shell = true
			m.ownDOS = true
			m.cpu.ram[biosDOS] = 0xF4
			m.cpu.write16(0x21*4, biosDOS-biosBase)
			m.cpu.write16(0x21*4+2, 0xE800)
			m.cpu.write16(0x20*4, biosDOS-biosBase)
			m.cpu.write16(0x20*4+2, 0xE800)
			m.note("force shell")
			m.launch(0, `\COMMAND.COM`, "")
		}
	}
	return svc
}

func (m *machine) note(s string) {
	if len(m.dosLog) < 60 {
		m.dosLog = append(m.dosLog, s)
	}
}

// dosCall answers the service the DOS dispatcher was about to call.
// Execution continues at the dispatcher's own return sequence, which copies
// AL and the saved flags back to the program that issued INT 21h.
func (m *machine) dosCall() {
	dos := uint32(m.cpu.ss) << 4
	usp := m.cpu.read16(dos + 0x0584)
	uss := m.cpu.read16(dos + 0x0586)
	frame := (uint32(uss)<<4 + uint32(usp)) & 0xFFFFF
	ax := m.cpu.read16(frame)
	ah := byte(ax >> 8)
	retCS := m.cpu.read16(frame + 20)
	retIP := m.cpu.read16(frame + 18)
	at := (uint32(retCS)<<4 + uint32(retIP)) & 0xFFFFF
	b0, b1 := byte(0), byte(0)
	if retIP >= 2 {
		b0, b1 = m.cpu.read8(at-2), m.cpu.read8(at-1)
	}
	// INT 2Fh shares this dispatcher because the startup image pointed every
	// vector from 21h up at it. An unanswered multiplex call returns as it came.
	if b0 == 0xCD && b1 == 0x2F {
		m.note(fmt.Sprintf("2F %04X at %04X:%04X", ax, retCS, retIP))
		m.cpu.ax = ax
		return
	}
	if retIP < 2 || b0 != 0xCD || (b1 != 0x21 && b1 != 0x20) {
		m.note(fmt.Sprintf("int %02X%02X ah=%02X ret %04X:%04X", b0, b1, ah, retCS, retIP))
		m.cpu.ax = ax
		return
	}
	dx := m.cpu.read16(frame + 6)
	ds := m.cpu.read16(frame + 14)
	bx := m.cpu.read16(frame + 2)
	cx := m.cpu.read16(frame + 4)
	es := m.cpu.read16(frame + 16)
	m.note(fmt.Sprintf("%02X %04X:%04X", ah, retCS, retIP))
	switch ah {
	case 0x02:
		m.putChar(byte(m.cpu.read16(frame + 6)))
	case 0x01, 0x07, 0x08:
		al, ok := m.readDOSKey()
		if !ok {
			m.cpu.ip -= 4
			m.holdKey = true
			return
		}
		m.holdKey = false
		m.setAL(frame, al)
		if ah == 0x01 && al >= 32 {
			m.putChar(al)
		}
	case 0x06:
		dl := byte(dx)
		if dl == 0xFF {
			if al, ok := m.readDOSKey(); ok {
				m.holdKey = false
				m.setAL(frame, al)
				m.setFlag(frame, flagZF, false)
			} else {
				m.setAL(frame, 0)
				m.setFlag(frame, flagZF, true)
			}
		} else {
			m.putChar(dl)
		}
	case 0x09:
		m.writeDollar(ds, dx)
	case 0x0B:
		if m.ext != 0 || len(m.keys) > 0 {
			m.setAL(frame, 0xFF)
		} else {
			m.setAL(frame, 0)
		}
	case 0x0C:
		if !m.holdKey {
			m.keys = nil
			m.ext = 0
		}
		switch byte(ax) {
		case 0x01, 0x06, 0x07, 0x08:
			al, ok := m.readDOSKey()
			if !ok {
				m.cpu.ip -= 4
				m.holdKey = true
				return
			}
			m.holdKey = false
			m.setAL(frame, al)
			if byte(ax) == 0x01 && al >= 32 {
				m.putChar(al)
			}
		}
	case 0x0D:
	case 0x0E:
		m.drive = byte(dx)
		m.setAL(frame, 16)
		// The disk's own startup continues into a PC bootstrap that copies
		// over the DOS services. The shell's startup file is already on the
		// disk, so run that program while the services are still intact.
		if !m.shell {
			m.shell = true
			m.setCF(frame, false)
			if m.launch(frame, `\COMMAND.COM`, "") == 1 {
				return
			}
		}
	case 0x19:
		m.setAL(frame, m.drive)
	case 0x1A:
		m.dtaSeg, m.dtaOff = ds, dx
	case 0x25:
		vec := uint32(ax&0xFF) * 4
		m.cpu.write16(vec, dx)
		m.cpu.write16(vec+2, ds)
	case 0x2A:
		m.setAX(frame, uint16((2026-1980)<<9|(11<<5)|2))
		m.setReg(frame, 4, 0)
	case 0x2C:
		m.setAX(frame, 0)
		m.setReg(frame, 6, 0)
	case 0x30:
		m.setAX(frame, 0x1406)
		m.setReg(frame, 2, 0)
	case 0x31:
		if m.child != nil {
			m.keepResident(dx)
			m.finishChild(byte(ax), false, true)
			return
		}
	case 0x33:
		m.setAL(frame, 0)
		m.setReg(frame, 6, 0)
	case 0x35:
		vec := uint32(ax&0xFF) * 4
		m.setReg(frame, 2, m.cpu.read16(vec))
		m.setReg(frame, 16, m.cpu.read16(vec+2))
	case 0x3B:
		if !m.chdir(m.abs(m.asciiZ(ds, dx))) {
			m.fail(frame, 3)
			return
		}
	case 0x3C, 0x3D, 0x6C:
		path := m.asciiZ(ds, dx)
		if ah == 0x6C {
			path = m.asciiZ(ds, m.cpu.read16(frame+8))
		}
		h, ok := m.openFile(m.abs(path))
		if !ok {
			m.fail(frame, 2)
			return
		}
		m.setAX(frame, uint16(h))
	case 0x3E:
		if int(bx) < len(m.files) && bx >= 5 {
			m.files[bx] = nil
		}
	case 0x3F:
		n := m.readFile(int(bx), ds, dx, int(cx))
		m.setAX(frame, uint16(n))
	case 0x40:
		n := m.writeFile(int(bx), ds, dx, int(cx))
		m.setAX(frame, uint16(n))
	case 0x42:
		pos, ok := m.seekFile(int(bx), byte(ax), int32(cx)<<16|int32(uint16(dx)))
		if !ok {
			m.fail(frame, 6)
			return
		}
		m.setAX(frame, uint16(pos))
		m.setReg(frame, 6, uint16(pos>>16))
	case 0x47:
		s := strings.TrimPrefix(m.cwd, `\`)
		m.writeZ(ds, m.cpu.read16(frame+8), s)
	case 0x48:
		seg, ok := m.alloc(bx, false)
		if !ok {
			m.setReg(frame, 2, m.freeParas())
			m.fail(frame, 8)
			return
		}
		m.setAX(frame, seg)
	case 0x49:
	case 0x4A:
		if !m.resize(es, bx) {
			m.setReg(frame, 2, m.freeParas())
			m.fail(frame, 8)
			return
		}
	case 0x4B:
		switch m.launch(frame, m.abs(m.asciiZ(ds, dx)), execTail(m, es, bx)) {
		case 1:
			return
		case 2:
		default:
			m.fail(frame, 2)
			return
		}
	case 0x4C, 0x00:
		if m.child != nil {
			m.finishChild(byte(ax), false, true)
			return
		}
		m.cpu.halted = true
		m.cpu.lastFault = "dos exit"
		return
	case 0x50:
		m.psp = bx
	case 0x51, 0x62:
		m.setReg(frame, 2, m.psp)
	default:
		m.fail(frame, 1)
		return
	}
	m.setCF(frame, false)
}

func (m *machine) fail(frame uint32, err byte) {
	m.setAX(frame, uint16(err))
	m.setCF(frame, true)
}

func (m *machine) setAL(frame uint32, al byte) {
	ax := m.cpu.read16(frame)
	m.setAX(frame, ax&0xFF00|uint16(al))
}

func (m *machine) setAX(frame uint32, ax uint16) {
	m.cpu.write16(frame, ax)
	m.cpu.ax = m.cpu.ax&0xFF00 | ax&0xFF
}

func (m *machine) setReg(frame uint32, off int, v uint16) {
	m.cpu.write16(frame+uint32(off), v)
}

func (m *machine) setCF(frame uint32, on bool) {
	m.setFlag(frame, flagCF, on)
}

func (m *machine) setFlag(frame uint32, bit uint16, on bool) {
	f := m.cpu.read16(frame + 22)
	if on {
		f |= bit
	} else {
		f &^= bit
	}
	m.cpu.write16(frame+22, f|2)
}

func (m *machine) asciiZ(seg, off uint16) string {
	var b []byte
	a := uint32(seg)<<4 + uint32(off)
	for i := 0; i < 128; i++ {
		c := m.cpu.read8(a + uint32(i))
		if c == 0 {
			break
		}
		b = append(b, c)
	}
	return string(b)
}

func (m *machine) writeZ(seg, off uint16, s string) {
	a := uint32(seg)<<4 + uint32(off)
	for i := 0; i < len(s); i++ {
		m.cpu.write8(a+uint32(i), s[i])
	}
	m.cpu.write8(a+uint32(len(s)), 0)
}

func (m *machine) writeDollar(seg, off uint16) {
	a := uint32(seg)<<4 + uint32(off)
	for i := 0; i < 256; i++ {
		c := m.cpu.read8(a + uint32(i))
		if c == '$' {
			return
		}
		m.putChar(c)
	}
}

func (m *machine) putChar(ch byte) {
	if !m.con {
		m.row = 0
		for r := 0; r < 25; r++ {
			blank := true
			for c := 0; c < 80; c++ {
				if m.cpu.ram[0xA0000+r*160+c*2] != 0 && m.cpu.ram[0xA0000+r*160+c*2] != ' ' {
					blank = false
					break
				}
			}
			if blank {
				m.row = r
				break
			}
			m.row = r
		}
		m.col = 0
		m.con = true
	}
	switch ch {
	case 13:
		m.col = 0
		return
	case 10:
		m.col = 0
		m.row++
	case 8:
		if m.col > 0 {
			m.col--
		}
		return
	default:
		if ch < 32 {
			return
		}
		off := 0xA0000 + m.row*160 + m.col*2
		m.cpu.ram[off] = ch
		m.cpu.ram[off+1] = 0xE1
		m.col++
		if m.col >= 80 {
			m.col = 0
			m.row++
		}
	}
	if m.row >= 25 {
		copy(m.cpu.ram[0xA0000:0xA0000+24*160], m.cpu.ram[0xA0000+160:0xA0000+25*160])
		for i := 0; i < 160; i += 2 {
			m.cpu.ram[0xA0000+24*160+i] = ' '
			m.cpu.ram[0xA0000+24*160+i+1] = 0xE1
		}
		m.row = 24
	}
}

func (m *machine) alloc(paras uint16, all bool) (uint16, bool) {
	if m.nextSeg == 0 {
		m.nextSeg = 0x2000
	}
	if paras == 0 {
		paras = 1
	}
	if m.nextSeg >= 0xA000 {
		return 0, false
	}
	avail := 0xA000 - m.nextSeg
	if all || paras > avail {
		paras = avail
	}
	if paras == 0 {
		return 0, false
	}
	seg := m.nextSeg
	m.nextSeg += paras
	if m.blocks == nil {
		m.blocks = map[uint16]uint16{}
	}
	m.blocks[seg] = paras
	return seg, true
}

func (m *machine) freeParas() uint16 {
	if m.nextSeg == 0 {
		m.nextSeg = 0x2000
	}
	if m.nextSeg >= 0xA000 {
		return 0
	}
	return 0xA000 - m.nextSeg
}

func (m *machine) resize(seg, paras uint16) bool {
	if paras == 0 || seg+paras > 0xA000 {
		return false
	}
	if m.blocks == nil {
		return true
	}
	cur, ok := m.blocks[seg]
	if !ok {
		return true
	}
	if seg+cur == m.nextSeg {
		m.nextSeg = seg + paras
	} else if paras > cur {
		return false
	}
	m.blocks[seg] = paras
	return true
}

func execTail(m *machine, es, bx uint16) string {
	pb := uint32(es)<<4 + uint32(bx)
	off := m.cpu.read16(pb + 2)
	seg := m.cpu.read16(pb + 4)
	n := int(m.cpu.read8(uint32(seg)<<4 + uint32(off)))
	if n > 126 {
		n = 126
	}
	b := make([]byte, n)
	for i := 0; i < n; i++ {
		b[i] = m.cpu.read8(uint32(seg)<<4 + uint32(off) + 1 + uint32(i))
	}
	return string(b)
}

// launch returns 1 when a program is running, 2 when the request completed
// without leaving a program running, and 0 when the file is missing.
func (m *machine) launch(frame uint32, path, tail string) int {
	path = m.abs(path)
	base := path[strings.LastIndex(path, `\`)+1:]
	if strings.EqualFold(base, "COMMAND.COM") {
		text, ok := m.openPath(m.abs(`\AUTOEXEC.BAT`))
		m.note(fmt.Sprintf("autoexec %v %d vol %v", ok, len(text), m.mount() != nil))
		if !ok {
			m.putString("A>")
			return 2
		}
		return m.startLines(frame, batchLines(string(text)))
	}
	if strings.HasSuffix(strings.ToUpper(base), ".BAT") {
		text, ok := m.openPath(path)
		if !ok {
			return 0
		}
		return m.startLines(frame, batchLines(string(text)))
	}
	img, ok := m.openPath(path)
	if !ok {
		img, path, ok = m.openNamed(path)
		if !ok {
			return 0
		}
	}
	if m.startImage(frame, path, tail, img, nil) {
		return 1
	}
	return 0
}

func (m *machine) startLines(frame uint32, lines []string) int {
	for len(lines) > 0 {
		line := lines[0]
		lines = lines[1:]
		verb, arg := m.prepLine(line)
		if verb == "" {
			continue
		}
		switch verb {
		case "REM", "ECHO", "PATH", "SET", "PAUSE", "PROMPT", "CLS", "VERIFY", "BREAK", "LH", "LOADHIGH":
			continue
		case "CD", "CHDIR":
			m.chdir(m.abs(arg))
			continue
		}
		if len(verb) == 2 && verb[1] == ':' {
			continue
		}
		img, full, ok := m.openNamed(m.abs(verb))
		if !ok {
			m.note("missing " + verb)
			continue
		}
		if m.startImage(frame, full, arg, img, lines) {
			return 1
		}
	}
	m.putString("A>")
	return 2
}

func (m *machine) openNamed(path string) ([]byte, string, bool) {
	if b, full, ok := m.openExt(path); ok {
		return b, full, true
	}
	base := path
	if i := strings.LastIndex(base, `\`); i >= 0 {
		base = base[i+1:]
	}
	if strings.Contains(path, `\`) {
		return nil, path, false
	}
	for _, dir := range strings.Split(m.path, ";") {
		dir = strings.Trim(strings.TrimSpace(dir), `\`)
		if dir == "" {
			continue
		}
		if b, full, ok := m.openExt(dir + `\` + base); ok {
			return b, full, true
		}
	}
	return nil, path, false
}

func (m *machine) openExt(path string) ([]byte, string, bool) {
	path = m.abs(path)
	leaf := path[strings.LastIndex(path, `\`)+1:]
	if strings.Contains(leaf, ".") {
		b, ok := m.openPath(path)
		return b, path, ok
	}
	for _, ext := range []string{".COM", ".EXE", ".BAT"} {
		p := path + ext
		if b, ok := m.openPath(p); ok {
			return b, p, true
		}
	}
	return nil, path, false
}

func (m *machine) prepLine(line string) (string, string) {
	line = strings.TrimSpace(line)
	up := strings.ToUpper(line)
	if strings.HasPrefix(up, "SET ") {
		line = strings.TrimSpace(line[4:])
		up = strings.ToUpper(line)
	}
	if strings.HasPrefix(up, "PATH=") {
		m.path = strings.TrimSpace(line[strings.Index(line, "=")+1:])
		return "", ""
	}
	verb, arg := batchCmd(line)
	if verb == "CALL" {
		verb, arg = batchCmd(arg)
	}
	if verb == "IF" {
		verb, arg = m.ifLine(arg)
	}
	if strings.HasPrefix(verb, "ECHO") {
		return "", ""
	}
	return verb, arg
}

func (m *machine) ifLine(arg string) (string, string) {
	not := false
	if strings.HasPrefix(strings.ToUpper(arg), "NOT ") {
		not = true
		arg = strings.TrimSpace(arg[4:])
	}
	if !strings.HasPrefix(strings.ToUpper(arg), "EXIST ") {
		return "", ""
	}
	arg = strings.TrimSpace(arg[6:])
	spec, cmd := arg, ""
	if i := strings.IndexAny(arg, " \t"); i >= 0 {
		spec, cmd = arg[:i], strings.TrimSpace(arg[i+1:])
	}
	_, ok := m.lookup(spec)
	if ok == not {
		return "", ""
	}
	return batchCmd(cmd)
}

func (m *machine) bannerUp() bool {
	if len(m.cpu.ram) < 0xA0000+160*8 {
		return false
	}
	var text [80 * 8]byte
	for i := range text {
		text[i] = m.cpu.ram[0xA0000+i*2]
	}
	return strings.Contains(string(text[:]), "MS-DOS")
}

func batchLines(s string) []string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	var lines []string
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimRight(line, "\x1a ")
		if strings.TrimSpace(line) != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

func batchCmd(line string) (string, string) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, ":") || strings.HasPrefix(line, ";") {
		return "", ""
	}
	if strings.HasPrefix(strings.ToUpper(line), "@ECHO") || strings.Contains(line, "=") {
		return "", ""
	}
	if line[0] == '@' {
		line = strings.TrimSpace(line[1:])
	}
	if i := strings.Index(line, " "); i >= 0 {
		return strings.ToUpper(line[:i]), strings.TrimSpace(line[i+1:])
	}
	return strings.ToUpper(line), ""
}

func heavy386(img []byte) bool {
	code := img
	if len(img) >= 28 && img[0] == 'M' && img[1] == 'Z' {
		hdr := (int(img[8]) | int(img[9])<<8) * 16
		if hdr > 0 && hdr < len(img) {
			code = img[hdr:]
		}
	}
	n := 0
	for _, b := range code {
		if b == 0x66 {
			n++
			if n > 200 {
				return true
			}
		}
	}
	return false
}

func (m *machine) startImage(frame uint32, name, tail string, img []byte, queue []string) bool {
	if len(img) >= 4 && (img[0] == 'M' && img[1] == 'Z') == false && strings.HasSuffix(strings.ToUpper(name), ".BAT") {
		return m.startLines(frame, append(batchLines(string(img)), queue...)) == 1
	}
	paras := programParas(img)
	if paras > m.freeParas() {
		paras = m.freeParas()
	}
	if paras < 0x20 {
		return false
	}
	seg, ok := m.alloc(paras, false)
	if !ok {
		return false
	}
	for i := uint32(0); i < 256; i++ {
		m.cpu.ram[uint32(seg)<<4+i] = 0
	}
	m.cpu.ram[uint32(seg)<<4] = 0xCD
	m.cpu.ram[uint32(seg)<<4+1] = 0x20
	end := seg + paras
	m.cpu.write16(uint32(seg)<<4+2, end)
	m.cpu.write16(uint32(seg)<<4+0x16, m.psp)
	m.cpu.ram[uint32(seg)<<4+0x50] = 0xCD
	m.cpu.ram[uint32(seg)<<4+0x51] = 0x21
	m.cpu.ram[uint32(seg)<<4+0x52] = 0xCB
	if !strings.HasPrefix(tail, " ") && tail != "" {
		tail = " " + tail
	}
	if len(tail) > 126 {
		tail = tail[:126]
	}
	m.cpu.ram[uint32(seg)<<4+0x80] = byte(len(tail))
	for i := 0; i < len(tail); i++ {
		m.cpu.ram[uint32(seg)<<4+0x81+uint32(i)] = tail[i]
	}
	m.cpu.ram[uint32(seg)<<4+0x81+uint32(len(tail))] = 13
	cs, ip, ss, sp, ok := loadProgram(m.cpu.ram, seg, img)
	if !ok {
		m.nextSeg = seg
		delete(m.blocks, seg)
		return false
	}
	ch := &execChild{
		prev: m.child, name: name, psp: m.psp,
		dosCS: m.cpu.cs, epiIP: m.cpu.ip, dosSS: m.cpu.ss,
		frameSS: m.cpu.read16(uint32(m.cpu.ss)<<4 + 0x0586),
		frameSP: m.cpu.read16(uint32(m.cpu.ss)<<4 + 0x0584),
		seg:     seg, paras: paras, queue: queue,
	}
	m.child = ch
	m.psp = seg
	m.cpu.cs, m.cpu.ip = cs, ip
	m.cpu.ss, m.cpu.sp = ss, sp
	m.cpu.ds, m.cpu.es = seg, seg
	m.cpu.ax = 0xFFFF
	m.cpu.bx, m.cpu.cx, m.cpu.dx = 0, 0, 0
	m.note(fmt.Sprintf("run %s at %04X:%04X ss %04X:%04X", name, cs, ip, ss, sp))
	return true
}

func programParas(img []byte) uint16 {
	if len(img) >= 28 && img[0] == 'M' && img[1] == 'Z' {
		hdr := (int(img[8]) | int(img[9])<<8) * 16
		n := len(img) - hdr
		if n < 0 {
			n = 0
		}
		paras := (n+15)/16 + 0x10 + 0x40
		if paras < 0x800 {
			paras = 0x800
		}
		if paras > 0xFFFF {
			paras = 0xFFFF
		}
		return uint16(paras)
	}
	return 0x1000
}

func loadProgram(ram []byte, psp uint16, img []byte) (cs, ip, ss, sp uint16, ok bool) {
	if len(img) < 28 || img[0] != 'M' || img[1] != 'Z' {
		dst := uint32(psp)<<4 + 0x100
		n := len(img)
		if dst+uint32(n) > uint32(len(ram)) {
			n = len(ram) - int(dst)
		}
		copy(ram[dst:], img[:n])
		ram[uint32(psp)<<4+0xFFFE] = 0
		ram[uint32(psp)<<4+0xFFFF] = 0
		return psp, 0x100, psp, 0xFFFE, true
	}
	u16 := func(o int) uint16 { return uint16(img[o]) | uint16(img[o+1])<<8 }
	hdr := int(u16(8)) * 16
	if hdr < 28 || hdr > len(img) {
		return 0, 0, 0, 0, false
	}
	load := psp + 0x10
	image := img[hdr:]
	dst := uint32(load) << 4
	if dst >= uint32(len(ram)) {
		return 0, 0, 0, 0, false
	}
	if dst+uint32(len(image)) > uint32(len(ram)) {
		image = image[:uint32(len(ram))-dst]
	}
	copy(ram[dst:], image)
	nrel := int(u16(6))
	roff := int(u16(24))
	for i := 0; i < nrel; i++ {
		o := roff + i*4
		if o+4 > len(img) {
			break
		}
		off := uint16(img[o]) | uint16(img[o+1])<<8
		seg := uint16(img[o+2]) | uint16(img[o+3])<<8
		at := (uint32(load+seg)<<4 + uint32(off)) & 0xFFFFF
		w := uint16(ram[at]) | uint16(ram[(at+1)&0xFFFFF])<<8
		w += load
		ram[at] = byte(w)
		ram[(at+1)&0xFFFFF] = byte(w >> 8)
	}
	return load + u16(22), u16(20), load + u16(14), u16(16), true
}

// keepResident leaves the program in memory the way DOS Keep does.
// The next batch line loads above it, and the vectors it hooked stay live.
func (m *machine) keepResident(keep uint16) {
	ch := m.child
	if ch == nil || ch.seg == 0 {
		return
	}
	if keep < 6 {
		keep = 6
	}
	oldEnd := ch.seg + ch.paras
	if ch.paras > 0 && keep > ch.paras {
		keep = ch.paras
	}
	if m.blocks == nil {
		m.blocks = map[uint16]uint16{}
	}
	m.blocks[ch.seg] = keep
	ch.paras = keep
	ch.resident = true
	if m.nextSeg == oldEnd || m.nextSeg < ch.seg+keep {
		m.nextSeg = ch.seg + keep
	}
	m.note(fmt.Sprintf("tsr %s %04X paras", ch.name, keep))
}

func (m *machine) finishChild(al byte, cf bool, fromInt bool) {
	ch := m.child
	if ch == nil {
		return
	}
	if fromInt && !cf && len(ch.queue) > 0 {
		lines := ch.queue
		ch.queue = nil
		if m.chain(ch, lines) {
			b := uint32(ch.dosSS)<<4 + 0x0321
			if m.cpu.ram[b] > 0 {
				m.cpu.ram[b]--
			}
			return
		}
	}
	m.child = ch.prev
	m.psp = ch.psp
	m.cpu.cs = ch.dosCS
	m.cpu.ip = ch.epiIP
	m.cpu.ss = ch.dosSS
	m.cpu.halted = false
	if m.cpu.lastFault == "instruction needs a 386" {
		m.cpu.lastFault = ""
	}
	base := uint32(ch.dosSS) << 4
	m.cpu.write16(base+0x0584, ch.frameSP)
	m.cpu.write16(base+0x0586, ch.frameSS)
	if fromInt && m.cpu.ram[base+0x0321] > 0 {
		m.cpu.ram[base+0x0321]--
	}
	if ch.seg != 0 && !ch.resident && m.nextSeg == ch.seg+ch.paras {
		m.nextSeg = ch.seg
		delete(m.blocks, ch.seg)
	}
	frame := uint32(ch.frameSS)<<4 + uint32(ch.frameSP)
	if cf {
		m.setAX(frame, 1)
	} else {
		m.setAL(frame, al)
	}
	m.setCF(frame, cf)
}

func (m *machine) chain(ch *execChild, lines []string) bool {
	for len(lines) > 0 {
		line := lines[0]
		lines = lines[1:]
		verb, arg := m.prepLine(line)
		if verb == "" {
			continue
		}
		switch verb {
		case "REM", "ECHO", "PATH", "SET", "PAUSE", "PROMPT", "CLS", "VERIFY", "BREAK", "LH", "LOADHIGH":
			continue
		case "CD", "CHDIR":
			m.chdir(m.abs(arg))
			continue
		}
		if len(verb) == 2 && verb[1] == ':' {
			continue
		}
		img, full, ok := m.openNamed(m.abs(verb))
		if !ok {
			continue
		}
		if ch.seg != 0 && !ch.resident && m.nextSeg == ch.seg+ch.paras {
			m.nextSeg = ch.seg
			delete(m.blocks, ch.seg)
		}
		paras := m.freeParas()
		seg, ok := m.alloc(paras, true)
		if !ok {
			return false
		}
		cs, ip, ss, sp, ok := loadProgram(m.cpu.ram, seg, img)
		if !ok {
			return false
		}
		ch.seg, ch.paras, ch.queue, ch.steps, ch.name = seg, paras, lines, 0, full
		ch.resident = false
		m.psp = seg
		m.cpu.cs, m.cpu.ip = cs, ip
		m.cpu.ss, m.cpu.sp = ss, sp
		m.cpu.ds, m.cpu.es = seg, seg
		m.note("run " + full)
		return true
	}
	return false
}

func (m *machine) putString(s string) {
	for i := 0; i < len(s); i++ {
		m.putChar(s[i])
	}
}

func (m *machine) openFile(path string) (int, bool) {
	b, ok := m.openPath(path)
	if !ok {
		return 0, false
	}
	if m.files == nil {
		m.files = make([]*dosFile, 5)
	}
	f := &dosFile{data: b}
	for i := 5; i < len(m.files); i++ {
		if m.files[i] == nil {
			m.files[i] = f
			return i, true
		}
	}
	m.files = append(m.files, f)
	return len(m.files) - 1, true
}

func (m *machine) readFile(h int, seg, off uint16, n int) int {
	if h < 5 || h >= len(m.files) || m.files[h] == nil || n <= 0 {
		return 0
	}
	f := m.files[h]
	if f.pos >= len(f.data) {
		return 0
	}
	if f.pos+n > len(f.data) {
		n = len(f.data) - f.pos
	}
	a := uint32(seg)<<4 + uint32(off)
	for i := 0; i < n; i++ {
		m.cpu.write8(a+uint32(i), f.data[f.pos+i])
	}
	f.pos += n
	return n
}

func (m *machine) writeFile(h int, seg, off uint16, n int) int {
	if n < 0 {
		return 0
	}
	a := uint32(seg)<<4 + uint32(off)
	if h == 1 || h == 2 {
		for i := 0; i < n; i++ {
			m.putChar(m.cpu.read8(a + uint32(i)))
		}
		return n
	}
	if h < 5 || h >= len(m.files) || m.files[h] == nil {
		return 0
	}
	f := m.files[h]
	for i := 0; i < n; i++ {
		b := m.cpu.read8(a + uint32(i))
		if f.pos < len(f.data) {
			f.data[f.pos] = b
		} else {
			f.data = append(f.data, b)
		}
		f.pos++
	}
	return n
}

func (m *machine) seekFile(h int, mode byte, delta int32) (int, bool) {
	if h < 5 || h >= len(m.files) || m.files[h] == nil {
		return 0, false
	}
	f := m.files[h]
	var pos int
	switch mode {
	case 0:
		pos = int(delta)
	case 1:
		pos = f.pos + int(delta)
	default:
		pos = len(f.data) + int(delta)
	}
	if pos < 0 {
		return 0, false
	}
	f.pos = pos
	return pos, true
}

// dosRegs answers INT 21h from the registers. Disks whose own DOS never
// reaches the 6.2 dispatcher use this after the door takes over the shell.
func (m *machine) dosRegs() {
	ah := byte(m.cpu.ax >> 8)
	switch ah {
	case 0x02:
		m.putChar(byte(m.cpu.dx))
	case 0x09:
		m.writeDollar(m.cpu.ds, m.cpu.dx)
	case 0x0E:
		m.drive = byte(m.cpu.dx)
		m.cpu.ax = 0x0010
	case 0x19:
		m.cpu.ax = m.cpu.ax&0xFF00 | uint16(m.drive)
	case 0x1A:
		m.dtaSeg, m.dtaOff = m.cpu.ds, m.cpu.dx
	case 0x25:
		n := uint32(m.cpu.ax&0xFF) * 4
		m.cpu.write16(n, m.cpu.dx)
		m.cpu.write16(n+2, m.cpu.ds)
	case 0x30:
		m.cpu.ax = 0x0005
		m.cpu.bx = 0
	case 0x31:
		if m.child != nil {
			m.keepResident(m.cpu.dx)
			m.finishChild(byte(m.cpu.ax), false, true)
			return
		}
	case 0x35:
		n := uint32(m.cpu.ax&0xFF) * 4
		m.cpu.bx = m.cpu.read16(n)
		m.cpu.es = m.cpu.read16(n + 2)
	case 0x3C, 0x3D:
		h, ok := m.openFile(m.abs(m.asciiZ(m.cpu.ds, m.cpu.dx)))
		if !ok {
			m.cpu.ax = 2
			m.dosRet(true)
			return
		}
		m.cpu.ax = uint16(h)
	case 0x3E:
		if int(m.cpu.bx) < len(m.files) && m.cpu.bx >= 5 {
			m.files[m.cpu.bx] = nil
		}
	case 0x3F:
		m.cpu.ax = uint16(m.readFile(int(m.cpu.bx), m.cpu.ds, m.cpu.dx, int(m.cpu.cx)))
	case 0x40:
		m.cpu.ax = uint16(m.writeFile(int(m.cpu.bx), m.cpu.ds, m.cpu.dx, int(m.cpu.cx)))
	case 0x42:
		pos, ok := m.seekFile(int(m.cpu.bx), byte(m.cpu.ax), int32(m.cpu.cx)<<16|int32(m.cpu.dx))
		if !ok {
			m.cpu.ax = 6
			m.dosRet(true)
			return
		}
		m.cpu.ax = uint16(pos)
		m.cpu.dx = uint16(pos >> 16)
	case 0x48:
		seg, ok := m.alloc(m.cpu.bx, false)
		if !ok {
			m.cpu.ax = 8
			m.cpu.bx = m.freeParas()
			m.dosRet(true)
			return
		}
		m.cpu.ax = seg
	case 0x4A:
		if !m.resize(m.cpu.es, m.cpu.bx) {
			m.cpu.ax = 8
			m.cpu.bx = m.freeParas()
			m.dosRet(true)
			return
		}
	case 0x4C, 0x00:
		if m.child != nil {
			m.finishChild(byte(m.cpu.ax), false, true)
			return
		}
	}
	m.dosRet(false)
}

func (m *machine) dosRet(cf bool) {
	m.cpu.setCarryStacked(cf)
	m.cpu.iret()
}
