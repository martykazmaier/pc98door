package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFocusDisks(t *testing.T) {
	dir := `\\bbs\ele\doors\pc98\roms`
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"Dead of the Brain -", "Dead of the Brain 2", "Alice's Cottage [", "Alice's Cottage II"}
	font, err := os.ReadFile(`\\bbs\ele\doors\pc98\bios\font.rom`)
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range want {
		var path string
		for _, e := range entries {
			if strings.Contains(e.Name(), w) && strings.HasSuffix(strings.ToLower(e.Name()), ".hdi") {
				path = filepath.Join(dir, e.Name())
				break
			}
		}
		if path == "" {
			t.Errorf("missing %s", w)
			continue
		}
		d, err := openHDI(path)
		if err != nil {
			t.Fatal(err)
		}
		m := bootMachine(d)
		if text, ok := m.openPath(`\AUTOEXEC.BAT`); ok {
			t.Logf("AUTOEXEC %s\n%s", w, string(text))
		} else {
			t.Logf("no AUTOEXEC %s", w)
		}
		if text, ok := m.openPath(`\CONFIG.SYS`); ok {
			s := string(text)
			if len(s) > 400 {
				s = s[:400]
			}
			t.Logf("CONFIG %s\n%s", w, s)
		}
		m.font = font
		var steps int
		for steps = 0; steps < 1200000 && !m.cpu.halted; steps++ {
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
		lin := (uint32(m.cpu.cs)<<4 + uint32(m.cpu.ip)) & 0xFFFFF
		var raw [16]byte
		for i := range raw {
			raw[i] = m.cpu.read8(lin + uint32(i))
		}
		page := m.textPage()
		if len(page) > 240 {
			page = page[:240]
		}
		t.Logf("%s steps=%d halt=%v fault=%q at %04X:%04X % X gfx=%d child=%v own=%v",
			w, steps, m.cpu.halted, m.cpu.lastFault, m.cpu.cs, m.cpu.ip, raw[:], planes, m.child != nil, m.ownDOS)
		for _, line := range m.dosLog {
			t.Log(" ", line)
		}
		t.Logf(" page %q", page)
		d.Close()
	}
}

func (m *machine) textHead() string {
	return fmt.Sprintf("%04X:%04X", m.cpu.cs, m.cpu.ip)
}
