package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	local := false
	for _, a := range args {
		if a == "-L" || a == "/L" || a == "-?" || a == "/?" {
			if a == "-?" || a == "/?" {
				fmt.Fprint(os.Stderr, "pc98door [-L]\n\nThe disk list is the first screen. Search, move the lightbar, and Enter loads that disk.\nbios, roms, and save sit beside this program.\n")
				return 0
			}
			local = true
		}
	}
	lib, err := exeLibrary()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	user := "Sysop"
	var handle uintptr
	if dir, err := os.Getwd(); err == nil {
		if path, err := findDoor32(dir); err == nil {
			drop, err := readDrop(path)
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				return 1
			}
			user = drop.Name
			if !local && drop.Comm != 0 && drop.Handle != 0 {
				handle = uintptr(drop.Handle)
			} else {
				local = true
			}
		} else {
			local = true
		}
	}
	var term doorTerm
	if local || handle == 0 {
		term, err = openConsole()
	} else {
		term, err = openDoorSocket(handle)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		logDoor(err)
		return 1
	}
	defer term.Close()
	disks, err := lib.listDisks()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	for {
		name, err := chooseDisk(term, term, user, disks)
		if err != nil {
			return 1
		}
		if name == "" {
			term.Write([]byte("\x1b[0m\x1b[?7h\r\n"))
			return 0
		}
		if err := runDisk(term, lib, user, name); err != nil {
			fmt.Fprintf(term, "\r\n%s\r\n", err.Error())
			term.WaitKey(-1)
		}
	}
}

func runDisk(term doorTerm, lib *library, user, name string) error {
	img, err := lib.OpenImage(name, user)
	if err != nil {
		return err
	}
	if err := img.Private(); err != nil {
		img.Close()
		return err
	}
	path := img.Path()
	img.Close()
	return playNP2(term, term, path, lib.root, lib.bios)
}

func logDoor(err error) {
	exe, e := os.Executable()
	if e != nil {
		return
	}
	f, e := os.OpenFile(filepath.Join(filepath.Dir(exe), "pc98door.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if e != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "%s %v\r\n", time.Now().Format("2006-01-02 15:04:05"), err)
}

type doorTerm interface {
	io.Writer
	keyboard
	Close() error
}
