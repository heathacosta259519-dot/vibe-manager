//go:build !windows

package winfs

import "os"

// Recycle 在非 Windows 平台退化为直接删除（这些平台没有统一回收站接口）。
func Recycle(path string) error {
	return os.RemoveAll(path)
}

// RemoveLong 在非 Windows 平台上与 Recycle 等价。
func RemoveLong(path string) error {
	return os.RemoveAll(path)
}
