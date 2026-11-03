//go:build !windows

package main

import (
	"fmt"
	"os"
	"time"
)

// stdTerm is the local terminal used by builds other than Windows.
// DOOR32 inherited sockets are a Windows BBS facility.
type stdTerm struct {
	in chan byte
	cr bool
}

func openConsole() (*stdTerm, error) {
	t := &stdTerm{in: make(chan byte, 256)}
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

func (t *stdTerm) Close() error { return nil }

func (t *stdTerm) Write(p []byte) (int, error) { return os.Stdout.Write(p) }

func (t *stdTerm) readByte(d time.Duration) (byte, error) {
	if d < 0 {
		b, ok := <-t.in
		if !ok {
			return 0, os.ErrClosed
		}
		return b, nil
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

func (t *stdTerm) WaitKey(d time.Duration) (int, bool, error) {
	b, err := t.readByte(d)
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
			return 0, false, nil
		}
		return k, true, nil
	}
	intro, err := t.readByte(80 * time.Millisecond)
	if err != nil {
		return keyEsc, true, nil
	}
	if intro != '[' && intro != 'O' {
		return keyEsc, true, nil
	}
	var params []byte
	for i := 0; i < 32; i++ {
		c, err := t.readByte(80 * time.Millisecond)
		if err != nil {
			return keyEsc, true, nil
		}
		if c >= 0x40 && c <= 0x7E {
			return mapSeq(intro, string(params), c), true, nil
		}
		params = append(params, c)
	}
	return keyNone, false, nil
}
