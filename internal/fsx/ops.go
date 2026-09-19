package fsx

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"vibe-manager/internal/winfs"
)

// ErrExists 表示目标已存在，前端据此弹出「覆盖 / 保留两者 / 取消」。
var ErrExists = errors.New("目标已存在")

const (
	PolicyFail      = "fail"
	PolicyOverwrite = "overwrite"
	PolicyRename    = "rename"
)

// removeFileFunc 默认把文件送进回收站；单测会替换它，避免真的往回收站塞文件。
var removeFileFunc = winfs.Recycle

// SetRemoveFunc 替换删除实现并返回旧实现，仅供测试使用。
func SetRemoveFunc(fn func(string) error) func(string) error {
	old := removeFileFunc
	removeFileFunc = fn
	return old
}

var reservedNames = map[string]bool{
	"CON": true, "PRN": true, "AUX": true, "NUL": true,
	"COM1": true, "COM2": true, "COM3": true, "COM4": true, "COM5": true,
	"COM6": true, "COM7": true, "COM8": true, "COM9": true,
	"LPT1": true, "LPT2": true, "LPT3": true, "LPT4": true, "LPT5": true,
	"LPT6": true, "LPT7": true, "LPT8": true, "LPT9": true,
}

// ValidateName 校验单个文件/文件夹名是否合法（不含路径分隔符与 Windows 保留名）。
func ValidateName(name string) error {
	if name == "" || name == "." || name == ".." {
		return fmt.Errorf("名称无效")
	}
	if strings.ContainsAny(name, `\/:*?"<>|`) {
		return fmt.Errorf(`名称不能包含 \ / : * ? " < > |`)
	}
	if strings.HasSuffix(name, ".") || strings.HasSuffix(name, " ") {
		return fmt.Errorf("名称不能以空格或点结尾")
	}
	base := name
	if i := strings.IndexByte(name, '.'); i > 0 {
		base = name[:i]
	}
	if reservedNames[strings.ToUpper(base)] {
		return fmt.Errorf("%s 是 Windows 保留名称", name)
	}
	return nil
}

// Mkdir 在 rel 目录下新建文件夹，返回新目录的项目根相对路径。
func Mkdir(root, rel, name string) (string, error) {
	if err := ValidateName(name); err != nil {
		return "", err
	}
	dirFull, dirRel, err := Resolve(root, rel)
	if err != nil {
		return "", err
	}
	if info, err := os.Stat(dirFull); err != nil || !info.IsDir() {
		return "", fmt.Errorf("目标不是一个目录")
	}
	target := filepath.Join(dirFull, name)
	if _, err := os.Lstat(target); err == nil {
		return "", fmt.Errorf("%w：%s", ErrExists, name)
	}
	if err := os.Mkdir(target, 0o755); err != nil {
		return "", fmt.Errorf("新建文件夹失败：%v", err)
	}
	return joinRel(dirRel, name), nil
}

// Rename 把 rel 指向的条目改名为 newName（仍在同一目录下）。
func Rename(root, rel, newName, policy string) (string, error) {
	if err := ValidateName(newName); err != nil {
		return "", err
	}
	srcFull, srcRel, err := Resolve(root, rel)
	if err != nil {
		return "", err
	}
	if srcRel == "" {
		return "", fmt.Errorf("不能重命名项目根目录")
	}
	parent := filepath.Dir(srcFull)
	target := filepath.Join(parent, newName)
	if filepath.Clean(target) == filepath.Clean(srcFull) {
		return srcRel, nil
	}
	target, err = resolveConflict(target, policy)
	if err != nil {
		return "", err
	}
	if err := os.Rename(srcFull, target); err != nil {
		return "", fmt.Errorf("重命名失败：%v", err)
	}
	return joinRel(parentOf(srcRel), filepath.Base(target)), nil
}

