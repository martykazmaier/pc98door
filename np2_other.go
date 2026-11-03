//go:build !((windows && 386) || (linux && (386 || amd64 || arm64)))

package main

import (
	"fmt"
	"io"
)

func playNP2(io.Writer, keyboard, string, string, string) error {
	return fmt.Errorf("Neko Project is not built for this system")
}
