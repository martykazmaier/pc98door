//go:build windows && 386

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"unsafe"
)

func call0(fn uintptr) uintptr {
	r, _, _ := syscall.Syscall(fn, 0, 0, 0, 0)
	return r
}

func call1(fn, a uintptr) uintptr {
	r, _, _ := syscall.Syscall(fn, 1, a, 0, 0)
	return r
}

// cdeclStub is a cdecl function that forwards to a stdcall Go callback.
func cdeclStub(n int, cb uintptr) uintptr {
	var b []byte
	if n == 0 {
		b = append(b, 0xB8, byte(cb), byte(cb>>8), byte(cb>>16), byte(cb>>24), 0xFF, 0xD0, 0xC3)
	} else {
		off := byte(4 * n)
		for i := 0; i < n; i++ {
			b = append(b, 0xFF, 0x74, 0x24, off)
		}
		b = append(b, 0xB8, byte(cb), byte(cb>>8), byte(cb>>16), byte(cb>>24), 0xFF, 0xD0, 0xC3)
	}
	const commit, exec = uintptr(0x1000), uintptr(0x40)
	p, _, _ := syscall.NewLazyDLL("kernel32.dll").NewProc("VirtualAlloc").Call(0, uintptr(len(b)), commit, exec)
	dst := unsafe.Slice((*byte)(unsafe.Pointer(p)), len(b))
	copy(dst, b)
	np2Code = append(np2Code, p)
	return p
}

var (
	envCB   uintptr
	videoCB uintptr
	audioCB uintptr
	batchCB uintptr
	pollCB  uintptr
	stateCB uintptr
	np2Code []uintptr
	np2Lib  syscall.Handle
)

func loadNP2(disk, root, bios string) error {
	dll := filepath.Join(root, "np2kai_libretro.dll")
	if _, err := os.Stat(dll); err != nil {
		alt := filepath.Join(root, "third_party", "np2kai_libretro.dll")
		if _, err2 := os.Stat(alt); err2 != nil {
			return fmt.Errorf("Neko Project is not next to the door")
		}
		dll = alt
	}
	if err := primeNP2(root, bios, filepath.Dir(dll)); err != nil {
		return err
	}
	envCB = syscall.NewCallback(envGo)
	videoCB = syscall.NewCallback(videoGo)
	audioCB = syscall.NewCallback(audioGo)
	batchCB = syscall.NewCallback(batchGo)
	pollCB = syscall.NewCallback(pollGo)
	stateCB = syscall.NewCallback(stateGo)
	h, err := syscall.LoadLibrary(dll)
	if err != nil {
		return err
	}
	np2Lib = h
	call1(np2Proc("retro_set_environment"), cdeclStub(2, envCB))
	call1(np2Proc("retro_set_video_refresh"), cdeclStub(4, videoCB))
	call1(np2Proc("retro_set_audio_sample"), cdeclStub(2, audioCB))
	call1(np2Proc("retro_set_audio_sample_batch"), cdeclStub(2, batchCB))
	call1(np2Proc("retro_set_input_poll"), cdeclStub(0, pollCB))
	call1(np2Proc("retro_set_input_state"), cdeclStub(4, stateCB))
	call0(np2Proc("retro_init"))
	np2Run = np2Proc("retro_run")
	info := gameInfo{path: cstr(disk)}
	if call1(np2Proc("retro_load_game"), uintptr(unsafe.Pointer(&info))) == 0 {
		return fmt.Errorf("Neko Project did not open %s", filepath.Base(disk))
	}
	return nil
}

func unloadNP2() {
	if np2Lib == 0 {
		return
	}
	call0(np2Proc("retro_unload_game"))
	call0(np2Proc("retro_deinit"))
	syscall.FreeLibrary(np2Lib)
	np2Lib = 0
}

func np2Proc(name string) uintptr {
	p, err := syscall.GetProcAddress(np2Lib, name)
	if err != nil {
		return 0
	}
	return p
}