// Remove 把若干条目移入回收站（可恢复，不做永久删除），返回实际处理的条目数。
// 已经不存在的条目视为已完成，避免「删两次报错」这种误报。
func Remove(root string, rels []string) (int, error) {
	var failed []string
	removed := 0
	for _, rel := range rels {
		full, norm, err := Resolve(root, rel)
		if err != nil {
			return removed, err
		}
		if norm == "" {
			return removed, fmt.Errorf("不能删除项目根目录")
		}
		if _, err := os.Lstat(full); os.IsNotExist(err) {
			removed++
			continue
		}
		if err := removeFileFunc(full); err != nil {
			failed = append(failed, fmt.Sprintf("%s（%v）", norm, err))
			continue
		}
		removed++
	}
	if len(failed) > 0 {
		return removed, fmt.Errorf("以下条目删除失败：%s", strings.Join(failed, "；"))
	}
	return removed, nil
}

// RemoveForce 永久删除（不进回收站）。
// 只用于回收站接口搞不定的极端情况（例如嵌套上千层、路径超过 32760 字符的目录），
// 调用方必须先向用户明确说明「不可恢复」并取得确认。
func RemoveForce(root string, rels []string) (int, error) {
	removed := 0
	for _, rel := range rels {
		full, norm, err := Resolve(root, rel)
		if err != nil {
			return removed, err
		}
		if norm == "" {
			return removed, fmt.Errorf("不能删除项目根目录")
		}
		if _, err := os.Lstat(full); os.IsNotExist(err) {
			removed++
			continue
		}
		if err := winfs.RemoveLong(full); err != nil {
			return removed, fmt.Errorf("永久删除 %s 失败：%v", norm, err)
		}
		removed++
	}
	return removed, nil
}

// CheckConflicts 返回 srcs 移动到/复制到 dstDir 时会撞名的条目名。
func CheckConflicts(root string, srcs []string, dstDir string) ([]string, error) {
	dstFull, dstRel, err := Resolve(root, dstDir)
	if err != nil {
		return nil, err
	}
	conflicts := []string{}
	for _, rel := range srcs {
		_, norm, err := Resolve(root, rel)
		if err != nil {
			return nil, err
		}
		if norm == "" {
			return nil, fmt.Errorf("不能移动或复制项目根目录")
		}
		if parentOf(norm) == dstRel {
			continue
		}
		name := filepath.Base(filepath.FromSlash(norm))
		if _, err := os.Lstat(filepath.Join(dstFull, name)); err == nil {
			conflicts = append(conflicts, name)
		}
	}
	return conflicts, nil
}

// Move 把 srcs 移动到 dstDir，policy 决定撞名时的行为。
func Move(root string, srcs []string, dstDir, policy string) error {
	dstFull, dstRel, err := Resolve(root, dstDir)
	if err != nil {
		return err
	}
	if info, err := os.Stat(dstFull); err != nil || !info.IsDir() {
		return fmt.Errorf("目标不是一个目录")
	}
	for _, rel := range srcs {
		srcFull, norm, err := Resolve(root, rel)
		if err != nil {
			return err
		}
		if norm == "" {
			return fmt.Errorf("不能移动项目根目录")
		}
		if parentOf(norm) == dstRel {
			continue
		}
		if intoItself(srcFull, dstFull) {
			return fmt.Errorf("不能把 %s 移动到它自己或它的子目录里", norm)
		}
		name := filepath.Base(filepath.FromSlash(norm))
		target, err := resolveConflict(filepath.Join(dstFull, name), policy)
		if err != nil {
			return err
		}
		if err := movePath(srcFull, target); err != nil {
			return fmt.Errorf("移动 %s 失败：%v", norm, err)
		}
	}
	return nil
}

