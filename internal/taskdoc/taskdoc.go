// Package taskdoc 识别并解析项目里的「任务书」文档。
//
// 约定（大小写不敏感，均在项目根目录）：
//
//	TASK*.md         任务书；TASK4_DONE.md 这种带 _DONE 的表示该任务已完成
//	PROGRESS.md      进度记录
//	BLOCKED.md       待裁决清单
package taskdoc

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"vibe-manager/internal/model"
)

const (
	maxDocBytes      = 512 * 1024
	maxBlockedBytes  = 64 * 1024
	maxProgressBytes = 128 * 1024
)

var (
	reTaskHeading = regexp.MustCompile(`^#{2,4}\s*任务\s*(\d+)\s*[:：]?\s*(.*)$`)
	reTaskBold    = regexp.MustCompile(`^\*\*\s*任务\s*(\d+)\s*[:：]?\s*(.*?)\s*\*\*\s*$`)
	reDigits      = regexp.MustCompile(`^\d+`)
	reInlineCode  = regexp.MustCompile("`([^`]+)`")
	reBullet      = regexp.MustCompile(`^\s*(?:[-*+]|\d+[.、])\s+(.*)$`)
)

type found struct {
	taskFiles map[int]string // 编号 -> 文件名
	done      map[int]bool   // 编号 -> 是否已标 _DONE
	progress  string
	blocked   string
	all       []string
}

// classify 从目录名列表里挑出任务相关文档。
func classify(names []string) found {
	f := found{taskFiles: map[int]string{}, done: map[int]bool{}}
	for _, name := range names {
		lower := strings.ToLower(name)
		if !strings.HasSuffix(lower, ".md") {
			continue
		}
		base := strings.TrimSuffix(lower, ".md")
		switch {
		case base == "blocked":
			f.blocked = name
			f.all = append(f.all, name)
		case base == "progress":
			f.progress = name
			f.all = append(f.all, name)
		case strings.HasPrefix(base, "task"):
			rest := strings.Trim(strings.TrimPrefix(base, "task"), "_- ")
			if strings.HasSuffix(rest, "_done") {
				f.done[taskNo(strings.TrimSuffix(rest, "_done"))] = true
				f.all = append(f.all, name)
				continue
			}
			f.taskFiles[taskNo(rest)] = name
			f.all = append(f.all, name)
		}
	}
	return f
}

func taskNo(s string) int {
	if d := reDigits.FindString(s); d != "" {
		n, _ := strconv.Atoi(d)
		return n
	}
	return 0
}

// Summarize 给项目列表用的轻量版本：只看有没有任务书、是否已完成、有没有待裁决。
func Summarize(projectRoot string) (hasTask, allDone, blocked bool) {
	names, err := listNames(projectRoot)
	if err != nil {
		return false, false, false
	}
	f := classify(names)
	highest, ok := highestTask(f)
	if !ok {
		return false, false, hasBlocked(projectRoot, f.blocked)
	}
	return true, f.done[highest], hasBlocked(projectRoot, f.blocked)
}

func highestTask(f found) (int, bool) {
	best, ok := -1, false
	for n := range f.taskFiles {
		if !ok || n > best {
			best, ok = n, true
		}
	}
	return best, ok
}

func hasBlocked(projectRoot, blockedName string) bool {
	if blockedName == "" {
		return false
	}
	content, err := readCapped(projectRoot, blockedName, maxBlockedBytes)
	if err != nil {
		return false
	}
	return HasBlockedContent(content)
}

// HasBlockedContent 判断待裁决清单是否真的有内容。
// 纯标题（`# BLOCKED`）、空行、「无 / none / -」都算没有内容。
func HasBlockedContent(content string) bool {
	var kept []string
	for _, raw := range strings.Split(content, "\n") {
		line := strings.TrimSpace(strings.TrimRight(raw, "\r"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimSpace(strings.TrimLeft(line, "-*+ "))
		line = strings.Trim(line, "*` ")
		if line != "" {
			kept = append(kept, line)
		}
	}
	switch strings.ToLower(strings.Join(kept, " ")) {
	case "", "无", "none", "n/a", "na":
		return false
	}
	return true
}

// Scan 读取项目里的任务文档，判断当前任务并解析其中的「任务 N」小节。
func Scan(projectRoot string) (model.TaskBoard, error) {
	names, err := listNames(projectRoot)
	if err != nil {
		return model.TaskBoard{}, fmt.Errorf("读取项目目录失败：%v", err)
	}
	f := classify(names)
	board := model.TaskBoard{Docs: f.all, Tasks: []model.TaskSection{}}

	// 当前任务 = 编号最大的任务书；它有没有 _DONE 决定「是否全部完成」。
	// 这样处理更贴合实际：早期文件可能本来就没有 _DONE 标记。
	if highest, ok := highestTask(f); ok {
		board.HasTaskDoc = true
		board.Current = f.taskFiles[highest]
		board.AllDone = f.done[highest]
		content, err := readCapped(projectRoot, board.Current, maxDocBytes)
		if err != nil {
			return board, fmt.Errorf("读取 %s 失败：%v", board.Current, err)
		}
		board.RawText = content
		board.Tasks = ParseTasks(content)
	}

	if f.blocked != "" {
		content, err := readCapped(projectRoot, f.blocked, maxBlockedBytes)
		if err == nil {
			board.BlockedText = content
			board.Blocked = HasBlockedContent(content)
		}
	}
	if f.progress != "" {
		content, err := readCapped(projectRoot, f.progress, maxProgressBytes)
		if err == nil {
			board.ProgressText = content
		}
	}
	return board, nil
}

func listNames(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			names = append(names, e.Name())
		}
	}
	return names, nil
}

