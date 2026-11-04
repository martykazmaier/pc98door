//go:build windows && 386

package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestNP2Paints(t *testing.T) {
	root := t.TempDir()
	src := `C:\Users\Owner\.cursor\projects\pc98\third_party\np2kai_libretro.dll`
	b, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "np2kai_libretro.dll"), b, 0o644); err != nil {
		t.Fatal(err)
	}
	roms := `\\bbs\ele\doors\pc98\roms`
	entries, err := os.ReadDir(roms)
	if err != nil {
		t.Fatal(err)
	}
	var hdi string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "Aegean") {
			hdi = filepath.Join(roms, e.Name())
			break
		}
	}
	if hdi == "" {
		t.Fatal("no disk")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := loadNP2(hdi, root, `\\bbs\ele\doors\pc98\bios`); err != nil {
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
	t.Logf("%s %dx%d format %d max %d", filepath.Base(hdi), np2W, np2H, np2Pix, maxC)
	if np2W != 640 || np2H != 400 {
		t.Fatalf("frame %dx%d", np2W, np2H)
	}
	if maxC < 250 {
		t.Fatalf("picture is black, max channel %d", maxC)
	}
	font, err := loadANK(`\\bbs\ele\doors\pc98\bios`)
	if err != nil {
		t.Fatal(err)
	}
	view := frameView{raw: np2Frame, width: np2W, height: np2H, pitch: np2Pitch, pix: np2Pix}
	var text []string
	for row := 0; row < np2H/16; row++ {
		line := strings.TrimRight(readRow(font, view, row, 80), " ")
		if line != "" {
			text = append(text, line)
		}
	}
	joined := strings.Join(text, "\n")
	t.Logf("text:\n%s", joined)
	if !strings.Contains(joined, "CPU") || !strings.Contains(joined, "MEMORY") {
		t.Fatal("boot text did not match the font")
	}
}
