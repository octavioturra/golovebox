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

// ExtractAlpineImage writes the embedded Alpine cloud qcow2 directly to
// destDir/base.img. The cloud image is already a ready-to-boot Alpine install
// with cloud-init enabled — no separate "install boot" step is required.
//
// Idempotency is driven by a marker file (.alpine-image-size) holding the
// embedded image's size in bytes. This avoids stale base.img files left over
// from earlier installs (e.g. the empty 8GB qcow2 created by older flows)
// silently surviving and causing "could not read the boot disk" at boot time.
func ExtractAlpineImage(destDir string) error {
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return fmt.Errorf("extract alpine: mkdir: %w", err)
	}

	destPath := filepath.Join(destDir, "base.img")
	markerPath := filepath.Join(destDir, ".alpine-image-size")

	data, err := ReadAlpineImage()
	if err != nil {
		return ErrAssetsNotFetched
	}
	if strings.Contains(string(data[:min(len(data), 256)]), "mage fetch") {
		return ErrAssetsNotFetched
	}

	// qcow2 magic check: protect against embedding a wrong/corrupt file.
	if !(len(data) >= 4 && data[0] == 'Q' && data[1] == 'F' && data[2] == 'I' && data[3] == 0xfb) {
		return fmt.Errorf("extract alpine: embedded image is not a qcow2 (magic mismatch) — re-run 'mage fetchAlpine'")
	}

	wantSize := fmt.Sprintf("%d", len(data))

	// Skip only if BOTH marker and base.img exist AND the marker matches.
	if markerData, err := os.ReadFile(markerPath); err == nil {
		if strings.TrimSpace(string(markerData)) == wantSize {
			if _, err := os.Stat(destPath); err == nil {
				return nil
			}
		}
	}

	// Force a clean overwrite: remove any stale base.img from previous installs.
	_ = os.Remove(destPath)
	_ = os.Remove(markerPath)

	fmt.Printf("[extract] writing base.img (%.1f MB)...\n", float64(len(data))/(1<<20))
	tmp := destPath + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("extract alpine: write: %w", err)
	}
	if err := os.Rename(tmp, destPath); err != nil {
		return err
	}
	return os.WriteFile(markerPath, []byte(wantSize), 0o644)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
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
