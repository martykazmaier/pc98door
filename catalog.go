package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
)

type diskTitle struct {
	Name  string
	Title string
}

var diskExt = map[string]bool{
	".hdi": true, ".nhd": true, ".hdm": true, ".hdn": true,
	".d88": true, ".fdi": true, ".tfd": true, ".nfd": true,
	".dup": true, ".img": true, ".dsk": true, ".hdd": true,
	".2hd": true, ".flp": true, ".xdf": true,
}

func (l *library) listDisks() ([]diskTitle, error) {
	entries, err := os.ReadDir(l.roms)
	if err != nil {
		return nil, err
	}
	var items []diskTitle
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		ext := strings.ToLower(filepath.Ext(name))
		if !diskExt[ext] {
			continue
		}
		items = append(items, diskTitle{Name: name, Title: strings.TrimSuffix(name, filepath.Ext(name))})
	}
	slices.SortFunc(items, func(a, b diskTitle) int {
		return strings.Compare(asciiFold(utf8ToCP437(a.Title)), asciiFold(utf8ToCP437(b.Title)))
	})
	return items, nil
}

func filterDisks(items []diskTitle, query string) []diskTitle {
	q := strings.TrimSpace(asciiFold(query))
	if q == "" {
		return items
	}
	words := strings.Fields(q)
	var out []diskTitle
	for _, it := range items {
		title := asciiFold(utf8ToCP437(it.Title))
		ok := true
		for _, w := range words {
			if !strings.Contains(title, w) {
				ok = false
				break
			}
		}
		if ok {
			out = append(out, it)
		}
	}
	return out
}
