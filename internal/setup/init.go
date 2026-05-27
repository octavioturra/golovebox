package setup

import (
	"archive/zip"
	"bufio"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/user/golovebox/internal/config"
	"github.com/user/golovebox/internal/sandbox"
)

// qemuZipURL is a placeholder — weilnetz.de distributes an installer (.exe), not a zip.
// Replace with a direct zip URL when one becomes available, or install QEMU manually.
const qemuZipURL = "https://qemu.weilnetz.de/w64/2024/qemu-w64-setup-20240423.exe"

// alpineImageURL is a placeholder for the custom Alpine+toolchain qcow2 image.
const alpineImageURL = "https://example.com/alpine-golovebox.qcow2"

func NewInitCmd() *cobra.Command {
	var repair bool
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize golovebox environment (download QEMU, VM image, configure)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runInit(repair)
		},
	}
	cmd.Flags().BoolVar(&repair, "repair", false, "skip steps already completed")
	return cmd
}

func runInit(repair bool) error {
	bd, err := config.BaseDir()
	if err != nil {
		return err
	}

	if err := step1Dirs(bd, repair); err != nil {
		return fmt.Errorf("step 1 (dirs): %w", err)
	}
	if err := step2QEMU(bd, repair); err != nil {
		return fmt.Errorf("step 2 (qemu): %w", err)
	}
	if err := step3AlpineImage(bd, repair); err != nil {
		return fmt.Errorf("step 3 (vm image): %w", err)
	}
	cfg, err := step4Config(bd, repair)
	if err != nil {
		return fmt.Errorf("step 4 (config): %w", err)
	}
	if err := step5SmokeTest(cfg); err != nil {
		return fmt.Errorf("step 5 (smoke test): %w", err)
	}
	return nil
}

func step1Dirs(bd string, repair bool) error {
	dirs := []string{
		filepath.Join(bd, "qemu"),
		filepath.Join(bd, "vm"),
		filepath.Join(bd, "memory"),
		filepath.Join(bd, "logs"),
	}
	for _, d := range dirs {
		if repair {
			if _, err := os.Stat(d); err == nil {
				continue
			}
		}
		fmt.Printf("Creating %s\n", d)
		if err := os.MkdirAll(d, 0o755); err != nil {
			return err
		}
	}
	return nil
}

func step2QEMU(bd string, repair bool) error {
	marker := filepath.Join(bd, "qemu", "qemu-system-x86_64.exe")
	if repair {
		if _, err := os.Stat(marker); err == nil {
			fmt.Println("[step 2] QEMU already installed, skipping.")
			return nil
		}
	}
	// TODO: replace qemuZipURL with a direct .zip download when available.
	// For now, extract manually or provide a pre-built qemu-system-x86_64.exe.
	fmt.Printf("[step 2] TODO: download QEMU from %s\n", qemuZipURL)
	fmt.Printf("[step 2] Place qemu-system-x86_64.exe in: %s\n", filepath.Join(bd, "qemu"))
	return nil
}

func step3AlpineImage(bd string, repair bool) error {
	dest := filepath.Join(bd, "vm", "base.img")
	if repair {
		if _, err := os.Stat(dest); err == nil {
			fmt.Println("[step 3] VM image already exists, skipping.")
			return nil
		}
	}
	if strings.HasPrefix(alpineImageURL, "https://example.com") {
		fmt.Printf("[step 3] TODO: set a real Alpine image URL (placeholder: %s)\n", alpineImageURL)
		return nil
	}
	fmt.Printf("[step 3] Downloading VM image to %s ...\n", dest)
	return downloadFile(alpineImageURL, dest)
}

func step4Config(bd string, repair bool) (*config.Config, error) {
	cf := filepath.Join(bd, "config.toml")
	if repair {
		if _, err := os.Stat(cf); err == nil {
			fmt.Println("[step 4] Config already exists, skipping.")
			return config.Load()
		}
	}

	scanner := bufio.NewScanner(os.Stdin)
	ask := func(label string) string {
		fmt.Printf("%s: ", label)
		scanner.Scan()
		return strings.TrimSpace(scanner.Text())
	}

	cfg := &config.Config{
		SSHPort: 2222,
		QMPPort: 4444,
	}
	cfg.LLMProvider = ask("LLM Provider (anthropic, openai, ollama, gemini)")
	defaultURL := config.DefaultBaseURL(cfg.LLMProvider)
	cfg.LLMBaseURL = ask(fmt.Sprintf("LLM Base URL (Enter for default: %s)", defaultURL))
	if cfg.LLMBaseURL == "" {
		cfg.LLMBaseURL = defaultURL
	}
	cfg.LLMModel = ask("LLM Model (e.g. claude-opus-4-5, gpt-4o, llama3)")
	cfg.APIKey = ask("API Key")
	cfg.GitHubToken = ask("GitHub Token")
	cfg.TelegramToken = ask("Telegram Token (optional, Enter to skip)")
	cfg.DefaultRepo = ask("Default GitHub repo (owner/repo, optional, Enter to skip)")
	cfg.QEMUPath = filepath.Join(bd, "qemu", "qemu-system-x86_64.exe")

	if err := config.Save(cfg); err != nil {
		return nil, err
	}
	fmt.Printf("[step 4] Config saved to %s\n", cf)
	return cfg, nil
}

func step5SmokeTest(cfg *config.Config) error {
	fmt.Println("[step 5] Starting smoke test...")
	vmDir, err := cfg.VMDir()
	if err != nil {
		return err
	}
	sshKeyPath := filepath.Join(vmDir, "id_rsa")

	mgr, err := sandbox.Start(*cfg)
	if err != nil {
		return fmt.Errorf("start VM: %w", err)
	}
	defer mgr.Stop() //nolint:errcheck

	client, err := sandbox.Dial("127.0.0.1", fmt.Sprintf("%d", cfg.SSHPort), "root", sshKeyPath)
	if err != nil {
		return fmt.Errorf("ssh dial: %w", err)
	}
	defer client.Close()

	stdout, _, err := sandbox.Exec(client, "echo ok")
	if err != nil {
		return fmt.Errorf("echo test: %w", err)
	}
	if strings.TrimSpace(stdout) != "ok" {
		return fmt.Errorf("unexpected echo response: %q", stdout)
	}
	fmt.Println("golovebox pronto ✓")
	return nil
}

func downloadFile(url, dest string) error {
	resp, err := http.Get(url) //nolint:gosec,noctx
	if err != nil {
		return fmt.Errorf("download %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s: HTTP %d", url, resp.StatusCode)
	}
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, resp.Body)
	return err
}

// extractZip extracts a zip archive to destDir with zip-slip protection.
func extractZip(zipPath, destDir string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()

	base := filepath.Clean(destDir) + string(os.PathSeparator)
	for _, f := range r.File {
		target := filepath.Join(destDir, filepath.FromSlash(f.Name))
		if !strings.HasPrefix(target, base) {
			return fmt.Errorf("zip slip: illegal path %s", f.Name)
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := extractZipEntry(f, target); err != nil {
			return err
		}
	}
	return nil
}

func extractZipEntry(f *zip.File, dest string) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, rc) //nolint:gosec
	return err
}
