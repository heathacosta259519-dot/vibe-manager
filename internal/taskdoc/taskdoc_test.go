package taskdoc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fixtureTASK 贴近 TASK.md 的版式：## 任务 N 标题，验收用行内代码。
const fixtureTASK = "# TASK\n\n你是执行者，这份 TASK.md 是你唯一的任务来源。\n\n" +
	"## 任务 1：数据模型与存储层\n" +
	"\n" +
	"目标：实现菜谱 CRUD 与 localStorage 持久化，导出/导入 JSON 备份。\n" +
	"从哪下手：先写数据模块再搭界面。\n" +
	"死规矩：菜谱对象必须含这些字段。\n" +
	"- 字段必须齐全\n" +
	"- 导出导入不能崩\n" +
	"验收：用 `grep -cE \"localStorage|cookbook_recipes_v1\" index.html`，结果 ≥2。\n" +
	"\n" +
	"## 任务 2：人数缩放引擎\n" +
	"\n" +
	"目标：详情页放置人数步进器。\n" +
	"死规矩：\n" +
	"- 缩放公式固定\n" +
	"验收：\n" +
	"```\n" +
	"grep -cE \"scaleMode|Math.round|0.25\" index.html   # ≥3\n" +
	"```\n"

// fixtureTASK4 贴近 TASK4.md 的版式：**任务 N：标题** 加粗行，验收用围栏代码块。
const fixtureTASK4 = "# TASK4 — 四期\n\n" +
	"**这活图什么**：家庭菜谱工具要支持自定义TAG与图片。\n\n" +
	"**神仙打架听谁的**：功能正确 > 离线约束 > 好看。\n\n" +
	"**任务 0：前提核验**\n" +
	"跑这些命令核对上面数字，对不上就停。\n\n" +
	"**任务 1：自定义TAG**\n" +
	"一句话目标：用户能在编辑表单里输入新TAG。\n" +
	"**死规矩：**\n" +
	"- 自定义TAG与预置TAG重名时要去重\n" +
	"- 输入框为空不许添加\n" +
	"验收：\n" +
	"```\n" +
	"grep -cE \"customTags|cookbook_custom_tags\" index.html    # ≥ 4\n" +
	"```\n"

func TestParseInlineChecksFormat(t *testing.T) {
	tasks := ParseTasks(fixtureTASK)
	if len(tasks) != 2 {
		t.Fatalf("应当解析出 2 个任务，得到 %d", len(tasks))
	}

	first := tasks[0]
	if first.No != 1 || first.Title != "数据模型与存储层" {
		t.Fatalf("第一个任务标题解析错误：%+v", first)
	}
	if !strings.Contains(first.Goal, "菜谱 CRUD") {
		t.Fatalf("目标没抽出来：%q", first.Goal)
	}
	if !strings.Contains(first.How, "先写数据模块") {
		t.Fatalf("「从哪下手」没抽出来：%q", first.How)
	}
	if len(first.Rules) != 2 {
		t.Fatalf("死规矩应有 2 条，得到 %d：%v", len(first.Rules), first.Rules)
	}
	if len(first.Checks) != 1 || !strings.Contains(first.Checks[0], "grep -cE") {
		t.Fatalf("行内代码形式的验收没抽出来：%v", first.Checks)
	}

	second := tasks[1]
	if len(second.Checks) != 1 || !strings.Contains(second.Checks[0], "scaleMode") {
		t.Fatalf("围栏代码块形式的验收没抽出来：%v", second.Checks)
	}
	if !strings.Contains(second.Checks[0], "#") {
		t.Fatalf("验收命令应当保留行尾注释便于直接粘贴：%q", second.Checks[0])
	}
}

func TestParseBoldHeadingFormat(t *testing.T) {
	tasks := ParseTasks(fixtureTASK4)
	if len(tasks) != 2 {
		t.Fatalf("应当解析出 2 个任务（0 与 1），得到 %d", len(tasks))
	}
	second := tasks[1]
	if second.No != 1 || second.Title != "自定义TAG" {
		t.Fatalf("加粗标题解析错误：%+v", second)
	}
	if !strings.Contains(second.Goal, "输入新TAG") {
		t.Fatalf("「一句话目标」没抽出来：%q", second.Goal)
	}
	if len(second.Rules) != 2 {
		t.Fatalf("死规矩应有 2 条，得到 %d：%v", len(second.Rules), second.Rules)
	}
	if len(second.Checks) != 1 || !strings.Contains(second.Checks[0], "customTags") {
		t.Fatalf("验收没抽出来：%v", second.Checks)
	}
}

