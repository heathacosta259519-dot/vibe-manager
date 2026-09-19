package archive

import (
	"os"
	"path/filepath"
	"testing"

	"vibe-manager/internal/model"
)

func TestArchiveRestore(t *testing.T) {
	root := t.TempDir()
	cfg := model.Config{ArchiveRoot: filepath.Join(root, "archive")}
	proj := filepath.Join(root, "demo")

	if err := os.MkdirAll(filepath.Join(proj, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(proj, "README.md"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(proj, "src", "main.go"), []byte("package main"), 0o644); err != nil {
		t.Fatal(err)
	}

	entry, err := Archive(cfg, proj, "demo")
	if err != nil {
		t.Fatalf("archive: %v", err)
	}
	if _, err := os.Stat(entry.ZipPath); err != nil {
		t.Fatalf("zip missing: %v", err)
	}
	if _, err := os.Stat(proj); !os.IsNotExist(err) {
		t.Fatal("original dir should be removed after archive")
	}

	list, err := List(cfg)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("want 1 index entry, got %d", len(list))
	}

	if _, err := Restore(cfg, entry.ZipPath); err != nil {
		t.Fatalf("restore: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(proj, "src", "main.go"))
	if err != nil {
		t.Fatalf("read restored file: %v", err)
	}
	if string(data) != "package main" {
		t.Fatalf("restored content mismatch: %q", data)
	}
	if _, err := os.Stat(entry.ZipPath); !os.IsNotExist(err) {
		t.Fatal("zip should be removed after restore")
	}
	list, _ = List(cfg)
	if len(list) != 0 {
		t.Fatalf("index should be empty after restore, got %d", len(list))
	}
}

func TestArchiveDelete(t *testing.T) {
	root := t.TempDir()
	cfg := model.Config{ArchiveRoot: filepath.Join(root, "archive")}
	proj := filepath.Join(root, "demo")

	if err := os.MkdirAll(proj, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(proj, "a.txt"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}

	entry, err := Archive(cfg, proj, "demo")
	if err != nil {
		t.Fatalf("archive: %v", err)
	}
	if err := Delete(cfg, entry.ZipPath); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := os.Stat(entry.ZipPath); !os.IsNotExist(err) {
		t.Fatal("zip should be gone after delete")
	}
	list, err := List(cfg)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("index should be empty after delete, got %d", len(list))
	}
	if _, err := os.Stat(proj); !os.IsNotExist(err) {
		t.Fatal("delete must not resurrect the project dir")
	}
	if err := Delete(cfg, entry.ZipPath); err == nil {
		t.Fatal("deleting a missing entry should fail")
	}
}
