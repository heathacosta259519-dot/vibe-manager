//go:build windows

package winfs

import (
	"os"
	"testing"
)

// TestRemoveLongHandlesPathBomb 复现用户遇到的「路径炸弹」：
// 目录层层嵌套到路径远超 MAX_PATH(260)，回收站接口无能为力，只能走长路径永久删除。
// 注意：这里刻意不调用 Recycle，免得真往系统回收站塞一个几百层的目录。
func TestRemoveLongHandlesPathBomb(t *testing.T) {
	base := t.TempDir()
	bomb := base + `\222`
	if err := os.Mkdir(longPrefix+bomb, 0o755); err != nil {
		t.Fatalf("创建第一层失败：%v", err)
	}

	cur := bomb
	const levels = 2000 // 足够深，递归实现会在这里爆栈
	for i := 0; i < levels; i++ {
		cur += `\222`
		if err := os.Mkdir(longPrefix+cur, 0o755); err != nil {
			t.Fatalf("第 %d 层创建失败：%v", i, err)
		}
	}
	if len(cur) <= 260 {
		t.Fatalf("路径长度 %d 没有超过 MAX_PATH，这个用例测不出问题", len(cur))
	}
	t.Logf("已造出 %d 层、最深路径 %d 字符的目录", levels+1, len(cur))

	if err := RemoveLong(bomb); err != nil {
		t.Fatalf("长路径删除失败：%v", err)
	}
	if _, err := os.Lstat(bomb); !os.IsNotExist(err) {
		t.Fatal("炸弹应当已被删除")
	}
}

func TestRemoveLongOnPlainItem(t *testing.T) {
	base := t.TempDir()
	file := base + `\a.txt`
	if err := os.WriteFile(file, []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := RemoveLong(file); err != nil {
		t.Fatalf("删除普通文件失败：%v", err)
	}
	if _, err := os.Lstat(file); !os.IsNotExist(err) {
		t.Fatal("文件应当已被删除")
	}
}
