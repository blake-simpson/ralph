//go:build windows

package main

import (
	"os"
	"os/exec"
	"os/signal"
)

// setSysProcAttr is a no-op on Windows (process groups use a different mechanism).
func setSysProcAttr(cmd *exec.Cmd) {}

// killProcessGroup terminates the process on Windows (no process group support).
func killProcessGroup(pid int) {
	if pid == 0 {
		return
	}
	p, err := os.FindProcess(pid)
	if err == nil {
		p.Kill()
	}
}

// signalProcessGroup terminates the process on Windows.
func signalProcessGroup(pgid int) {
	if pgid == 0 {
		return
	}
	p, err := os.FindProcess(pgid)
	if err == nil {
		p.Kill()
	}
}

// notifySignals registers the channel to receive os.Interrupt on Windows.
func notifySignals(ch chan<- os.Signal) {
	signal.Notify(ch, os.Interrupt)
}
