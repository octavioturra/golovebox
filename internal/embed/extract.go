package embedassets

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// ErrAssetsNotFetched is returned when placeholder assets are found instead of
// real binaries. The user must run "mage fetch" before building.
var ErrAssetsNotFetched = fmt.Errorf(
	"QEMU/Alpine assets not embedded: run 'mage fetch' before 'mage build'",
)

// ExtractQEMU extracts the embedded QEMU binaries into destDir.
// Idempotent: skips files that already exist with the correct size.
func ExtractQEMU(destDir string) error {
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return fmt.Errorf("extract qemu: mkdir: %w", err)
	}

	entries, err := QEMUAssets.ReadDir(qemuAssetDir)
	if err != nil {
		return fmt.Errorf("extract qemu: readdir: %w", err)
	}

	// Detect placeholder-only directory.
	if len(entries) == 1 && entries[0].Name() == "placeholder.txt" {
		return ErrAssetsNotFetched
	}

	return fs.WalkDir(QEMUAssets, qemuAssetDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(qemuAssetDir, filepath.FromSlash(path))
		dest := filepath.Join(destDir, rel)

		if d.IsDir() {
			return os.MkdirAll(dest, 0o755)
		}
		if d.Name() == "placeholder.txt" {
			return nil
		}
		return extractEmbedFile(QEMUAssets, path, dest, isExecutable(d.Name()))
	})
}

// ExtractAlpineISO writes the embedded Alpine ISO to destDir/alpine-virt-x86_64.iso.
// Idempotent: skips if the file already exists with the correct size.
func ExtractAlpineISO(destDir string) error {
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return fmt.Errorf("extract alpine: mkdir: %w", err)
	}

	destPath := filepath.Join(destDir, alpineISOName)

	data, err := ReadAlpineISO()
	if err != nil {
		// File might be placeholder.txt — check size to distinguish
		return ErrAssetsNotFetched
	}

	// Check if it's just the placeholder.
	if strings.Contains(string(data), "mage fetch") {
		return ErrAssetsNotFetched
	}

	// Idempotency: skip if destination has the same size.
	if fi, err := os.Stat(destPath); err == nil && fi.Size() == int64(len(data)) {
		return nil
	}

	tmp := destPath + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("extract alpine: write: %w", err)
	}
	return os.Rename(tmp, destPath)
}

// QEMUExePath returns the path to qemu-system-x86_64 in the given directory.
// The weilnetz.de NSIS installer extracts to a flat layout (no bin/ subdir).
func QEMUExePath(qemuDir string) string {
	if runtime.GOOS == "windows" {
		return filepath.Join(qemuDir, "qemu-system-x86_64.exe")
	}
	return filepath.Join(qemuDir, "qemu-system-x86_64")
}

// QEMUImgPath returns the path to qemu-img in the given directory.
func QEMUImgPath(qemuDir string) string {
	if runtime.GOOS == "windows" {
		return filepath.Join(qemuDir, "qemu-img.exe")
	}
	return filepath.Join(qemuDir, "qemu-img")
}

// extractEmbedFile writes one file from an embed.FS to dest.
func extractEmbedFile(fsys fs.FS, src, dest string, executable bool) error {
	fi, err := fs.Stat(fsys, src)
	if err != nil {
		return err
	}

	// Idempotency: skip if sizes match.
	if di, err := os.Stat(dest); err == nil && di.Size() == fi.Size() {
		return nil
	}

	f, err := fsys.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()

	mode := fs.FileMode(0o644)
	if executable {
		mode = 0o755
	}

	tmp := dest + ".tmp"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, f); err != nil {
		out.Close()
		os.Remove(tmp)
		return err
	}
	out.Close()
	return os.Rename(tmp, dest)
}

func isExecutable(name string) bool {
	return strings.HasSuffix(name, ".exe") ||
		name == "qemu-system-x86_64" ||
		name == "qemu-img"
}
