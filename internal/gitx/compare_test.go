package gitx

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"vibe-manager/internal/model"
)

// 这些测试一律用本地临时仓库，不碰网络，也不碰真实仓库。

func newCompareRepo(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	if err := Init(dir); err != nil {
		t.Fatalf("init: %v", err)
	}
	// 行尾不做转换，否则重命名 / 内容比较会被自动换行搅乱
	if _, errs, err := run(dir, "config", "core.autocrlf", "false"); err != nil {
		t.Fatalf("config: %s", errs)
	}
	writeFile(t, dir, "keep.txt", "one\ntwo\n")
	writeFile(t, dir, "old.txt", "alpha\nbeta\ngamma\n")
	writeFile(t, dir, "del.txt", "bye\n")
	writeFile(t, dir, "bin.dat", "\x01\x00\x02\x00")
	if _, _, err := CommitAll(dir, "c1"); err != nil {
		t.Fatalf("commit c1: %v", err)
	}
	c1, _, err := run(dir, "rev-parse", "HEAD")
	if err != nil {
		t.Fatalf("rev-parse: %v", err)
	}
	return dir, strings.TrimSpace(c1)
}

func findStat(t *testing.T, files []model.DiffFileStat, path string) model.DiffFileStat {
	t.Helper()
	for _, f := range files {
		if f.Path == path {
			return f
		}
	}
	t.Fatalf("清单里没有 %s：%+v", path, files)
	return model.DiffFileStat{}
}

func TestDiffRefsBetweenCommits(t *testing.T) {
	dir, c1 := newCompareRepo(t)

	// 改动、删除、重命名、二进制改动各来一个，再提交成 c2
	writeFile(t, dir, "keep.txt", "one\nTWO\nthree\n")
	if err := os.Remove(filepath.Join(dir, "del.txt")); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(dir, "old.txt"), filepath.Join(dir, "new.txt")); err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, "bin.dat", "\x09\x00\x08\x00\x07")
	if _, _, err := CommitAll(dir, "c2"); err != nil {
		t.Fatalf("commit c2: %v", err)
	}
	c2, _, _ := run(dir, "rev-parse", "HEAD")

	summary, err := DiffRefs(dir, c1, strings.TrimSpace(c2))
	if err != nil {
		t.Fatalf("DiffRefs: %v", err)
	}
	if len(summary.Files) != 4 {
		t.Fatalf("应当有 4 个文件变更，得到 %d：%+v", len(summary.Files), summary.Files)
	}

	keep := findStat(t, summary.Files, "keep.txt")
	if keep.Status != "M" || keep.Adds != 2 || keep.Dels != 1 {
		t.Fatalf("keep.txt 统计不对：%+v", keep)
	}

	del := findStat(t, summary.Files, "del.txt")
	if del.Status != "D" {
		t.Fatalf("del.txt 应当是删除：%+v", del)
	}

	ren := findStat(t, summary.Files, "new.txt")
	if ren.Status != "R" || ren.OldPath != "old.txt" {
		t.Fatalf("重命名识别不对：%+v", ren)
	}

	bin := findStat(t, summary.Files, "bin.dat")
	if !bin.Binary || bin.Adds != -1 || bin.Dels != -1 {
		t.Fatalf("二进制文件的统计应当是未知：%+v", bin)
	}

	if summary.Adds <= 0 || summary.Dels <= 0 {
		t.Fatalf("汇总的增删行数不对：%+v", summary)
	}
}

func TestDiffRefsWorktreeIncludesUntracked(t *testing.T) {
	dir, c1 := newCompareRepo(t)
	writeFile(t, dir, "keep.txt", "one\nCHANGED\n")
	writeFile(t, dir, "fresh.txt", "n1\nn2\n")

	summary, err := DiffRefs(dir, c1, "")
	if err != nil {
		t.Fatalf("DiffRefs: %v", err)
	}
	fresh := findStat(t, summary.Files, "fresh.txt")
	if fresh.Status != "A" || !fresh.Untracked {
		t.Fatalf("未跟踪的新文件应当被补进清单：%+v", fresh)
	}
	if got := findStat(t, summary.Files, "keep.txt"); got.Status != "M" {
		t.Fatalf("已跟踪的改动丢了：%+v", got)
	}

	// 比工作区时不该冒出提交之间才有的东西
	for _, f := range summary.Files {
		if f.Path == "old.txt" || f.Path == "del.txt" {
			t.Fatalf("工作区没有这些改动：%+v", f)
		}
	}
}

