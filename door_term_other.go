//go:build !windows && !linux

package main

func prepareConsole() (func(), error) {
	return func() {}, nil
}
