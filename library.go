package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// A library is the door's on-disk layout, beside the executable:
//
//	bios/  firmware shared by every caller (bios.rom, font.rom, itf.rom, sound.rom)
//	roms/  disk images shared by every caller, opened read-only
//	save/  one subdirectory per caller; the name comes from door32.sys in the BBS working directory
type library struct {
	root string
	bios string
	roms string
	save string
}

// openLibrary creates bios, roms, and save under root when they are missing.
func openLibrary(root string) (*library, error) {
	l := &library{
		root: root,
		bios: filepath.Join(root, "bios"),
		roms: filepath.Join(root, "roms"),
		save: filepath.Join(root, "save"),
	}
	for _, dir := range []string{l.bios, l.roms, l.save} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
	}
	return l, nil
}

// exeLibrary is the library next to this program.
func exeLibrary() (*library, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	return openLibrary(filepath.Dir(exe))
}

// userDir is save/<name>, created if needed. name comes from door32.sys.
func (l *library) userDir(name string) (string, error) {
	dir := filepath.Join(l.save, safeUser(name))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

// safeUser makes the door32.sys user name usable as a single path component.
func safeUser(name string) string {
	name = strings.TrimSpace(name)
	var b strings.Builder
	for _, r := range name {
		switch r {
		case '\\', '/', ':', '*', '?', '"', '<', '>', '|':
			b.WriteByte('_')
		default:
			if r < 32 {
				b.WriteByte('_')
			} else {
				b.WriteRune(r)
			}
		}
	}
	s := strings.TrimRight(b.String(), ". ")
	if s == "" || s == "." || s == ".." {
		return "local"
	}
	if len(s) > 80 {
		s = strings.TrimRight(s[:80], ". ")
		if s == "" {
			return "local"
		}
	}
	if reservedName(s) {
		s += "_"
	}
	return s
}

func reservedName(name string) bool {
	switch strings.ToUpper(name) {
	case "CON", "PRN", "AUX", "NUL",
		"COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9",
		"LPT1", "LPT2", "LPT3", "LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9":
		return true
	}
	return false
}

// Firmware is the PC-9801 ROM set read from bios/. Missing files are nil.
type Firmware struct {
	BIOS  []byte
	Font  []byte
	ITF   []byte
	Sound []byte
}

// LoadFirmware reads bios.rom, font.rom, itf.rom, and sound.rom from bios/.
// Names are matched without regard to case.
func (l *library) LoadFirmware() (Firmware, error) {
	var fw Firmware
	err := filepath.WalkDir(l.bios, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		switch strings.ToLower(d.Name()) {
		case "bios.rom":
			fw.BIOS = data
		case "font.rom":
			fw.Font = data
		case "itf.rom":
			fw.ITF = data
		case "sound.rom":
			fw.Sound = data
		}
		return nil
	})
	return fw, err
}

// relROM cleans name into a path relative to roms/. Absolute paths and
// paths that leave roms/ are rejected.
func (l *library) relROM(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", errors.New("no disk image named")
	}
	cleaned := filepath.Clean(name)
	if filepath.IsAbs(cleaned) {
		rel, err := filepath.Rel(l.roms, cleaned)
		if err != nil || !inside(rel) {
			return "", fmt.Errorf("%s is outside the roms directory", name)
		}
		return rel, nil
	}
	if !inside(cleaned) {
		return "", fmt.Errorf("%s is outside the roms directory", name)
	}
	return cleaned, nil
}

func inside(rel string) bool {
	if rel == "" || rel == "." {
		return false
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return false
	}
	return true
}

// imageFile is a disk image. Shared images in roms/ stay unmodified; the
// first write copies the image into the caller's save directory.
type imageFile struct {
	lib    *library
	rel    string
	user   string
	f      *os.File
	shared bool
}

// OpenImage opens a disk from the caller's save directory, or from roms/
// when that caller has no private copy yet.
func (l *library) OpenImage(name, user string) (*imageFile, error) {
	rel, err := l.relROM(name)
	if err != nil {
		return nil, err
	}
	userDir, err := l.userDir(user)
	if err != nil {
		return nil, err
	}
	personal := filepath.Join(userDir, rel)
	if f, err := os.OpenFile(personal, os.O_RDWR, 0); err == nil {
		return &imageFile{lib: l, rel: rel, user: user, f: f}, nil
	}
	shared := filepath.Join(l.roms, rel)
	f, err := os.Open(shared)
	if err != nil {
		return nil, fmt.Errorf("cannot open %s: %w", rel, err)
	}
	return &imageFile{lib: l, rel: rel, user: user, f: f, shared: true}, nil
}