func TestDiffRefFile(t *testing.T) {
	dir, c1 := newCompareRepo(t)
	writeFile(t, dir, "keep.txt", "one\nTWO\nthree\n")
	if _, _, err := CommitAll(dir, "c2"); err != nil {
		t.Fatalf("commit c2: %v", err)
	}
	c2, _, _ := run(dir, "rev-parse", "HEAD")

	text, err := DiffRefFile(dir, c1, strings.TrimSpace(c2), "keep.txt", false)
	if err != nil {
		t.Fatalf("DiffRefFile: %v", err)
	}
	if !strings.Contains(text.Text, "+TWO") || !strings.Contains(text.Text, "-two") {
		t.Fatalf("diff 内容不对：%s", text.Text)
	}
	if text.Truncated || text.Binary {
		t.Fatalf("不该被截断或当成二进制：%+v", text)
	}
}

func TestDiffRefFileUntracked(t *testing.T) {
	dir, c1 := newCompareRepo(t)
	writeFile(t, dir, "fresh.txt", "hello\nworld\n")

	text, err := DiffRefFile(dir, c1, "", "fresh.txt", false)
	if err != nil {
		t.Fatalf("DiffRefFile: %v", err)
	}
	if !strings.Contains(text.Text, "+hello") {
		t.Fatalf("未跟踪文件应当整份算新增：%s", text.Text)
	}
}

func TestDiffRefFileTruncatesHugeDiff(t *testing.T) {
	dir, _ := newCompareRepo(t)
	writeFile(t, dir, "huge.txt", strings.Repeat("a", 200_000))
	if _, _, err := CommitAll(dir, "big"); err != nil {
		t.Fatalf("commit: %v", err)
	}
	big, _, _ := run(dir, "rev-parse", "HEAD")
	writeFile(t, dir, "huge.txt", strings.Repeat("b", 200_000))
	if _, _, err := CommitAll(dir, "bigger"); err != nil {
		t.Fatalf("commit: %v", err)
	}
	huge, _, _ := run(dir, "rev-parse", "HEAD")

	cmp, err := DiffRefFile(dir, strings.TrimSpace(big), strings.TrimSpace(huge), "huge.txt", false)
	if err != nil {
		t.Fatalf("DiffRefFile: %v", err)
	}
	if !cmp.Truncated {
		t.Fatalf("超长 diff 应当被截断，实际长度 %d", len(cmp.Text))
	}
}

func TestDiffRefFileFullContext(t *testing.T) {
	dir, _ := newCompareRepo(t)
	lines := make([]string, 40)
	for i := range lines {
		lines[i] = fmt.Sprintf("L%02d", i+1)
	}
	writeFile(t, dir, "long.txt", strings.Join(lines, "\n")+"\n")
	if _, _, err := CommitAll(dir, "add long"); err != nil {
		t.Fatalf("commit: %v", err)
	}
	before, _, _ := run(dir, "rev-parse", "HEAD")

	// 只动首尾两行，中间一大段没变
	lines[0] = "L01-CHANGED"
	lines[39] = "L40-CHANGED"
	writeFile(t, dir, "long.txt", strings.Join(lines, "\n")+"\n")
	if _, _, err := CommitAll(dir, "touch ends"); err != nil {
		t.Fatalf("commit: %v", err)
	}
	after, _, _ := run(dir, "rev-parse", "HEAD")
	b, a := strings.TrimSpace(before), strings.TrimSpace(after)

	hunks, err := DiffRefFile(dir, b, a, "long.txt", false)
	if err != nil {
		t.Fatalf("DiffRefFile(hunks): %v", err)
	}
	if !strings.Contains(hunks.Text, "+L01-CHANGED") {
		t.Fatalf("hunk 视图应当包含改动：%s", hunks.Text)
	}
	if strings.Contains(hunks.Text, "L20") {
		t.Fatal("hunk 视图不该把中间没变的行也带出来")
	}

	full, err := DiffRefFile(dir, b, a, "long.txt", true)
	if err != nil {
		t.Fatalf("DiffRefFile(full): %v", err)
	}
	// 全量视图要能把整份文件带回来，界面才画得出完整代码 + 行号
	if !strings.Contains(full.Text, "L20") || !strings.Contains(full.Text, "L40-CHANGED") {
		t.Fatalf("全量视图应当是整份文件：%s", full.Text)
	}
	if full.Truncated {
		t.Fatal("这个体量不该被截断")
	}
}

