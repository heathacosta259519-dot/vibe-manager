package fsx

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadText(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "a.txt"), "你好\n世界\n")
	mustWriteBytes(t, filepath.Join(root, "b.bin"), append([]byte{0, 1}, []byte("x")...))
	mustWrite(t, filepath.Join(root, "big.txt"), strings.Repeat("x", 4096))

	tf, err := ReadText(root, "a.txt", 0)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if tf.Content != "你好\n世界\n" || tf.Binary || tf.Truncated {
		t.Fatalf("普通文本读取不对：%+v", tf)
	}
	if tf.Size != int64(len("你好\n世界\n")) {
		t.Fatalf("size 不对：%d", tf.Size)
	}

	tf, err = ReadText(root, "b.bin", 0)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !tf.Binary {
		t.Fatal("二进制文件应标记 Binary")
	}

	tf, err = ReadText(root, "big.txt", 100)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !tf.Truncated || len(tf.Content) != 100 {
		t.Fatalf("超限应截断到 100：truncated=%v len=%d", tf.Truncated, len(tf.Content))
	}

	if _, err := ReadText(root, "", 0); err == nil {
		t.Fatal("目录不能当文件读")
	}
	if _, err := ReadText(root, "../escape.txt", 0); err == nil {
		t.Fatal("越界路径应被拒绝")
	}
}

func TestWriteTextIsAtomic(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "sub", "a.txt"), "旧内容")

	if err := WriteText(root, "sub/a.txt", "新内容"); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(root, "sub", "a.txt"))
	if err != nil || string(got) != "新内容" {
		t.Fatalf("内容没写进去：%q %v", got, err)
	}

	// 临时文件必须被清掉，不能留在项目里
	entries, err := os.ReadDir(filepath.Join(root, "sub"))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".vibe-pm-") {
			t.Fatalf("残留了临时文件：%s", e.Name())
		}
	}

	// 可以新建文件
	if err := WriteText(root, "sub/new.txt", "新的"); err != nil {
		t.Fatalf("新建写入失败：%v", err)
	}
	if b, err := os.ReadFile(filepath.Join(root, "sub", "new.txt")); err != nil || string(b) != "新的" {
		t.Fatalf("新建文件内容不对：%q %v", b, err)
	}

	// 越界与目录必须被拒绝
	if err := WriteText(root, "../escape.txt", "x"); err == nil {
		t.Fatal("越界写入应被拒绝")
	}
	if err := WriteText(root, "", "x"); err == nil {
		t.Fatal("不能写入项目根目录")
	}
	if err := WriteText(root, "sub", "x"); err == nil {
		t.Fatal("不能把目录当文件写")
	}
}

func TestWriteTextKeepsLongFileIntact(t *testing.T) {
	root := t.TempDir()
	long := strings.Repeat("行内容\n", 10000)
	mustWrite(t, filepath.Join(root, "big.txt"), "short")

	if err := WriteText(root, "big.txt", long); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(root, "big.txt"))
	if err != nil || string(got) != long {
		t.Fatalf("长内容写入不完整：len=%d err=%v", len(got), err)
	}
}
