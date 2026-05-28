package setup

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
	embedassets "github.com/user/golovebox/internal/embed"

	"github.com/user/golovebox/internal/config"
	"github.com/user/golovebox/internal/sandbox"
)

func NewInitCmd() *cobra.Command {
	var repair bool
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize golovebox environment (extract QEMU, install Alpine VM, configure)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runInit(cmd.Context(), repair)
		},
	}
	cmd.Flags().BoolVar(&repair, "repair", false, "re-run only steps that have not completed")
	return cmd
}

func runInit(ctx context.Context, repair bool) error {
	bd, err := config.BaseDir()
	if err != nil {
		return err
	}
	qemuDir := filepath.Join(bd, "qemu")
	vmDir := filepath.Join(bd, "vm")

	if err := step1Dirs(bd); err != nil {
		return fmt.Errorf("step 1 (dirs): %w", err)
	}
	if err := step2ExtractQEMU(qemuDir); err != nil {
		return fmt.Errorf("step 2 (qemu): %w", err)
	}
	if err := step3ExtractAlpine(vmDir); err != nil {
		return fmt.Errorf("step 3 (alpine): %w", err)
	}
	pubKey, err := step4SSHKeypair(vmDir)
	if err != nil {
		return fmt.Errorf("step 4 (keygen): %w", err)
	}
	if err := step5CreateDisk(qemuDir, vmDir); err != nil {
		return fmt.Errorf("step 5 (disk): %w", err)
	}
	if err := step6CloudInit(vmDir, pubKey); err != nil {
		return fmt.Errorf("step 6 (cloud-init): %w", err)
	}
	if err := step7InstallBoot(ctx, qemuDir, vmDir); err != nil {
		return fmt.Errorf("step 7 (install): %w", err)
	}
	cfg, err := step8Config(bd, repair)
	if err != nil {
		return fmt.Errorf("step 8 (config): %w", err)
	}
	if err := step9SmokeTest(cfg); err != nil {
		return fmt.Errorf("step 9 (smoke test): %w", err)
	}
	return nil
}

func step1Dirs(bd string) error {
	dirs := []string{
		filepath.Join(bd, "qemu"),
		filepath.Join(bd, "vm"),
		filepath.Join(bd, "memory"),
		filepath.Join(bd, "logs"),
		filepath.Join(bd, "runs"),
		filepath.Join(bd, "skills"),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return err
		}
	}
	fmt.Println("[init] Directories created.")
	return nil
}

func step2ExtractQEMU(qemuDir string) error {
	fmt.Println("[init] Extracting QEMU binaries...")
	if err := embedassets.ExtractQEMU(qemuDir); err != nil {
		return err
	}
	fmt.Println("[init] QEMU ready.")
	return nil
}

func step3ExtractAlpine(vmDir string) error {
	fmt.Println("[init] Extracting Alpine ISO...")
	if err := embedassets.ExtractAlpineISO(vmDir); err != nil {
		return err
	}
	fmt.Println("[init] Alpine ISO ready.")
	return nil
}

func step4SSHKeypair(vmDir string) (string, error) {
	fmt.Println("[init] Generating SSH keypair...")
	pubKey, err := embedassets.GenerateSSHKeypair(vmDir)
	if err != nil {
		return "", err
	}
	fmt.Println("[init] SSH keypair ready.")
	return pubKey, nil
}

func step5CreateDisk(qemuDir, vmDir string) error {
	fmt.Println("[init] Creating VM disk image...")
	if err := embedassets.CreateDisk(qemuDir, vmDir); err != nil {
		return err
	}
	fmt.Println("[init] Disk image ready.")
	return nil
}

func step6CloudInit(vmDir, pubKey string) error {
	fmt.Println("[init] Building cloud-init CIDATA disk...")
	cidataPath := filepath.Join(vmDir, "cidata.iso")
	if err := embedassets.CreateCloudInitISO(cidataPath, pubKey); err != nil {
		return err
	}
	fmt.Println("[init] Cloud-init disk ready.")
	return nil
}

func step7InstallBoot(ctx context.Context, qemuDir, vmDir string) error {
	fmt.Println("[init] Running first-boot Alpine install (up to 10 min)...")
	if err := embedassets.RunInstallBoot(ctx, qemuDir, vmDir); err != nil {
		return err
	}
	fmt.Println("[init] Alpine installed.")
	return nil
}

func step8Config(bd string, repair bool) (*config.Config, error) {
	cf := filepath.Join(bd, "config.toml")
	if repair {
		if _, err := os.Stat(cf); err == nil {
			fmt.Println("[init] Config already exists, skipping.")
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

	qemuExe := "qemu-system-x86_64"
	if runtime.GOOS == "windows" {
		qemuExe = filepath.Join("bin", qemuExe+".exe")
	}
	cfg.QEMUPath = filepath.Join(bd, "qemu", qemuExe)

	if err := config.Save(cfg); err != nil {
		return nil, err
	}
	fmt.Printf("[init] Config saved to %s\n", cf)
	return cfg, nil
}

func step9SmokeTest(cfg *config.Config) error {
	fmt.Println("[init] Running smoke test...")
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