func TestDiffCommitFile(t *testing.T) {
	dir, _ := newCompareRepo(t)
	writeFile(t, dir, "keep.txt", "one\nTWO\nthree\n")
	if _, _, err := CommitAll(dir, "c2"); err != nil {
		t.Fatalf("commit c2: %v", err)
	}
	c2, _, _ := run(dir, "rev-parse", "HEAD")
	sha := strings.TrimSpace(c2)

	text, err := DiffCommitFile(dir, sha, "keep.txt")
	if err != nil {
		t.Fatalf("DiffCommitFile: %v", err)
	}
	if !strings.Contains(text.Text, "+TWO") {
		t.Fatalf("diff 内容不对：%s", text.Text)
	}

	// 提交里没被动过的文件不该报错，只要给一句人话
	clean, err := DiffCommitFile(dir, sha, "bin.dat")
	if err != nil {
		t.Fatalf("DiffCommitFile(未改动文件): %v", err)
	}
	if strings.TrimSpace(clean.Text) == "" {
		t.Fatal("未改动文件也应当有说明文字")
	}

	if _, err := DiffCommitFile(dir, "--upload-pack=x", "keep.txt"); err == nil {
		t.Fatal("形如选项的 sha 应当被拒绝")
	}
	if _, err := DiffCommitFile(dir, sha, ""); err == nil {
		t.Fatal("空路径应当被拒绝")
	}
}

func TestDiffCommitFileRootCommit(t *testing.T) {
	dir, c1 := newCompareRepo(t)
	// 根提交没有父，git diff 会失败；git show 必须能把整份文件当新增
	text, err := DiffCommitFile(dir, c1, "keep.txt")
	if err != nil {
		t.Fatalf("根提交取 diff 失败：%v", err)
	}
	if !strings.Contains(text.Text, "+one") {
		t.Fatalf("根提交应当整份算新增：%s", text.Text)
	}
}

func TestCommitFilesKeepsRealPathForRename(t *testing.T) {
	dir, _ := newCompareRepo(t)
	if err := os.Rename(filepath.Join(dir, "old.txt"), filepath.Join(dir, "new.txt")); err != nil {
		t.Fatal(err)
	}
	if _, _, err := CommitAll(dir, "rename"); err != nil {
		t.Fatalf("commit: %v", err)
	}
	sha, _, _ := run(dir, "rev-parse", "HEAD")

	files, err := CommitFiles(dir, strings.TrimSpace(sha))
	if err != nil {
		t.Fatalf("CommitFiles: %v", err)
	}
	var ren *model.FileChange
	for i := range files {
		if files[i].Status == "R" {
			ren = &files[i]
		}
	}
	if ren == nil {
		t.Fatalf("没识别出重命名：%+v", files)
	}
	// Path 必须是可以直接拿去取 diff 的新路径，旧路径另放 OldPath
	if ren.Path != "new.txt" || ren.OldPath != "old.txt" {
		t.Fatalf("重命名路径拆分不对：%+v", ren)
	}
	if _, err := DiffCommitFile(dir, strings.TrimSpace(sha), ren.Path); err != nil {
		t.Fatalf("拿 Path 取 diff 失败：%v", err)
	}
}

func TestDiffRefsRejectsBadRefs(t *testing.T) {
	dir, _ := newCompareRepo(t)

	// 形如选项的字符串不能被当成版本名，否则等于把参数交给 git 自己解释
	for _, bad := range []string{"", "--upload-pack=touch x", "-x"} {
		if _, err := DiffRefs(dir, bad, ""); err == nil {
			t.Fatalf("%q 应当被拒绝", bad)
		}
	}
	if _, err := DiffRefs(dir, "HEAD", "no-such-ref"); err == nil {
		t.Fatal("不存在的版本应当被拒绝")
	}
}

func TestDiffRefsEmptyRepo(t *testing.T) {
	dir := t.TempDir()
	if err := Init(dir); err != nil {
		t.Fatalf("init: %v", err)
	}
	if _, err := DiffRefs(dir, "HEAD", ""); err == nil {
		t.Fatal("还没有提交时应当明确报错，而不是静默返回")
	}
	if _, err := DiffRefs(t.TempDir(), "HEAD", ""); err == nil {
		t.Fatal("非 git 目录应当被拒绝")
	}
}
