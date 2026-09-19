package fsx

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"vibe-manager/internal/model"
)

// 内容搜索的代价上限：宁可少搜一点，也不要把界面卡住。
const (
	searchMaxFileSize  = 1 << 20 // 单文件 1 MB
	searchMaxFiles     = 5000    // 单项目最多看这么多文件
	searchPerFileHits  = 5       // 单文件最多这么多条命中
	searchSniffLen     = 8192    // 二进制嗅探长度
	searchLineMaxRunes = 200     // 命中行最长展示多少字符
)

// SearchFiles 在单个项目内搜索文件名与文件内容。
// 跳过噪音目录（node_modules/.git/dist 等）、二进制文件与过大的文件。
func SearchFiles(root, query string) ([]model.SearchHit, error) {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return []model.SearchHit{}, nil
	}
	hits := []model.SearchHit{}
	scanned := 0

	walkErr := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // 读不了的项直接跳过
		}
		name := d.Name()
		if d.IsDir() {
			if path == root {
				return nil
			}
			if strings.HasPrefix(name, ".") || noiseNames[strings.ToLower(name)] {
				return fs.SkipDir
			}
			return nil
		}

		scanned++
		if scanned > searchMaxFiles {
			return fs.SkipAll
		}

		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return nil
		}
		relSlash := filepath.ToSlash(rel)

		if strings.Contains(strings.ToLower(name), q) {
			hits = append(hits, model.SearchHit{Path: relSlash, Kind: "file"})
		}

		info, infoErr := d.Info()
		if infoErr != nil || info.Size() > searchMaxFileSize {
			return nil
		}
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		sniff := content
		if len(sniff) > searchSniffLen {
			sniff = sniff[:searchSniffLen]
		}
		if bytes.IndexByte(sniff, 0) >= 0 {
			return nil // 二进制，不做内容匹配
		}

		count := 0
		for i, line := range strings.Split(string(content), "\n") {
			if count >= searchPerFileHits {
				break
			}
			if strings.Contains(strings.ToLower(line), q) {
				hits = append(hits, model.SearchHit{
					Path:   relSlash,
					Line:   clip(strings.TrimSpace(line), searchLineMaxRunes),
					LineNo: i + 1,
					Kind:   "content",
				})
				count++
			}
		}
		return nil
	})
	if walkErr != nil {
		return hits, walkErr
	}
	return hits, nil
}

func clip(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "…"
}
