//go:build windows

package update

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"vibe-manager/internal/execx"
)

// RelaunchDetached 安排「等本进程退出之后再启动 exe」。
//
// 必须等退出：程序带单实例锁，旧进程还活着时新进程会立刻自己退出。
// 所以这里起一个隐藏的 PowerShell 轮询本进程，消失之后再拉起。
func RelaunchDetached(exePath string, pid int, wait time.Duration) error {
	script := fmt.Sprintf(`$ErrorActionPreference = 'SilentlyContinue'
$deadline = (Get-Date).AddSeconds(%d)
while ((Get-Process -Id %d -ErrorAction SilentlyContinue) -and ((Get-Date) -lt $deadline)) {
    Start-Sleep -Milliseconds 300
}
Start-Sleep -Milliseconds 400
Start-Process -FilePath '%s' -WorkingDirectory '%s'
Remove-Item -LiteralPath $MyInvocation.MyCommand.Path -Force
`, int(wait.Seconds()), pid, psQuote(exePath), psQuote(filepath.Dir(exePath)))

	f, err := os.CreateTemp("", "vibe-manager-relaunch-*.ps1")
	if err != nil {
		return fmt.Errorf("无法创建重启脚本：%v", err)
	}
	path := f.Name()
	if _, err := f.WriteString(script); err != nil {
		_ = f.Close()
		_ = os.Remove(path)
		return fmt.Errorf("无法写入重启脚本：%v", err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(path)
		return fmt.Errorf("无法写入重启脚本：%v", err)
	}

	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive",
		"-ExecutionPolicy", "Bypass", "-WindowStyle", "Hidden", "-File", path)
	execx.Hide(cmd)
	if err := cmd.Start(); err != nil {
		_ = os.Remove(path)
		return fmt.Errorf("无法安排自动重启：%v", err)
	}
	return nil
}

// psQuote 把路径塞进 PowerShell 的单引号字符串里。
func psQuote(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}
