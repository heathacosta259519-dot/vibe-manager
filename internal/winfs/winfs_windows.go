//go:build windows

package winfs

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

type shFileOpStructW struct {
	hwnd                  uintptr
	wFunc                 uint32
	pFrom                 *uint16
	pTo                   *uint16
	fFlags                uint16
	fAnyOperationsAborted int32
	hNameMappings         uintptr
	lpszProgressTitle     *uint16
}

const (
	foDelete     = 3
	fofSilent    = 0x0004
	fofNoConfirm = 0x0010
	fofAllowUndo = 0x0040
	fofNoErrorUI = 0x0400
)

// longPrefix 让 Win32 接受超过 MAX_PATH(260) 的路径。
const longPrefix = `\\?\`

var (
	shell32             = syscall.NewLazyDLL("shell32.dll")
	procSHFileOperation = shell32.NewProc("SHFileOperationW")
)

// Recycle 把文件或目录移入 Windows 回收站（可恢复），不会永久删除。
// 注意：回收站接口不支持超过 MAX_PATH 的路径，那种情况会失败并报 145/206。
func Recycle(path string) error {
	from, err := syscall.UTF16FromString(path)
	if err != nil {
		return fmt.Errorf("路径无效：%v", err)
	}
	from = append(from, 0) // SHFileOperationW 要求双 NUL 结尾

	op := shFileOpStructW{
		wFunc:  foDelete,
		pFrom:  &from[0],
		fFlags: fofSilent | fofNoConfirm | fofAllowUndo | fofNoErrorUI,
	}
	ret, _, _ := procSHFileOperation.Call(uintptr(unsafe.Pointer(&op)))
	if ret != 0 {
		return fmt.Errorf("移入回收站失败：%s（错误码 %d，%s）", path, ret, explain(uint32(ret)))
	}
	if op.fAnyOperationsAborted != 0 {
		return fmt.Errorf("移入回收站被中止：%s", path)
	}
	return nil
}

// RemoveLong 永久删除（不进回收站），用 \\?\ 前缀自底向上删，可处理超长路径。
// 这是给「回收站接口搞不定的极端目录」留的出口，调用方必须先取得用户明确确认。
func RemoveLong(path string) error {
	if path == "" {
		return fmt.Errorf("路径为空")
	}
	return removeTree(path)
}

// removeTree 自底向上删除，用显式栈而不是递归：
// 「路径炸弹」可能嵌套上万层，递归会爆栈（Windows 自带的 rmdir 就是这么崩的，0xC00000FD）。
func removeTree(p string) error {
	fi, err := os.Lstat(longPrefix + p)
	if err != nil {
		return err
	}
	if !fi.IsDir() {
		return os.Remove(longPrefix + p)
	}

	dirs := make([]string, 0, 64)
	stack := []string{p}
	for len(stack) > 0 {
		d := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		dirs = append(dirs, d)

		entries, err := os.ReadDir(longPrefix + d)
		if err != nil {
			return err
		}
		for _, e := range entries {
			child := d + `\` + e.Name()
			if e.IsDir() {
				stack = append(stack, child)
				continue
			}
			if err := os.Remove(longPrefix + child); err != nil {
				return err
			}
		}
	}

	// dirs 是先序（父在子前），倒着删才是子先于父
	for i := len(dirs) - 1; i >= 0; i-- {
		if err := os.Remove(longPrefix + dirs[i]); err != nil {
			return err
		}
	}
	return nil
}

// explain 把 SHFileOperation 常见的错误码翻译成人话，便于定位。
func explain(code uint32) string {
	switch code {
	case 2, 0x402:
		return "找不到该项目，可能已被移动或删除"
	case 3:
		return "路径不存在"
	case 5, 0x71:
		return "拒绝访问，可能被其它程序占用或权限不足"
	case 32:
		return "文件正被其它程序使用"
	case 145:
		return "目录非空或嵌套过深，超出回收站接口的能力"
	case 206:
		return "路径过长，超出 Windows 回收站接口的限制"
	case 0x7C:
		return "名称或路径无效"
	case 0x78:
		return "目标位置访问被拒绝"
	}
	return "未知原因"
}
