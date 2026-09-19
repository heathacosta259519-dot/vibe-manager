package project

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"vibe-manager/internal/gitx"
	"vibe-manager/internal/model"
)

func TestValidName(t *testing.T) {
	if !ValidName("my-proj") {
		t.Fatal("my-proj should be valid")
	}
	for _, bad := range []string{"My_Proj", "my proj", "-bad", "bad-", "Bad", ""} {
		if ValidName(bad) {
			t.Fatalf("%q should be invalid", bad)
		}
	}
}

// 项目根没设时必须拒绝：否则 filepath.Join("", name) 会退化成相对路径，
// 在进程当前目录里凭空建出目录。
func TestCreateRejectsEmptyRoot(t *testing.T) {
	for _, root := range []string{"", "   "} {
		if _, err := Create(root, model.CreateOptions{Name: "no-root-proj"}); err == nil {
			t.Fatalf("root=%q 应当被拒绝", root)
		}
	}
	if _, err := os.Stat("no-root-proj"); err == nil {
		t.Fatal("被拒绝后不应在当前目录留下目录")
	}
}

func TestCreateAndScan(t *testing.T) {
	root := t.TempDir()

	opts := model.CreateOptions{
		Name:      "my-proj",
		Readme:    true,
		Gitignore: true,
		License:   "MIT",
		Src:       true,
		Agents:    true,
		GitInit:   true,
		Author:    "测试署名",
	}
	path, err := Create(root, opts)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	for _, f := range []string{"README.md", "LICENSE", ".gitignore", "AGENTS.md", "src/.gitkeep"} {
		if _, err := os.Stat(filepath.Join(path, filepath.FromSlash(f))); err != nil {
			t.Fatalf("missing %s: %v", f, err)
		}
	}
	if !gitx.IsRepo(path) {
		t.Fatal("new project should be a git repo")
	}
	lic, err := os.ReadFile(filepath.Join(path, "LICENSE"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(lic), "MIT License") || !strings.Contains(string(lic), "测试署名") {
		t.Fatalf("MIT 协议内容不对：%s", string(lic)[:80])
	}
	agents, err := os.ReadFile(filepath.Join(path, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(agents), "改完就提交") {
		t.Fatal("AGENTS.md 里应当有提交纪律")
	}
	if _, err := Create(root, opts); err == nil {
		t.Fatal("duplicate project name should fail")
	}

	if err := os.MkdirAll(filepath.Join(root, "plain"), 0o755); err != nil {
		t.Fatal(err)
	}
	list, err := Scan(root)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("want 2 projects, got %d", len(list))
	}
}

// 勾选项应当真的生效：什么都不勾就只建一个空目录，也不初始化 git。
func TestCreateRespectsCheckboxes(t *testing.T) {
	root := t.TempDir()
	path, err := Create(root, model.CreateOptions{Name: "bare"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("不勾任何项时不应生成文件，却得到 %d 个", len(entries))
	}
	if gitx.IsRepo(path) {
		t.Fatal("不勾 git init 时不应初始化仓库")
	}

	// 只勾协议 + 不勾其它
	path2, err := Create(root, model.CreateOptions{Name: "only-license", License: "Apache-2.0", Author: "甲"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := os.Stat(filepath.Join(path2, "LICENSE")); err != nil {
		t.Fatal("应当生成 LICENSE")
	}
	if _, err := os.Stat(filepath.Join(path2, "README.md")); !os.IsNotExist(err) {
		t.Fatal("没勾 README 就不该生成")
	}
}

func TestLicenseText(t *testing.T) {
	want := map[string]string{
		"MIT":          "MIT License",
		"Apache-2.0":   "END OF TERMS AND CONDITIONS",
		"BSD-3-Clause": "BSD 3-Clause License",
		"ISC":          "ISC License",
		"MPL-2.0":      "Mozilla Public License Version 2.0",
		"GPL-3.0":      "GNU GENERAL PUBLIC LICENSE",
	}
	for id, marker := range want {
		text, err := LicenseText(id, "某人")
		if err != nil {
			t.Fatalf("%s 读取失败：%v", id, err)
		}
		if len(text) < 500 {
			t.Fatalf("%s 正文太短（%d 字节），可能没读全", id, len(text))
		}
		if !strings.Contains(text, marker) {
			t.Fatalf("%s 不像该协议的正文（找不到 %q）", id, marker)
		}
		if !strings.HasSuffix(text, "\n") {
			t.Fatalf("%s 应以换行结尾", id)
		}
	}

	// 短协议的署名与年份要填进去
	mit, _ := LicenseText("MIT", "张三")
	if !strings.Contains(mit, "张三") {
		t.Fatal("MIT 应填署名")
	}
	if !strings.Contains(mit, fmt.Sprintf("%d", time.Now().Year())) {
		t.Fatal("MIT 应填年份")
	}
	// 若协议文本里带附录占位符，应当被替换掉
	ap, _ := LicenseText("Apache-2.0", "李四")
	if strings.Contains(ap, "[yyyy] [name of copyright owner]") {
		t.Fatal("Apache-2.0 的附录占位符应被替换")
	}

	// 空 id 表示不加协议；未知 id 也不应报错
	if text, err := LicenseText("", "谁"); err != nil || text != "" {
		t.Fatalf("空 id 应返回空文本：%q %v", text, err)
	}
	if text, err := LicenseText("WTFPL", "谁"); err != nil || text != "" {
		t.Fatalf("未知 id 应返回空文本：%q %v", text, err)
	}
}

func TestLicenseOptionsCoverRegistry(t *testing.T) {
	for _, o := range LicenseOptions {
		if o.ID == "" {
			continue
		}
		if _, ok := licenseFiles[o.ID]; !ok {
			t.Fatalf("选项 %s 没有对应的正文文件", o.ID)
		}
		if LicenseName(o.ID) != o.Name {
			t.Fatalf("LicenseName(%s) 不一致", o.ID)
		}
	}
}

// 真仓库在子目录里时（如 winglass\WinGlass），git 信息必须来自实际根目录。
func TestDescribeUsesRootOverride(t *testing.T) {
	root := t.TempDir()
	outer := filepath.Join(root, "winglass")
	inner := filepath.Join(outer, "WinGlass")
	if err := os.MkdirAll(inner, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := gitx.Init(inner); err != nil {
		t.Fatalf("init: %v", err)
	}
	if err := os.WriteFile(filepath.Join(inner, "README.md"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := gitx.CommitAll(inner, "chore: init"); err != nil {
		t.Fatalf("commit: %v", err)
	}
	if err := os.WriteFile(filepath.Join(outer, "notes.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err) // 外层不是 git 仓库
	}

	// 没设实际根：外层不是仓库，识别不出来
	p := Describe(Entry{Path: outer})
	if p.IsGit {
		t.Fatal("外层不该被当成 git 仓库")
	}
	if p.RootOverridden {
		t.Fatal("没设根时不该标记为已覆盖")
	}

	// 设了实际根：git 信息来自内层
	p = Describe(Entry{Path: outer, Root: inner, Alias: "窗外"})
	if !p.IsGit {
		t.Fatal("设了实际根后应当识别为 git 仓库")
	}
	if !p.RootOverridden || p.Path != filepath.Clean(inner) || p.Entry != filepath.Clean(outer) {
		t.Fatalf("根覆盖字段不对：%+v", p)
	}
	if p.Alias != "窗外" || p.Name != "winglass" {
		t.Fatalf("备注名/名称不对：%+v", p)
	}
	if p.LastCommit == "" {
		t.Fatal("应当能读到提交信息")
	}
}

func TestScanOnlyListsDirectChildren(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"a", "b"} {
		if err := os.MkdirAll(filepath.Join(root, name, "inner"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "file.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	entries, err := Scan(root)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("应当只列出 2 个直接子目录，得到 %d", len(entries))
	}
}
