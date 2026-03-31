//go:build !windows

package main

import (
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"
)

// setSysProcAttr configures the command to run in its own process group.
func setSysProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// killProcessGroup sends SIGTERM then SIGKILL to the process group led by pid.
func killProcessGroup(pid int) {
	if pid == 0 {
		return
	}
	syscall.Kill(-pid, syscall.SIGTERM)
	time.Sleep(500 * time.Millisecond)
	syscall.Kill(-pid, syscall.SIGKILL)
}

// signalProcessGroup sends SIGTERM to the process group led by pgid.
func signalProcessGroup(pgid int) {
	if pgid != 0 {
		syscall.Kill(-pgid, syscall.SIGTERM)
	}
}

// notifySignals registers the channel to receive SIGINT and SIGTERM.
func notifySignals(ch chan<- os.Signal) {
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
}
