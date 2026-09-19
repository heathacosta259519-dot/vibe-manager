package gitx

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	mustGit(t, dir, "init")
	mustGit(t, dir, "config", "user.email", "t@example.com")
	mustGit(t, dir, "config", "user.name", "tester")
	mustGit(t, dir, "checkout", "-b", "main")
	writeText(t, filepath.Join(dir, "keep.txt"), "keep\n")
	writeText(t, filepath.Join(dir, "mod.txt"), "old\n")
	writeText(t, filepath.Join(dir, "del.txt"), "bye\n")
	mustGit(t, dir, "add", "-A")
	mustGit(t, dir, "commit", "-m", "init")
	return dir
}

func TestWorktreeFilesMixedStates(t *testing.T) {
	dir := newRepo(t)

	writeText(t, filepath.Join(dir, "new.txt"), "new\n")             // 新增（未跟踪）
	writeText(t, filepath.Join(dir, "mod.txt"), "changed\n")         // 修改
	if err := os.Remove(filepath.Join(dir, "del.txt")); err != nil { // 删除
		t.Fatal(err)
	}
	// 文件名里带空格，验证解析不会被切开
	writeText(t, filepath.Join(dir, "with space.txt"), "sp\n")

	files, err := WorktreeFiles(dir)
	if err != nil {
		t.Fatalf("WorktreeFiles: %v", err)
	}
	got := map[string]string{}
	staged := map[string]bool{}
	for _, f := range files {
		got[f.Path] = f.Status
		staged[f.Path] = f.Staged
	}
	want := map[string]string{"new.txt": "A", "mod.txt": "M", "del.txt": "D", "with space.txt": "A"}
	for path, status := range want {
		if got[path] != status {
			t.Fatalf("%s 状态应为 %s，得到 %q（全部：%+v）", path, status, got[path], got)
		}
	}
	if staged["mod.txt"] {
		t.Fatal("未暂存的修改不该标记为 staged")
	}

	// 暂存一部分后，Staged 应为真
	mustGit(t, dir, "add", "--", "mod.txt")
	files, _ = WorktreeFiles(dir)
	for _, f := range files {
		if f.Path == "mod.txt" && !f.Staged {
			t.Fatalf("暂存后应为 staged：%+v", f)
		}
	}
}

func TestDiffFile(t *testing.T) {
	dir := newRepo(t)
	writeText(t, filepath.Join(dir, "mod.txt"), "brand new line\n")

	diff, err := DiffFile(dir, "mod.txt", false)
	if err != nil {
		t.Fatalf("DiffFile: %v", err)
	}
	if !strings.Contains(diff, "brand new line") || !strings.Contains(diff, "-old") {
		t.Fatalf("diff 内容不对：\n%s", diff)
	}

	// 未跟踪文件没有 diff，应当给一句人话
	writeText(t, filepath.Join(dir, "untracked.txt"), "x\n")
	empty, err := DiffFile(dir, "untracked.txt", false)
	if err != nil {
		t.Fatalf("DiffFile: %v", err)
	}
	if !strings.Contains(empty, "没有可显示的差异") {
		t.Fatalf("未跟踪文件应给提示，得到：%q", empty)
	}
}

func TestCommitPathsOnlyCommitsSelected(t *testing.T) {
	dir := newRepo(t)
	writeText(t, filepath.Join(dir, "a.txt"), "a\n")
	writeText(t, filepath.Join(dir, "b.txt"), "b\n")

	sha, changed, err := CommitPaths(dir, []string{"a.txt"}, "feat: 只提交 a", false)
	if err != nil {
		t.Fatalf("CommitPaths: %v", err)
	}
	if !changed || sha == "" {
		t.Fatal("应当产生提交")
	}

	// a.txt 已提交，b.txt 还留在工作区
	files, _ := WorktreeFiles(dir)
	if len(files) != 1 || files[0].Path != "b.txt" {
		t.Fatalf("只应剩下 b.txt 未提交，得到 %+v", files)
	}
	// 提交信息正确
	subject := strings.TrimSpace(mustGit(t, dir, "log", "-1", "--pretty=%s"))
	if subject != "feat: 只提交 a" {
		t.Fatalf("提交信息不对：%q", subject)
	}
}

func TestCommitPathsGuards(t *testing.T) {
	dir := newRepo(t)
	writeText(t, filepath.Join(dir, "a.txt"), "a\n")

	if _, _, err := CommitPaths(dir, []string{"a.txt"}, "   ", false); err == nil {
		t.Fatal("空提交信息应被拒绝")
	}
	if _, _, err := CommitPaths(dir, nil, "msg", false); err == nil {
		t.Fatal("没勾选文件应被拒绝")
	}
	if _, _, err := CommitPaths(dir, []string{"nope.txt"}, "msg", false); err == nil {
		t.Fatal("不存在的路径应被拒绝")
	}
}

func TestCommitPathsAmend(t *testing.T) {
	dir := newRepo(t)
	writeText(t, filepath.Join(dir, "a.txt"), "a\n")
	if _, _, err := CommitPaths(dir, []string{"a.txt"}, "feat: 原始信息", false); err != nil {
		t.Fatalf("CommitPaths: %v", err)
	}
	before := strings.Count(mustGit(t, dir, "log", "--pretty=%H"), "\n")

	// amend 只改信息，不新增提交
	writeText(t, filepath.Join(dir, "a.txt"), "a2\n")
	if _, _, err := CommitPaths(dir, []string{"a.txt"}, "feat: 改过的信息", true); err != nil {
		t.Fatalf("amend: %v", err)
	}
	after := strings.Count(mustGit(t, dir, "log", "--pretty=%H"), "\n")
	if after != before {
		t.Fatalf("amend 不该增加提交数：%d → %d", before, after)
	}
	subject := strings.TrimSpace(mustGit(t, dir, "log", "-1", "--pretty=%s"))
	if subject != "feat: 改过的信息" {
		t.Fatalf("amend 后信息不对：%q", subject)
	}
	// 内容是 amend 后的
	body, _ := os.ReadFile(filepath.Join(dir, "a.txt"))
	if string(body) != "a2\n" {
		t.Fatalf("amend 应带上新内容，得到 %q", body)
	}
}

func TestStatusOfMappings(t *testing.T) {
	cases := map[string]string{
		"M.": "M", ".M": "M", "MM": "M",
		"A.": "A", ".A": "A",
		"D.": "D", ".D": "D",
		"R.": "R", "C.": "R",
		"UU": "U", "AA": "U", "DD": "U",
	}
	for xy, want := range cases {
		if got := statusOf(xy); got != want {
			t.Fatalf("statusOf(%q) = %q，期望 %q", xy, got, want)
		}
	}
}

func TestAfterSpaces(t *testing.T) {
	if got := afterSpaces("1 M. N... 100644 100644 100644 abc def src/a.go", 8); got != "src/a.go" {
		t.Fatalf("afterSpaces 取路径失败：%q", got)
	}
	if got := afterSpaces("a b c", 9); got != "" {
		t.Fatalf("不足 n 个空格时应返回空：%q", got)
	}
}