func readCapped(dir, name string, limit int64) (string, error) {
	f, err := os.Open(dir + string(os.PathSeparator) + name)
	if err != nil {
		return "", err
	}
	defer f.Close()
	buf := make([]byte, 0, 4096)
	tmp := make([]byte, 8192)
	var total int64
	for total < limit {
		n, err := f.Read(tmp)
		if n > 0 {
			buf = append(buf, tmp[:n]...)
			total += int64(n)
		}
		if err != nil {
			break
		}
	}
	return string(buf), nil
}

/* ---------------- 任务小节解析 ---------------- */

type block struct {
	kind  string // goal | how | rules | checks
	lines []string
}

// markerOf 识别小节里的标记行（容忍 **加粗**、# 标题、全角冒号）。
func markerOf(line string) (string, string) {
	s := strings.TrimSpace(line)
	stripped := strings.Trim(s, "*# \t")
	for _, m := range []struct {
		words []string
		kind  string
	}{
		{[]string{"目标", "一句话目标", "这活图什么", "这活为什么干", "这活图"}, "goal"},
		{[]string{"从哪下手", "怎么下手"}, "how"},
		{[]string{"死规矩", "死规则", "规矩", "高压线", "硬约束"}, "rules"},
		{[]string{"验收", "是骡子是马"}, "checks"},
	} {
		for _, w := range m.words {
			if strings.HasPrefix(stripped, w) {
				rest := strings.TrimSpace(strings.TrimPrefix(stripped, w))
				rest = strings.TrimLeft(rest, "：:— -*")
				return m.kind, strings.TrimSpace(rest)
			}
		}
	}
	return "", ""
}

// splitBlocks 把一个小节的正文按标记切成若干块。
func splitBlocks(lines []string) []block {
	var out []block
	cur := -1
	for _, raw := range lines {
		line := strings.TrimRight(raw, "\r")
		kind, rest := markerOf(line)
		if kind != "" {
			b := block{kind: kind}
			if rest != "" {
				b.lines = append(b.lines, rest)
			}
			out = append(out, b)
			cur = len(out) - 1
			continue
		}
		if cur >= 0 {
			out[cur].lines = append(out[cur].lines, line)
		}
	}
	return out
}

type heading struct {
	no    int
	title string
	body  []string
}

// splitSections 按「任务 N」标题切分任务书。
func splitSections(content string) []heading {
	var out []heading
	cur := -1
	for _, raw := range strings.Split(content, "\n") {
		line := strings.TrimRight(raw, "\r")
		if m := reTaskHeading.FindStringSubmatch(line); m != nil {
			no, _ := strconv.Atoi(m[1])
			out = append(out, heading{no: no, title: strings.TrimSpace(m[2])})
			cur = len(out) - 1
			continue
		}
		if m := reTaskBold.FindStringSubmatch(line); m != nil {
			no, _ := strconv.Atoi(m[1])
			out = append(out, heading{no: no, title: strings.TrimSpace(m[2])})
			cur = len(out) - 1
			continue
		}
		if cur >= 0 {
			out[cur].body = append(out[cur].body, line)
		}
	}
	return out
}

// ParseTasks 解析任务书正文里的「任务 N」小节。
// 认不出结构时返回空切片，调用方应退回展示原文。
func ParseTasks(content string) []model.TaskSection {
	heads := splitSections(content)
	out := make([]model.TaskSection, 0, len(heads))
	for _, h := range heads {
		sec := model.TaskSection{
			No:     h.no,
			Title:  h.title,
			Raw:    strings.TrimSpace(strings.Join(h.body, "\n")),
			Rules:  []string{},
			Checks: []string{},
		}
		for _, b := range splitBlocks(h.body) {
			switch b.kind {
			case "goal":
				if sec.Goal == "" {
					sec.Goal = firstText(b.lines)
				}
			case "how":
				if sec.How == "" {
					sec.How = firstText(b.lines)
				}
			case "rules":
				sec.Rules = append(sec.Rules, bullets(b.lines)...)
			case "checks":
				sec.Checks = append(sec.Checks, checksOf(b.lines)...)
			}
		}
		out = append(out, sec)
	}
	return out
}

func firstText(lines []string) string {
	for _, raw := range lines {
		s := strings.TrimSpace(raw)
		if s == "" {
			continue
		}
		return strings.Trim(s, "*` ")
	}
	return ""
}

func bullets(lines []string) []string {
	var out []string
	for _, raw := range lines {
		if m := reBullet.FindStringSubmatch(raw); m != nil {
			if s := strings.TrimSpace(m[1]); s != "" {
				out = append(out, s)
			}
		}
	}
	return out
}

// checksOf 从「验收」块里抽命令。
// 优先认围栏代码块里的整行；块里只要出现过围栏，就只认围栏——
// 否则正文里提到的 `xxx`（如「把 `cookbook_custom_tags_v1` 改名」）会被误当成命令。
// 没有围栏时才退回「行内代码」写法，因为两种版式在实际任务书里都出现过。
func checksOf(lines []string) []string {
	var fenced, inline []string
	inFence, hasFence := false, false
	for _, raw := range lines {
		trimmed := strings.TrimSpace(strings.TrimRight(raw, "\r"))
		if strings.HasPrefix(trimmed, "```") {
			hasFence = true
			inFence = !inFence
			continue
		}
		if inFence {
			if trimmed != "" {
				fenced = append(fenced, trimmed)
			}
			continue
		}
		for _, m := range reInlineCode.FindAllStringSubmatch(trimmed, -1) {
			if cmd := strings.TrimSpace(m[1]); cmd != "" {
				inline = append(inline, cmd)
			}
		}
	}
	if hasFence {
		return fenced
	}
	return inline
}
