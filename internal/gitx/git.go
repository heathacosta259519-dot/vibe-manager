package gitx

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"vibe-manager/internal/execx"
	"vibe-manager/internal/model"
)

const unitSep = "\x1f"

// BackupTagPrefix 是回滚前自动打保底标签的前缀。
// 项目改叫 Vibe-Manager 之后这里仍保留 vibe-pm：换了前缀，
// 用户仓库里已有的保底标签就再也列不出来，「备份」区等于清空。
const BackupTagPrefix = "vibe-pm-backup-"

func run(dir string, args ...string) (string, string, error) {
	cmd := exec.Command("git", args...)
	execx.Hide(cmd)
	cmd.Dir = dir
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	err := cmd.Run()
	return out.String(), errb.String(), err
}

func IsRepo(dir string) bool {
	fi, err := os.Stat(dir)
	if err != nil || !fi.IsDir() {
		return false
	}
	if _, err := os.Stat(dir + string(os.PathSeparator) + ".git"); err != nil {
		return false
	}
	return true
}

func Init(dir string) error {
	if _, errs, err := run(dir, "init"); err != nil {
		return fmt.Errorf("%s", strings.TrimSpace(errs))
	}
	return nil
}

func gitMTime(dir string) int64 {
	var newest int64
	for _, name := range []string{".git/index", ".git/HEAD"} {
		if fi, err := os.Stat(dir + string(os.PathSeparator) + name); err == nil {
			if t := fi.ModTime().Unix(); t > newest {
				newest = t
			}
		}
	}
	return newest
}

func Status(dir string) (model.GitStatus, error) {
	out, errs, err := run(dir, "status", "--porcelain=v2", "--branch")
	if err != nil {
		return model.GitStatus{}, fmt.Errorf("%s", strings.TrimSpace(errs))
	}
	st := model.GitStatus{MTime: gitMTime(dir)}
	for _, raw := range strings.Split(out, "\n") {
		line := strings.TrimRight(raw, "\r")
		switch {
		case strings.HasPrefix(line, "# branch.head "):
			st.Branch = strings.TrimSpace(strings.TrimPrefix(line, "# branch.head "))
		case strings.HasPrefix(line, "# branch.upstream "):
			st.HasUpstream = true
		case strings.HasPrefix(line, "# branch.ab "):
			for _, f := range strings.Fields(strings.TrimPrefix(line, "# branch.ab ")) {
				n, _ := strconv.Atoi(strings.TrimLeft(f, "+-"))
				switch {
				case strings.HasPrefix(f, "+"):
					st.Ahead = n
				case strings.HasPrefix(f, "-"):
					st.Behind = n
				}
			}
		default:
			if strings.TrimSpace(line) != "" && !strings.HasPrefix(line, "#") {
				st.Dirty = true
			}
		}
	}
	return st, nil
}

func LastCommit(dir string) (string, int64, error) {
	out, errs, err := run(dir, "log", "-1", "--format=%ct"+unitSep+"%s")
	if err != nil {
		msg := strings.TrimSpace(errs)
		if strings.Contains(msg, "does not have any commits") || strings.Contains(msg, "unknown revision") {
			return "", 0, nil
		}
		return "", 0, fmt.Errorf("%s", msg)
	}
	line := strings.TrimSpace(strings.TrimRight(out, "\r\n"))
	if line == "" {
		return "", 0, nil
	}
	parts := strings.SplitN(line, unitSep, 2)
	if len(parts) < 2 {
		return "", 0, nil
	}
	ts, _ := strconv.ParseInt(parts[0], 10, 64)
	return parts[1], ts, nil
}

