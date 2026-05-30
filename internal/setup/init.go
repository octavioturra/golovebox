package setup

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"

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
	// Run interactive config NOW — before any heavy work. This way the user
	// never loses prior input if a later step fails, and config.toml is on
	// disk as early as possible.
	cfg, err := step2Config(bd, repair)
	if err != nil {
		return fmt.Errorf("step 2 (config): %w", err)
	}
	if err := step3ExtractQEMU(qemuDir); err != nil {
		return fmt.Errorf("step 3 (qemu): %w", err)
	}
	if err := step4ExtractAlpine(vmDir); err != nil {
		return fmt.Errorf("step 4 (alpine image): %w", err)
	}
	pubKey, err := step5SSHKeypair(vmDir)
	if err != nil {
		return fmt.Errorf("step 5 (keygen): %w", err)
	}
	if err := step6CloudInit(vmDir, pubKey); err != nil {
		return fmt.Errorf("step 6 (cloud-init): %w", err)
	}
	if err := step7SmokeTest(ctx, cfg); err != nil {
		return fmt.Errorf("step 7 (smoke test): %w", err)
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

func step3ExtractQEMU(qemuDir string) error {
	fmt.Println("[init] Extracting QEMU binaries...")
	if err := embedassets.ExtractQEMU(qemuDir); err != nil {
		return err
	}
	fmt.Println("[init] QEMU ready.")
	return nil
}

func step4ExtractAlpine(vmDir string) error {
	fmt.Println("[init] Extracting Alpine cloud image → base.img...")
	if err := embedassets.ExtractAlpineImage(vmDir); err != nil {
		return err
	}
	fmt.Println("[init] Alpine base.img ready.")
	return nil
}

func step5SSHKeypair(vmDir string) (string, error) {
	fmt.Println("[init] Generating SSH keypair...")
	pubKey, err := embedassets.GenerateSSHKeypair(vmDir)
	if err != nil {
		return "", err
	}
	fmt.Println("[init] SSH keypair ready.")
	return pubKey, nil
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

func step2Config(bd string, repair bool) (*config.Config, error) {
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
		qemuExe += ".exe"
	}
	cfg.QEMUPath = filepath.Join(bd, "qemu", qemuExe)

	if err := config.Save(cfg); err != nil {
		return nil, err
	}
	fmt.Printf("[init] Config saved to %s\n", cf)
	return cfg, nil
}

func step7SmokeTest(ctx context.Context, cfg *config.Config) error {
	fmt.Println("[init] Booting VM (cloud-init runs on first boot — may take 2-4 min)...")
	vmDir, err := cfg.VMDir()
	if err != nil {
		return err
	}
	sshKeyPath := filepath.Join(vmDir, "id_rsa")

	// Cloud-init takes longer than QMP — give QEMU plenty of time to bind QMP.
	sandbox.StartTimeout = 60 * time.Second
	mgr, err := sandbox.Start(*cfg)
	if err != nil {
		return fmt.Errorf("start VM: %w", err)
	}
	defer mgr.Stop() //nolint:errcheck

	// Poll SSH for up to 5 minutes — cloud-init needs to install packages
	// and start sshd on the first boot.
	deadline := time.Now().Add(5 * time.Minute)
	var client *ssh.Client
	var lastErr error
	for time.Now().Before(deadline) {
		client, lastErr = sandbox.Dial("127.0.0.1", fmt.Sprintf("%d", cfg.SSHPort), "root", sshKeyPath)
		if lastErr == nil {
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(5 * time.Second):
		}
		fmt.Print(".")
	}
	fmt.Println()
	if client == nil {
		return fmt.Errorf("ssh dial timed out after 5 min: %w", lastErr)
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
