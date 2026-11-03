package main

import (
	"os"
	"strings"
	"testing"
)

func TestAegeanBootShowsDos(t *testing.T) {
	const path = `\\bbs\ele\doors\pc98\roms\Aegean Kai No Shizuku [T-En by Svipur v1.0].hdi`
	d, err := openHDI(path)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	m := bootMachine(d)
	font, err := os.ReadFile(`\\bbs\ele\doors\pc98\bios\font.rom`)
	if err != nil {
		t.Fatal(err)
	}
	m.font = font
	for i := 0; i < 5000000 && !m.cpu.halted; i++ {
		svc := m.step()
		if svc == svcPause || (svc == svcKey && byte(m.cpu.ax>>8) == 0 && len(m.keys) == 0) {
			break
		}
		if svc != svcNone {
			m.service(svc)
		}
	}
	if m.reads < 20 {
		t.Fatalf("disk reads = %d, the image did not load", m.reads)
	}
	page := m.textPage()
	if !strings.Contains(page, "MS-DOS") {
		t.Fatalf("text screen missing MS-DOS:\n%s", page)
	}
	blob, err := m.jxlBlob()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(blob), "SyncTERM:C;DrawJXLBlob;") {
		t.Fatal("frame is not a DrawJXLBlob")
	}
	q := string(jxlQuery())
	if !strings.Contains(q, "SyncTERM:Q;JXL") {
		t.Fatal("JXL query was not formed")
	}
}

func TestDosContinues(t *testing.T) {
	const path = `\\bbs\ele\doors\pc98\roms\Aegean Kai No Shizuku [T-En by Svipur v1.0].hdi`
	d, err := openHDI(path)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	m := bootMachine(d)
	var steps int
	for steps = 0; steps < 2000000 && !m.cpu.halted; steps++ {
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
	started := false
	for _, line := range m.dosLog {
		if strings.HasPrefix(line, "run ") {
			started = true
		}
	}
	if !started {
		t.Fatalf("the disk did not start its program: %v", m.dosLog)
	}
	if m.cpu.lastFault != "" {
		t.Fatalf("stopped at %04X:%04X %s", m.cpu.cs, m.cpu.ip, m.cpu.lastFault)
	}
	if planes < 1000 {
		t.Fatalf("graphics planes have %d bytes set after %d steps", planes, steps)
	}
}
