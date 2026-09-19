package project

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"

	"vibe-manager/internal/agents"
	"vibe-manager/internal/gitx"
	"vibe-manager/internal/model"
)

var namePattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

func ValidName(name string) bool {
	return namePattern.MatchString(name)
}

// Entry 是项目列表里的一条：Entry 是它在哪被发现，Root 是它真正的仓库位置。
// 两者不同时，一切操作（git、文件、任务）都作用于 Root。
// 例：Active\projects\winglass 里的真仓库在 winglass\WinGlass。
type Entry struct {
	Path     string // 条目位置（项目根下的目录，或用户导入的目录）
	Root     string // 实际根目录；为空表示与 Path 相同
	Alias    string
	Imported bool
}

func (e Entry) effectiveRoot() string {
	if strings.TrimSpace(e.Root) != "" {
		return e.Root
	}
	return e.Path
}

// Scan 只枚举项目根下的直接子目录，不跑 git。
func Scan(root string) ([]Entry, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("读取项目根失败：%v", err)
	}
	out := make([]Entry, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			out = append(out, Entry{Path: filepath.Join(root, e.Name())})
		}
	}
	return out, nil
}

// DescribeAll 并发补全每个项目的 git 状态与任务文档信息。
func DescribeAll(entries []Entry) []model.Project {
	results := make([]model.Project, len(entries))
	sem := make(chan struct{}, 8)
	var wg sync.WaitGroup
	for i, e := range entries {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, e Entry) {
			defer wg.Done()
			defer func() { <-sem }()
			results[i] = Describe(e)
		}(i, e)
	}
	wg.Wait()
	sort.Slice(results, func(i, j int) bool { return results[i].ModTime > results[j].ModTime })
	return results
}

// Describe 补全单个项目的信息。
func Describe(e Entry) model.Project {
	work := e.effectiveRoot()
	base := filepath.Base(filepath.Clean(e.Path))
	p := model.Project{
		Name:           base,
		Alias:          e.Alias,
		Path:           filepath.Clean(work),
		Entry:          filepath.Clean(e.Path),
		Imported:       e.Imported,
		RootOverridden: filepath.Clean(work) != filepath.Clean(e.Path),
	}
	if fi, err := os.Stat(work); err == nil {
		p.ModTime = fi.ModTime().Unix()
	}
	if gitx.IsRepo(work) {
		p.IsGit = true
		if st, err := gitx.Status(work); err == nil {
			p.Branch = st.Branch
			p.Dirty = st.Dirty
			p.HasUpstream = st.HasUpstream
			p.Ahead = st.Ahead
			p.Behind = st.Behind
			if st.MTime > p.ModTime {
				p.ModTime = st.MTime
			}
		}
		if subject, when, err := gitx.LastCommit(work); err == nil {
			p.LastCommit = subject
			p.LastCommitAt = when
			if when > p.ModTime {
				p.ModTime = when
			}
		}
	}
	return p
}

// Names 只列出项目根下的直接子目录名（不跑 git），供搜索这类需要快速枚举的场景使用。
func Names(root string) ([]string, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("读取项目根失败：%v", err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	return names, nil
}

// Create 按勾选项生成项目骨架。
func Create(root string, opts model.CreateOptions) (string, error) {
	if !ValidName(opts.Name) {
		return "", fmt.Errorf("项目名必须是 kebab-case：小写字母/数字，单词间用单个连字符")
	}
	// 项目根没设就往下走的话，filepath.Join("", name) 会变成相对路径，
	// 在进程当前目录（对打包后的 exe 来说可能是任意位置）里建出目录。
	if strings.TrimSpace(root) == "" {
		return "", fmt.Errorf("请先在设置里指定项目根目录")
	}
	path := filepath.Join(root, opts.Name)
	if _, err := os.Stat(path); err == nil {
		return "", fmt.Errorf("目标目录已存在：%s", path)
	}
	if err := os.MkdirAll(path, 0o755); err != nil {
		return "", err
	}

	write := func(rel, content string) error {
		full := filepath.Join(path, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return err
		}
		return os.WriteFile(full, []byte(content), 0o644)
	}

	holder := strings.TrimSpace(opts.Author)
	if holder == "" {
		holder = opts.Name
	}

	if opts.Readme {
		if err := write("README.md", readmeTemplate(opts.Name)); err != nil {
			return "", err
		}
	}
	if opts.Gitignore {
		if err := write(".gitignore", gitignoreTemplate()); err != nil {
			return "", err
		}
	}
	if opts.Src {
		if err := write("src/.gitkeep", ""); err != nil {
			return "", err
		}
	}
	if opts.Agents {
		body := agents.Render(opts.AgentsTemplate, opts.Name)
		if body == "" {
			body = agents.Render(agents.DefaultID, opts.Name)
		}
		if body != "" {
			if err := write("AGENTS.md", body); err != nil {
				return "", err
			}
		}
	}
	if opts.License != "" {
		text, err := LicenseText(opts.License, holder)
		if err != nil {
			return "", err
		}
		if text != "" {
			if err := write("LICENSE", text); err != nil {
				return "", err
			}
		}
	}

	if opts.GitInit {
		if err := gitx.Init(path); err != nil {
			return "", err
		}
		if _, _, err := gitx.CommitAll(path, "chore: 初始化项目骨架"); err != nil {
			return "", err
		}
	}
	return path, nil
}
