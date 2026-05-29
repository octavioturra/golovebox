//go:build mage

// Build pipeline for golovebox.
// Install mage: go install github.com/magefile/mage@latest
//
// Usage:
//
//	mage fetch          — download all assets for the current host OS
//	mage fetchAlpine    — download Alpine Virt ISO only
//	mage fetchWindows   — download Windows QEMU via official installer + 7-zip
//	mage fetchLinux     — copy Linux QEMU from system installation
//	mage fetchDarwin    — copy macOS QEMU from Homebrew or system PATH
//	mage build          — compile for the current OS/arch
//	mage buildWindows   — cross-compile for Windows amd64
//	mage check          — verify embedded assets are present and correctly sized
//	mage clean          — remove downloaded assets (keeps placeholder.txt)
package main

import (
	"crypto/sha512"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
)

// — constants ——————————————————————————————————————————————————————————————

const (
	// Alpine Virt ISO (x86_64, minimal, cloud-init capable).
	alpineISOURL  = "https://dl-cdn.alpinelinux.org/alpine/v3.21/releases/x86_64/alpine-virt-3.21.3-x86_64.iso"
	alpineISODest = "internal/embed/assets/alpine/alpine-virt-x86_64.iso"

	// Official QEMU Windows builds from Stefan Weil.
	weilnetzBase = "https://qemu.weilnetz.de/w64/"

	// 7-zip standalone binaries (mage build-time tool, never embedded).
	// 7zr.exe supports only 7z/xz/lzma — not NSIS. We use it to bootstrap 7za.exe
	// (from the extra package) which does support NSIS installers.
	sz7rExeURL  = "https://www.7-zip.org/a/7zr.exe"                 // Windows — 7z format only
	sz7zExtraURL = "https://www.7-zip.org/a/7z2409-extra.7z"        // contains x64/7za.exe (NSIS-capable)
	sz7zLinURL  = "https://www.7-zip.org/a/7z2409-linux-x64.tar.xz" // Linux amd64
	sz7zMacURL  = "https://www.7-zip.org/a/7z2409-mac.tar.xz"       // macOS (universal)

	// Build-time directories (all gitignored).
	toolsDir = "build/tools"
	tmpDir   = "build/tmp"

	// Embed asset destinations (contents gitignored, placeholder.txt committed).
	winQEMUDir = "internal/embed/assets/qemu/windows-amd64"
	linQEMUDir = "internal/embed/assets/qemu/linux-amd64"
	darQEMUDir = "internal/embed/assets/qemu/darwin-arm64"
)

// — public targets —————————————————————————————————————————————————————————

// Fetch downloads all required assets for the current host OS.
func Fetch() error {
	if err := FetchAlpine(); err != nil {
		return err
	}
	switch runtime.GOOS {
	case "windows":
		return FetchWindows()
	case "linux":
		return FetchLinux()
	case "darwin":
		return FetchDarwin()
	default:
		fmt.Printf("[fetch] Unsupported host OS %q — place QEMU binaries manually.\n", runtime.GOOS)
	}
	return nil
}

// FetchAlpine downloads the Alpine Virt x86_64 ISO into the embed asset directory.
func FetchAlpine() error {
	if fi, err := os.Stat(alpineISODest); err == nil && fi.Size() > 10<<20 {
		fmt.Println("[fetch] Alpine ISO already present, skipping.")
		return nil
	}
	fmt.Println("[fetch] Downloading Alpine Virt ISO...")
	return downloadFile(alpineISOURL, alpineISODest)
}

