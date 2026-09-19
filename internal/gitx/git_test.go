package gitx

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestCommitLogRollback(t *testing.T) {
	dir := t.TempDir()
	if err := Init(dir); err != nil {
		t.Fatalf("init: %v", err)
	}
	writeFile(t, dir, "a.txt", "one")
	sha1, changed, err := CommitAll(dir, "first")
	if err != nil {
		t.Fatalf("commit1: %v", err)
	}
	if !changed || sha1 == "" {
		t.Fatal("expected first commit to record a change")
	}

	writeFile(t, dir, "b.txt", "two")
	if _, changed, err = CommitAll(dir, "second"); err != nil || !changed {
		t.Fatalf("commit2: changed=%v err=%v", changed, err)
	}

	commits, err := Log(dir, 10)
	if err != nil {
		t.Fatalf("log: %v", err)
	}
	if len(commits) != 2 {
		t.Fatalf("want 2 commits, got %d", len(commits))
	}
	if commits[0].Subject != "second" {
		t.Fatalf("unexpected newest subject %q", commits[0].Subject)
	}
	if commits[0].Files != 1 {
		t.Fatalf("want 1 changed file, got %d", commits[0].Files)
	}

	writeFile(t, dir, "c.txt", "three")
	if _, err := Rollback(dir, sha1, true, "123"); err != nil {
		t.Fatalf("rollback: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "b.txt")); !os.IsNotExist(err) {
		t.Fatal("b.txt should be gone after hard rollback")
	}
	commits, _ = Log(dir, 10)
	if len(commits) != 1 {
		t.Fatalf("want 1 commit after rollback, got %d", len(commits))
	}
	out, _, err := run(dir, "tag", "-l", "vibe-pm-backup-123")
	if err != nil || strings.TrimSpace(out) == "" {
		t.Fatalf("expected backup tag to exist: %q %v", out, err)
	}
}

func TestStatusBranchAndDirty(t *testing.T) {
	dir := t.TempDir()
	if err := Init(dir); err != nil {
		t.Fatalf("init: %v", err)
	}
	writeFile(t, dir, "a.txt", "1")
	if _, _, err := CommitAll(dir, "first"); err != nil {
		t.Fatalf("commit: %v", err)
	}

	st, err := Status(dir)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if st.Branch == "" {
		t.Fatal("branch should not be empty")
	}
	if st.Dirty {
		t.Fatal("should be clean right after commit")
	}

	writeFile(t, dir, "b.txt", "2")
	if st, err = Status(dir); err != nil {
		t.Fatalf("status: %v", err)
	}
	if !st.Dirty {
		t.Fatal("should be dirty with an untracked file")
	}
	if st.HasUpstream {
		t.Fatal("fresh repo should not report an upstream")
	}
}

func TestLastCommit(t *testing.T) {
	dir := t.TempDir()
	if err := Init(dir); err != nil {
		t.Fatalf("init: %v", err)
	}
	subject, when, err := LastCommit(dir)
	if err != nil {
		t.Fatalf("last: %v", err)
	}
	if subject != "" || when != 0 {
		t.Fatalf("empty repo should have no last commit, got %q %d", subject, when)
	}

	writeFile(t, dir, "a.txt", "1")
	if _, _, err := CommitAll(dir, "hello"); err != nil {
		t.Fatalf("commit: %v", err)
	}
	subject, when, err = LastCommit(dir)
	if err != nil {
		t.Fatalf("last: %v", err)
	}
	if subject != "hello" {
		t.Fatalf("subject = %q, want hello", subject)
	}
	if when == 0 {
		t.Fatal("commit time should be set")
	}
}

func TestStashesAndBackupTags(t *testing.T) {
	dir := t.TempDir()
	if err := Init(dir); err != nil {
		t.Fatalf("init: %v", err)
	}
	writeFile(t, dir, "a.txt", "1")
	if _, _, err := CommitAll(dir, "first"); err != nil {
		t.Fatalf("commit: %v", err)
	}

	writeFile(t, dir, "a.txt", "2")
	if _, err := DiscardChanges(dir, "20260101-000000"); err != nil {
		t.Fatalf("discard: %v", err)
	}

	stashes, err := Stashes(dir)
	if err != nil {
		t.Fatalf("stashes: %v", err)
	}
	if len(stashes) != 1 {
		t.Fatalf("want 1 stash, got %d", len(stashes))
	}
	if stashes[0].Kind != "stash" || stashes[0].Ref == "" {
		t.Fatalf("bad stash entry: %+v", stashes[0])
	}
	if !strings.Contains(stashes[0].Message, "保底备份") {
		t.Fatalf("stash message lost: %q", stashes[0].Message)
	}
	if stashes[0].When == 0 {
		t.Fatalf("stash time missing: %+v", stashes[0])
	}

	writeFile(t, dir, "b.txt", "3")
	if _, _, err := CommitAll(dir, "second"); err != nil {
		t.Fatalf("commit2: %v", err)
	}
	head, _, _ := run(dir, "rev-parse", "HEAD")
	if _, err := Rollback(dir, strings.TrimSpace(head), true, "20260101-000001"); err != nil {
		t.Fatalf("rollback: %v", err)
	}

	tags, err := BackupTags(dir, BackupTagPrefix)
	if err != nil {
		t.Fatalf("tags: %v", err)
	}
	if len(tags) != 1 {
		t.Fatalf("want 1 backup tag, got %d", len(tags))
	}
	if tags[0].Kind != "tag" || tags[0].When == 0 {
		t.Fatalf("bad tag entry: %+v", tags[0])
	}

	if _, err := ApplyStash(dir, stashes[0].Ref); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if _, err := DropStash(dir, stashes[0].Ref); err != nil {
		t.Fatalf("drop: %v", err)
	}
	if left, _ := Stashes(dir); len(left) != 0 {
		t.Fatalf("want 0 stashes after drop, got %d", len(left))
	}
	if _, err := DeleteTag(dir, tags[0].Ref); err != nil {
		t.Fatalf("delete tag: %v", err)
	}
	if left, _ := BackupTags(dir, BackupTagPrefix); len(left) != 0 {
		t.Fatalf("want 0 tags after delete, got %d", len(left))
	}
}

func TestCommitFiles(t *testing.T) {
	dir := t.TempDir()
	if err := Init(dir); err != nil {
		t.Fatalf("init: %v", err)
	}
	writeFile(t, dir, "a.txt", "1")
	if _, _, err := CommitAll(dir, "first"); err != nil {
		t.Fatalf("commit: %v", err)
	}
	writeFile(t, dir, "a.txt", "changed")
	writeFile(t, dir, "b.txt", "new")
	if _, _, err := CommitAll(dir, "second"); err != nil {
		t.Fatalf("commit2: %v", err)
	}

	files, err := CommitFiles(dir, "HEAD")
	if err != nil {
		t.Fatalf("files: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("want 2 changed files, got %d: %+v", len(files), files)
	}
	kinds := map[string]bool{}
	for _, f := range files {
		kinds[f.Status] = true
	}
	if !kinds["M"] || !kinds["A"] {
		t.Fatalf("expected one M and one A, got %+v", files)
	}
}

func TestDiscardChanges(t *testing.T) {
	dir := t.TempDir()
	if err := Init(dir); err != nil {
		t.Fatalf("init: %v", err)
	}
	writeFile(t, dir, "a.txt", "1")
	if _, _, err := CommitAll(dir, "first"); err != nil {
		t.Fatalf("commit: %v", err)
	}
	writeFile(t, dir, "a.txt", "2")
	if _, err := DiscardChanges(dir, "456"); err != nil {
		t.Fatalf("discard: %v", err)
	}
	dirty, err := isDirty(dir)
	if err != nil {
		t.Fatalf("dirty: %v", err)
	}
	if dirty {
		t.Fatal("working tree should be clean after discard")
	}
}
