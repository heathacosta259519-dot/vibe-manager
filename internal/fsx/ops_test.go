package fsx

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// stubRecycle 用桩替换回收站，避免单测真的往系统回收站塞文件。
func stubRecycle(t *testing.T) *[]string {
	t.Helper()
	var recycled []string
	old := removeFileFunc
	removeFileFunc = func(path string) error {
		recycled = append(recycled, path)
		return os.RemoveAll(path)
	}
	t.Cleanup(func() { removeFileFunc = old })
	return &recycled
}

func TestValidateName(t *testing.T) {
	for _, good := range []string{"docs", "a.txt", "my-file.md", ".gitignore"} {
		if err := ValidateName(good); err != nil {
			t.Fatalf("%q 应当合法：%v", good, err)
		}
	}
	for _, bad := range []string{"", ".", "..", "a/b", `a\b`, "a:b", "CON", "nul", "trail ", "trail."} {
		if err := ValidateName(bad); err == nil {
			t.Fatalf("%q 应当被拒绝", bad)
		}
	}
}

func TestMkdirAndRename(t *testing.T) {
	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, "src"))

	rel, err := Mkdir(root, "", "docs")
	if err != nil || rel != "docs" {
		t.Fatalf("mkdir: %q %v", rel, err)
	}
	if _, err := Mkdir(root, "", "docs"); !errors.Is(err, ErrExists) {
		t.Fatalf("重复新建应返回 ErrExists，得到 %v", err)
	}
	if _, err := Mkdir(root, "", "bad/name"); err == nil {
		t.Fatal("含分隔符的名称应被拒绝")
	}
	if _, err := Mkdir(root, "", "CON"); err == nil {
		t.Fatal("Windows 保留名应被拒绝")
	}

	if _, err := Mkdir(root, "src", "inner"); err != nil {
		t.Fatalf("子目录新建失败：%v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "src", "inner")); err != nil {
		t.Fatal("子目录未创建")
	}

	newRel, err := Rename(root, "docs", "docs2", PolicyFail)
	if err != nil || newRel != "docs2" {
		t.Fatalf("rename: %q %v", newRel, err)
	}
	if _, err := os.Stat(filepath.Join(root, "docs2")); err != nil {
		t.Fatal("重命名后目标不存在")
	}

	mustWrite(t, filepath.Join(root, "a.txt"), "a")
	if _, err := Rename(root, "a.txt", "docs2", PolicyFail); !errors.Is(err, ErrExists) {
		t.Fatalf("撞名应返回 ErrExists，得到 %v", err)
	}
	renamed, err := Rename(root, "a.txt", "docs2", PolicyRename)
	if err != nil {
		t.Fatalf("改名策略失败：%v", err)
	}
	if renamed != "docs2 (1)" {
		t.Fatalf("自动改名结果 = %q", renamed)
	}
	if _, err := Rename(root, "", "x", PolicyFail); err == nil {
		t.Fatal("不能重命名项目根目录")
	}
}

func TestRemoveUsesRecycleBin(t *testing.T) {
	root := t.TempDir()
	recycled := stubRecycle(t)
	mustWrite(t, filepath.Join(root, "a.txt"), "a")
	mustMkdir(t, filepath.Join(root, "d"))

	if err := Remove2(t, root, []string{"a.txt", "d"}); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if len(*recycled) != 2 {
		t.Fatalf("应当经过 2 次回收站，实际 %d", len(*recycled))
	}
	if _, err := os.Stat(filepath.Join(root, "a.txt")); !os.IsNotExist(err) {
		t.Fatal("a.txt 应已被删除")
	}
	// 已经不存在的条目视为已完成，而不是报错
	if err := Remove2(t, root, []string{"a.txt"}); err != nil {
		t.Fatalf("重复删除不应报错：%v", err)
	}
	if _, err := Remove(root, []string{""}); err == nil {
		t.Fatal("不能删除项目根目录")
	}
	if _, err := Remove(root, []string{"../x"}); err == nil {
		t.Fatal("越界删除应被拒绝")
	}
}

// Remove2 是 Remove 的测试包装，只关心错误。
func Remove2(t *testing.T, root string, rels []string) error {
	t.Helper()
	_, err := Remove(root, rels)
	return err
}