// FetchWindows downloads the official QEMU Windows installer from qemu.weilnetz.de,
// verifies its SHA512, extracts it using the 7-zip standalone binary (auto-downloaded),
// and copies the needed files (exe, DLLs, firmware) into the embed asset directory.
//
// The installer is an NSIS package — 7-zip can extract it without running it.
// Only qemu-system-x86_64.exe, qemu-img.exe, all *.dll, and share/qemu/ are kept.
func FetchWindows() error {
	marker := filepath.Join(winQEMUDir, "qemu-system-x86_64.exe")
	if _, err := os.Stat(marker); err == nil {
		fmt.Println("[fetch] Windows QEMU already present, skipping.")
		return nil
	}

	szPath, err := fetch7za()
	if err != nil {
		return fmt.Errorf("fetch 7-zip: %w", err)
	}

	installerURL, sha512URL, err := parseLatestQEMUURL()
	if err != nil {
		return fmt.Errorf("find QEMU installer: %w", err)
	}

	if err := os.MkdirAll(tmpDir, 0o755); err != nil {
		return err
	}
	installerPath := filepath.Join(tmpDir, "qemu-installer.exe")
	defer os.Remove(installerPath)

	fmt.Printf("[fetch] Downloading QEMU installer from %s\n", installerURL)
	if err := downloadFile(installerURL, installerPath); err != nil {
		return err
	}

	if sha512URL != "" {
		fmt.Println("[fetch] Verifying SHA512...")
		if err := verifySHA512(installerPath, sha512URL); err != nil {
			return err
		}
	}

	rawDir := filepath.Join(tmpDir, "qemu-raw")
	os.RemoveAll(rawDir)
	defer os.RemoveAll(rawDir)

	fmt.Println("[fetch] Extracting with 7-zip (NSIS format)...")
	cmd := exec.Command(szPath, "x", "-y", "-o"+rawDir, installerPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("7-zip extract: %w", err)
	}

	fmt.Println("[fetch] Filtering and copying QEMU files...")
	return filterAndCopyQEMU(rawDir, winQEMUDir)
}

// FetchLinux copies qemu-system-x86_64 and qemu-img from the system installation.
// If QEMU is not installed, prints instructions and continues (non-fatal).
// Note: the Linux binary will not be portable across distributions.
func FetchLinux() error {
	marker := filepath.Join(linQEMUDir, "qemu-system-x86_64")
	if _, err := os.Stat(marker); err == nil {
		fmt.Println("[fetch] Linux QEMU already present, skipping.")
		return nil
	}

	if err := os.MkdirAll(linQEMUDir, 0o755); err != nil {
		return err
	}

	allFound := true
	for _, name := range []string{"qemu-system-x86_64", "qemu-img"} {
		src, err := exec.LookPath(name)
		if err != nil {
			fmt.Printf("[fetch] WARNING: %s not found in PATH.\n", name)
			fmt.Println("        Install QEMU first:")
			fmt.Println("          apt install qemu-system-x86   # Debian/Ubuntu")
			fmt.Println("          apk add qemu-system-x86_64    # Alpine")
			fmt.Println("        The Linux binary will NOT embed QEMU (system binary required).")
			allFound = false
			continue
		}
		dest := filepath.Join(linQEMUDir, name)
		fmt.Printf("[fetch] Copying %s → %s\n", src, dest)
		if err := copyFile(src, dest); err != nil {
			return err
		}
		os.Chmod(dest, 0o755) //nolint:errcheck
	}
	if !allFound {
		fmt.Println("[fetch] Linux QEMU not fully available — mage build will use placeholder.")
	}
	return nil
}

// FetchDarwin copies qemu-system-x86_64 and qemu-img from the Homebrew prefix
// or system PATH (whichever is available first).
// Prerequisite: brew install qemu
func FetchDarwin() error {
	marker := filepath.Join(darQEMUDir, "qemu-system-x86_64")
	if _, err := os.Stat(marker); err == nil {
		fmt.Println("[fetch] macOS QEMU already present, skipping.")
		return nil
	}

	if err := os.MkdirAll(darQEMUDir, 0o755); err != nil {
		return err
	}

	// Try Homebrew prefix first (more reliable than PATH on macOS).
	srcDir := ""
	if out, err := exec.Command("brew", "--prefix", "qemu").Output(); err == nil {
		srcDir = filepath.Join(strings.TrimSpace(string(out)), "bin")
		fmt.Printf("[fetch] Using Homebrew QEMU from %s\n", srcDir)
	}

	for _, name := range []string{"qemu-system-x86_64", "qemu-img"} {
		var src string
		if srcDir != "" {
			candidate := filepath.Join(srcDir, name)
			if _, err := os.Stat(candidate); err == nil {
				src = candidate
			}
		}
		if src == "" {
			var err error
			src, err = exec.LookPath(name)
			if err != nil {
				fmt.Printf("[fetch] WARNING: %s not found — install with: brew install qemu\n", name)
				continue
			}
		}
		dest := filepath.Join(darQEMUDir, name)
		fmt.Printf("[fetch] Copying %s → %s\n", src, dest)
		if err := copyFile(src, dest); err != nil {
			return err
		}
		os.Chmod(dest, 0o755) //nolint:errcheck
	}
	return nil
}

