package fsx

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"vibe-manager/internal/gitx"
	"vibe-manager/internal/model"
)

const (
	maxEntries        = 3000
	previewTextLimit  = 256 * 1024
	previewImageLimit = 3 * 1024 * 1024
	binarySniffLen    = 8192
)

// noiseNames 默认隐藏的「噪音」目录：项目管理里通常不关心，但一个开关就能显示。
var noiseNames = map[string]bool{
	".git": true, "node_modules": true, ".venv": true, "venv": true,
	"__pycache__": true, "target": true, "dist": true, "build": true,
	"out": true, "coverage": true, ".idea": true, ".vscode": true,
	".next": true, ".cache": true, ".pytest_cache": true, ".mypy_cache": true,
	"vendor": true, ".gradle": true, "obj": true, "bin": true,
}

var specialNames = map[string]bool{
	"README.md": true, "readme.md": true, "TASK.md": true, "task.md": true,
	"LICENSE": true, "license": true, ".gitignore": true, "go.mod": true,
	"package.json": true, "Cargo.toml": true, "pyproject.toml": true,
	"requirements.txt": true, "Makefile": true, "tsconfig.json": true,
	"docker-compose.yml": true, "wails.json": true,
}

var imageMIME = map[string]string{
	".png": "image/png", ".jpg": "image/jpeg", ".jpeg": "image/jpeg",
	".gif": "image/gif", ".webp": "image/webp", ".bmp": "image/bmp",
	".ico": "image/x-icon",
}

// Resolve 把项目根相对路径解析为绝对路径，并确保它没有越出项目根。
// 返回的第二个值是规范化后的斜杠相对路径（项目根本身为空串）。
func Resolve(root, rel string) (string, string, error) {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", "", err
	}
	rel = strings.TrimSpace(rel)
	if rel == "" || rel == "." || rel == "/" {
		return rootAbs, "", nil
	}
	native := filepath.FromSlash(rel)
	if filepath.IsAbs(native) {
		return "", "", fmt.Errorf("路径越出项目目录")
	}
	full := filepath.Clean(filepath.Join(rootAbs, native))
	out, err := filepath.Rel(rootAbs, full)
	if err != nil || out == ".." || strings.HasPrefix(out, ".."+string(os.PathSeparator)) {
		return "", "", fmt.Errorf("路径越出项目目录")
	}
	// 若路径已存在，再按真实路径校验一次，防止符号链接逃逸。
	rootReal := rootAbs
	if r, err := filepath.EvalSymlinks(rootAbs); err == nil {
		rootReal = r
	}
	if real, err := filepath.EvalSymlinks(full); err == nil {
		r, err := filepath.Rel(rootReal, real)
		if err != nil || r == ".." || strings.HasPrefix(r, ".."+string(os.PathSeparator)) {
			return "", "", fmt.Errorf("路径越出项目目录")
		}
	}
	normRel := filepath.ToSlash(filepath.Clean(out))
	if normRel == "." {
		normRel = ""
	}
	return full, normRel, nil
}

