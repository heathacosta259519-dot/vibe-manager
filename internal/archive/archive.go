package archive

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"vibe-manager/internal/model"
)

func indexPath(cfg model.Config) string {
	return filepath.Join(cfg.ArchiveRoot, "index.json")
}

func List(cfg model.Config) ([]model.ArchiveEntry, error) {
	data, err := os.ReadFile(indexPath(cfg))
	if err != nil {
		if os.IsNotExist(err) {
			return []model.ArchiveEntry{}, nil
		}
		return nil, err
	}
	var entries []model.ArchiveEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, err
	}
	return entries, nil
}

func saveIndex(cfg model.Config, entries []model.ArchiveEntry) error {
	if err := os.MkdirAll(cfg.ArchiveRoot, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(indexPath(cfg), data, 0o644)
}

func Archive(cfg model.Config, projPath, name string) (model.ArchiveEntry, error) {
	if err := os.MkdirAll(cfg.ArchiveRoot, 0o755); err != nil {
		return model.ArchiveEntry{}, err
	}
	now := time.Now()
	zipName := fmt.Sprintf("%s-%s.zip", name, now.Format("20060102-150405"))
	zipPath := filepath.Join(cfg.ArchiveRoot, zipName)

	if err := zipDir(projPath, zipPath); err != nil {
		os.Remove(zipPath)
		return model.ArchiveEntry{}, err
	}
	fi, err := os.Stat(zipPath)
	if err != nil {
		return model.ArchiveEntry{}, err
	}
	if err := os.RemoveAll(projPath); err != nil {
		return model.ArchiveEntry{}, fmt.Errorf("删除原目录失败：%v", err)
	}
	entry := model.ArchiveEntry{
		Name:         name,
		ZipPath:      zipPath,
		OriginalPath: projPath,
		ArchivedAt:   now.Unix(),
		Size:         fi.Size(),
	}
	entries, err := List(cfg)
	if err != nil {
		return model.ArchiveEntry{}, err
	}
	if err := saveIndex(cfg, append(entries, entry)); err != nil {
		return model.ArchiveEntry{}, err
	}
	return entry, nil
}

func Delete(cfg model.Config, zipPath string) error {
	entries, err := List(cfg)
	if err != nil {
		return err
	}
	idx := -1
	for i, e := range entries {
		if strings.EqualFold(e.ZipPath, zipPath) {
			idx = i
			break
		}
	}
	if idx < 0 {
		return fmt.Errorf("归档记录不存在：%s", zipPath)
	}
	if err := os.Remove(entries[idx].ZipPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("删除 zip 失败：%v", err)
	}
	return saveIndex(cfg, append(entries[:idx], entries[idx+1:]...))
}

func Restore(cfg model.Config, zipPath string) (model.ArchiveEntry, error) {
	entries, err := List(cfg)
	if err != nil {
		return model.ArchiveEntry{}, err
	}
	idx := -1
	for i, e := range entries {
		if strings.EqualFold(e.ZipPath, zipPath) {
			idx = i
			break
		}
	}
	if idx < 0 {
		return model.ArchiveEntry{}, fmt.Errorf("归档记录不存在：%s", zipPath)
	}
	entry := entries[idx]
	if _, err := os.Stat(entry.OriginalPath); err == nil {
		return model.ArchiveEntry{}, fmt.Errorf("原路径已被占用，无法还原：%s", entry.OriginalPath)
	}
	if err := unzip(entry.ZipPath, entry.OriginalPath); err != nil {
		return model.ArchiveEntry{}, err
	}
	os.Remove(entry.ZipPath)
	rest := append(entries[:idx], entries[idx+1:]...)
	if err := saveIndex(cfg, rest); err != nil {
		return model.ArchiveEntry{}, err
	}
	return entry, nil
}

func zipDir(src, dst string) error {
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	zw := zip.NewWriter(out)
	defer zw.Close()

	base := filepath.Clean(src)
	return filepath.Walk(base, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if path == base {
			return nil
		}
		rel, err := filepath.Rel(base, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if info.IsDir() {
			_, err := zw.Create(rel + "/")
			return err
		}
		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		header.Name = rel
		header.Method = zip.Deflate
		w, err := zw.CreateHeader(header)
		if err != nil {
			return err
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = io.Copy(w, f)
		return err
	})
}

func unzip(src, dst string) error {
	zr, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer zr.Close()
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	for _, f := range zr.File {
		target := filepath.Join(dst, filepath.FromSlash(f.Name))
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, f.Mode())
		if err != nil {
			rc.Close()
			return err
		}
		_, err = io.Copy(out, rc)
		rc.Close()
		out.Close()
		if err != nil {
			return err
		}
	}
	return nil
}