// Build compiles golovebox for the current OS/arch (or GOOS/GOARCH from environment).
func Build() error {
	if err := os.MkdirAll("build", 0o755); err != nil {
		return err
	}
	goos := envOr("GOOS", runtime.GOOS)
	goarch := envOr("GOARCH", runtime.GOARCH)
	out := filepath.Join("build", "golovebox")
	if goos == "windows" {
		out += ".exe"
	}
	fmt.Printf("[build] %s/%s → %s\n", goos, goarch, out)
	return runWithEnv([]string{
		"GOOS=" + goos,
		"GOARCH=" + goarch,
		"CGO_ENABLED=0",
	}, "go", "build", "-o", out, "./cmd/golovebox")
}

// BuildWindows cross-compiles golovebox for Windows amd64 with CGO disabled.
func BuildWindows() error {
	os.Setenv("GOOS", "windows")
	os.Setenv("GOARCH", "amd64")
	return Build()
}

// Check verifies that all required embedded assets are present and sized correctly.
func Check() error {
	type assetCheck struct {
		path    string
		minSize int64
		desc    string
	}
	checks := []assetCheck{
		{filepath.Join(winQEMUDir, "qemu-system-x86_64.exe"), 10_000_000, "Windows qemu-system-x86_64.exe"},
		{filepath.Join(winQEMUDir, "qemu-img.exe"), 1_000_000, "Windows qemu-img.exe"},
		{filepath.Join(winQEMUDir, "share", "qemu", "bios-256k.bin"), 100_000, "QEMU BIOS firmware"},
		{alpineISODest, 40_000_000, "Alpine Virt ISO"},
	}

	ok := true
	for _, c := range checks {
		fi, err := os.Stat(c.path)
		if err != nil {
			fmt.Printf("[check] MISSING   %s (%s)\n", c.path, c.desc)
			ok = false
			continue
		}
		if fi.Size() < c.minSize {
			fmt.Printf("[check] TOO SMALL %s: %.1f MB < %.1f MB min\n",
				c.desc, float64(fi.Size())/(1<<20), float64(c.minSize)/(1<<20))
			ok = false
			continue
		}
		fmt.Printf("[check] OK        %s (%.1f MB)\n", c.desc, float64(fi.Size())/(1<<20))
	}

	// Count DLLs.
	dllCount := 0
	entries, _ := os.ReadDir(winQEMUDir)
	for _, e := range entries {
		if strings.HasSuffix(strings.ToLower(e.Name()), ".dll") {
			dllCount++
		}
	}
	if dllCount < 50 {
		fmt.Printf("[check] TOO FEW   Windows DLLs: %d (expected > 50)\n", dllCount)
		ok = false
	} else {
		fmt.Printf("[check] OK        Windows DLL count: %d\n", dllCount)
	}

	if !ok {
		return fmt.Errorf("asset check failed — run 'mage fetch' first")
	}
	fmt.Println("[check] All assets OK.")
	return nil
}

// Clean removes downloaded assets from embed directories and build scratch space.
// placeholder.txt files in embed directories are preserved (they allow go:embed to compile).
func Clean() error {
	// Embed asset directories: keep placeholder.txt.
	embedDirs := []string{
		"internal/embed/assets/alpine",
		winQEMUDir,
		linQEMUDir,
		darQEMUDir,
	}
	for _, dir := range embedDirs {
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
				os.RemoveAll(path)
			} else {
				os.Remove(path)
			}
			fmt.Printf("[clean] %s\n", path)
		}
	}
	// Build scratch directories: remove entirely.
	for _, dir := range []string{toolsDir, tmpDir} {
		if err := os.RemoveAll(dir); err == nil {
			fmt.Printf("[clean] %s/\n", dir)
		}
	}
	return nil
}

