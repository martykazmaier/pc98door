//go:build !windows || !386

package main

import (
	"fmt"
	"io"
)

func playNP2(io.Writer, keyboard, string, string, string) error {
	return fmt.Errorf("the door runs Neko Project as a 32-bit program")
}
