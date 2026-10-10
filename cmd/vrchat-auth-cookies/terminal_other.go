//go:build !linux

package main

import "os"

// Automatic prompting is only enabled where a real terminal can be verified.
// Other platforms can opt in explicitly with -interactive.
func terminalInput(_ *os.File) bool {
	return false
}