// — internal helpers ———————————————————————————————————————————————————————

// fetch7za downloads the 7-zip standalone binary for the current host OS
// into build/tools/ and returns its path. Skips download if already present.
//
// - Windows: 7zr.exe (~750 KB), no external dependencies, handles NSIS installers.
// - Linux/macOS: 7za from the tar.xz bundle, extracted via system tar.
func fetch7za() (string, error) {
	if err := os.MkdirAll(toolsDir, 0o755); err != nil {
		return "", err
	}

	switch runtime.GOOS {
	case "windows":
		// We need 7za.exe (NSIS-capable) to extract the QEMU installer.
		// Bootstrap: download 7zr.exe (7z-only), use it to extract 7za.exe from the extra package.
		dest := filepath.Join(toolsDir, "7za.exe")
		if _, err := os.Stat(dest); err == nil {
			return dest, nil
		}
		szrPath := filepath.Join(toolsDir, "7zr.exe")
		if _, err := os.Stat(szrPath); err != nil {
			fmt.Println("[tools] Downloading 7zr.exe (bootstrap)...")
			if err := downloadFile(sz7rExeURL, szrPath); err != nil {
				return "", err
			}
		}
		extraPath := filepath.Join(toolsDir, "7z-extra.7z")
		fmt.Println("[tools] Downloading 7z-extra.7z (contains 7za.exe)...")
		if err := downloadFile(sz7zExtraURL, extraPath); err != nil {
			return "", err
		}
		defer os.Remove(extraPath)
		// Extract only x64/7za.exe from the archive.
		cmd := exec.Command(szrPath, "e", "-y", "-o"+toolsDir, extraPath, "x64/7za.exe")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return "", fmt.Errorf("extract 7za.exe: %w", err)
		}
		if _, err := os.Stat(dest); err != nil {
			return "", fmt.Errorf("7za.exe not found after extraction")
		}
		return dest, nil

	case "linux", "darwin":
		dest := filepath.Join(toolsDir, "7za")
		if _, err := os.Stat(dest); err == nil {
			return dest, nil
		}
		url := sz7zLinURL
		if runtime.GOOS == "darwin" {
			url = sz7zMacURL
		}
		tarPath := filepath.Join(toolsDir, "7z.tar.xz")
		fmt.Printf("[tools] Downloading 7-zip for %s...\n", runtime.GOOS)
		if err := downloadFile(url, tarPath); err != nil {
			return "", err
		}
		defer os.Remove(tarPath)
		// System tar supports .xz on Linux (GNU tar) and macOS 12+ (BSD tar).
		cmd := exec.Command("tar", "-xJf", tarPath, "-C", toolsDir, "7za")
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return "", fmt.Errorf("extract 7za: %w (requires tar with xz support)", err)
		}
		os.Chmod(dest, 0o755) //nolint:errcheck
		return dest, nil
	}
	return "", fmt.Errorf("fetch7za: unsupported host OS %q", runtime.GOOS)
}

