//go:build windows

package main

import (
	"bytes"
	"io"
	"syscall"
	"time"
	"unsafe"
)

var (
	kernel32                = syscall.NewLazyDLL("kernel32.dll")
	procSetConsoleMode      = kernel32.NewProc("SetConsoleMode")
	procGetConsoleMode      = kernel32.NewProc("GetConsoleMode")
	procReadConsoleInputW   = kernel32.NewProc("ReadConsoleInputW")
	procSetConsoleOutputCP  = kernel32.NewProc("SetConsoleOutputCP")
	procGetConsoleOutputCP  = kernel32.NewProc("GetConsoleOutputCP")
	procSetConsoleCP        = kernel32.NewProc("SetConsoleCP")
	procGetConsoleCP        = kernel32.NewProc("GetConsoleCP")
	procWaitForSingleObject = kernel32.NewProc("WaitForSingleObject")
)

type coord struct{ X, Y int16 }

type inputRecord struct {
	EventType uint16
	_         uint16
	KeyDown   int32
	Repeat    uint16
	VK        uint16
	Scan      uint16
	Char      uint16
	Control   uint32
}

type console struct {
	in, out syscall.Handle
	restore func()
	pending []int
}

func openConsole() (*console, error) {
	in, err := syscall.GetStdHandle(syscall.STD_INPUT_HANDLE)
	if err != nil {
		return nil, err
	}
	out, err := syscall.GetStdHandle(syscall.STD_OUTPUT_HANDLE)
	if err != nil {
		return nil, err
	}
	var inMode, outMode uint32
	procGetConsoleMode.Call(uintptr(in), uintptr(unsafe.Pointer(&inMode)))
	procGetConsoleMode.Call(uintptr(out), uintptr(unsafe.Pointer(&outMode)))
	oldOut, _, _ := procGetConsoleOutputCP.Call()
	oldIn, _, _ := procGetConsoleCP.Call()
	procSetConsoleMode.Call(uintptr(in), 0x0080)
	procSetConsoleMode.Call(uintptr(out), uintptr(outMode|0x0001|0x0004))
	procSetConsoleOutputCP.Call(437)
	procSetConsoleCP.Call(437)
	c := &console{in: in, out: out}
	c.restore = func() {
		procSetConsoleMode.Call(uintptr(in), uintptr(inMode))
		procSetConsoleMode.Call(uintptr(out), uintptr(outMode))
		procSetConsoleOutputCP.Call(oldOut)
		procSetConsoleCP.Call(oldIn)
	}
	return c, nil
}

func (c *console) Close() error {
	if c.restore != nil {
		c.restore()
	}
	return nil
}

func (c *console) Write(p []byte) (int, error) {
	var n uint32
	err := syscall.WriteFile(c.out, p, &n, nil)
	return int(n), err
}

func (c *console) WaitKey(d time.Duration) (int, bool, error) {
	if len(c.pending) > 0 {
		k := c.pending[0]
		c.pending = c.pending[1:]
		return k, true, nil
	}
	wait := uint32(0xFFFFFFFF)
	if d >= 0 {
		wait = uint32(d / time.Millisecond)
	}
	r, _, _ := procWaitForSingleObject.Call(uintptr(c.in), uintptr(wait))
	if r == 0x102 {
		return 0, false, nil
	}
	var rec inputRecord
	var n uint32
	ok, _, _ := procReadConsoleInputW.Call(uintptr(c.in), uintptr(unsafe.Pointer(&rec)), 1, uintptr(unsafe.Pointer(&n)))
	if ok == 0 || n == 0 || rec.EventType != 1 || rec.KeyDown == 0 {
		return 0, false, nil
	}
	k := mapConsoleKey(rec)
	if k == keyNone {
		return 0, false, nil
	}
	return k, true, nil
}

func mapConsoleKey(r inputRecord) int {
	switch r.VK {
	case 0x25:
		return keyLeft
	case 0x26:
		return keyUp
	case 0x27:
		return keyRight
	case 0x28:
		return keyDown
	case 0x21:
		return keyPgUp
	case 0x22:
		return keyPgDn
	case 0x24:
		return keyHome
	case 0x23:
		return keyEnd
	case 0x08:
		return keyBack
	case 0x2E:
		return keyDel
	case 0x1B:
		return keyEsc
	case 0x0D:
		return keyEnter
	}
	ch := rune(r.Char)
	if ch == 0 {
		return keyNone
	}
	if ch < 0x80 {
		return int(ch)
	}
	if b, ok := runeToCP437[ch]; ok {
		return int(b)
	}
	return keyNone
}

type socketDoor struct {
	h    syscall.Handle
	dup  bool
	raw  chan byte
	push []byte
	cr   bool
	btn  int
	mice []mouseSample
}

func (s *socketDoor) TakeMouse() []mouseSample {
	v := s.mice
	s.mice = nil
	return v
}

func (s *socketDoor) noteMouse(params string, final byte) {
	x, y, btn, ok := parseSGR(params, final, s.btn)
	if !ok {
		return
	}
	s.btn = btn
	s.mice = append(s.mice, mouseSample{x: x, y: y, btn: btn})
}

// JXL reports that this session is a SyncTERM socket and can draw JPEG XL.
func (s *socketDoor) JXL() bool { return true }

func (s *socketDoor) consumeReport() (int, bool, error) {
	for i := 0; i < 48; i++ {
		c, err := s.readByte(80 * time.Millisecond)
		if err != nil {
			return keyNone, false, nil
		}
		if c >= 0x40 && c <= 0x7E {
			return keyNone, false, nil
		}
	}
	return keyNone, false, nil
}