func TestParseUnknownFormatFallsBackToRaw(t *testing.T) {
	tasks := ParseTasks("# 随手笔记\n\n今天修了个 bug。\n")
	if len(tasks) != 0 {
		t.Fatalf("认不出结构时应返回空切片，得到 %d", len(tasks))
	}
}

// 块里出现围栏时只认围栏，别把正文里提到的 `标识符` 当成命令。
func TestChecksPreferFenceOverInlineCode(t *testing.T) {
	content := "## 任务 4：回归自检\n" +
		"验收：\n" +
		"```\n" +
		"grep -cE \"a|b\" index.html   # ≥ 4\n" +
		"```\n" +
		"反向验证：把 `cookbook_custom_tags_v1` 改名，跑检查必须变红；不许用 `|| true`。\n"
	tasks := ParseTasks(content)
	if len(tasks) != 1 {
		t.Fatalf("应当解析出 1 个任务，得到 %d", len(tasks))
	}
	checks := tasks[0].Checks
	if len(checks) != 1 {
		t.Fatalf("应当只抽出围栏里的 1 条命令，得到 %d：%v", len(checks), checks)
	}
	if strings.Contains(checks[0], "cookbook_custom_tags_v1") || strings.Contains(checks[0], "|| true") {
		t.Fatalf("正文里的行内代码被误当成命令：%v", checks)
	}
}

func TestHasBlockedContent(t *testing.T) {
	for _, empty := range []string{"", "   ", "\n\n", "无", "# BLOCKED\n\n无\n", "-"} {
		if HasBlockedContent(empty) {
			t.Fatalf("%q 不应算作有待裁决", empty)
		}
	}
	for _, real := range []string{"1. 这里有个问题", "- 待裁决：xxx"} {
		if !HasBlockedContent(real) {
			t.Fatalf("%q 应当算作有待裁决", real)
		}
	}
}

func TestScanFindsCurrentAndBlocked(t *testing.T) {
	root := t.TempDir()
	write(t, root, "TASK.md", "# TASK\n\n老任务\n")
	write(t, root, "TASK2.md", fixtureTASK)
	write(t, root, "PROGRESS.md", "进度：做完了任务 1\n")
	write(t, root, "BLOCKED.md", "无\n")

	board, err := Scan(root)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if !board.HasTaskDoc {
		t.Fatal("应当识别到任务书")
	}
	if board.Current != "TASK2.md" {
		t.Fatalf("当前任务应为编号最大的 TASK2.md，得到 %q", board.Current)
	}
	if board.AllDone {
		t.Fatal("TASK2 没有 _DONE，不应算全部完成")
	}
	if board.Blocked {
		t.Fatal("BLOCKED.md 内容是「无」，不应算有待裁决")
	}
	if !strings.Contains(board.ProgressText, "进度") {
		t.Fatalf("进度没读到：%q", board.ProgressText)
	}
	if len(board.Tasks) != 2 {
		t.Fatalf("当前任务书应解析出 2 个任务，得到 %d", len(board.Tasks))
	}

	// 标上 _DONE 之后应当变成全部完成
	write(t, root, "TASK2_DONE.md", "TASK2 complete\n")
	board, err = Scan(root)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if !board.AllDone {
		t.Fatal("TASK2 已有 _DONE，应当算全部完成")
	}

	// 待裁决有真内容
	write(t, root, "BLOCKED.md", "- 需要裁决：用哪种存储\n")
	board, _ = Scan(root)
	if !board.Blocked || !strings.Contains(board.BlockedText, "需要裁决") {
		t.Fatalf("待裁决没识别：%+v", board.Blocked)
	}

	hasTask, allDone, blocked := Summarize(root)
	if !hasTask || !allDone || !blocked {
		t.Fatalf("Summarize = %v/%v/%v，期望 true/true/true", hasTask, allDone, blocked)
	}
}

func TestScanEmptyProject(t *testing.T) {
	root := t.TempDir()
	board, err := Scan(root)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if board.HasTaskDoc || len(board.Docs) != 0 {
		t.Fatalf("空项目不应有任务文档：%+v", board)
	}
}

func write(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
