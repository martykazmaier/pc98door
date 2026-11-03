//go:build (windows && 386) || (linux && (386 || amd64 || arm64))

package main

import (
	"io"
	"os"
	"path/filepath"
	"runtime"
	"time"
	"unsafe"
)

var (
	np2Pinned [][]byte
	np2Sys    uintptr
	np2Save   uintptr
	np2DLL    uintptr
	np2Keys   = map[uint16]int{}
	np2MouseX int
	np2MouseY int
	np2Btn    int
	np2LastX  = -1
	np2LastY  = -1
	np2Pix    uint32
	np2Frame  []byte
	np2W      int
	np2H      int
	np2Pitch  int
	np2Dirty  bool
	np2Opt    = map[string]uintptr{}
	np2Run    uintptr
	np2PCM    []byte
	np2Heard  bool
)

func playNP2(w io.Writer, keys keyboard, disk, root, bios string) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := loadNP2(disk, root, bios); err != nil {
		return err
	}
	defer unloadNP2()
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
		defer func() {
			np2PCM = nil
			_, _ = w.Write(audioStop())
		}()
		_, _, _ = keys.WaitKey(150 * time.Millisecond)
	}
	next := time.Time{}
	var sum uint64
	for {
		call0(np2Run)
		for id, left := range np2Keys {
			if left <= 1 {
				delete(np2Keys, id)
			} else {
				np2Keys[id] = left - 1
			}
		}
		takeMice(keys)
		if useJXL {
			if err := flushAudio(w); err != nil {
				return err
			}
		}
		if useJXL && np2Dirty && time.Now().After(next) {
			pix := np2RGBA()
			h := hashPix(pix)
			if h != sum {
				blob, err := jxlDraw(pix, np2W, np2H)
				if err != nil {
					return err
				}
				if _, err := w.Write(blob); err != nil {
					return err
				}
				sum = h
			}
			np2Dirty = false
			next = time.Now().Add(100 * time.Millisecond)
		}
		k, ok, err := keys.WaitKey(12 * time.Millisecond)
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		if k == keyCtrlQ {
			return nil
		}
		holdKey(k)
	}
}

// primeNP2 points Neko Project at this caller's firmware and save directory.
func primeNP2(root, bios, libDir string) error {
	sys, err := linkBIOS(root, bios)
	if err != nil {
		return err
	}
	np2Sys = cstr(sys)
	np2Save = cstr(filepath.Join(root, "save"))
	np2DLL = cstr(libDir)
	np2Opt = map[string]uintptr{
		"np2kai_uselasthddmount": cstr("OFF"),
		"np2kai_inputmouse":      cstr("ON"),
		"np2kai_keyboard":        cstr("Us"),
		"np2kai_model":           cstr("PC-9801VX"),
	}
	np2PCM = nil
	np2Heard = false
	np2Keys = map[uint16]int{}
	np2MouseX, np2MouseY, np2Btn = 0, 0, 0
	np2LastX, np2LastY = -1, -1
	np2Dirty = false
	return nil
}

type gameInfo struct {
	path uintptr
	data uintptr
	size uintptr
	meta uintptr
}

func envGo(cmd, data uintptr) uintptr {
	if data == 0 {
		return 0
	}
	switch cmd {
	case 3:
		*(*byte)(unsafe.Pointer(data)) = 1
		return 1
	case 9:
		*(*uintptr)(unsafe.Pointer(data)) = np2Sys
		return 1
	case 10:
		np2Pix = *(*uint32)(unsafe.Pointer(data))
		if np2Pix > 2 {
			return 0
		}
		return 1
	case 11, 16, 18, 35:
		return 1
	case 15:
		key := goStr(*(*uintptr)(unsafe.Pointer(data)))
		slot := unsafe.Pointer(data + unsafe.Sizeof(uintptr(0)))
		if v, ok := np2Opt[key]; ok {
			*(*uintptr)(slot) = v
			return 1
		}
		*(*uintptr)(slot) = 0
		return 0
	case 17:
		*(*byte)(unsafe.Pointer(data)) = 0
		return 1
	case 19:
		*(*uintptr)(unsafe.Pointer(data)) = np2DLL
		return 1
	case 24:
		*(*uint32)(unsafe.Pointer(data)) = 1<<2 | 1<<3
		*(*uint32)(unsafe.Pointer(data + 4)) = 0
		return 1
	case 31:
		*(*uintptr)(unsafe.Pointer(data)) = np2Save
		return 1
	case 39:
		*(*uint32)(unsafe.Pointer(data)) = 0
		return 1
	default:
		return 0
	}
}

func videoGo(data, width, height, pitch uintptr) uintptr {
	w, h, p := int(width), int(height), int(pitch)
	if data == 0 || w < 1 || h < 1 || w > 1024 || h > 1024 || p < 1 {
		return 0
	}
	n := p * h
	buf := make([]byte, n)
	src := unsafe.Slice((*byte)(unsafe.Pointer(data)), n)
	copy(buf, src)
	np2Frame, np2W, np2H, np2Pitch = buf, w, h, p
	np2Dirty = true
	return 0
}

func audioGo(uintptr, uintptr) uintptr { return 0 }

func batchGo(data, frames uintptr) uintptr {
	n := int(frames) * 4
	if data == 0 || n < 4 || n > 65536 {
		return frames
	}
	src := unsafe.Slice((*byte)(unsafe.Pointer(data)), n)
	np2PCM = append(np2PCM, src...)
	if len(np2PCM) > 44100*8 {
		np2PCM = append([]byte(nil), np2PCM[len(np2PCM)-44100*4:]...)
	}
	return frames
}