func openDoorSocket(handle uintptr) (*socketDoor, error) {
	var data syscall.WSAData
	if err := syscall.WSAStartup(0x0202, &data); err != nil {
		return nil, err
	}
	h := syscall.Handle(handle)
	dup := false
	if !socketAlive(h) {
		got, ok := duplicateSocket(h)
		if !ok {
			return nil, io.EOF
		}
		h = got
		dup = true
	}
	s := &socketDoor{h: h, dup: dup, raw: make(chan byte, 8192)}
	go s.readLoop()
	return s, nil
}

func socketAlive(h syscall.Handle) bool {
	var kind int32
	n := int32(4)
	if err := syscall.Getsockopt(h, 0xffff, 0x1008, (*byte)(unsafe.Pointer(&kind)), &n); err != nil {
		return false
	}
	_, err := syscall.Getpeername(h)
	return err == nil || err == syscall.Errno(10057)
}

func duplicateSocket(h syscall.Handle) (syscall.Handle, bool) {
	const access = 0x0040
	self, _ := syscall.GetCurrentProcess()
	snap, err := syscall.CreateToolhelp32Snapshot(0x2, 0)
	if err != nil {
		return 0, false
	}
	defer syscall.CloseHandle(snap)
	parent := map[uint32]uint32{}
	var pe syscall.ProcessEntry32
	pe.Size = uint32(unsafe.Sizeof(pe))
	for err = syscall.Process32First(snap, &pe); err == nil; err = syscall.Process32Next(snap, &pe) {
		parent[pe.ProcessID] = pe.ParentProcessID
	}
	pid := uint32(syscall.Getpid())
	for i := 0; i < 6; i++ {
		pp := parent[pid]
		if pp == 0 {
			break
		}
		ph, err := syscall.OpenProcess(access, false, pp)
		if err == nil {
			var dup syscall.Handle
			err = syscall.DuplicateHandle(ph, h, self, &dup, 0, false, syscall.DUPLICATE_SAME_ACCESS)
			syscall.CloseHandle(ph)
			if err == nil && socketAlive(dup) {
				return dup, true
			}
			if dup != 0 {
				syscall.CloseHandle(dup)
			}
		}
		pid = pp
	}
	return 0, false
}

func (s *socketDoor) Close() error {
	if s.dup {
		syscall.Closesocket(s.h)
	}
	return nil
}

func (s *socketDoor) Write(p []byte) (int, error) {
	if bytes.IndexByte(p, 0xFF) >= 0 {
		p = bytes.ReplaceAll(p, []byte{0xFF}, []byte{0xFF, 0xFF})
	}
	sent := 0
	for len(p) > 0 {
		buf := syscall.WSABuf{Len: uint32(len(p)), Buf: &p[0]}
		var n uint32
		err := syscall.WSASend(s.h, &buf, 1, &n, 0, nil, nil)
		if err == syscall.Errno(10035) {
			time.Sleep(5 * time.Millisecond)
			continue
		}
		if err != nil {
			return sent, err
		}
		sent += int(n)
		p = p[n:]
	}
	return sent, nil
}

func (s *socketDoor) readLoop() {
	buf := make([]byte, 2048)
	state := 0
	for {
		wb := syscall.WSABuf{Len: uint32(len(buf)), Buf: &buf[0]}
		var n, flags uint32
		err := syscall.WSARecv(s.h, &wb, 1, &n, &flags, nil, nil)
		if err == syscall.Errno(10035) {
			time.Sleep(10 * time.Millisecond)
			continue
		}
		if err != nil || n == 0 {
			close(s.raw)
			return
		}
		for _, b := range buf[:n] {
			switch state {
			case 0:
				if b == 255 {
					state = 1
				} else {
					s.raw <- b
				}
			case 1:
				if b == 255 {
					s.raw <- 255
					state = 0
				} else if b == 250 {
					state = 2
				} else if b >= 251 {
					state = 3
				} else {
					state = 0
				}
			case 2:
				if b == 255 {
					state = 4
				}
			case 3:
				state = 0
			default:
				if b == 240 {
					state = 0
				} else {
					state = 2
				}
			}
		}
	}
}

func (s *socketDoor) readByte(d time.Duration) (byte, error) {
	if len(s.push) > 0 {
		b := s.push[0]
		s.push = s.push[1:]
		return b, nil
	}
	if d < 0 {
		b, ok := <-s.raw
		if !ok {
			return 0, io.EOF
		}
		return b, nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case b, ok := <-s.raw:
		if !ok {
			return 0, io.EOF
		}
		return b, nil
	case <-t.C:
		return 0, errTimeout
	}
}

func (s *socketDoor) WaitKey(d time.Duration) (int, bool, error) {
	wait := d
	if d < 0 {
		wait = -1
	}
	b, err := s.readByte(wait)
	if err == errTimeout {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	if b != 27 {
		k, cr := mapByte(b, s.cr)
		s.cr = cr
		if k == keyNone {
			return 0, false, nil
		}
		return k, true, nil
	}
	intro, err := s.readByte(80 * time.Millisecond)
	if err == errTimeout || err != nil {
		return keyEsc, true, nil
	}
	if intro != '[' && intro != 'O' {
		if intro == '=' || intro == '<' {
			return s.consumeReport()
		}
		s.push = append(s.push, intro)
		return keyEsc, true, nil
	}
	var params []byte
	for i := 0; i < 32; i++ {
		c, err := s.readByte(80 * time.Millisecond)
		if err != nil {
			return keyEsc, true, nil
		}
		if c >= 0x40 && c <= 0x7E {
			if len(params) > 0 && params[0] == '<' && (c == 'M' || c == 'm') {
				s.noteMouse(string(params), c)
				return keyNone, false, nil
			}
			return mapSeq(intro, string(params), c), true, nil
		}
		params = append(params, c)
	}
	return keyNone, false, nil
}