func Stashes(dir string) ([]model.Backup, error) {
	out, errs, err := run(dir, "stash", "list", "--format=%gd"+unitSep+"%ct"+unitSep+"%s")
	if err != nil {
		return nil, fmt.Errorf("%s", strings.TrimSpace(errs))
	}
	var list []model.Backup
	for _, raw := range strings.Split(out, "\n") {
		line := strings.TrimRight(raw, "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		parts := strings.SplitN(line, unitSep, 3)
		if len(parts) < 3 {
			continue
		}
		ts, _ := strconv.ParseInt(parts[1], 10, 64)
		list = append(list, model.Backup{
			Ref:     parts[0],
			Kind:    "stash",
			Message: stripStashPrefix(parts[2]),
			When:    ts,
		})
	}
	return list, nil
}

// stripStashPrefix 去掉 git 给 stash 自动加上的 "On <branch>: " 前缀。
func stripStashPrefix(s string) string {
	if !strings.HasPrefix(s, "On ") {
		return s
	}
	if i := strings.Index(s, ": "); i >= 0 {
		return s[i+2:]
	}
	return s
}

func BackupTags(dir, prefix string) ([]model.Backup, error) {
	out, errs, err := run(dir, "for-each-ref", "--sort=-creatordate",
		"--format=%(refname:short) %(creatordate:unix)", "refs/tags/"+prefix+"*")
	if err != nil {
		return nil, fmt.Errorf("%s", strings.TrimSpace(errs))
	}
	var list []model.Backup
	for _, raw := range strings.Split(out, "\n") {
		line := strings.TrimSpace(strings.TrimRight(raw, "\r"))
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, " ", 2)
		if len(parts) < 2 {
			continue
		}
		ts, _ := strconv.ParseInt(strings.TrimSpace(parts[1]), 10, 64)
		list = append(list, model.Backup{
			Ref:     parts[0],
			Kind:    "tag",
			Message: parts[0],
			When:    ts,
		})
	}
	return list, nil
}

func ApplyStash(dir, ref string) (string, error) {
	if _, errs, err := run(dir, "stash", "apply", ref); err != nil {
		return "", fmt.Errorf("%s", strings.TrimSpace(errs))
	}
	return "已应用 " + ref, nil
}

func DropStash(dir, ref string) (string, error) {
	if _, errs, err := run(dir, "stash", "drop", ref); err != nil {
		return "", fmt.Errorf("%s", strings.TrimSpace(errs))
	}
	return "已删除 " + ref, nil
}

func DeleteTag(dir, tag string) (string, error) {
	if _, errs, err := run(dir, "tag", "-d", tag); err != nil {
		return "", fmt.Errorf("%s", strings.TrimSpace(errs))
	}
	return "已删除标签 " + tag, nil
}

// StatusMap 返回指定子目录下的变更状态表，键为「相对仓库根」的斜杠路径。
// 限定 pathspec 可避免大仓库全量扫描；未列出的条目即为无变更。
func StatusMap(dir, relPath string) (map[string]string, error) {
	target := "."
	if relPath != "" && relPath != "." {
		target = relPath
	}
	out, errs, err := run(dir, "status", "--porcelain", "-uall", "--", target)
	if err != nil {
		return nil, fmt.Errorf("%s", strings.TrimSpace(errs))
	}
	states := map[string]string{}
	for _, raw := range strings.Split(out, "\n") {
		line := strings.TrimRight(raw, "\r")
		if len(line) < 4 {
			continue
		}
		code := strings.TrimSpace(line[:2])
		path := strings.TrimSpace(line[3:])
		if i := strings.Index(path, " -> "); i >= 0 {
			path = path[i+4:]
		}
		if path == "" {
			continue
		}
		states[filepath.ToSlash(path)] = classifyCode(code)
	}
	return states, nil
}

func classifyCode(code string) string {
	switch {
	case code == "??":
		return "??"
	case code == "!!":
		return "!!"
	case strings.Contains(code, "M"):
		return "M"
	case strings.Contains(code, "A"):
		return "A"
	case strings.Contains(code, "D"):
		return "D"
	case strings.Contains(code, "R"):
		return "R"
	}
	return "M"
}

