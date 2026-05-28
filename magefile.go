//go:build mage

// Build pipeline for golovebox.
// Install mage: go install github.com/magefile/mage@latest
//
// Usage:
//
//	mage fetch         — download QEMU binaries + Alpine ISO for the current OS
//	mage fetchWindows  — download Windows amd64 QEMU binaries
//	mage fetchAlpine   — download Alpine Virt ISO only
//	mage build         — compile for the current OS/arch
//	mage buildWindows  — cross-compile for Windows amd64
//	mage clean         — remove downloaded assets (keeps placeholder.txt)
//	mage check         — run go vet ./...
package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	// Alpine Virt ISO: minimal x86_64 image designed for QEMU.
	alpineISOURL  = "https://dl-cdn.alpinelinux.org/alpine/v3.21/releases/x86_64/alpine-virt-3.21.3-x86_64.iso"
	alpineISODest = "internal/embed/assets/alpine/alpine-virt-x86_64.iso"

	// MSYS2 mingw64 QEMU package for Windows amd64.
	// Check https://repo.msys2.org/mingw/mingw64/ for newer versions.
	msys2QEMUURL = "https://repo.msys2.org/mingw/mingw64/mingw-w64-x86_64-qemu-9.2.2-1-any.pkg.tar.zst"
	winQEMUDest  = "internal/embed/assets/qemu/windows-amd64"

	buildDir = "build"
)

// Fetch downloads all embedded assets for the current host OS.
func Fetch() error {
	if err := FetchAlpine(); err != nil {
		return err
	}
	return FetchQemu()
}

// FetchAlpine downloads the Alpine Virt x86_64 ISO into the embed assets directory.
func FetchAlpine() error {
	if fi, err := os.Stat(alpineISODest); err == nil && fi.Size() > 10<<20 {
		fmt.Println("[fetch] Alpine ISO already present, skipping.")
		return nil
	}
	fmt.Printf("[fetch] Downloading Alpine Virt ISO...\n")
	return downloadFile(alpineISOURL, alpineISODest)
}

// FetchQemu downloads QEMU binaries for the current host OS.
// Override with TARGET_OS=windows|linux environment variable.
func FetchQemu() error {
	target := os.Getenv("TARGET_OS")
	if target == "" {
		target = runtime.GOOS
	}
	switch target {
	case "windows":
		return FetchWindows()
	case "linux":
		return fetchQEMULinux()
	default:
		fmt.Printf("[fetch] Automated QEMU fetch for %q not supported.\n", target)
		fmt.Printf("        Place QEMU binaries in internal/embed/assets/qemu/%s-*/\n", target)
		return nil
	}
}

// FetchWindows downloads and extracts the Windows amd64 QEMU package from MSYS2.
// Requires: tar with zstd support (GNU tar 1.31+ or BSD tar on macOS 12+).
func FetchWindows() error {
	marker := filepath.Join(winQEMUDest, "bin", "qemu-system-x86_64.exe")
	if _, err := os.Stat(marker); err == nil {
		fmt.Println("[fetch] Windows QEMU already present, skipping.")
		return nil
	}

	tmp := filepath.Join(os.TempDir(), "qemu-mingw64.pkg.tar.zst")
	fmt.Printf("[fetch] Downloading MSYS2 QEMU package...\n")
	if err := downloadFile(msys2QEMUURL, tmp); err != nil {
		return err
	}
	defer os.Remove(tmp)

	fmt.Printf("[fetch] Extracting QEMU to %s ...\n", winQEMUDest)
	return extractMSYS2Package(tmp, winQEMUDest)
}

