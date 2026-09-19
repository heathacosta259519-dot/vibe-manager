package gitx

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"vibe-manager/internal/model"
)

// compareDiffMaxChars 是单个文件 diff 回传给界面的长度上限，与工作区 diff 取同一个量级。
// 整仓库的跨版本 diff 可能是几十 MB，所以这里只按文件取，绝不一次性拉全量。
const compareDiffMaxChars = 120_000

// 全量视图要把整份文件当上下文带回来，上限相应放宽；再大就截断并如实说明。
const fullDiffMaxChars = 400_000

// fullContextLines 是"显示全部代码"时的上下文行数，给足即可等价于整份文件。
const fullContextLines = 200_000

// numstat 是一条 --numstat 记录。二进制文件没有行数，只有 binary 标记。
type numstat struct {
	adds   int
	dels   int
	binary bool
}

// DiffRefs 列出 base → target 之间变更的文件。target 为空表示拿工作区来比。
func DiffRefs(dir, base, target string) (model.DiffSummary, error) {
	if !IsRepo(dir) {
		return model.DiffSummary{}, fmt.Errorf("这不是一个 git 仓库")
	}
	base, target = strings.TrimSpace(base), strings.TrimSpace(target)
	if err := verifyRef(dir, base); err != nil {
		return model.DiffSummary{}, err
	}
	if target != "" {
		if err := verifyRef(dir, target); err != nil {
			return model.DiffSummary{}, err
		}
	}

	span := diffSpan(base, target)
	nameOut, errs, err := run(dir, append(append([]string{}, span...), "--name-status", "-M", "-z")...)
	if err != nil {
		return model.DiffSummary{}, fmt.Errorf("%s", strings.TrimSpace(errs))
	}
	numOut, errs, err := run(dir, append(append([]string{}, span...), "--numstat", "-M", "-z")...)
	if err != nil {
		return model.DiffSummary{}, fmt.Errorf("%s", strings.TrimSpace(errs))
	}

	files := parseNameStatus(nameOut, parseNumstat(numOut))

	// 未跟踪的新文件根本不参与 git diff，比工作区时要单独补上，否则"这次改了什么"会漏掉新文件
	if target == "" {
		untracked, err := untrackedFiles(dir)
		if err != nil {
			return model.DiffSummary{}, err
		}
		for _, p := range untracked {
			files = append(files, model.DiffFileStat{Status: "A", Path: p, Adds: -1, Dels: -1, Untracked: true})
		}
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })

	summary := model.DiffSummary{Base: base, Target: target, Files: files}
	if summary.Files == nil {
		summary.Files = []model.DiffFileStat{}
	}
	for _, f := range files {
		if f.Adds > 0 {
			summary.Adds += f.Adds
		}
		if f.Dels > 0 {
			summary.Dels += f.Dels
		}
	}
	return summary, nil
}

// DiffRefFile 返回单个文件在 base → target 之间的 diff。target 为空表示工作区。
//
// fullContext 为真时把整份文件都当作上下文返回（配合界面的行号视图，
// 能看到改动前后完整的代码，而不是只有几个 hunk）。
func DiffRefFile(dir, base, target, path string, fullContext bool) (model.DiffText, error) {
	if !IsRepo(dir) {
		return model.DiffText{}, fmt.Errorf("这不是一个 git 仓库")
	}
	base, target, path = strings.TrimSpace(base), strings.TrimSpace(target), strings.TrimSpace(path)
	if path == "" {
		return model.DiffText{}, fmt.Errorf("请先选一个文件")
	}
	if err := verifyRef(dir, base); err != nil {
		return model.DiffText{}, err
	}
	if target != "" {
		if err := verifyRef(dir, target); err != nil {
			return model.DiffText{}, err
		}
	}

	unified := "--unified=3"
	maxChars := compareDiffMaxChars
	if fullContext {
		unified = fmt.Sprintf("--unified=%d", fullContextLines)
		maxChars = fullDiffMaxChars
	}

	var out string
	if target == "" && !isTracked(dir, path) {
		out = diffUntracked(dir, path, unified)
	} else {
		args := append(diffSpan(base, target), "--no-color", unified, "--", path)
		var errs string
		var err error
		out, errs, err = run(dir, args...)
		if err != nil {
			return model.DiffText{}, fmt.Errorf("%s", strings.TrimSpace(errs))
		}
	}

	return shapeDiff(out, "（这两个版本之间，这个文件没有差异）", maxChars), nil
}

// DiffCommitFile 返回某次提交里单个文件的改动。
// 这里刻意用 git show 而不是 diff：根提交没有父，diff 会直接报错，
// 而 show 会正确地把整份文件当成新增。
func DiffCommitFile(dir, sha, path string) (model.DiffText, error) {
	if !IsRepo(dir) {
		return model.DiffText{}, fmt.Errorf("这不是一个 git 仓库")
	}
	sha, path = strings.TrimSpace(sha), strings.TrimSpace(path)
	if path == "" {
		return model.DiffText{}, fmt.Errorf("请先选一个文件")
	}
	if err := verifyRef(dir, sha); err != nil {
		return model.DiffText{}, err
	}
	out, errs, err := run(dir, "show", "--no-color", "--unified=3", "--format=", sha, "--", path)
	if err != nil {
		return model.DiffText{}, fmt.Errorf("%s", strings.TrimSpace(errs))
	}
	return shapeDiff(out, "（这次提交里这个文件没有内容变化）", compareDiffMaxChars), nil
}

