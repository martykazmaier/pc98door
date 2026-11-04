//go:build !windows

package main

import (
	"fmt"
	"os"
	"time"
)

// stdTerm is the local terminal used by builds other than Windows.
// DOOR32 inherited sockets are a Windows BBS facility. SyncTERM on Linux
// reads the same JPEG XL and mouse sequences from this terminal.
type stdTerm struct {
	in      chan byte
	cr      bool
	push    []byte
	btn     int
	mice    []mouseSample
	restore func()
}

func openConsole() (*stdTerm, error) {
	restore, err := prepareConsole()
	if err != nil {
		return nil, err
	}
	t := &stdTerm{in: make(chan byte, 256), restore: restore}
	go func() {
		buf := make([]byte, 1)
		for {
			n, err := os.Stdin.Read(buf)
			if n == 1 {
				t.in <- buf[0]
			}
			if err != nil {
				close(t.in)
				return
			}
		}
	}()
	return t, nil
}

func openDoorSocket(uintptr) (*stdTerm, error) {
	return nil, fmt.Errorf("DOOR32 sockets are provided by the Windows build")
}

func (t *stdTerm) Close() error {
	if t.restore != nil {
		t.restore()
	}
	return nil
}

func (t *stdTerm) Write(p []byte) (int, error) { return os.Stdout.Write(p) }

func (t *stdTerm) JXL() bool { return true }

func (t *stdTerm) TakeMouse() []mouseSample {
	v := t.mice
	t.mice = nil
	return v
}

func (t *stdTerm) noteMouse(params string, final byte) {
	x, y, btn, ok := parseSGR(params, final, t.btn)
	if !ok {
		return
	}
	t.btn = btn
	t.mice = append(t.mice, mouseSample{x: x, y: y, btn: btn})
}

func (t *stdTerm) readByte(d time.Duration) (byte, error) {
	if len(t.push) > 0 {
		b := t.push[0]
		t.push = t.push[1:]
		return b, nil
	}
	if d < 0 {
		b, ok := <-t.in
		if !ok {
			return 0, os.ErrClosed
		}
		return b, nil
	}
	if d == 0 {
		select {
		case b, ok := <-t.in:
			if !ok {
				return 0, os.ErrClosed
			}
			return b, nil
		default:
			return 0, errTimeout
		}
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case b, ok := <-t.in:
		if !ok {
			return 0, os.ErrClosed
		}
		return b, nil
	case <-timer.C:
		return 0, errTimeout
	}
}

func (t *stdTerm) consumeReport() (int, bool, error) {
	for i := 0; i < 48; i++ {
		c, err := t.readByte(80 * time.Millisecond)
		if err != nil {
			return keyNone, false, nil
		}
		if c >= 0x40 && c <= 0x7E {
			return keyNone, false, nil
		}
	}
	return keyNone, false, nil
}

func (t *stdTerm) WaitKey(d time.Duration) (int, bool, error) {
	wait := d
	for {
		b, err := t.readByte(wait)
		if err == errTimeout {
			return 0, false, nil
		}
		if err != nil {
			return 0, false, err
		}
		if b != 27 {
			k, cr := mapByte(b, t.cr)
			t.cr = cr
			if k == keyNone {
				wait = 0
				continue
			}
			return k, true, nil
		}
		intro, err := t.readByte(80 * time.Millisecond)
		if err != nil {
			return keyEsc, true, nil
		}
		if intro != '[' && intro != 'O' {
			if intro == '=' || intro == '<' {
				_, _, _ = t.consumeReport()
				wait = 0
				continue
			}
			t.push = append(t.push, intro)
			return keyEsc, true, nil
		}
		var params []byte
		mouse := false
		for i := 0; i < 32; i++ {
			c, err := t.readByte(80 * time.Millisecond)
			if err != nil {
				return keyEsc, true, nil
			}
			if c >= 0x40 && c <= 0x7E {
				if len(params) > 0 && params[0] == '<' && (c == 'M' || c == 'm') {
					t.noteMouse(string(params), c)
					mouse = true
				} else {
					return mapSeq(intro, string(params), c), true, nil
				}
				break
			}
			params = append(params, c)
		}
		if mouse {
			wait = 0
			continue
		}
		return keyNone, false, nil
	}
}
