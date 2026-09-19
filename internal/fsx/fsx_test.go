package fsx

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"vibe-manager/internal/model"
)

func TestResolveRejectsEscape(t *testing.T) {
	root := t.TempDir()
	for _, rel := range []string{
		"..",
		"../x",
		"../../x",
		"a/../../x",
		`C:\Windows`,
		`..\x`,
		`\\server\share`,
	} {
		if _, _, err := Resolve(root, rel); err == nil {
			t.Fatalf("rel %q 应当被拒绝", rel)
		}
	}

	// 形如 /etc/passwd 的「根相对」路径在 Windows 上不算绝对路径，
	// 会被安全地解析为项目内的 etc/passwd —— 不越界即可。
	full, norm, err := Resolve(root, "/etc/passwd")
	if err != nil {
		t.Fatalf("根相对路径不应报错：%v", err)
	}
	if norm != "etc/passwd" || !strings.HasPrefix(full, root) {
		t.Fatalf("应当落在项目内：norm=%q full=%q root=%q", norm, full, root)
	}

	full, norm, err = Resolve(root, "sub/dir")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if norm != "sub/dir" {
		t.Fatalf("norm = %q", norm)
	}
	if !strings.HasPrefix(full, root) {
		t.Fatalf("full = %q 不在 %q 之下", full, root)
	}

	full, norm, err = Resolve(root, "")
	if err != nil || norm != "" || full == "" {
		t.Fatalf("根目录解析失败：%q %q %v", full, norm, err)
	}
}

func TestResolveRejectsSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "link")); err != nil {
		t.Skipf("当前环境无法创建符号链接：%v", err)
	}
	if _, _, err := Resolve(root, "link"); err == nil {
		t.Fatal("指向外部的符号链接应当被拒绝")
	}
}

func TestListEntriesAndFlags(t *testing.T) {
	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, "src"))
	mustMkdir(t, filepath.Join(root, "node_modules"))
	mustMkdir(t, filepath.Join(root, ".hidden"))
	mustWrite(t, filepath.Join(root, "README.md"), "hi")
	mustWrite(t, filepath.Join(root, "src", "a.go"), "package main")

	l, err := List(root, "")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if l.GitAvailable {
		t.Fatal("临时目录不应被识别为 git 仓库")
	}
	found := map[string]model.FileEntry{}
	for _, e := range l.Entries {
		found[e.Name] = e
	}
	if !found["src"].IsDir {
		t.Fatal("src 应当是目录")
	}
	if found["src"].Path != "src" {
		t.Fatalf("path = %q", found["src"].Path)
	}
	if !found["README.md"].Special {
		t.Fatal("README.md 应标记为关键文件")
	}
	if !found["node_modules"].Hidden {
		t.Fatal("node_modules 应默认隐藏")
	}
	if !found[".hidden"].Hidden {
		t.Fatal("点开头的目录应默认隐藏")
	}

	sub, err := List(root, "src")
	if err != nil {
		t.Fatalf("list sub: %v", err)
	}
	if len(sub.Entries) != 1 || sub.Entries[0].Path != "src/a.go" {
		t.Fatalf("子目录列举 = %+v", sub.Entries)
	}

	if _, err := List(root, "../outside"); err == nil {
		t.Fatal("越界列举应当失败")
	}
}

func TestPreviewTextImageBinary(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "a.txt"), "hello 世界")
	mustWriteBytes(t, filepath.Join(root, "b.bin"), []byte{1, 2, 0, 3})
	mustWriteBytes(t, filepath.Join(root, "c.png"), []byte{0x89, 'P', 'N', 'G'})

	p, err := Preview(root, "a.txt")
	if err != nil || p.Kind != "text" || p.Text != "hello 世界" {
		t.Fatalf("文本预览 = %+v, %v", p, err)
	}

	p, err = Preview(root, "b.bin")
	if err != nil || !p.Binary || p.Kind != "other" {
		t.Fatalf("二进制预览 = %+v, %v", p, err)
	}

	p, err = Preview(root, "c.png")
	if err != nil || p.Kind != "image" || !strings.HasPrefix(p.DataURL, "data:image/png;base64,") {
		t.Fatalf("图片预览 = %+v, %v", p, err)
	}

	if _, err := Preview(root, "../escape.txt"); err == nil {
		t.Fatal("越界预览应当失败")
	}
}

func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	mustWriteBytes(t, path, []byte(content))
}

func mustWriteBytes(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}
