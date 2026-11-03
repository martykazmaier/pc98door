//go:build linux

package main

import (
	"os"

	"golang.org/x/term"
)

// prepareConsole puts the local terminal in raw mode so keys arrive one at a
// time. A pipe or a file is left alone.
func prepareConsole() (func(), error) {
	fd := int(os.Stdin.Fd())
	old, err := term.MakeRaw(fd)
	if err != nil {
		return func() {}, nil
	}
	return func() { _ = term.Restore(fd, old) }, nil
}
