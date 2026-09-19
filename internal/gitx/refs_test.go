package gitx

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBranchesAndCheckout(t *testing.T) {
	dir := newRepo(t)
	mustGit(t, dir, "branch", "feature")

	list, err := Branches(dir)
	if err != nil {
		t.Fatalf("Branches: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("应当有 2 个分支，得到 %d：%+v", len(list), list)
	}
	cur := ""
	for _, b := range list {
		if b.Current {
			cur = b.Name
		}
		if b.When == 0 || b.Subject == "" {
			t.Fatalf("分支信息不全：%+v", b)
		}
	}
	if cur != "main" {
		t.Fatalf("当前分支应为 main，得到 %q", cur)
	}

	if _, err := Checkout(dir, "feature"); err != nil {
		t.Fatalf("Checkout: %v", err)
	}
	list, _ = Branches(dir)
	for _, b := range list {
		if b.Name == "feature" && !b.Current {
			t.Fatal("切换后 feature 应当成为当前分支")
		}
	}
}

func TestCreateAndDeleteBranch(t *testing.T) {
	dir := newRepo(t)

	res, err := CreateBranch(dir, "topic", true)
	if err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}
	if !strings.Contains(res.Message, "已新建并切换") {
		t.Fatalf("结果说明不对：%q", res.Message)
	}

	// 非法名
	if _, err := CreateBranch(dir, "bad name", false); err == nil {
		t.Fatal("带空格的分支名应被拒绝")
	}
	// 当前分支不能删
	if _, err := DeleteBranch(dir, "topic", false); err == nil {
		t.Fatal("删除当前分支应当失败")
	}

	if _, err := Checkout(dir, "main"); err != nil {
		t.Fatalf("Checkout: %v", err)
	}
	// topic 与 main 内容相同，可以直接删
	if _, err := DeleteBranch(dir, "topic", false); err != nil {
		t.Fatalf("删除已合并分支：%v", err)
	}
}

func TestDeleteBranchRefusesUnmerged(t *testing.T) {
	dir := newRepo(t)
	mustGit(t, dir, "checkout", "-b", "work")
	writeText(t, filepath.Join(dir, "w.txt"), "w\n")
	mustGit(t, dir, "add", "-A")
	mustGit(t, dir, "commit", "-m", "work commit")
	mustGit(t, dir, "checkout", "main")

	_, err := DeleteBranch(dir, "work", false)
	if err == nil {
		t.Fatal("未合并的分支不该被直接删掉")
	}
	if !strings.Contains(err.Error(), "未合并") {
		t.Fatalf("应当说明未合并，得到：%v", err)
	}
	if _, err := DeleteBranch(dir, "work", true); err != nil {
		t.Fatalf("强制删除应当成功：%v", err)
	}
}

func TestMergeCleanAndConflict(t *testing.T) {
	dir := newRepo(t)

	// 干净合并：side 只动自己的新文件
	mustGit(t, dir, "checkout", "-b", "side")
	writeText(t, filepath.Join(dir, "side.txt"), "s\n")
	mustGit(t, dir, "add", "-A")
	mustGit(t, dir, "commit", "-m", "side only")
	mustGit(t, dir, "checkout", "main")

	if _, err := Merge(dir, "side"); err != nil {
		t.Fatalf("干净合并应当成功：%v", err)
	}
	res, err := Merge(dir, "side")
	if err != nil {
		t.Fatalf("重复合并不该报错：%v", err)
	}
	if !strings.Contains(res.Message, "无需合并") {
		t.Fatalf("重复合并应提示无需合并：%q", res.Message)
	}

	// 冲突：两边改同一个文件的同一处
	mustGit(t, dir, "checkout", "-b", "conflict-side")
	writeText(t, filepath.Join(dir, "mod.txt"), "side value\n")
	mustGit(t, dir, "add", "-A")
	mustGit(t, dir, "commit", "-m", "side mod")
	mustGit(t, dir, "checkout", "main")
	writeText(t, filepath.Join(dir, "mod.txt"), "main value\n")
	mustGit(t, dir, "add", "-A")
	mustGit(t, dir, "commit", "-m", "main mod")

	_, err = Merge(dir, "conflict-side")
	if err == nil {
		t.Fatal("必然冲突的合并应当报错")
	}
	if !strings.Contains(err.Error(), "mod.txt") {
		t.Fatalf("冲突错误里应列出冲突文件，得到：%v", err)
	}
	mustGit(t, dir, "merge", "--abort") // 收尾，别把仓库留在合并中
}

