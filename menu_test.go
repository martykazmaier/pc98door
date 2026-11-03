package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestMenuSelectsDiskBySearch(t *testing.T) {
	disks := []diskTitle{
		{Name: "Alpha.hdi", Title: "Alpha"},
		{Name: "Aegean Kai No Shizuku [T-En by Svipur v1.0].hdi", Title: "Aegean Kai No Shizuku [T-En by Svipur v1.0]"},
		{Name: "Touhou 2.hdi", Title: "Touhou 2"},
	}
	got := filterDisks(disks, "aegean shizuku")
	if len(got) != 1 || got[0].Name != disks[1].Name {
		t.Fatalf("search = %+v", got)
	}
	keys := &scriptKeys{keys: []int{'a', 'e', 'g', keyEnter}}
	var buf bytes.Buffer
	name, err := chooseDisk(&buf, keys, "Ada Lovelace", disks)
	if err != nil {
		t.Fatal(err)
	}
	if name != disks[1].Name {
		t.Fatalf("selected %q", name)
	}
	if !bytes.Contains(buf.Bytes(), []byte{0xC9}) {
		t.Fatal("menu is not CP437")
	}
	if bytes.Contains(buf.Bytes(), []byte{0xE2, 0x95, 0x94}) {
		t.Fatal("menu used UTF-8 box drawing")
	}
	if !bytes.Contains(buf.Bytes(), []byte("Aegean")) {
		t.Fatal("menu omitted the matching title")
	}
	if !bytes.Contains(buf.Bytes(), []byte("Ctrl-Q quits the game")) {
		t.Fatal("status omits Ctrl-Q")
	}
}

func TestLightbarHomeEndAndPages(t *testing.T) {
	disks := make([]diskTitle, 40)
	for i := range disks {
		disks[i] = diskTitle{Name: fmt.Sprintf("%02d.hdi", i), Title: fmt.Sprintf("Title %02d", i)}
	}
	p := &picker{all: disks}
	p.handle(keyEnd)
	if p.sel != 39 || p.top != 21 {
		t.Fatalf("End sel=%d top=%d", p.sel, p.top)
	}
	p.handle(keyHome)
	if p.sel != 0 || p.top != 0 {
		t.Fatalf("Home sel=%d top=%d", p.sel, p.top)
	}
	p.handle(keyPgDn)
	if p.sel != 18 || p.top != 0 {
		t.Fatalf("first PgDn sel=%d top=%d", p.sel, p.top)
	}
	p.handle(keyPgDn)
	if p.sel != 37 || p.top != 19 {
		t.Fatalf("second PgDn sel=%d top=%d", p.sel, p.top)
	}
	p.handle(keyPgUp)
	if p.sel != 19 || p.top != 19 {
		t.Fatalf("PgUp to page top sel=%d top=%d", p.sel, p.top)
	}
	p.handle(keyPgUp)
	if p.sel != 0 || p.top != 0 {
		t.Fatalf("PgUp page sel=%d top=%d", p.sel, p.top)
	}
	p.handle(keyDown)
	frame := p.frame()
	bar := []byte("\x1b[1;37;44m Title 01")
	if !bytes.Contains(frame, bar) {
		t.Fatal("the current row is not a lightbar")
	}
	if bytes.Count(frame, []byte("\x1b[1;37;44m")) != 1 {
		t.Fatal("more than one lightbar is drawn")
	}
}

func TestListDisksSkipsFirmware(t *testing.T) {
	root := t.TempDir()
	lib, err := openLibrary(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"bios.rom", "Demo Game.hdi", "notes.txt"} {
		if err := os.WriteFile(filepath.Join(lib.roms, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	items, err := lib.listDisks()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Title != "Demo Game" {
		t.Fatalf("disks = %+v", items)
	}
}

type scriptKeys struct{ keys []int }

func (s *scriptKeys) WaitKey(time.Duration) (int, bool, error) {
	if len(s.keys) == 0 {
		return 0, false, io.EOF
	}
	k := s.keys[0]
	s.keys = s.keys[1:]
	return k, true, nil
}
