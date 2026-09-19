package fsx

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestSearchFilesNameAndContent(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "README.md"), "# 标题\n这里提到红烧肉\n")
	mustWrite(t, filepath.Join(root, "src", "note.txt"), "第一行\n第二行有红烧肉\n第三行\n")
	mustWrite(t, filepath.Join(root, "node_modules", "junk.txt"), "红烧肉\n")
	mustWrite(t, filepath.Join(root, ".git", "config"), "红烧肉\n")
	mustWriteBytes(t, filepath.Join(root, "binary.bin"), append([]byte{0, 1, 2}, []byte("红烧")...))

	hits, err := SearchFiles(root, "红烧肉")
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	content := map[string]int{}
	for _, h := range hits {
		if strings.Contains(h.Path, "node_modules") || strings.Contains(h.Path, ".git") {
			t.Fatalf("不该搜进噪音目录：%+v", h)
		}
		if h.Path == "binary.bin" {
			t.Fatal("二进制文件不该有命中")
		}
		if h.Kind == "content" {
			content[h.Path]++
		}
	}
	if content["README.md"] != 1 {
		t.Fatalf("README.md 应有 1 条内容命中：%v", content)
	}
	if content["src/note.txt"] != 1 {
		t.Fatalf("src/note.txt 应有 1 条内容命中：%v", content)
	}
	for _, h := range hits {
		if h.Path == "src/note.txt" {
			if h.LineNo != 2 {
				t.Fatalf("行号应为 2，得到 %d", h.LineNo)
			}
			if !strings.Contains(h.Line, "红烧肉") {
				t.Fatalf("命中行内容不对：%q", h.Line)
			}
		}
	}
}

func TestSearchFilesMatchesName(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "src", "note.txt"), "无关内容\n")

	hits, err := SearchFiles(root, "note")
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	found := false
	for _, h := range hits {
		if h.Kind == "file" && h.Path == "src/note.txt" {
			found = true
		}
	}
	if !found {
		t.Fatalf("文件名命中没找到：%+v", hits)
	}
}

func TestSearchFilesEmptyQuery(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "a.txt"), "x\n")
	hits, err := SearchFiles(root, "   ")
	if err != nil || len(hits) != 0 {
		t.Fatalf("空查询应返回空结果：%v %v", hits, err)
	}
}

func TestSearchFilesSkipsLargeFile(t *testing.T) {
	root := t.TempDir()
	big := strings.Repeat("找me\n", 300000) // 远超 1 MB
	mustWrite(t, filepath.Join(root, "big.txt"), big)

	hits, err := SearchFiles(root, "找me")
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(hits) != 0 {
		t.Fatalf("超过 1 MB 的文件不该做内容搜索，却得到 %d 条", len(hits))
	}
}

func TestSearchFilesCapsPerFileHits(t *testing.T) {
	root := t.TempDir()
	var sb strings.Builder
	for i := 0; i < 50; i++ {
		sb.WriteString("命中命中\n")
	}
	mustWrite(t, filepath.Join(root, "many.txt"), sb.String())

	hits, err := SearchFiles(root, "命中")
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(hits) != searchPerFileHits {
		t.Fatalf("单文件命中应被截到 %d 条，得到 %d", searchPerFileHits, len(hits))
	}
}