func TestRevert(t *testing.T) {
	dir := newRepo(t)

	writeText(t, filepath.Join(dir, "mod.txt"), "v2\n")
	mustGit(t, dir, "add", "-A")
	mustGit(t, dir, "commit", "-m", "改一下")
	target := strings.TrimSpace(mustGit(t, dir, "rev-parse", "HEAD"))
	before := strings.Count(mustGit(t, dir, "log", "--pretty=%H"), "\n")

	msg, err := Revert(dir, target)
	if err != nil {
		t.Fatalf("Revert: %v", err)
	}
	if !strings.Contains(msg, "撤销了") {
		t.Fatalf("结果说明不对：%q", msg)
	}

	// 内容回到上一次提交的状态（git 按 autocrlf 检出，统一换行再比）
	body, _ := os.ReadFile(filepath.Join(dir, "mod.txt"))
	if got := strings.ReplaceAll(string(body), "\r\n", "\n"); got != "old\n" {
		t.Fatalf("撤销后内容应为 old，得到 %q", got)
	}
	// 并且是**新增**了一条提交，而不是改写历史
	after := strings.Count(mustGit(t, dir, "log", "--pretty=%H"), "\n")
	if after != before+1 {
		t.Fatalf("撤销应当新增一条提交：%d → %d", before, after)
	}
	// 原来的提交还在
	if !strings.Contains(mustGit(t, dir, "log", "--pretty=%H"), target) {
		t.Fatal("被撤销的提交不该从历史里消失")
	}

	if _, err := Revert(dir, "不存在的sha"); err == nil {
		t.Fatal("无效 sha 应当报错")
	}
}

func TestTagsLifecycle(t *testing.T) {
	dir := newRepo(t)

	if _, err := CreateTag(dir, "v1.0.0", ""); err != nil {
		t.Fatalf("轻量标签：%v", err)
	}
	if _, err := CreateTag(dir, "v1.1.0", "第二个版本"); err != nil {
		t.Fatalf("附注标签：%v", err)
	}
	if _, err := CreateTag(dir, "v1.1.0", ""); err == nil {
		t.Fatal("重复标签应当失败")
	}
	if _, err := CreateTag(dir, "bad name", ""); err == nil {
		t.Fatal("带空格的标签名应被拒绝")
	}

	list, err := Tags(dir)
	if err != nil {
		t.Fatalf("Tags: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("应当有 2 个标签，得到 %d：%+v", len(list), list)
	}
	byName := map[string]bool{}
	for _, tag := range list {
		byName[tag.Name] = tag.Annotated
	}
	if byName["v1.0.0"] {
		t.Fatal("v1.0.0 是轻量标签")
	}
	if !byName["v1.1.0"] {
		t.Fatal("v1.1.0 是附注标签")
	}

	if _, err := DeleteTag(dir, "v1.0.0"); err != nil {
		t.Fatalf("删除标签：%v", err)
	}
	list, _ = Tags(dir)
	if len(list) != 1 || list[0].Name != "v1.1.0" {
		t.Fatalf("删除后应当只剩 v1.1.0：%+v", list)
	}
}

func TestPushTagToLocalRemote(t *testing.T) {
	origin, work := newRemoteFixture(t)
	if _, err := Push(work, "origin", "main", true); err != nil {
		t.Fatalf("Push: %v", err)
	}
	if _, err := CreateTag(work, "v0.1.0", "首个版本"); err != nil {
		t.Fatalf("CreateTag: %v", err)
	}
	if _, err := PushTag(work, "v0.1.0", "origin"); err != nil {
		t.Fatalf("PushTag: %v", err)
	}
	if tags := mustGit(t, origin, "tag", "-l"); !strings.Contains(tags, "v0.1.0") {
		t.Fatalf("远端应当有 v0.1.0，得到 %q", tags)
	}
	if _, err := PushTag(work, "v0.1.0", ""); err == nil {
		t.Fatal("没有远程时应给出提示")
	}
}
