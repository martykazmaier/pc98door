package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOtherDisksLeaveDos(t *testing.T) {
	dir := `\\bbs\ele\doors\pc98\roms`
	// Names above may not match exactly. Use the directory listing.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"Touhou 1", "Rance III", "Yu-No", "Dragon Knight [", "Alice's Cottage ["}
	var paths []string
	for _, w := range want {
		for _, e := range entries {
			if strings.Contains(e.Name(), w) && strings.HasSuffix(strings.ToLower(e.Name()), ".hdi") {
				paths = append(paths, filepath.Join(dir, e.Name()))
				break
			}
		}
	}
	font, err := os.ReadFile(`\\bbs\ele\doors\pc98\bios\font.rom`)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range paths {
		d, err := openHDI(path)
		if err != nil {
			t.Fatal(err)
		}
		m := bootMachine(d)
		m.font = font
		var steps int
		for steps = 0; steps < 800000 && !m.cpu.halted; steps++ {
			svc := m.step()
			if svc == svcPause || (svc == svcKey && byte(m.cpu.ax>>8) == 0 && len(m.keys) == 0) {
				break
			}
			if svc != svcNone {
				m.service(svc)
			}
		}
		planes := 0
		for _, b := range m.cpu.ram[0xA8000:0xE0000] {
			if b != 0 {
				planes++
			}
		}
		page := m.textPage()
		if i := strings.Index(page, "MS-DOS"); i >= 0 {
			page = page[i:]
		}
		if len(page) > 180 {
			page = page[:180]
		}
		t.Logf("%s steps=%d halt=%v fault=%q gfx=%d child=%v", filepath.Base(path)[:28], steps, m.cpu.halted, m.cpu.lastFault, planes, m.child != nil)
		for _, line := range m.dosLog {
			t.Log(" ", line)
		}
		t.Logf(" page %q", page)
		d.Close()
	}
}