// IgnoredSet 用一次 git 进程批量判断哪些路径被 .gitignore 忽略。
// 传空切片时不做任何事；git 在「没有匹配」时退出码为 1，不算错误。
func IgnoredSet(dir string, rels []string) (map[string]bool, error) {
	set := map[string]bool{}
	if len(rels) == 0 {
		return set, nil
	}
	cmd := exec.Command("git", "check-ignore", "--stdin")
	execx.Hide(cmd)
	cmd.Dir = dir
	cmd.Stdin = strings.NewReader(strings.Join(rels, "\n") + "\n")
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) && ee.ExitCode() == 1 {
			return set, nil
		}
		return nil, fmt.Errorf("%s", strings.TrimSpace(errb.String()))
	}
	for _, raw := range strings.Split(out.String(), "\n") {
		if line := strings.TrimSpace(strings.TrimRight(raw, "\r")); line != "" {
			set[filepath.ToSlash(line)] = true
		}
	}
	return set, nil
}

func CommitFiles(dir, sha string) ([]model.FileChange, error) {
	out, errs, err := run(dir, "show", "--name-status", "-M", "--format=", sha)
	if err != nil {
		return nil, fmt.Errorf("%s", strings.TrimSpace(errs))
	}
	var files []model.FileChange
	for _, raw := range strings.Split(out, "\n") {
		line := strings.TrimRight(raw, "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) < 2 {
			continue
		}
		status := parts[0]
		if status == "" {
			status = "?"
		}
		// 重命名 / 复制给的是「旧<TAB>新」两个字段。旧路径只留作展示，
		// Path 必须是新路径——调用方要拿它去 git show / git diff 取内容。
		path, oldPath := parts[1], ""
		if (status[0] == 'R' || status[0] == 'C') && len(parts) >= 3 {
			oldPath, path = parts[1], parts[2]
		}
		files = append(files, model.FileChange{Status: string(status[0]), Path: path, OldPath: oldPath})
	}
	return files, nil
}

func Log(dir string, limit int) ([]model.Commit, error) {
	format := "@@@" + strings.Join([]string{"%H", "%h", "%ct", "%an", "%s"}, unitSep)
	out, errs, err := run(dir, "log", fmt.Sprintf("-n%d", limit), "--pretty=format:"+format, "--name-only")
	if err != nil {
		msg := strings.TrimSpace(errs)
		if strings.Contains(msg, "does not have any commits") || strings.Contains(msg, "unknown revision") {
			return []model.Commit{}, nil
		}
		return nil, fmt.Errorf("%s", msg)
	}
	var commits []model.Commit
	for _, raw := range strings.Split(out, "\n") {
		line := strings.TrimRight(raw, "\r")
		if strings.HasPrefix(line, "@@@") {
			parts := strings.Split(strings.TrimPrefix(line, "@@@"), unitSep)
			if len(parts) < 5 {
				continue
			}
			ts, _ := strconv.ParseInt(parts[2], 10, 64)
			commits = append(commits, model.Commit{
				Hash:    parts[0],
				Short:   parts[1],
				When:    ts,
				Author:  parts[3],
				Subject: parts[4],
			})
			continue
		}
		if strings.TrimSpace(line) != "" && len(commits) > 0 {
			commits[len(commits)-1].Files++
		}
	}
	return commits, nil
}

