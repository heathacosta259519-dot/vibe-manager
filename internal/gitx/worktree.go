package gitx

import (
	"fmt"
	"strings"

	"vibe-manager/internal/model"
)

// diffMaxChars 是回传给界面的 diff 长度上限（避免几万行卡住渲染）。
const diffMaxChars = 120_000

// afterSpaces 返回跳过前 n 个空格之后的剩余内容，用于从 porcelain 记录里取路径
// （路径本身可能带空格，所以不能简单按空格切分）。
func afterSpaces(s string, n int) string {
	idx := 0
	for i := 0; i < n; i++ {
		j := strings.IndexByte(s[idx:], ' ')
		if j < 0 {
			return ""
		}
		idx += j + 1
	}
	return s[idx:]
}

func xyOf(rec string) string {
	f := strings.Fields(rec)
	if len(f) < 2 {
		return ".."
	}
	return f[1]
}

// statusOf 把 porcelain 的 XY 两位状态归成一个字母：A 新增 / M 修改 / D 删除 / R 重命名 / U 冲突。
// 优先看暂存位（X），没有再看工作区位（Y）。
func statusOf(xy string) string {
	if len(xy) < 2 {
		return "M"
	}
	// 未合并的组合（两位都非 '.'，且含 U，或 AA / DD）算冲突，不能误报成新增/删除
	if xy[0] != '.' && xy[1] != '.' {
		if strings.ContainsRune(xy, 'U') || xy == "AA" || xy == "DD" {
			return "U"
		}
	}

	pick := func(c byte) string {
		switch c {
		case 'A':
			return "A"
		case 'M', 'T':
			return "M"
		case 'D':
			return "D"
		case 'R', 'C':
			return "R"
		case 'U':
			return "U"
		}
		return ""
	}
	if s := pick(xy[0]); s != "" {
		return s
	}
	if s := pick(xy[1]); s != "" {
		return s
	}
	return "M"
}

// WorktreeFiles 列出工作区里所有有改动的文件（含未跟踪文件），相对仓库根。
func WorktreeFiles(dir string) ([]model.FileChange, error) {
	out, errs, err := run(dir, "status", "--porcelain=v2", "-z", "--untracked-files=all")
	if err != nil {
		return nil, fmt.Errorf("%s", strings.TrimSpace(errs))
	}

	parts := strings.Split(out, "\x00")
	list := make([]model.FileChange, 0, len(parts))
	for i := 0; i < len(parts); i++ {
		rec := parts[i]
		if rec == "" {
			continue
		}
		switch rec[0] {
		case '1': // 普通改动：1 <XY> <sub> <mH> <mI> <mW> <hH> <hI> <path>
			path := afterSpaces(rec, 8)
			if path == "" {
				continue
			}
			xy := xyOf(rec)
			list = append(list, model.FileChange{Status: statusOf(xy), Path: path, Staged: xy[0] != '.'})
		case '2': // 重命名/复制：多一个 <X><score> 字段，后面还跟一个原始路径
			path := afterSpaces(rec, 9)
			if path == "" {
				continue
			}
			xy := xyOf(rec)
			list = append(list, model.FileChange{Status: "R", Path: path, Staged: xy[0] != '.'})
			if i+1 < len(parts) {
				i++ // 跳过紧跟的原始路径记录
			}
		case '?':
			path := strings.TrimSpace(rec[1:])
			if path != "" {
				list = append(list, model.FileChange{Status: "A", Path: path})
			}
		case 'u': // 冲突：u <XY> <sub> <m1> <m2> <m3> <mW> <h1> <h2> <h3> <path>
			path := afterSpaces(rec, 10)
			if path != "" {
				list = append(list, model.FileChange{Status: "U", Path: path})
			}
		}
	}
	return list, nil
}

// DiffFile 返回单个文件的 diff 文本。staged 为真时看已暂存的那一份。
func DiffFile(dir, path string, staged bool) (string, error) {
	args := []string{"diff", "--no-color", "--unified=3"}
	if staged {
		args = append(args, "--cached")
	}
	args = append(args, "--", path)
	out, errs, err := run(dir, args...)
	if err != nil {
		return "", fmt.Errorf("%s", strings.TrimSpace(errs))
	}
	if strings.TrimSpace(out) == "" {
		return "（没有可显示的差异：可能是未跟踪的新文件，或改动都在另一边）", nil
	}
	if len(out) > diffMaxChars {
		out = out[:diffMaxChars] + "\n\n…（diff 过长，已截断）\n"
	}
	return out, nil
}

// CommitPaths 只提交指定路径（先 add 再按 pathspec 提交，避免把别人已暂存的改动一起带上）。
// amend 为真且 paths 为空时，表示"修改上一次提交"（改写信息 + 当前的暂存内容）。
func CommitPaths(dir string, paths []string, message string, amend bool) (string, bool, error) {
	if !IsRepo(dir) {
		return "", false, fmt.Errorf("这不是一个 git 仓库")
	}
	msg := strings.TrimSpace(message)
	if msg == "" {
		return "", false, fmt.Errorf("提交信息不能为空")
	}
	if len(paths) == 0 && !amend {
		return "", false, fmt.Errorf("请先勾选要提交的文件")
	}

	if len(paths) > 0 {
		args := append([]string{"add", "--"}, paths...)
		if _, errs, err := run(dir, args...); err != nil {
			return "", false, fmt.Errorf("暂存文件失败：%s", strings.TrimSpace(errs))
		}
	}

	args := []string{"commit", "-m", msg}
	if amend {
		args = append(args, "--amend")
	}
	if len(paths) > 0 {
		args = append(args, "--")
		args = append(args, paths...)
	}
	out, errs, err := run(dir, args...)
	if err != nil {
		combined := out + errs
		if strings.Contains(combined, "nothing to commit") {
			return "", false, fmt.Errorf("没有需要提交的改动")
		}
		return "", false, fmt.Errorf("%s", strings.TrimSpace(errs))
	}

	sha, _, _ := run(dir, "rev-parse", "HEAD")
	return strings.TrimSpace(sha), true, nil
}
