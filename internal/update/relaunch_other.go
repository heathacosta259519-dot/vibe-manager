//go:build !windows

package update

import (
	"errors"
	"time"
)

// RelaunchDetached 只在 Windows 上有实现：换掉运行中的 exe、随后自动重启
// 这些动作依赖 Windows 的文件语义，其它平台不做。
func RelaunchDetached(exePath string, pid int, wait time.Duration) error {
	return errors.New("自动更新只支持 Windows")
}
