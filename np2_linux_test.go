//go:build linux && (386 || amd64 || arm64)

package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestNP2Paints(t *testing.T) {
	bios := os.Getenv("NP2_BIOS")
	disk := os.Getenv("NP2_DISK")
	if bios == "" || disk == "" {
		t.Skip("NP2_BIOS and NP2_DISK are not set")
	}
	root := t.TempDir()
	src := filepath.Join("third_party", "linux-"+runtime.GOARCH, "np2kai_libretro.so")
	b, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "np2kai_libretro.so"), b, 0o755); err != nil {
		t.Fatal(err)
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := loadNP2(disk, root, bios); err != nil {
		t.Fatal(err)
	}
	defer unloadNP2()
	for i := 0; i < 200; i++ {
		call0(np2Run)
	}
	pix := np2RGBA()
	maxC := 0
	for _, c := range pix {
		if int(c) > maxC {
			maxC = int(c)
		}
	}
	t.Logf("%s %dx%d format %d max %d", filepath.Base(disk), np2W, np2H, np2Pix, maxC)
	if np2W != 640 || np2H != 400 {
		t.Fatalf("frame %dx%d", np2W, np2H)
	}
	if maxC < 250 {
		t.Fatalf("picture is black, max channel %d", maxC)
	}
}
