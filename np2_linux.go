//go:build linux && (386 || amd64 || arm64)

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"unsafe"

	"github.com/ebitengine/purego"
)

func call0(fn uintptr) uintptr {
	r, _, _ := purego.SyscallN(fn)
	return r
}

func call1(fn, a uintptr) uintptr {
	r, _, _ := purego.SyscallN(fn, a)
	return r
}

var (
	np2Lib uintptr
	np2Sym = map[string]uintptr{}
	np2CB  []uintptr
)

func loadNP2(disk, root, bios string) error {
	path, err := corePath(root)
	if err != nil {
		return err
	}
	if err := primeNP2(root, bios, filepath.Dir(path)); err != nil {
		return err
	}
	lib, err := purego.Dlopen(path, purego.RTLD_NOW|purego.RTLD_LOCAL)
	if err != nil {
		return fmt.Errorf("Neko Project did not load: %w", err)
	}
	np2Lib = lib
	np2Sym = map[string]uintptr{}
	env := purego.NewCallback(envGo)
	video := purego.NewCallback(videoGo)
	audio := purego.NewCallback(audioGo)
	batch := purego.NewCallback(batchGo)
	poll := purego.NewCallback(pollGo)
	state := purego.NewCallback(stateGo)
	np2CB = []uintptr{env, video, audio, batch, poll, state}
	if !setCB("retro_set_environment", env) ||
		!setCB("retro_set_video_refresh", video) ||
		!setCB("retro_set_audio_sample", audio) ||
		!setCB("retro_set_audio_sample_batch", batch) ||
		!setCB("retro_set_input_poll", poll) ||
		!setCB("retro_set_input_state", state) {
		return fmt.Errorf("Neko Project is missing an entry point")
	}
	call0(np2Proc("retro_init"))
	np2Run = np2Proc("retro_run")
	if np2Run == 0 {
		return fmt.Errorf("Neko Project is missing an entry point")
	}
	info := gameInfo{path: cstr(disk)}
	if call1(np2Proc("retro_load_game"), uintptr(unsafe.Pointer(&info))) == 0 {
		return fmt.Errorf("Neko Project did not open %s", filepath.Base(disk))
	}
	return nil
}

func setCB(name string, cb uintptr) bool {
	fn := np2Proc(name)
	if fn == 0 {
		return false
	}
	call1(fn, cb)
	return true
}

func unloadNP2() {
	if np2Lib == 0 {
		return
	}
	if p := np2Proc("retro_unload_game"); p != 0 {
		call0(p)
	}
	if p := np2Proc("retro_deinit"); p != 0 {
		call0(p)
	}
	_ = purego.Dlclose(np2Lib)
	np2Lib = 0
	np2Sym = nil
}

func np2Proc(name string) uintptr {
	if p, ok := np2Sym[name]; ok {
		return p
	}
	p, err := purego.Dlsym(np2Lib, name)
	if err != nil {
		return 0
	}
	np2Sym[name] = p
	return p
}

func corePath(root string) (string, error) {
	name := "np2kai_libretro.so"
	list := []string{
		filepath.Join(root, name),
		filepath.Join(root, "third_party", name),
		filepath.Join(root, "third_party", "linux-"+runtime.GOARCH, name),
	}
	for _, p := range list {
		st, err := os.Stat(p)
		if err != nil || st.IsDir() || st.Size() == 0 {
			continue
		}
		abs, err := filepath.Abs(p)
		if err != nil {
			return p, nil
		}
		return abs, nil
	}
	return "", fmt.Errorf("Neko Project is not next to the door")
}