func (m *imageFile) ReadAt(p []byte, off int64) (int, error) {
	return m.f.ReadAt(p, off)
}

func (m *imageFile) Size() (int64, error) {
	st, err := m.f.Stat()
	if err != nil {
		return 0, err
	}
	return st.Size(), nil
}

func (m *imageFile) WriteAt(p []byte, off int64) (int, error) {
	if m.shared {
		if err := m.promote(); err != nil {
			return 0, err
		}
	}
	return m.f.WriteAt(p, off)
}

func (m *imageFile) Path() string {
	if m.f == nil {
		return ""
	}
	return m.f.Name()
}

// Private copies a shared disk into the caller's save directory.
func (m *imageFile) Private() error {
	if !m.shared {
		return nil
	}
	return m.promote()
}

func (m *imageFile) Close() error {
	if m.f == nil {
		return nil
	}
	err := m.f.Close()
	m.f = nil
	return err
}

// promote copies the shared image into the caller's save directory and
// continues on that private copy.
func (m *imageFile) promote() error {
	userDir, err := m.lib.userDir(m.user)
	if err != nil {
		return err
	}
	personal := filepath.Join(userDir, m.rel)
	if err := os.MkdirAll(filepath.Dir(personal), 0o755); err != nil {
		return err
	}
	dst, err := os.OpenFile(personal, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		if !errors.Is(err, os.ErrExist) {
			return err
		}
		dst, err = os.OpenFile(personal, os.O_RDWR, 0)
		if err != nil {
			return err
		}
	} else {
		if _, err := m.f.Seek(0, io.SeekStart); err != nil {
			dst.Close()
			os.Remove(personal)
			return err
		}
		if _, err := io.Copy(dst, m.f); err != nil {
			dst.Close()
			os.Remove(personal)
			return err
		}
	}
	m.f.Close()
	if _, err := dst.Seek(0, io.SeekStart); err != nil {
		dst.Close()
		return err
	}
	m.f = dst
	m.shared = false
	return nil
}

// bbsUser reads the caller's name from door32.sys in the directory the BBS
// was running in when it started this door.
func bbsUser() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return doorUser(dir)
}

// doorUser reads the caller's real name from line 6 of door32.sys. A directory
// argument is searched for that drop file and nowhere else.
func doorUser(path string) (string, error) {
	if st, err := os.Stat(path); err == nil && st.IsDir() {
		found, err := findDoor32(path)
		if err != nil {
			return "", err
		}
		path = found
	}
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	var lines []string
	for sc.Scan() && len(lines) < 6 {
		lines = append(lines, strings.TrimRight(sc.Text(), "\r"))
	}
	if err := sc.Err(); err != nil {
		return "", err
	}
	if len(lines) < 6 || strings.TrimSpace(lines[5]) == "" {
		return "", fmt.Errorf("%s has no user name on line 6", path)
	}
	return strings.TrimSpace(lines[5]), nil
}

// findDoor32 returns door32.sys in dir. The search is not recursive.
func findDoor32(dir string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	for _, e := range entries {
		if !e.IsDir() && strings.EqualFold(e.Name(), "door32.sys") {
			return filepath.Join(dir, e.Name()), nil
		}
	}
	return "", fmt.Errorf("door32.sys was not found in %s", dir)
}

// dropFile is the DOOR32.SYS information the menu needs.
type dropFile struct {
	Comm    int
	Handle  int64
	Name    string
	Minutes int
}

// readDrop reads door32.sys. A directory argument is searched for that file
// and nowhere else. Comm 0 is a local login. The name is line 6.
func readDrop(path string) (dropFile, error) {
	if st, err := os.Stat(path); err == nil && st.IsDir() {
		found, err := findDoor32(path)
		if err != nil {
			return dropFile{}, err
		}
		path = found
	}
	f, err := os.Open(path)
	if err != nil {
		return dropFile{}, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	var lines []string
	for sc.Scan() {
		lines = append(lines, strings.TrimSpace(sc.Text()))
	}
	if err := sc.Err(); err != nil {
		return dropFile{}, err
	}
	if len(lines) < 6 || lines[5] == "" {
		return dropFile{}, fmt.Errorf("%s has no user name on line 6", path)
	}
	comm, _ := strconv.Atoi(lines[0])
	if comm == 1 {
		return dropFile{}, errors.New("serial connections are not supported")
	}
	handle, _ := strconv.ParseInt(lines[1], 10, 64)
	var minutes int
	if len(lines) > 8 {
		minutes, _ = strconv.Atoi(lines[8])
	}
	return dropFile{Comm: comm, Handle: handle, Name: lines[5], Minutes: minutes}, nil
}