func TestMoveAndCopyPolicies(t *testing.T) {
	root := t.TempDir()
	recycled := stubRecycle(t)
	mustMkdir(t, filepath.Join(root, "dst"))
	mustWrite(t, filepath.Join(root, "a.txt"), "a")

	if err := Move(root, []string{"a.txt"}, "dst", PolicyFail); err != nil {
		t.Fatalf("move: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "dst", "a.txt")); err != nil {
		t.Fatal("移动后目标不存在")
	}

	mustWrite(t, filepath.Join(root, "a.txt"), "second")
	conflicts, err := CheckConflicts(root, []string{"a.txt"}, "dst")
	if err != nil || len(conflicts) != 1 || conflicts[0] != "a.txt" {
		t.Fatalf("冲突检测 = %v, %v", conflicts, err)
	}
	if err := Move(root, []string{"a.txt"}, "dst", PolicyFail); !errors.Is(err, ErrExists) {
		t.Fatalf("应报 ErrExists，得到 %v", err)
	}
	if err := Move(root, []string{"a.txt"}, "dst", PolicyRename); err != nil {
		t.Fatalf("保留两者移动失败：%v", err)
	}
	// 自动改名沿用 Windows 惯例：主名后加 " (1)"
	if _, err := os.Stat(filepath.Join(root, "dst", "a (1).txt")); err != nil {
		t.Fatal("自动改名后的目标不存在")
	}

	// 覆盖：旧目标先送回收站，再移动新文件进去
	before := len(*recycled)
	mustWrite(t, filepath.Join(root, "b.txt"), "new")
	mustWrite(t, filepath.Join(root, "dst", "b.txt"), "old")
	if err := Move(root, []string{"b.txt"}, "dst", PolicyOverwrite); err != nil {
		t.Fatalf("覆盖移动失败：%v", err)
	}
	if len(*recycled) != before+1 {
		t.Fatal("覆盖前应当先把旧目标送进回收站")
	}
	got, err := os.ReadFile(filepath.Join(root, "dst", "b.txt"))
	if err != nil || string(got) != "new" {
		t.Fatalf("覆盖后内容 = %q, %v", got, err)
	}

	// 同目录移动等同无操作
	if err := Move(root, []string{"dst/a.txt"}, "dst", PolicyFail); err != nil {
		t.Fatalf("同目录移动应无操作：%v", err)
	}

	// 目录递归复制
	mustMkdir(t, filepath.Join(root, "tree", "sub"))
	mustWrite(t, filepath.Join(root, "tree", "sub", "x.txt"), "x")
	if err := Copy(root, []string{"tree"}, "dst", PolicyFail); err != nil {
		t.Fatalf("复制目录失败：%v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "dst", "tree", "sub", "x.txt")); err != nil {
		t.Fatal("深层文件未复制")
	}
	if _, err := os.Stat(filepath.Join(root, "tree", "sub", "x.txt")); err != nil {
		t.Fatal("复制后源目录不应消失")
	}

	// 不能把目录移进/复制进自己的子目录
	mustMkdir(t, filepath.Join(root, "outer", "inner"))
	if err := Move(root, []string{"outer"}, "outer/inner", PolicyFail); err == nil {
		t.Fatal("移进自身子目录应被拒绝")
	}
	if err := Copy(root, []string{"outer"}, "outer/inner", PolicyFail); err == nil {
		t.Fatal("复制进自身子目录应被拒绝")
	}

	// 也不能把目录复制/移动到自己身上 —— 漏掉这一种会无限递归造出「路径炸弹」
	if err := Copy(root, []string{"outer"}, "outer", PolicyFail); err == nil {
		t.Fatal("复制到自己身上应被拒绝")
	}
	if err := Move(root, []string{"outer"}, "outer", PolicyFail); err == nil {
		t.Fatal("移动到自己身上应被拒绝")
	}
	if _, err := os.Stat(filepath.Join(root, "outer", "inner", "inner")); !os.IsNotExist(err) {
		t.Fatal("不应产生任何嵌套副本")
	}

	// 同理：在 outer 里面粘贴 outer
	mustWrite(t, filepath.Join(root, "outer", "f.txt"), "f")
	if err := Copy(root, []string{"outer"}, "outer", PolicyFail); err == nil {
		t.Fatal("在自身内部粘贴自身应被拒绝")
	}

	// 越界目标应被拒绝
	if err := Move(root, []string{"dst/a.txt"}, "../outside", PolicyFail); err == nil {
		t.Fatal("越界目标应被拒绝")
	}
}