// fetchQEMULinux prints instructions for obtaining Linux QEMU binaries.
func fetchQEMULinux() error {
	destDir := "internal/embed/assets/qemu/linux-amd64"
	marker := filepath.Join(destDir, "qemu-system-x86_64")
	if _, err := os.Stat(marker); err == nil {
		fmt.Println("[fetch] Linux QEMU already present, skipping.")
		return nil
	}
	fmt.Println("[fetch] Linux QEMU: install via your package manager, then copy binaries:")
	fmt.Println("        apt install qemu-system-x86  # Debian/Ubuntu")
	fmt.Println("        apk add qemu-system-x86_64   # Alpine")
	fmt.Printf("        cp $(which qemu-system-x86_64) $(which qemu-img) %s/\n", destDir)
	return nil
}

// extractMSYS2Package extracts a MSYS2 .pkg.tar.zst file to destDir.
// The MSYS2 package layout (mingw64/bin/, mingw64/share/, ...) is stripped
// of the leading "mingw64" component so the embedded FS stays self-contained.
// Requires GNU tar with --zstd support.
func extractMSYS2Package(pkgPath, destDir string) error {
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return err
	}
	// --strip-components=1 removes the leading "mingw64" from each path.
	// Extracting only the "mingw64" subtree skips the .PKGINFO/.MTREE metadata.
	cmd := exec.Command("tar",
		"--zstd",
		"--strip-components=1",
		"-xf", pkgPath,
		"-C", destDir,
		"mingw64",
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("tar extract MSYS2 package: %w\n(ensure tar supports --zstd: GNU tar 1.31+ or BSD tar on macOS 12+)", err)
	}
	return nil
}

// Build compiles golovebox for the current (or GOOS/GOARCH env) platform.
func Build() error {
	if err := os.MkdirAll(buildDir, 0o755); err != nil {
		return err
	}
	goos := envOr("GOOS", runtime.GOOS)
	goarch := envOr("GOARCH", runtime.GOARCH)
	out := filepath.Join(buildDir, "golovebox")
	if goos == "windows" {
		out += ".exe"
	}
	fmt.Printf("[build] %s/%s → %s\n", goos, goarch, out)
	env := append(os.Environ(),
		"GOOS="+goos,
		"GOARCH="+goarch,
		"CGO_ENABLED=0",
	)
	return runWithEnv(env, "go", "build", "-o", out, "./cmd/golovebox")
}

// BuildWindows cross-compiles golovebox for Windows amd64 with CGO disabled.
func BuildWindows() error {
	os.Setenv("GOOS", "windows")
	os.Setenv("GOARCH", "amd64")
	return Build()
}

// Clean removes downloaded assets from internal/embed/assets/, preserving placeholder.txt.
func Clean() error {
	dirs := []string{
		"internal/embed/assets/alpine",
		"internal/embed/assets/qemu/windows-amd64",
		"internal/embed/assets/qemu/linux-amd64",
		"internal/embed/assets/qemu/darwin-arm64",
	}
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.Name() == "placeholder.txt" {
				continue
			}
			path := filepath.Join(dir, e.Name())
			if e.IsDir() {
				if err := os.RemoveAll(path); err != nil {
					return err
				}
			} else {
				if err := os.Remove(path); err != nil {
					return err
				}
			}
			fmt.Printf("[clean] Removed %s\n", path)
		}
	}
	return nil
}

// Check runs go vet on all packages.
func Check() error {
	return runCmd("go", "vet", "./...")
}

// — helpers —

func downloadFile(url, dest string) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	resp, err := http.Get(url) //nolint:gosec,noctx
	if err != nil {
		return fmt.Errorf("download %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s: HTTP %d", url, resp.StatusCode)
	}

	tmp := dest + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	n, copyErr := io.Copy(f, resp.Body)
	f.Close()
	if copyErr != nil {
		os.Remove(tmp)
		return fmt.Errorf("write %s: %w", dest, copyErr)
	}
	fmt.Printf("[fetch] Wrote %s (%.1f MB)\n", dest, float64(n)/(1<<20))
	return os.Rename(tmp, dest)
}

func runCmd(name string, args ...string) error {
	return runWithEnv(os.Environ(), name, args...)
}

func runWithEnv(env []string, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Env = env
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
	}
	return nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
