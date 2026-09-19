//go:build windows

package execx

import (
	"os/exec"
	"syscall"
)

const createNoWindow = 0x08000000

// Hide 让子进程不创建可见的控制台窗口。
// GUI 程序启动控制台子进程（git/cmd 等）时，默认会各弹一个黑窗，必须显式禁止。
func Hide(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: createNoWindow,
	}
}