// List 列出项目内某个目录的直接子项，并附上 git 变更状态与隐藏/关键标记。
func List(root, rel string) (model.DirListing, error) {
	full, normRel, err := Resolve(root, rel)
	if err != nil {
		return model.DirListing{}, err
	}
	infos, err := os.ReadDir(full)
	if err != nil {
		return model.DirListing{}, fmt.Errorf("读取目录失败：%v", err)
	}

	listing := model.DirListing{RelPath: normRel, Entries: []model.FileEntry{}}
	states := map[string]string{}
	ignored := map[string]bool{}
	if gitx.IsRepo(root) {
		listing.GitAvailable = true
		if s, err := gitx.StatusMap(root, normRel); err == nil {
			states = s
		}
		names := make([]string, 0, len(infos))
		for _, e := range infos {
			names = append(names, joinRel(normRel, e.Name()))
		}
		if ig, err := gitx.IgnoredSet(root, names); err == nil {
			ignored = ig
		}
	}

	for _, e := range infos {
		name := e.Name()
		info, err := e.Info()
		if err != nil {
			continue
		}
		key := joinRel(normRel, name)
		state := states[key]
		if state == "" && e.IsDir() {
			state = summarizeDir(states, key)
		}
		listing.Entries = append(listing.Entries, model.FileEntry{
			Name:     name,
			Path:     key,
			Ext:      strings.ToLower(filepath.Ext(name)),
			IsDir:    e.IsDir(),
			Size:     info.Size(),
			ModTime:  info.ModTime().Unix(),
			GitState: state,
			Hidden:   strings.HasPrefix(name, ".") || noiseNames[strings.ToLower(name)] || ignored[key],
			Special:  specialNames[name],
		})
	}

	listing.Total = len(listing.Entries)
	if listing.Total > maxEntries {
		listing.Entries = listing.Entries[:maxEntries]
		listing.Truncated = true
	}
	return listing, nil
}

// Preview 返回只读预览：图片给 data URL，文本给内容，其余只给元信息。
func Preview(root, rel string) (model.FilePreview, error) {
	full, _, err := Resolve(root, rel)
	if err != nil {
		return model.FilePreview{}, err
	}
	fi, err := os.Stat(full)
	if err != nil {
		return model.FilePreview{}, fmt.Errorf("读取失败：%v", err)
	}
	p := model.FilePreview{Size: fi.Size()}
	if fi.IsDir() {
		p.Kind = "other"
		p.Message = "这是一个目录"
		return p, nil
	}

	if mime, ok := imageMIME[strings.ToLower(filepath.Ext(full))]; ok {
		if fi.Size() > previewImageLimit {
			p.Kind = "other"
			p.Message = fmt.Sprintf("图片超过 %s，未生成预览", humanSize(previewImageLimit))
			return p, nil
		}
		data, err := os.ReadFile(full)
		if err != nil {
			return model.FilePreview{}, fmt.Errorf("读取失败：%v", err)
		}
		p.Kind = "image"
		p.DataURL = "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data)
		return p, nil
	}

	f, err := os.Open(full)
	if err != nil {
		return model.FilePreview{}, fmt.Errorf("读取失败：%v", err)
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, previewTextLimit+1))
	if err != nil {
		return model.FilePreview{}, fmt.Errorf("读取失败：%v", err)
	}
	if len(data) > previewTextLimit {
		data = data[:previewTextLimit]
		p.Truncated = true
	}
	sniff := data
	if len(sniff) > binarySniffLen {
		sniff = sniff[:binarySniffLen]
	}
	if bytes.IndexByte(sniff, 0) >= 0 {
		p.Kind = "other"
		p.Binary = true
		p.Message = "二进制文件，无法预览内容"
		return p, nil
	}
	p.Kind = "text"
	p.Text = string(data)
	return p, nil
}

func joinRel(dir, name string) string {
	if dir == "" {
		return name
	}
	return dir + "/" + name
}

// summarizeDir 把目录下子项的变更汇总成目录自身的状态。
func summarizeDir(states map[string]string, key string) string {
	prefix := key + "/"
	rank := map[string]int{"M": 4, "A": 3, "R": 3, "D": 2, "??": 1}
	best, bestRank := "", 0
	for k, v := range states {
		if r, ok := rank[v]; ok && strings.HasPrefix(k, prefix) && r > bestRank {
			best, bestRank = v, r
		}
	}
	return best
}

func humanSize(n int64) string {
	switch {
	case n >= 1024*1024:
		return fmt.Sprintf("%.0f MB", float64(n)/1024/1024)
	case n >= 1024:
		return fmt.Sprintf("%.0f KB", float64(n)/1024)
	}
	return fmt.Sprintf("%d B", n)
}