func flushAudio(w io.Writer) error {
	const chunk = 4410 * 4
	if len(np2PCM) < chunk {
		return nil
	}
	n := len(np2PCM) &^ 3
	pcm := np2PCM[:n]
	np2PCM = np2PCM[n:]
	_, err := w.Write(audioAPC(pcm, !np2Heard))
	np2Heard = true
	return err
}

func pollGo() uintptr { return 0 }

func stateGo(port, device, index, id uintptr) uintptr {
	if port != 0 {
		return 0
	}
	switch device {
	case 2:
		switch id {
		case 0:
			v := np2MouseX
			np2MouseX = 0
			return uintptr(int16(v))
		case 1:
			v := np2MouseY
			np2MouseY = 0
			return uintptr(int16(v))
		case 2:
			if np2Btn&1 != 0 {
				return 1
			}
		case 3:
			if np2Btn&2 != 0 {
				return 1
			}
		case 6:
			if np2Btn&4 != 0 {
				return 1
			}
		}
	case 3:
		if np2Keys[uint16(id)] > 0 {
			return 1
		}
	}
	return 0
}

func holdKey(k int) {
	k &^= keyShift | keyCtrl
	var id uint16
	switch k {
	case keyBack:
		id = 8
	case keyEnter:
		id = 13
	case keyEsc:
		id = 27
	case keyUp:
		id = 273
	case keyDown:
		id = 274
	case keyRight:
		id = 275
	case keyLeft:
		id = 276
	case keyHome:
		id = 278
	case keyEnd:
		id = 279
	case keyPgUp:
		id = 280
	case keyPgDn:
		id = 281
	case keyDel:
		id = 127
	default:
		if k >= 1 && k <= 26 {
			np2Keys[306] = 8
			np2Keys[uint16('a'+k-1)] = 8
			return
		}
		if k >= 'A' && k <= 'Z' {
			np2Keys[304] = 8
			np2Keys[uint16(k-'A'+'a')] = 8
			return
		}
		if k >= 32 && k < 127 {
			id = uint16(k)
		} else {
			return
		}
	}
	np2Keys[id] = 8
}

func takeMice(keys keyboard) {
	src, ok := keys.(interface{ TakeMouse() []mouseSample })
	if !ok {
		return
	}
	for _, s := range src.TakeMouse() {
		if np2LastX >= 0 {
			np2MouseX += s.x - np2LastX
			np2MouseY += s.y - np2LastY
		}
		np2LastX, np2LastY = s.x, s.y
		np2Btn = s.btn
	}
}

func np2RGBA() []byte {
	w, h, p := np2W, np2H, np2Pitch
	dst := make([]byte, w*h*4)
	src := np2Frame
	for y := 0; y < h; y++ {
		row := src[y*p:]
		for x := 0; x < w; x++ {
			var r, g, b byte
			switch np2Pix {
			case 1:
				if x*4+2 < len(row) {
					b, g, r = row[x*4], row[x*4+1], row[x*4+2]
				}
			case 2:
				if x*2+1 < len(row) {
					v := uint16(row[x*2]) | uint16(row[x*2+1])<<8
					r = scale565((v>>11)&31, 31)
					g = scale565((v>>5)&63, 63)
					b = scale565(v&31, 31)
				}
			default:
				if x*2+1 < len(row) {
					v := uint16(row[x*2]) | uint16(row[x*2+1])<<8
					r = scale565((v>>10)&31, 31)
					g = scale565((v>>5)&31, 31)
					b = scale565(v&31, 31)
				}
			}
			o := (y*w + x) * 4
			dst[o], dst[o+1], dst[o+2], dst[o+3] = r, g, b, 255
		}
	}
	return dst
}

// scale565 expands a 5- or 6-bit channel. The multiply has to stay in an
// int: a byte multiply wraps, and white (31) was coming out as 7.
func scale565(v uint16, full int) byte {
	return byte(int(v) * 255 / full)
}

func hashPix(pix []byte) uint64 {
	h := uint64(14695981039346656037)
	step := 64
	if len(pix) < step {
		step = 1
	}
	for i := 0; i < len(pix); i += step {
		h ^= uint64(pix[i])
		h *= 1099511628211
	}
	return h
}

func cstr(s string) uintptr {
	b := append([]byte(s), 0)
	np2Pinned = append(np2Pinned, b)
	return uintptr(unsafe.Pointer(&b[0]))
}

func goStr(p uintptr) string {
	if p == 0 {
		return ""
	}
	var b []byte
	for i := uintptr(0); i < 128; i++ {
		c := *(*byte)(unsafe.Pointer(p + i))
		if c == 0 {
			break
		}
		b = append(b, c)
	}
	return string(b)
}

func linkBIOS(root, bios string) (string, error) {
	sys := filepath.Join(root, "np2sys")
	if err := os.MkdirAll(sys, 0o755); err != nil {
		return "", err
	}
	dest := filepath.Join(sys, "np2kai")
	if st, err := os.Stat(filepath.Join(dest, "bios.rom")); err == nil && st.Size() > 0 {
		return sys, nil
	}
	if err := copyBIOS(bios, dest); err != nil {
		return "", err
	}
	return sys, nil
}

func copyBIOS(bios, dest string) error {
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return err
	}
	entries, err := os.ReadDir(bios)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		in, err := os.Open(filepath.Join(bios, e.Name()))
		if err != nil {
			return err
		}
		out, err := os.Create(filepath.Join(dest, e.Name()))
		if err != nil {
			in.Close()
			return err
		}
		_, err = io.Copy(out, in)
		in.Close()
		out.Close()
		if err != nil {
			return err
		}
	}
	return nil
}