// shapeDiff 收尾：识别二进制、截断超长内容、给空结果一句人话。
func shapeDiff(out, empty string, maxChars int) model.DiffText {
	res := model.DiffText{Text: out, Binary: strings.Contains(out, "Binary files") && strings.Contains(out, "differ")}
	if len(out) > maxChars {
		res.Text = out[:maxChars] + "\n\n…（diff 过长，已截断）\n"
		res.Truncated = true
		return res
	}
	if strings.TrimSpace(out) == "" {
		res.Text = empty
	}
	return res
}

// diffSpan 组装比较范围的参数：给了 target 就是两点比较，没给就是「base 对工作区」。
func diffSpan(base, target string) []string {
	if target == "" {
		return []string{"diff", base}
	}
	return []string{"diff", base, target}
}

// verifyRef 确认 ref 指向一个提交。
// 顺带挡掉以 "-" 开头的字符串——git 会把它们当成选项而不是版本名。
func verifyRef(dir, ref string) error {
	if ref == "" {
		return fmt.Errorf("请先选择要比较的版本")
	}
	if strings.HasPrefix(ref, "-") {
		return fmt.Errorf("版本名不合法：%s", ref)
	}
	if _, _, err := run(dir, "rev-parse", "--verify", "--quiet", ref+"^{commit}"); err != nil {
		return fmt.Errorf("找不到这个版本：%s", ref)
	}
	return nil
}

// parseNumstat 解析 `git diff --numstat -M -z`，键为目标侧路径。
// 普通文件是一片「增<TAB>删<TAB>路径」，重命名则拆成「增<TAB>删<TAB>」加旧路径、新路径三片。
func parseNumstat(out string) map[string]numstat {
	counts := map[string]numstat{}
	tokens := strings.Split(out, "\x00")
	for i := 0; i < len(tokens); {
		rec := tokens[i]
		if rec == "" {
			i++
			continue
		}
		fields := strings.SplitN(rec, "\t", 3)
		if len(fields) < 3 {
			i++
			continue
		}
		n := numstatOf(fields[0], fields[1])
		if fields[2] == "" {
			if i+2 >= len(tokens) {
				break
			}
			counts[tokens[i+2]] = n
			i += 3
			continue
		}
		counts[fields[2]] = n
		i++
	}
	return counts
}

func numstatOf(adds, dels string) numstat {
	a, errA := strconv.Atoi(strings.TrimSpace(adds))
	d, errD := strconv.Atoi(strings.TrimSpace(dels))
	if errA != nil || errD != nil {
		return numstat{adds: -1, dels: -1, binary: true}
	}
	return numstat{adds: a, dels: d}
}

// parseNameStatus 解析 `git diff --name-status -M -z`：
// 普通文件是一片「状态<NUL>路径」，重命名 / 复制会多带上旧路径。
func parseNameStatus(out string, counts map[string]numstat) []model.DiffFileStat {
	tokens := strings.Split(out, "\x00")
	files := []model.DiffFileStat{}
	for i := 0; i < len(tokens); {
		status := tokens[i]
		if status == "" {
			i++
			continue
		}
		if c := status[0]; c == 'R' || c == 'C' {
			if i+2 >= len(tokens) {
				break
			}
			files = append(files, fileStat("R", tokens[i+1], tokens[i+2], counts))
			i += 3
			continue
		}
		if i+1 >= len(tokens) {
			break
		}
		files = append(files, fileStat(status, "", tokens[i+1], counts))
		i += 2
	}
	return files
}

func fileStat(status, oldPath, path string, counts map[string]numstat) model.DiffFileStat {
	f := model.DiffFileStat{Status: "M", Path: path, OldPath: oldPath}
	switch status[0] {
	case 'A':
		f.Status = "A"
	case 'D':
		f.Status = "D"
	case 'R', 'C':
		f.Status = "R"
	case 'U':
		f.Status = "U"
	}
	if n, ok := counts[path]; ok {
		f.Adds, f.Dels, f.Binary = n.adds, n.dels, n.binary
	} else {
		// 统计缺失（典型是重命名）时不要假装成 0，交给界面显示「—」
		f.Adds, f.Dels = -1, -1
	}
	return f
}

func untrackedFiles(dir string) ([]string, error) {
	out, errs, err := run(dir, "ls-files", "--others", "--exclude-standard", "-z")
	if err != nil {
		return nil, fmt.Errorf("%s", strings.TrimSpace(errs))
	}
	list := []string{}
	for _, p := range strings.Split(out, "\x00") {
		if strings.TrimSpace(p) != "" {
			list = append(list, p)
		}
	}
	return list, nil
}

// isTracked 判断路径是否已被 git 跟踪；未跟踪的文件要用 --no-index 单独造 diff。
func isTracked(dir, path string) bool {
	_, _, err := run(dir, "ls-files", "--error-unmatch", "--", path)
	return err == nil
}

// diffUntracked 把未跟踪的新文件当成"整文件新增"。
// --no-index 在有差异时退出码是 1，那是正常结果而不是失败。
func diffUntracked(dir, path, unified string) string {
	out, _, err := run(dir, "diff", "--no-index", "--no-color", unified, "--", "/dev/null", path)
	if err != nil && strings.TrimSpace(out) == "" {
		return "（读不到这个未跟踪文件）"
	}
	return out
}
