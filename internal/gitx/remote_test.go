package gitx

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"vibe-manager/internal/model"
)

// mustGit 在指定目录跑一条 git 命令，失败即 t.Fatal。
func mustGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		t.Fatalf("git %v 失败：%v\n%s", strings.Join(args, " "), err, errb.String())
	}
	return out.String()
}

func writeText(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// newRemoteFixture 造一套本地环境：一个裸仓库当 origin + 一个已提交的本地仓库。
// 全程不联网，也绝不碰用户真实的仓库。
func newRemoteFixture(t *testing.T) (origin string, work string) {
	t.Helper()
	base := t.TempDir()
	origin = filepath.Join(base, "origin.git")
	work = filepath.Join(base, "work")

	mustGit(t, base, "init", "--bare", origin)
	// 裸仓库默认装的是 master，显式指向 main，否则 clone 出来没有 main 分支
	mustGit(t, origin, "symbolic-ref", "HEAD", "refs/heads/main")
	mustGit(t, base, "init", work)
	mustGit(t, work, "config", "user.email", "t@example.com")
	mustGit(t, work, "config", "user.name", "tester")
	mustGit(t, work, "checkout", "-b", "main")

	writeText(t, filepath.Join(work, "a.txt"), "a\n")
	mustGit(t, work, "add", "-A")
	mustGit(t, work, "commit", "-m", "first")
	mustGit(t, work, "remote", "add", "origin", filepath.ToSlash(origin))
	return origin, work
}

func TestRemotesAndPushRoundTrip(t *testing.T) {
	origin, work := newRemoteFixture(t)

	remotes, err := Remotes(work)
	if err != nil {
		t.Fatalf("Remotes: %v", err)
	}
	if len(remotes) != 1 || remotes[0].Name != "origin" {
		t.Fatalf("远程列表不对：%+v", remotes)
	}
	if !strings.Contains(remotes[0].URL, "origin.git") {
		t.Fatalf("远程地址不对：%+v", remotes[0])
	}

	// 还没推送过：没有上游
	st, err := Status(work)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if st.HasUpstream {
		t.Fatal("此时不该有上游")
	}

	// 推送并设上游
	res, err := Push(work, "origin", "main", true)
	if err != nil {
		t.Fatalf("Push: %v", err)
	}
	if !strings.Contains(res.Message, "已推送") {
		t.Fatalf("推送结果不对：%q", res.Message)
	}

	st, _ = Status(work)
	if !st.HasUpstream || st.Ahead != 0 || st.Behind != 0 {
		t.Fatalf("推送后状态不对：%+v", st)
	}

	// 再来一个本地提交 → ahead 1
	writeText(t, filepath.Join(work, "b.txt"), "b\n")
	mustGit(t, work, "add", "-A")
	mustGit(t, work, "commit", "-m", "second")
	st, _ = Status(work)
	if st.Ahead != 1 {
		t.Fatalf("提交后应当领先 1，得到 %+v", st)
	}

	// 重复推送到已是最新的情况
	if _, err := Push(work, "origin", "main", false); err != nil {
		t.Fatalf("Push: %v", err)
	}
	res, err = Push(work, "origin", "main", false)
	if err != nil {
		t.Fatalf("Push: %v", err)
	}
	if !strings.Contains(res.Message, "最新") {
		t.Fatalf("重复推送应当提示已是最新：%q", res.Message)
	}

	// 换个 clone 往远端推一条 → 本地 fetch 后应该落后 1
	other := filepath.Join(filepath.Dir(work), "other")
	mustGit(t, filepath.Dir(work), "clone", filepath.ToSlash(origin), other)
	mustGit(t, other, "config", "user.email", "t2@example.com")
	mustGit(t, other, "config", "user.name", "tester2")
	writeText(t, filepath.Join(other, "c.txt"), "c\n")
	mustGit(t, other, "add", "-A")
	mustGit(t, other, "commit", "-m", "third")
	mustGit(t, other, "push", "origin", "main")

	if _, err := Fetch(work, "origin", true); err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	st, _ = Status(work)
	if st.Behind != 1 {
		t.Fatalf("抓取后应当落后 1，得到 %+v", st)
	}

	// 拉取应当把远端的提交合并进来
	res, err = Pull(work, "origin", "main", false)
	if err != nil {
		t.Fatalf("Pull: %v", err)
	}
	if res.Message == "" {
		t.Fatal("拉取应当有结果说明")
	}
	if _, err := os.Stat(filepath.Join(work, "c.txt")); err != nil {
		t.Fatalf("拉取后应当有 c.txt：%v", err)
	}
	st, _ = Status(work)
	if st.Behind != 0 || st.Ahead != 0 {
		t.Fatalf("拉取后应当与远端一致：%+v", st)
	}
}

func TestPushWithoutRemote(t *testing.T) {
	dir := t.TempDir()
	mustGit(t, dir, "init")
	_, err := Push(dir, "", "main", false)
	if err == nil || !strings.Contains(err.Error(), "远程仓库") {
		t.Fatalf("没有远程时应当给出明确提示，得到：%v", err)
	}
}

func TestPushRejectedIsExplained(t *testing.T) {
	origin, work := newRemoteFixture(t)
	if _, err := Push(work, "origin", "main", true); err != nil {
		t.Fatalf("Push: %v", err)
	}

	// 另一个人推了一条，本地就落后了
	other := filepath.Join(filepath.Dir(work), "other2")
	mustGit(t, filepath.Dir(work), "clone", filepath.ToSlash(origin), other)
	mustGit(t, other, "config", "user.email", "t3@example.com")
	mustGit(t, other, "config", "user.name", "tester3")
	writeText(t, filepath.Join(other, "d.txt"), "d\n")
	mustGit(t, other, "add", "-A")
	mustGit(t, other, "commit", "-m", "remote ahead")
	mustGit(t, other, "push", "origin", "main")

	// 本地也提交一条 → 推送必然被拒（非快进）
	writeText(t, filepath.Join(work, "e.txt"), "e\n")
	mustGit(t, work, "add", "-A")
	mustGit(t, work, "commit", "-m", "local ahead")

	_, err := Push(work, "origin", "main", false)
	if err == nil {
		t.Fatal("非快进推送应当失败")
	}
	if !strings.Contains(err.Error(), "先「拉取」再推送") {
		t.Fatalf("应当把非快进翻译成人话，得到：%v", err)
	}
}

func TestExplainMappings(t *testing.T) {
	cases := map[string]string{
		"fatal: could not read Username for 'https://github.com'":  "认证失败",
		"! [rejected]        main -> main (non-fast-forward)":      "先「拉取」再推送",
		"fatal: The current branch main has no upstream branch.":   "还没有上游",
		"fatal: unable to access 'https://...': Failed to connect": "连不上远端",
		"remote: Repository not found.":                            "远端仓库不存在",
		"error: Your local changes would be overwritten by merge":  "未提交改动",
		"fatal: refusing to merge unrelated histories":             "历史无关",
		"一些完全没见过的报错":                                               "",
	}
	for stderr, want := range cases {
		got := Explain(stderr)
		if want == "" {
			if got != "" {
				t.Fatalf("%q 不该被翻译：%q", stderr, got)
			}
			continue
		}
		if !strings.Contains(got, want) {
			t.Fatalf("%q 应包含 %q，得到 %q", stderr, want, got)
		}
	}
}

func TestConflictFilesExtraction(t *testing.T) {
	out := `Auto-merging a.txt
CONFLICT (content): Merge conflict in a.txt
CONFLICT (modify/delete): b.txt deleted in HEAD and modified in other
	both modified:   c.txt
`
	got := conflictFiles(out)
	want := []string{"a.txt", "b.txt", "c.txt"}
	if len(got) != len(want) {
		t.Fatalf("冲突文件提取不对：%v", got)
	}
	for i, w := range want {
		if got[i] != w {
			t.Fatalf("冲突文件提取不对：%v，期望 %v", got, want)
		}
	}
	if n := len(conflictFiles("没有冲突")); n != 0 {
		t.Fatalf("没有冲突时不该有文件，得到 %d", n)
	}
}

func TestTailTruncates(t *testing.T) {
	long := strings.Repeat("x", tailChars+100)
	got := tail(long)
	if len([]rune(got)) > tailChars+1 { // 多一个前导省略号
		t.Fatalf("tail 没截断：%d", len([]rune(got)))
	}
	if !strings.HasPrefix(got, "…") {
		t.Fatal("截断后应有省略号前缀")
	}
	var _ model.Remote
}
