//go:build linux

package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTerminalInputRejectsNonTTYInputs(t *testing.T) {
	devNull, err := os.Open("/dev/null")
	if err != nil {
		t.Fatal(err)
	}
	defer devNull.Close()
	if terminalInput(devNull) {
		t.Fatal("/dev/null was incorrectly classified as a terminal")
	}

	readPipe, writePipe, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer readPipe.Close()
	defer writePipe.Close()
	if terminalInput(readPipe) {
		t.Fatal("pipe was incorrectly classified as a terminal")
	}

	regular, err := os.OpenFile(filepath.Join(t.TempDir(), "input"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer regular.Close()
	if terminalInput(regular) {
		t.Fatal("regular file was incorrectly classified as a terminal")
	}
}

func TestTerminalInputAcceptsPTY(t *testing.T) {
	ptmx, err := os.OpenFile("/dev/ptmx", os.O_RDWR, 0)
	if err != nil {
		t.Skipf("open /dev/ptmx: %v", err)
	}
	defer ptmx.Close()
	if !terminalInput(ptmx) {
		t.Fatal("PTY master was not classified as a terminal")
	}
}
