package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

const (
	bbsRoot   = `\\bbs\ele\doors\pc98`
	aegeanHDI = `Aegean Kai No Shizuku [T-En by Svipur v1.0].hdi`
)

func TestAegeanDisk(t *testing.T) {
	path := filepath.Join(bbsRoot, "roms", aegeanHDI)
	img, err := openHDI(path)
	if err != nil {
		t.Fatal(err)
	}
	defer img.Close()

	if img.SectorSize != 256 || img.Sectors != 33 || img.Heads != 8 || img.Cylinders != 310 {
		t.Fatalf("geometry = %d-byte sectors, %d x %d x %d", img.SectorSize, img.Sectors, img.Heads, img.Cylinders)
	}
	if img.SectorCount() != 33*8*310 {
		t.Fatalf("sector count %d", img.SectorCount())
	}

	boot, err := img.ReadSector(0)
	if err != nil {
		t.Fatal(err)
	}
	if boot[0] != 0xEB || !bytes.Contains(boot, []byte("IPL1")) {
		t.Fatalf("sector 0 is not an IPL: %x", boot[:32])
	}
	ipl := make([]byte, 2048)
	if _, err := img.ReadAt(ipl, 0); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(ipl, []byte("MS-DOS 6.20")) {
		t.Fatal("boot area has no MS-DOS 6.20 signature")
	}

	for _, n := range []int64{0, 1, 33, img.SectorCount() / 2, img.SectorCount() - 1} {
		sec, err := img.ReadSector(n)
		if err != nil {
			t.Fatalf("sector %d: %v", n, err)
		}
		if len(sec) != 256 {
			t.Fatalf("sector %d length %d", n, len(sec))
		}
		if n == 0 && bytes.Equal(sec, make([]byte, 256)) {
			t.Fatal("boot sector is empty")
		}
	}

	bios := filepath.Join(bbsRoot, "bios")
	for _, name := range []string{"bios.rom", "font.rom", "itf.rom", "sound.rom"} {
		st, err := os.Stat(filepath.Join(bios, name))
		if err != nil {
			t.Fatal(err)
		}
		if st.Size() == 0 {
			t.Fatalf("%s is empty", name)
		}
	}
}
