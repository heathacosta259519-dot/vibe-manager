package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"vibe-manager/internal/fsx"
	"vibe-manager/internal/gitx"
	"vibe-manager/internal/model"
)

// stubRemove 替换删除实现，避免单测真的往系统回收站塞文件。
func stubRemove(t *testing.T) {
	t.Helper()
	old := fsx.SetRemoveFunc(func(path string) error { return os.RemoveAll(path) })
	t.Cleanup(func() { fsx.SetRemoveFunc(old) })
}

func newTestApp(t *testing.T) (*App, string, string) {
	t.Helper()
	root := t.TempDir()
	proj := filepath.Join(root, "proj")
	if err := os.MkdirAll(filepath.Join(proj, "222", "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(proj, "222", "sub", "x.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(proj, "keep.md"), []byte("k"), 0o644); err != nil {
		t.Fatal(err)
	}
	return &App{cfg: model.Config{ProjectRoot: root}}, root, proj
}

func TestEnsureProject(t *testing.T) {
	app, root, proj := newTestApp(t)

	if err := app.ensureProject(proj); err != nil {
		t.Fatalf("直接子目录应当通过：%v", err)
	}
	for _, bad := range []string{root, filepath.Join(root, ".."), filepath.Join(proj, "222"), ""} {
		if err := app.ensureProject(bad); err == nil {
			t.Fatalf("%q 应当被拒绝", bad)
		}
	}
}

// 首次运行项目根是空的：列表应当为空、不报错，导入的项目照旧列出。
func TestListProjectsWithNoRoot(t *testing.T) {
	t.Setenv("APPDATA", t.TempDir())
	imported := t.TempDir()

	app := &App{cfg: model.Config{}}
	if _, err := app.ImportProject(imported); err != nil {
		t.Fatalf("导入失败：%v", err)
	}

	list, err := app.ListProjects()
	if err != nil {
		t.Fatalf("项目根为空时不应当报错：%v", err)
	}
	if len(list) != 1 {
		t.Fatalf("应当只列出导入的那一个项目，得到 %d 个", len(list))
	}
	if want := filepath.Base(imported); list[0].Name != want {
		t.Fatalf("项目名 = %q，想要 %q", list[0].Name, want)
	}
}

// 但填了却读不到（路径写错、盘没挂）仍旧要报错，否则用户不知道该去哪儿改。
func TestListProjectsWithUnreadableRoot(t *testing.T) {
	t.Setenv("APPDATA", t.TempDir())

	app := &App{cfg: model.Config{ProjectRoot: filepath.Join(t.TempDir(), "not-here")}}
	if _, err := app.ListProjects(); err == nil {
		t.Fatal("填了但读不到的项目根应当报错")
	}
}

func TestDeleteEntriesThroughBinding(t *testing.T) {
	stubRemove(t)
	app, _, proj := newTestApp(t)

	msg, err := app.DeleteEntries(proj, []string{"222"})
	if err != nil {
		t.Fatalf("删除失败：%v", err)
	}
	if _, err := os.Stat(filepath.Join(proj, "222")); !os.IsNotExist(err) {
		t.Fatal("222 应当已被删除")
	}
	if _, err := os.Stat(filepath.Join(proj, "keep.md")); err != nil {
		t.Fatal("不应动到其它文件")
	}
	t.Logf("返回信息: %s", msg)

	if _, err := app.DeleteEntries(proj, []string{".."}); err == nil {
		t.Fatal("越界删除应被拒绝")
	}
	if _, err := app.DeleteEntries(filepath.Join(proj, "x"), []string{"a"}); err == nil {
		t.Fatal("不在项目根内的 root 应被拒绝")
	}
}

const fixtureTaskBook = "## 任务 1：做点什么\n" +
	"目标：把功能做出来。\n" +
	"死规矩：\n" +
	"- 不许偷懒\n" +
	"验收：\n" +
	"```\n" +
	"grep -c x file\n" +
	"```\n"

func TestTaskBoardThroughBinding(t *testing.T) {
	app, _, proj := newTestApp(t)
	if err := os.WriteFile(filepath.Join(proj, "TASK.md"), []byte(fixtureTaskBook), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(proj, "BLOCKED.md"), []byte("- 待裁决：选哪种方案\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	board, err := app.GetTaskBoard(proj)
	if err != nil {
		t.Fatalf("GetTaskBoard: %v", err)
	}
	if !board.HasTaskDoc || board.Current != "TASK.md" {
		t.Fatalf("任务书识别失败：%+v", board)
	}
	if !board.Blocked {
		t.Fatal("应当识别到待裁决")
	}
	if len(board.Tasks) != 1 || len(board.Tasks[0].Checks) != 1 || len(board.Tasks[0].Rules) != 1 {
		t.Fatalf("任务小节解析失败：%+v", board.Tasks)
	}
	if _, err := app.GetTaskBoard(filepath.Join(proj, "222")); err == nil {
		t.Fatal("不在项目根内的路径应被拒绝")
	}
}

func TestSearchAllThroughBinding(t *testing.T) {
	app, root, proj := newTestApp(t)

	other := filepath.Join(root, "alpha")
	if err := os.MkdirAll(filepath.Join(other, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(other, "docs", "note.md"), []byte("第一行\n这里有个关键词 unicorn\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(proj, "noise.md"), []byte("无关内容\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	hits, err := app.SearchAll("unicorn")
	if err != nil {
		t.Fatalf("SearchAll: %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("应当只有 1 条内容命中，得到 %d：%+v", len(hits), hits)
	}
	h := hits[0]
	if h.Project != "alpha" || h.Kind != "content" || h.Path != "docs/note.md" || h.LineNo != 2 {
		t.Fatalf("命中内容不对：%+v", h)
	}

	// 项目名命中排在最前
	hits, err = app.SearchAll("alp")
	if err != nil {
		t.Fatalf("SearchAll: %v", err)
	}
	if len(hits) == 0 || hits[0].Kind != "project" || hits[0].Project != "alpha" {
		t.Fatalf("项目名命中应当排最前：%+v", hits)
	}

	hits, err = app.SearchAll("   ")
	if err != nil || len(hits) != 0 {
		t.Fatalf("空查询应返回空结果：%v %v", hits, err)
	}
}

func TestAliasAndCreateThroughBinding(t *testing.T) {
	// 备注名存在 %APPDATA% 下，测试必须隔离，别动到真实配置
	t.Setenv("APPDATA", t.TempDir())

	app, _, proj := newTestApp(t)

	if _, err := app.SetAlias(proj, "我的项目"); err != nil {
		t.Fatalf("SetAlias: %v", err)
	}
	list, err := app.ListProjects()
	if err != nil {
		t.Fatalf("ListProjects: %v", err)
	}
	if len(list) != 1 || list[0].Alias != "我的项目" {
		t.Fatalf("备注名没合并进来：%+v", list)
	}

	if _, err := app.SetAlias(proj, "  "); err != nil {
		t.Fatalf("清空备注名: %v", err)
	}
	list, _ = app.ListProjects()
	if list[0].Alias != "" {
		t.Fatal("备注名应被清除")
	}

	if _, err := app.SetAlias(filepath.Join(proj, "222"), "x"); err == nil {
		t.Fatal("不在项目根内的路径应被拒绝")
	}

	if len(app.GetLicenseOptions()) < 3 {
		t.Fatalf("协议选项太少：%v", app.GetLicenseOptions())
	}

	if _, err := app.CreateProject(model.CreateOptions{
		Name: "new-one", Readme: true, Agents: true, License: "GPL-3.0", GitInit: true, Author: "甲",
	}); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	made := filepath.Join(app.cfg.ProjectRoot, "new-one")
	for _, f := range []string{"README.md", "AGENTS.md", "LICENSE"} {
		if _, err := os.Stat(filepath.Join(made, f)); err != nil {
			t.Fatalf("应当生成 %s：%v", f, err)
		}
	}
	for _, f := range []string{".gitignore", "src"} {
		if _, err := os.Stat(filepath.Join(made, f)); !os.IsNotExist(err) {
			t.Fatalf("没勾选就不该生成 %s", f)
		}
	}
	lic, err := os.ReadFile(filepath.Join(made, "LICENSE"))
	if err != nil || !strings.Contains(string(lic), "GNU GENERAL PUBLIC LICENSE") {
		t.Fatalf("LICENSE 不是 GPL 正文：%v", err)
	}
}

func TestImportAndRootOverrideThroughBinding(t *testing.T) {
	// 元数据存在 %APPDATA% 下，必须隔离
	t.Setenv("APPDATA", t.TempDir())
	app, root, proj := newTestApp(t)

	// —— 导入项目根之外的目录 ——
	outside := filepath.Join(t.TempDir(), "real-repo")
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := gitx.Init(outside); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "a.txt"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := gitx.CommitAll(outside, "chore: init"); err != nil {
		t.Fatal(err)
	}

	if _, err := app.ImportProject(outside); err != nil {
		t.Fatalf("ImportProject: %v", err)
	}
	if err := app.ensureProject(outside); err != nil {
		t.Fatalf("导入的项目应当被放行：%v", err)
	}
	list, err := app.ListProjects()
	if err != nil {
		t.Fatalf("ListProjects: %v", err)
	}
	var found *model.Project
	for i := range list {
		if list[i].Entry == filepath.Clean(outside) {
			found = &list[i]
		}
	}
	if found == nil {
		t.Fatalf("导入的项目没出现在列表里：%+v", list)
	}
	if !found.Imported || !found.IsGit {
		t.Fatalf("导入项目的标记不对：%+v", found)
	}

	// 不存在的目录要拒绝
	if _, err := app.ImportProject(filepath.Join(outside, "nope")); err == nil {
		t.Fatal("不存在的目录不该能导入")
	}

	// 移出列表后不再放行
	if _, err := app.RemoveImportedProject(outside); err != nil {
		t.Fatalf("RemoveImportedProject: %v", err)
	}
	if err := app.ensureProject(outside); err == nil {
		t.Fatal("移出后不该再放行")
	}

	// —— 真仓库在子目录里：winglass\WinGlass ——
	outer := filepath.Join(root, "winglass")
	nested := filepath.Join(outer, "WinGlass")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := gitx.Init(nested); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "b.txt"), []byte("b"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := gitx.CommitAll(nested, "chore: init"); err != nil {
		t.Fatal(err)
	}

	// 设之前：外层不是仓库
	list, _ = app.ListProjects()
	if p := findByEntry(t, list, outer); p.IsGit || p.RootOverridden {
		t.Fatalf("设根之前不该识别为仓库：%+v", p)
	}

	if _, err := app.SetProjectRoot(outer, nested); err != nil {
		t.Fatalf("SetProjectRoot: %v", err)
	}
	if err := app.ensureProject(nested); err != nil {
		t.Fatalf("实际根应当被放行：%v", err)
	}
	list, _ = app.ListProjects()
	p := findByEntry(t, list, outer)
	if !p.IsGit || !p.RootOverridden || p.Path != filepath.Clean(nested) {
		t.Fatalf("设根之后应当用子目录里的仓库：%+v", p)
	}
	if p.Name != "winglass" {
		t.Fatalf("名称应保持条目名：%+v", p)
	}

	// 恢复默认
	if _, err := app.SetProjectRoot(outer, ""); err != nil {
		t.Fatalf("恢复默认: %v", err)
	}
	list, _ = app.ListProjects()
	if p := findByEntry(t, list, outer); p.RootOverridden || p.IsGit {
		t.Fatalf("恢复后不该再有覆盖：%+v", p)
	}

	// 不存在的根要拒绝；与条目相同也要拒绝
	if _, err := app.SetProjectRoot(outer, filepath.Join(outer, "nope")); err == nil {
		t.Fatal("不存在的根不该能设置")
	}
	if _, err := app.SetProjectRoot(outer, outer); err == nil {
		t.Fatal("与当前相同的根不该能设置")
	}

	_ = proj
}

func findByEntry(t *testing.T, list []model.Project, entry string) model.Project {
	t.Helper()
	for _, p := range list {
		if p.Entry == filepath.Clean(entry) {
			return p
		}
	}
	t.Fatalf("列表里找不到条目 %s：%+v", entry, list)
	return model.Project{}
}

func TestFileOpsThroughBinding(t *testing.T) {
	stubRemove(t)
	app, _, proj := newTestApp(t)

	if _, err := app.NewFolder(proj, "", "docs"); err != nil {
		t.Fatalf("新建文件夹失败：%v", err)
	}
	if _, err := app.RenameEntry(proj, "keep.md", "kept.md", "fail"); err != nil {
		t.Fatalf("重命名失败：%v", err)
	}
	conflicts, err := app.CheckConflicts(proj, []string{"kept.md"}, "")
	if err != nil {
		t.Fatalf("冲突检测失败：%v", err)
	}
	if len(conflicts) != 0 {
		t.Fatalf("同目录不应报冲突：%v", conflicts)
	}
	if _, err := app.MoveEntries(proj, []string{"kept.md"}, "docs", "fail"); err != nil {
		t.Fatalf("移动失败：%v", err)
	}
	if _, err := app.CopyEntries(proj, []string{"docs/kept.md"}, "", "fail"); err != nil {
		t.Fatalf("复制失败：%v", err)
	}
	if _, err := app.CopyEntries(proj, []string{"docs/kept.md"}, "", "fail"); err == nil {
		t.Fatal("无策略重复复制应报冲突")
	}
	if _, err := app.CopyEntries(proj, []string{"docs/kept.md"}, "", "rename"); err != nil {
		t.Fatalf("保留两者复制失败：%v", err)
	}
	if _, err := os.Stat(filepath.Join(proj, "kept (1).md")); err != nil {
		t.Fatal("保留两者应当生成 kept (1).md")
	}
}
