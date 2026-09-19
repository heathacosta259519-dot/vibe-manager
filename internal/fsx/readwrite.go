package fsx

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"vibe-manager/internal/model"
)

// EditorMaxBytes 是编辑器单文件读取上限，超过则只读前一段并标记截断。
const EditorMaxBytes = 2 << 20 // 2 MB

// ReadText 读取文本内容供编辑器使用。
// 目录、二进制、超过上限的文件都会如实标注，由调用方决定怎么展示。
func ReadText(root, rel string, maxBytes int64) (model.TextFile, error) {
	full, _, err := Resolve(root, rel)
	if err != nil {
		return model.TextFile{}, err
	}
	fi, err := os.Stat(full)
	if err != nil {
		return model.TextFile{}, fmt.Errorf("读取失败：%v", err)
	}
	if fi.IsDir() {
		return model.TextFile{}, fmt.Errorf("这是一个目录，不能编辑")
	}
	if maxBytes <= 0 {
		maxBytes = EditorMaxBytes
	}

	tf := model.TextFile{Size: fi.Size()}
	f, err := os.Open(full)
	if err != nil {
		return model.TextFile{}, fmt.Errorf("读取失败：%v", err)
	}
	defer f.Close()

	data, err := io.ReadAll(io.LimitReader(f, maxBytes+1))
	if err != nil {
		return model.TextFile{}, fmt.Errorf("读取失败：%v", err)
	}
	if int64(len(data)) > maxBytes {
		data = data[:maxBytes]
		tf.Truncated = true
	}
	sniff := data
	if len(sniff) > binarySniffLen {
		sniff = sniff[:binarySniffLen]
	}
	if bytes.IndexByte(sniff, 0) >= 0 {
		tf.Binary = true
		return tf, nil
	}
	tf.Content = string(data)
	return tf, nil
}

// WriteText 原子写回文本文件：先写同目录下的临时文件，再改名覆盖。
// 这样即使中途失败，原文件也不会被写坏。
func WriteText(root, rel, content string) error {
	full, norm, err := Resolve(root, rel)
	if err != nil {
		return err
	}
	if norm == "" {
		return fmt.Errorf("不能写入项目根目录")
	}
	if fi, err := os.Stat(full); err == nil && fi.IsDir() {
		return fmt.Errorf("目标是目录，不能写入")
	}

	dir := filepath.Dir(full)
	tmp, err := os.CreateTemp(dir, ".vibe-pm-*.tmp")
	if err != nil {
		return fmt.Errorf("创建临时文件失败：%v", err)
	}
	tmpName := tmp.Name()
	cleanup := func() {
		tmp.Close()
		os.Remove(tmpName)
	}
	if _, err := tmp.WriteString(content); err != nil {
		cleanup()
		return fmt.Errorf("写入失败：%v", err)
	}
	if err := tmp.Sync(); err != nil {
		cleanup()
		return fmt.Errorf("写入失败：%v", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("写入失败：%v", err)
	}

	// 保留原文件权限（若已存在）
	if fi, err := os.Stat(full); err == nil {
		_ = os.Chmod(tmpName, fi.Mode())
	}
	if err := os.Rename(tmpName, full); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("替换原文件失败：%v", err)
	}
	return nil
}
