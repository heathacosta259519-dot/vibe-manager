package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// AliasesPath 是项目元数据的存储位置（备注名 / 实际根目录 / 导入列表）。
// 刻意不放进项目目录：那样会污染仓库（出现在 git status 与文件列表里）。
func AliasesPath() string {
	return filepath.Join(filepath.Dir(FilePath()), "projects.json")
}

// TemplatesPath 是用户自建 AGENTS.md 模板的存储位置。
func TemplatesPath() string {
	return filepath.Join(filepath.Dir(FilePath()), "agents-templates.json")
}

// ProjectStore 是所有项目的附加信息。键都是 NormalizePath 后的路径。
type ProjectStore struct {
	Aliases map[string]string `json:"aliases,omitempty"`
	Roots   map[string]string `json:"roots,omitempty"`
	Imports []string          `json:"imports,omitempty"`
}

// NormalizePath 把路径归一化成绝对、统一大小写、清理过的形式，用作存储键。
func NormalizePath(p string) string {
	if p == "" {
		return ""
	}
	if abs, err := filepath.Abs(p); err == nil {
		p = abs
	}
	return strings.ToLower(filepath.Clean(p))
}

// AliasKey 保留旧名字，等价于 NormalizePath。
func AliasKey(p string) string { return NormalizePath(p) }

func emptyStore() ProjectStore {
	return ProjectStore{
		Aliases: map[string]string{},
		Roots:   map[string]string{},
		Imports: []string{},
	}
}

// LoadProjectStore 读取项目元数据。
// 兼容旧格式（整份文件就是一个「路径 → 备注名」的扁平映射）。
func LoadProjectStore() ProjectStore {
	s := emptyStore()
	data, err := os.ReadFile(AliasesPath())
	if err != nil {
		return s
	}

	var parsed ProjectStore
	if err := json.Unmarshal(data, &parsed); err == nil &&
		(parsed.Aliases != nil || parsed.Roots != nil || parsed.Imports != nil) {
		for k, v := range parsed.Aliases {
			if strings.TrimSpace(v) != "" {
				s.Aliases[NormalizePath(k)] = strings.TrimSpace(v)
			}
		}
		for k, v := range parsed.Roots {
			if strings.TrimSpace(v) != "" {
				s.Roots[NormalizePath(k)] = v
			}
		}
		seen := map[string]bool{}
		for _, p := range parsed.Imports {
			if strings.TrimSpace(p) == "" {
				continue
			}
			n := NormalizePath(p)
			if seen[n] {
				continue
			}
			seen[n] = true
			s.Imports = append(s.Imports, p)
		}
		return s
	}

	// 旧格式：扁平 map[路径]备注名
	var legacy map[string]string
	if err := json.Unmarshal(data, &legacy); err == nil {
		for k, v := range legacy {
			if strings.TrimSpace(v) != "" {
				s.Aliases[NormalizePath(k)] = strings.TrimSpace(v)
			}
		}
	}
	return s
}

func saveProjectStore(s ProjectStore) error {
	p := AliasesPath()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	// 排序后再写，diff 友好
	out := ProjectStore{Aliases: map[string]string{}, Roots: map[string]string{}, Imports: []string{}}
	for _, k := range sortedKeys(s.Aliases) {
		out.Aliases[k] = s.Aliases[k]
	}
	for _, k := range sortedKeys(s.Roots) {
		out.Roots[k] = s.Roots[k]
	}
	imports := append([]string{}, s.Imports...)
	sort.Slice(imports, func(i, j int) bool {
		return strings.ToLower(imports[i]) < strings.ToLower(imports[j])
	})
	out.Imports = imports

	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, data, 0o644)
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// SetProjectMeta 设置某个项目条目的备注名与实际根目录（空字符串表示清除）。
func SetProjectMeta(entry, alias, root string) error {
	s := LoadProjectStore()
	key := NormalizePath(entry)
	if key == "" {
		return os.ErrInvalid
	}

	if strings.TrimSpace(alias) == "" {
		delete(s.Aliases, key)
	} else {
		s.Aliases[key] = strings.TrimSpace(alias)
	}

	if strings.TrimSpace(root) == "" || NormalizePath(root) == key {
		delete(s.Roots, key)
	} else {
		s.Roots[key] = filepath.Clean(root)
	}
	return saveProjectStore(s)
}

// SetAlias 只改备注名，保留实际根目录。
func SetAlias(entry, alias string) error {
	s := LoadProjectStore()
	root := s.Roots[NormalizePath(entry)]
	return SetProjectMeta(entry, alias, root)
}

// Roots 返回「条目 → 实际根目录」的映射（值已归一化）。
func Roots() map[string]string {
	s := LoadProjectStore()
	out := make(map[string]string, len(s.Roots))
	for k, v := range s.Roots {
		out[k] = NormalizePath(v)
	}
	return out
}

// Aliases 返回「条目 → 备注名」的映射。
func Aliases() map[string]string {
	return LoadProjectStore().Aliases
}

// AddImport 把一个外部目录加入项目列表。
func AddImport(path string) error {
	s := LoadProjectStore()
	key := NormalizePath(path)
	for _, p := range s.Imports {
		if NormalizePath(p) == key {
			return nil
		}
	}
	s.Imports = append(s.Imports, filepath.Clean(path))
	return saveProjectStore(s)
}

// RemoveImport 把某个目录移出项目列表（不动磁盘文件）。
func RemoveImport(path string) error {
	s := LoadProjectStore()
	key := NormalizePath(path)
	kept := make([]string, 0, len(s.Imports))
	for _, p := range s.Imports {
		if NormalizePath(p) != key {
			kept = append(kept, p)
		}
	}
	s.Imports = kept
	// 顺带清掉这条目上的元数据
	delete(s.Aliases, key)
	delete(s.Roots, key)
	return saveProjectStore(s)
}

// IsRegisteredPath 判断某个绝对路径是不是"本工具管着的项目"：
// 是某个条目的位置，或是某个条目的实际根目录。用于项目的越界校验。
func IsRegisteredPath(p string) bool {
	key := NormalizePath(p)
	if key == "" {
		return false
	}
	s := LoadProjectStore()
	if _, ok := s.Aliases[key]; ok {
		return true
	}
	if _, ok := s.Roots[key]; ok {
		return true
	}
	for _, imp := range s.Imports {
		if NormalizePath(imp) == key {
			return true
		}
	}
	for _, r := range s.Roots {
		if NormalizePath(r) == key {
			return true
		}
	}
	return false
}
