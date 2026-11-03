package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSafeUser(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Ada Lovelace", "Ada Lovelace"},
		{"  Ada  ", "Ada"},
		{`Ada/Lovelace`, "Ada_Lovelace"},
		{`Ada\Lovelace`, "Ada_Lovelace"},
		{"con", "con_"},
		{"..", "local"},
		{"", "local"},
		{"Name.", "Name"},
	}
	for _, c := range cases {
		if got := safeUser(c.in); got != c.want {
			t.Errorf("safeUser(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestLibrarySeparatesFirmwareROMsAndUsers(t *testing.T) {
	root := t.TempDir()
	lib, err := openLibrary(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{lib.bios, lib.roms, lib.save} {
		if st, err := os.Stat(dir); err != nil || !st.IsDir() {
			t.Fatalf("%s missing: %v", dir, err)
		}
	}
	if err := os.WriteFile(filepath.Join(lib.bios, "BIOS.ROM"), []byte("bios-bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(lib.bios, "font.rom"), []byte("font-bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	fw, err := lib.LoadFirmware()
	if err != nil {
		t.Fatal(err)
	}
	if string(fw.BIOS) != "bios-bytes" || string(fw.Font) != "font-bytes" || fw.ITF != nil || fw.Sound != nil {
		t.Fatalf("firmware = bios %q font %q itf %v sound %v", fw.BIOS, fw.Font, fw.ITF, fw.Sound)
	}

	shared := []byte("SHARED-IMAGE-DATA")
	if err := os.MkdirAll(filepath.Join(lib.roms, "games"), 0o755); err != nil {
		t.Fatal(err)
	}
	rom := filepath.Join(lib.roms, "games", "demo.d88")
	if err := os.WriteFile(rom, shared, 0o644); err != nil {
		t.Fatal(err)
	}

	ada, err := lib.OpenImage(`games/demo.d88`, "Ada Lovelace")
	if err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, len(shared))
	if _, err := ada.ReadAt(buf, 0); err != nil {
		t.Fatal(err)
	}
	if string(buf) != string(shared) {
		t.Fatalf("first read = %q", buf)
	}
	if _, err := ada.WriteAt([]byte("USER"), 0); err != nil {
		t.Fatal(err)
	}
	ada.Close()

	got, err := os.ReadFile(rom)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(shared) {
		t.Fatalf("shared rom was modified: %q", got)
	}
	personal := filepath.Join(lib.save, "Ada Lovelace", "games", "demo.d88")
	got, err = os.ReadFile(personal)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(got), "USER") || string(got[4:]) != string(shared[4:]) {
		t.Fatalf("private copy = %q", got)
	}

	grace, err := lib.OpenImage("games/demo.d88", "Grace Hopper")
	if err != nil {
		t.Fatal(err)
	}
	defer grace.Close()
	if _, err := grace.ReadAt(buf, 0); err != nil {
		t.Fatal(err)
	}
	if string(buf) != string(shared) {
		t.Fatalf("second caller saw %q", buf)
	}
	if _, err := lib.OpenImage(`..\secret.d88`, "Ada Lovelace"); err == nil {
		t.Fatal("path outside roms was accepted")
	}
}

func TestDoorUserIsLine6(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "DOOR32.SYS")
	body := "0\r\n1234\r\n38400\r\nEleBBS\r\n1\r\nAda Lovelace\r\nAda\r\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	name, err := doorUser(dir)
	if err != nil {
		t.Fatal(err)
	}
	if name != "Ada Lovelace" {
		t.Fatalf("user = %q", name)
	}
}

func TestBBSUserComesFromWorkingDirectory(t *testing.T) {
	bbs := t.TempDir()
	elsewhere := t.TempDir()
	writeDrop := func(dir, name string) {
		t.Helper()
		body := "0\r\n99\r\n38400\r\nEleBBS\r\n7\r\n" + name + "\r\nHandle\r\n"
		if err := os.WriteFile(filepath.Join(dir, "door32.sys"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	writeDrop(bbs, "Grace Hopper")
	writeDrop(elsewhere, "Wrong Name")
	t.Chdir(bbs)
	name, err := bbsUser()
	if err != nil {
		t.Fatal(err)
	}
	if name != "Grace Hopper" {
		t.Fatalf("user = %q", name)
	}
	empty := filepath.Join(bbs, "empty")
	if err := os.Mkdir(empty, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := doorUser(empty); err == nil {
		t.Fatal("a directory without door32.sys was accepted")
	}
}