// Copy 把 srcs 复制到 dstDir，policy 决定撞名时的行为。
func Copy(root string, srcs []string, dstDir, policy string) error {
	dstFull, dstRel, err := Resolve(root, dstDir)
	if err != nil {
		return err
	}
	if info, err := os.Stat(dstFull); err != nil || !info.IsDir() {
		return fmt.Errorf("目标不是一个目录")
	}
	for _, rel := range srcs {
		srcFull, norm, err := Resolve(root, rel)
		if err != nil {
			return err
		}
		if norm == "" {
			return fmt.Errorf("不能复制项目根目录")
		}
		if parentOf(norm) == dstRel {
			continue
		}
		if intoItself(srcFull, dstFull) {
			return fmt.Errorf("不能把 %s 复制到它自己或它的子目录里", norm)
		}
		name := filepath.Base(filepath.FromSlash(norm))
		target, err := resolveConflict(filepath.Join(dstFull, name), policy)
		if err != nil {
			return err
		}
		if err := copyPath(srcFull, target); err != nil {
			return fmt.Errorf("复制 %s 失败：%v", norm, err)
		}
	}
	return nil
}

// resolveConflict 按策略处理「目标已存在」。覆盖时先把旧目标送回收站，不做永久删除。
func resolveConflict(target, policy string) (string, error) {
	if _, err := os.Lstat(target); err != nil {
		return target, nil
	}
	switch policy {
	case PolicyOverwrite:
		if err := removeFileFunc(target); err != nil {
			return "", err
		}
		return target, nil
	case PolicyRename:
		dir := filepath.Dir(target)
		return filepath.Join(dir, uniqueName(dir, filepath.Base(target))), nil
	default:
		return "", fmt.Errorf("%w：%s", ErrExists, filepath.Base(target))
	}
}

func movePath(src, dst string) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	// 跨卷等情况回退为「复制 + 删除来源」（内容已在目标处，不会丢数据）
	if err := copyPath(src, dst); err != nil {
		return err
	}
	return os.RemoveAll(src)
}

// maxCopyDepth 是目录复制的嵌套上限，纯属纵深防御：
// 正常的项目不会嵌套这么深，超过基本意味着又出现了「复制进自己」这类环。
const maxCopyDepth = 200

func copyPath(src, dst string) error {
	return copyPathDepth(src, dst, 0)
}

func copyPathDepth(src, dst string, depth int) error {
	if depth > maxCopyDepth {
		return fmt.Errorf("目录嵌套超过 %d 层，已中止复制", maxCopyDepth)
	}
	info, err := os.Lstat(src)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return copyFile(src, dst, info.Mode())
	}
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if err := copyPathDepth(filepath.Join(src, e.Name()), filepath.Join(dst, e.Name()), depth+1); err != nil {
			return err
		}
	}
	return nil
}

func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}

func uniqueName(dir, name string) string {
	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)
	for i := 1; i < 1000; i++ {
		candidate := fmt.Sprintf("%s (%d)%s", base, i, ext)
		if _, err := os.Lstat(filepath.Join(dir, candidate)); os.IsNotExist(err) {
			return candidate
		}
	}
	return name
}

// isAncestor 判断 a 是否是 b 的祖先目录（含相等时为假）。
func isAncestor(a, b string) bool {
	rel, err := filepath.Rel(a, b)
	if err != nil {
		return false
	}
	return rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// samePath 判断两个绝对路径是否指向同一位置（Windows 路径不区分大小写）。
func samePath(a, b string) bool {
	return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
}

// intoItself 判断 dst 是否就是 src 本身、或位于 src 之内。
// 复制/移动时必须拦住这种情况，否则 copyPath 会把自己刚建出的子目录再复制一遍，
// 造成无限递归，直到撞上系统路径长度上限。
func intoItself(srcFull, dstFull string) bool {
	return samePath(srcFull, dstFull) || isAncestor(srcFull, dstFull)
}

func parentOf(rel string) string {
	if i := strings.LastIndex(rel, "/"); i >= 0 {
		return rel[:i]
	}
	return ""
}
