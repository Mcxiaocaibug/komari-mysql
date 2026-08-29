package dbcore

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCollectPersistentFilesIncludesConfiguredRoots(t *testing.T) {
	root := t.TempDir()
	paths := map[string]string{
		"favicon.ico":              "icon",
		"theme/demo/index.html":    "theme",
		"plugin/demo/script.js":    "plugin",
		"plugin-data/demo/db.json": "state",
	}
	for name, content := range paths {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o640); err != nil {
			t.Fatal(err)
		}
	}
	files, err := collectPersistentFiles(root)
	if err != nil {
		t.Fatalf("collect persistent files: %v", err)
	}
	if len(files) != len(paths) {
		t.Fatalf("collected %d files, want %d", len(files), len(paths))
	}
	for _, file := range files {
		if _, ok := paths[file.meta.Path]; !ok {
			t.Errorf("unexpected persistent path %q", file.meta.Path)
		}
		if file.meta.SHA256 == "" || file.meta.ChunkSize != mysqlPersistentFileChunkSize {
			t.Errorf("incomplete metadata for %q: %#v", file.meta.Path, file.meta)
		}
	}
}

func TestPersistentFileTargetRejectsEscapes(t *testing.T) {
	root := t.TempDir()
	for _, path := range []string{"../outside", "../../outside", "/absolute"} {
		if _, err := persistentFileTarget(root, path); err == nil {
			t.Errorf("persistentFileTarget accepted %q", path)
		}
	}
	target, err := persistentFileTarget(root, "plugin/demo/state.json")
	if err != nil {
		t.Fatalf("valid persistent target: %v", err)
	}
	if target != filepath.Join(root, "plugin", "demo", "state.json") {
		t.Fatalf("target = %q", target)
	}
}

func TestPersistentFileRootPresentTracksSingletonDeletion(t *testing.T) {
	root := t.TempDir()
	if !persistentFileRootPresent(root, "favicon.ico") {
		t.Fatal("data directory should make a missing singleton an intentional deletion")
	}
	if persistentFileRootPresent(root, "theme/demo/index.html") {
		t.Fatal("missing theme root should not delete a database mirror before restore")
	}
	if err := os.Mkdir(filepath.Join(root, "theme"), 0o755); err != nil {
		t.Fatal(err)
	}
	if !persistentFileRootPresent(root, "theme/demo/index.html") {
		t.Fatal("existing theme root should make a missing theme file an intentional deletion")
	}
}
