package main

import "strings"

// volume is the FAT filesystem stored in an Anex86 hard-disk image.
// PC-98 disks keep a 256-byte physical sector and a larger logical sector.
type volume struct {
	base    int64
	bps     int
	spc     int
	rootSec int
	rootEnt int
	dataSec int
	fat     []byte
	fat16   bool
}

type dirEntry struct {
	name    string
	attr    byte
	cluster int
	size    int
}

func (m *machine) mount() *volume {
	if m.vol != nil {
		return m.vol
	}
	if m.disk == nil {
		return nil
	}
	phys := m.disk.SectorSize
	if phys == 0 {
		return nil
	}
	buf := make([]byte, 64)
	limit := int(m.disk.SectorCount())
	if limit > 4000 {
		limit = 4000
	}
	for sec := 0; sec < limit; sec++ {
		if _, err := m.disk.ReadAt(buf, int64(sec*phys)); err != nil {
			break
		}
		if buf[0] != 0xEB && buf[0] != 0xE9 {
			continue
		}
		bps := int(buf[11]) | int(buf[12])<<8
		spc := int(buf[13])
		if (bps != 256 && bps != 512 && bps != 1024) || spc == 0 || spc&(spc-1) != 0 {
			continue
		}
		fats := int(buf[16])
		spf := int(buf[22]) | int(buf[23])<<8
		root := int(buf[17]) | int(buf[18])<<8
		media := buf[21]
		if fats < 1 || fats > 2 || spf == 0 || root == 0 || media < 0xF0 {
			continue
		}
		reserved := int(buf[14]) | int(buf[15])<<8
		if reserved == 0 {
			reserved = 1
		}
		v := &volume{
			base:    int64(sec * phys),
			bps:     bps,
			spc:     spc,
			rootEnt: root,
		}
		v.rootSec = reserved + fats*spf
		rootSecs := (root*32 + bps - 1) / bps
		v.dataSec = v.rootSec + rootSecs
		tot := int(buf[19]) | int(buf[20])<<8
		if tot == 0 {
			tot = int(buf[32]) | int(buf[33])<<8 | int(buf[34])<<16 | int(buf[35])<<24
		}
		clusters := 0
		if spc > 0 && tot > v.dataSec {
			clusters = (tot - v.dataSec) / spc
		}
		v.fat16 = clusters >= 4085
		fat, err := v.read(m.disk, reserved, spf*bps)
		// PC-98 marks the first FAT byte with FEh rather than the BPB media byte.
		if err != nil || len(fat) < 3 || fat[0] < 0xF0 {
			continue
		}
		v.fat = fat
		m.vol = v
		if m.cwd == "" {
			m.cwd = `\`
		}
		return v
	}
	return nil
}

func (v *volume) read(disk *hdiImage, sec, n int) ([]byte, error) {
	buf := make([]byte, n)
	_, err := disk.ReadAt(buf, v.base+int64(sec)*int64(v.bps))
	return buf, err
}

func (v *volume) next(c int) int {
	if v.fat16 {
		o := c * 2
		if o+1 >= len(v.fat) {
			return 0xFFFF
		}
		return int(v.fat[o]) | int(v.fat[o+1])<<8
	}
	o := c + c/2
	if o+1 >= len(v.fat) {
		return 0xFFF
	}
	x := int(v.fat[o]) | int(v.fat[o+1])<<8
	if c&1 == 1 {
		return x >> 4
	}
	return x & 0x0FFF
}

func (v *volume) live(c int) bool {
	if c < 2 {
		return false
	}
	if v.fat16 {
		return c < 0xFFF8
	}
	return c < 0xFF8
}

func (v *volume) readChain(disk *hdiImage, cluster, max int) []byte {
	var out []byte
	cs := v.spc * v.bps
	for n := 0; v.live(cluster) && len(out) < max && n < 4096; n++ {
		sec := v.dataSec + (cluster-2)*v.spc
		b, err := v.read(disk, sec, cs)
		if err != nil {
			break
		}
		out = append(out, b...)
		cluster = v.next(cluster)
	}
	if len(out) > max {
		out = out[:max]
	}
	return out
}

func (v *volume) list(disk *hdiImage, cluster int) []dirEntry {
	var raw []byte
	if cluster == 0 {
		raw, _ = v.read(disk, v.rootSec, v.rootEnt*32)
	} else {
		raw = v.readChain(disk, cluster, 1<<16)
	}
	var ents []dirEntry
	for i := 0; i+32 <= len(raw); i += 32 {
		e := raw[i : i+32]
		if e[0] == 0 {
			break
		}
		if e[0] == 0xE5 || e[11] == 0x0F || e[11]&0x08 != 0 {
			continue
		}
		name := strings.TrimRight(string(e[:8]), " ")
		ext := strings.TrimRight(string(e[8:11]), " ")
		if ext != "" {
			name += "." + ext
		}
		ents = append(ents, dirEntry{
			name:    strings.ToUpper(name),
			attr:    e[11],
			cluster: int(e[26]) | int(e[27])<<8,
			size:    int(e[28]) | int(e[29])<<8 | int(e[30])<<16 | int(e[31])<<24,
		})
	}
	return ents
}

func (m *machine) abs(path string) string {
	if m.cwd == "" {
		m.cwd = `\`
	}
	path = strings.ToUpper(strings.ReplaceAll(path, "/", `\`))
	path = strings.TrimSpace(path)
	if len(path) >= 2 && path[1] == ':' {
		path = path[2:]
	}
	if path == "" {
		return m.cwd
	}
	full := path
	if path[0] != '\\' {
		if m.cwd == `\` {
			full = `\` + path
		} else {
			full = m.cwd + `\` + path
		}
	}
	var parts []string
	for _, p := range strings.Split(full, `\`) {
		switch p {
		case "", ".":
		case "..":
			if len(parts) > 0 {
				parts = parts[:len(parts)-1]
			}
		default:
			parts = append(parts, p)
		}
	}
	if len(parts) == 0 {
		return `\`
	}
	return `\` + strings.Join(parts, `\`)
}

func (m *machine) lookup(path string) (dirEntry, bool) {
	v := m.mount()
	if v == nil {
		return dirEntry{}, false
	}
	path = m.abs(path)
	if path == `\` {
		return dirEntry{name: `\`, attr: 0x10}, true
	}
	cluster := 0
	var ent dirEntry
	for i, part := range strings.Split(path[1:], `\`) {
		found := false
		for _, e := range v.list(m.disk, cluster) {
			if e.name == part {
				ent = e
				found = true
				break
			}
		}
		if !found {
			return dirEntry{}, false
		}
		if i != strings.Count(path[1:], `\`) {
			if ent.attr&0x10 == 0 {
				return dirEntry{}, false
			}
			cluster = ent.cluster
		}
	}
	return ent, true
}

func (m *machine) chdir(path string) bool {
	ent, ok := m.lookup(path)
	if !ok || ent.attr&0x10 == 0 {
		return false
	}
	m.cwd = m.abs(path)
	return true
}

func (m *machine) openPath(path string) ([]byte, bool) {
	ent, ok := m.lookup(path)
	if !ok || ent.attr&0x10 != 0 {
		return nil, false
	}
	if ent.size == 0 || ent.cluster < 2 {
		return []byte{}, true
	}
	v := m.mount()
	return v.readChain(m.disk, ent.cluster, ent.size), true
}
