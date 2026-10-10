//go:build linux

package main

import (
	"os"
	"runtime"
	"syscall"
	"unsafe"
)

func terminalInput(file *os.File) bool {
	var termios syscall.Termios
	_, _, errno := syscall.Syscall(
		syscall.SYS_IOCTL,
		file.Fd(),
		uintptr(syscall.TCGETS),
		uintptr(unsafe.Pointer(&termios)),
	)
	runtime.KeepAlive(file)
	return errno == 0
}