// parseLatestQEMUURL scrapes qemu.weilnetz.de/w64/ and returns the URL of the
// most recent QEMU Windows installer and its SHA512 checksum URL.
func parseLatestQEMUURL() (installerURL, sha512URL string, err error) {
	resp, err := http.Get(weilnetzBase) //nolint:noctx
	if err != nil {
		return "", "", fmt.Errorf("GET %s: %w", weilnetzBase, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", err
	}

	re := regexp.MustCompile(`href="(qemu-w64-setup-(\d{8})\.exe)"`)
	matches := re.FindAllStringSubmatch(string(body), -1)
	if len(matches) == 0 {
		return "", "", fmt.Errorf("no installer found at %s", weilnetzBase)
	}

	// Sort descending by the 8-digit date; take the most recent.
	sort.Slice(matches, func(i, j int) bool {
		return matches[i][2] > matches[j][2]
	})

	filename := matches[0][1]
	installerURL = weilnetzBase + filename
	sha512URL = installerURL + ".sha512"
	return installerURL, sha512URL, nil
}

// filterAndCopyQEMU walks srcDir (7-zip extraction of the QEMU NSIS installer)
// and copies only the files needed at runtime into destDir:
//   - qemu-system-x86_64.exe and qemu-img.exe → destDir/ (flat)
//   - *.dll                                   → destDir/ (flat)
//   - share/qemu/*                            → destDir/share/qemu/ (firmware)
//
// All other files (qemu-system-arm*.exe, uninstall*.exe, *.html, etc.) are skipped.
func filterAndCopyQEMU(srcDir, destDir string) error {
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return err
	}
	var dlls, exes, fw int

	err := filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(srcDir, path)
		slashRel := filepath.ToSlash(rel)
		base := strings.ToLower(info.Name())

		var destRel string
		switch {
		case strings.HasSuffix(base, ".dll"):
			destRel = info.Name() // flat: all DLLs at root
			dlls++
		case strings.Contains(slashRel, "share/qemu/"):
			// Preserve share/qemu/ hierarchy for firmware files.
			idx := strings.Index(slashRel, "share/qemu/")
			destRel = filepath.FromSlash(slashRel[idx:])
			fw++
		case base == "qemu-system-x86_64.exe" || base == "qemu-img.exe":
			destRel = info.Name() // flat: executables at root
			exes++
		default:
			return nil // skip uninstaller, docs, other arch binaries, etc.
		}

		dest := filepath.Join(destDir, destRel)
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return err
		}
		return copyFile(path, dest)
	})
	if err != nil {
		return err
	}
	fmt.Printf("[fetch] Copied: %d DLLs, %d exes, %d firmware files → %s\n",
		dlls, exes, fw, destDir)
	if dlls < 20 {
		return fmt.Errorf("too few DLLs (%d) — NSIS extraction may have failed; check 7-zip output", dlls)
	}
	return nil
}

// verifySHA512 downloads the .sha512 checksum file and verifies filePath against it.
// Non-fatal: if the checksum URL is unreachable or returns a non-200 status,
// the check is skipped with a warning rather than aborting the fetch.
func verifySHA512(filePath, sha512URL string) error {
	resp, err := http.Get(sha512URL) //nolint:noctx
	if err != nil {
		fmt.Printf("[fetch] Warning: SHA512 unavailable (%v), skipping check.\n", err)
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		fmt.Printf("[fetch] Warning: SHA512 file returned HTTP %d, skipping check.\n", resp.StatusCode)
		return nil
	}

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	// Format is "<hash>  <filename>" or just "<hash>"; take the first field.
	fields := strings.Fields(string(raw))
	if len(fields) == 0 {
		fmt.Println("[fetch] Warning: empty SHA512 file, skipping check.")
		return nil
	}
	expected := strings.ToLower(fields[0])

	// Sanity-check: SHA-512 hex digest is exactly 128 characters.
	if len(expected) != 128 {
		fmt.Printf("[fetch] Warning: unexpected SHA512 format (got %d chars, want 128), skipping check.\n", len(expected))
		return nil
	}

	f, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer f.Close()

	h := sha512.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	actual := fmt.Sprintf("%x", h.Sum(nil))

	if actual != expected {
		return fmt.Errorf("SHA512 mismatch:\n  expected: %s\n  actual:   %s", expected, actual)
	}
	fmt.Println("[fetch] SHA512 OK")
	return nil
}

// downloadFile fetches url and writes it to dest atomically (via .tmp rename).
func downloadFile(url, dest string) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	resp, err := http.Get(url) //nolint:gosec,noctx
	if err != nil {
		return fmt.Errorf("GET %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: HTTP %d", url, resp.StatusCode)
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
	fmt.Printf("[fetch] %s (%.1f MB)\n", filepath.Base(dest), float64(n)/(1<<20))
	return os.Rename(tmp, dest)
}

// copyFile copies src to dest, skipping if both files have the same size.
func copyFile(src, dest string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	si, _ := in.Stat()
	if di, err := os.Stat(dest); err == nil && di.Size() == si.Size() {
		return nil // idempotent
	}

	out, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, in)
	out.Close()
	return err
}

// runWithEnv runs a command with extra environment variables appended to os.Environ().
func runWithEnv(extra []string, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Env = append(os.Environ(), extra...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
	}
	return nil
}

// envOr returns the value of key or fallback if key is unset.
func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
