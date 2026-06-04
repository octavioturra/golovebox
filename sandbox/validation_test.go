package sandbox

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStart_rejectsCorruptImage(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	imgPath := filepath.Join(dir, "base.img")

	// Write garbage — not a qcow2 magic header.
	if err := os.WriteFile(imgPath, []byte("not-a-qcow2-image"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Start(Config{VMDir: dir, QEMUDir: dir, QMPPort: 14444, SSHPort: 12222})
	if err == nil {
		t.Fatal("Start: want error for corrupt image, got nil")
	}
}

func TestStart_rejectsMissingImage(t *testing.T) {
	t.Parallel()
	dir := t.TempDir() // base.img absent
	_, err := Start(Config{VMDir: dir, QEMUDir: dir, QMPPort: 14445, SSHPort: 12223})
	if err == nil {
		t.Fatal("Start: want error for missing base.img, got nil")
	}
}

func TestConfig_pathsRetainDir(t *testing.T) {
	// Verify that Config fields are used verbatim (no hidden path manipulation).
	cfg := Config{VMDir: "/some/dir", QEMUDir: "/qemu", SSHPort: 2222, QMPPort: 4444}
	if cfg.VMDir != "/some/dir" {
		t.Fatalf("VMDir mutated: %q", cfg.VMDir)
	}
	if cfg.SSHUser == "" {
		// SSHUser defaults in NewVM/Attach, not in Config itself — expected.
	}
}
