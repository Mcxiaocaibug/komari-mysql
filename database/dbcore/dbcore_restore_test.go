package dbcore

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

func writeZip(t *testing.T, path string, files map[string]string) {
	t.Helper()

	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create zip failed: %v", err)
	}
	defer f.Close()

	zw := zip.NewWriter(f)
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("create zip entry %s failed: %v", name, err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatalf("write zip entry %s failed: %v", name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip failed: %v", err)
	}
}

func TestApplyBackupZipIfPresentSuccess(t *testing.T) {
	root := t.TempDir()
	dataDir := filepath.Join(root, "data")
	backupDir := filepath.Join(root, "backup")
	backupZipPath := filepath.Join(dataDir, "backup.zip")

	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatalf("mkdir data dir failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, "old.txt"), []byte("old"), 0o644); err != nil {
		t.Fatalf("write old file failed: %v", err)
	}
	writeZip(t, backupZipPath, map[string]string{
		"komari-backup-markup": "backup marker",
		"new/note.txt":         "new-data",
	})

	if err := applyBackupZipIfPresent(dataDir, backupDir, backupZipPath); err != nil {
		t.Fatalf("apply backup failed: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dataDir, "old.txt")); !os.IsNotExist(err) {
		t.Fatalf("old file should be removed after restore, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(dataDir, "new", "note.txt")); err != nil {
		t.Fatalf("restored file is missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dataDir, "komari-backup-markup")); !os.IsNotExist(err) {
		t.Fatalf("backup markup should be removed after restore, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(dataDir, "backup.zip")); !os.IsNotExist(err) {
		t.Fatalf("backup zip should be removed after restore, err=%v", err)
	}

	backupEntries, err := os.ReadDir(backupDir)
	if err != nil {
		t.Fatalf("read backup dir failed: %v", err)
	}
	if len(backupEntries) == 0 {
		t.Fatalf("expected pre-restore snapshot to be created")
	}
}

func TestApplyBackupZipIfPresentInvalidArchiveKeepsExistingData(t *testing.T) {
	root := t.TempDir()
	dataDir := filepath.Join(root, "data")
	backupDir := filepath.Join(root, "backup")
	backupZipPath := filepath.Join(dataDir, "backup.zip")

	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatalf("mkdir data dir failed: %v", err)
	}
	oldPath := filepath.Join(dataDir, "old.txt")
	if err := os.WriteFile(oldPath, []byte("old"), 0o644); err != nil {
		t.Fatalf("write old file failed: %v", err)
	}
	writeZip(t, backupZipPath, map[string]string{
		"new/note.txt": "new-data",
	})

	if err := applyBackupZipIfPresent(dataDir, backupDir, backupZipPath); err == nil {
		t.Fatalf("expected restore to fail when markup file is missing")
	}

	if _, err := os.Stat(oldPath); err != nil {
		t.Fatalf("old data should remain untouched when restore fails: %v", err)
	}
	if _, err := os.Stat(backupZipPath); err != nil {
		t.Fatalf("backup zip should remain for retry when restore fails: %v", err)
	}
}
