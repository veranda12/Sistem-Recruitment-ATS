//go:build windows

package cvparse

import (
	"os/exec"
	"strconv"
	"syscall"
)

func setProcAttr(cmd *exec.Cmd) {
	// Tanpa jendela konsol; proses baru dalam grup sendiri agar dapat dimatikan bersama anaknya.
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x00000200 | 0x08000000} // NEW_PROCESS_GROUP | CREATE_NO_WINDOW
}

// killTree mematikan proses beserta turunannya (soffice.exe meluncurkan soffice.bin).
func killTree(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	_ = exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(cmd.Process.Pid)).Run()
	return cmd.Process.Kill()
}