// Subjects 只取最近若干条提交的短 SHA 与标题，比 Log 轻（不取变更文件列表）。
func Subjects(dir string, limit int) ([]model.Commit, error) {
	format := "%h" + unitSep + "%s"
	out, errs, err := run(dir, "log", fmt.Sprintf("-n%d", limit), "--pretty=format:"+format)
	if err != nil {
		msg := strings.TrimSpace(errs)
		if strings.Contains(msg, "does not have any commits") || strings.Contains(msg, "unknown revision") {
			return []model.Commit{}, nil
		}
		return nil, fmt.Errorf("%s", msg)
	}
	var commits []model.Commit
	for _, raw := range strings.Split(out, "\n") {
		line := strings.TrimRight(raw, "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		parts := strings.SplitN(line, unitSep, 2)
		if len(parts) < 2 {
			continue
		}
		commits = append(commits, model.Commit{Short: parts[0], Subject: parts[1]})
	}
	return commits, nil
}

// GlobalUserName 读取 git 全局 user.name，取不到返回空串。
func GlobalUserName() string {
	out, _, err := run(".", "config", "--global", "user.name")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

func idArgs(dir string) []string {
	if out, _, err := run(dir, "config", "user.email"); err == nil && strings.TrimSpace(out) != "" {
		return nil
	}
	return []string{"-c", "user.name=vibe-pm", "-c", "user.email=vibe-pm@localhost"}
}

func isDirty(dir string) (bool, error) {
	out, errs, err := run(dir, "status", "--porcelain")
	if err != nil {
		return false, fmt.Errorf("%s", strings.TrimSpace(errs))
	}
	return strings.TrimSpace(out) != "", nil
}

func CommitAll(dir, message string) (string, bool, error) {
	dirty, err := isDirty(dir)
	if err != nil {
		return "", false, err
	}
	if !dirty {
		return "", false, nil
	}
	if _, errs, err := run(dir, "add", "-A"); err != nil {
		return "", false, fmt.Errorf("%s", strings.TrimSpace(errs))
	}
	args := append(idArgs(dir), "commit", "-m", message)
	if _, errs, err := run(dir, args...); err != nil {
		return "", false, fmt.Errorf("%s", strings.TrimSpace(errs))
	}
	sha, _, _ := run(dir, "rev-parse", "HEAD")
	return strings.TrimSpace(sha), true, nil
}

func stashBackup(dir, label string) (bool, error) {
	dirty, err := isDirty(dir)
	if err != nil {
		return false, err
	}
	if !dirty {
		return false, nil
	}
	args := append(idArgs(dir), "stash", "push", "-u", "-m", label)
	if _, errs, err := run(dir, args...); err != nil {
		return false, fmt.Errorf("%s", strings.TrimSpace(errs))
	}
	return true, nil
}

func tagBackup(dir, label string) (string, error) {
	if _, _, err := run(dir, "rev-parse", "--verify", "HEAD"); err != nil {
		return "", nil
	}
	if _, errs, err := run(dir, "tag", label); err != nil {
		return "", fmt.Errorf("%s", strings.TrimSpace(errs))
	}
	return label, nil
}

func Rollback(dir, sha string, hard bool, stamp string) (string, error) {
	if _, errs, err := run(dir, "rev-parse", "--verify", sha+"^{commit}"); err != nil {
		return "", fmt.Errorf("无效的提交 %s: %s", sha, strings.TrimSpace(errs))
	}
	notes := []string{}
	stashed, err := stashBackup(dir, "Vibe-Manager 保底备份 "+stamp)
	if err != nil {
		return "", err
	}
	if stashed {
		notes = append(notes, "未提交改动已存入 stash")
	}
	tag, err := tagBackup(dir, BackupTagPrefix+stamp)
	if err != nil {
		return "", err
	}
	if tag != "" {
		notes = append(notes, "当前提交已打标签 "+tag)
	}
	mode, modeName := "--soft", "软回滚"
	if hard {
		mode, modeName = "--hard", "硬回滚"
	}
	if _, errs, err := run(dir, "reset", mode, sha); err != nil {
		return "", fmt.Errorf("%s", strings.TrimSpace(errs))
	}
	msg := fmt.Sprintf("%s完成，已回到 %s", modeName, shortSHA(sha))
	if len(notes) > 0 {
		msg += "（" + strings.Join(notes, "；") + "）"
	}
	return msg, nil
}

func DiscardChanges(dir, stamp string) (string, error) {
	stashed, err := stashBackup(dir, "Vibe-Manager 保底备份 "+stamp)
	if err != nil {
		return "", err
	}
	if !stashed {
		return "工作区本来就没有未提交改动", nil
	}
	return "已撤销未提交改动（原改动已存入 stash，可随时恢复）", nil
}

func shortSHA(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}

func Stamp() string {
	return time.Now().Format("20060102-150405")
}
