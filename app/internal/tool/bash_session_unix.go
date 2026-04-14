//go:build !windows

package tool

import "syscall"

const sigTerm = syscall.SIGTERM
const sigKill = syscall.SIGKILL

// killPID sends signal sig to the process with the given PID.
func killPID(pid int, sig syscall.Signal) {
	_ = syscall.Kill(pid, sig)
}

// killPGID sends signal sig to the process group with the given PGID.
// Passing a negative PID to Kill targets the process group.
func killPGID(pgid int, sig syscall.Signal) {
	_ = syscall.Kill(-pgid, sig)
}
