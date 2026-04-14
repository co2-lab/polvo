//go:build windows

package tool

import "syscall"

const sigTerm = syscall.Signal(0x1) // SIGHUP as approximation
const sigKill = syscall.Signal(0x9) // SIGKILL

func killPID(pid int, sig syscall.Signal) {
	// Windows does not support POSIX signals in the same way.
	// Signal sending is a no-op on Windows for persistent bash sessions.
}

func killPGID(pgid int, sig syscall.Signal) {
	// No-op on Windows.
}
