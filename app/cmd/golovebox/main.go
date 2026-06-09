package main

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/user/golovebox/app/internal/config"
	sandboxpkg "github.com/user/golovebox/sandbox"
	"github.com/user/golovebox/app/internal/setup"
	"github.com/user/golovebox/app/internal/web"
)

func main() {
	var logLevel, logFormat string
	root := &cobra.Command{
		Use:   "golovebox",
		Short: "Box (QEMU) + luva (Canvas + mediação)",
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			initLogger(logFormat, logLevel)
		},
	}
	root.PersistentFlags().StringVar(&logLevel, "log-level", "info", "Log level: debug, info, warn, error")
	root.PersistentFlags().StringVar(&logFormat, "log-format", "text", "Log format: text, json")
	root.AddCommand(setup.NewInitCmd())
	root.AddCommand(newResetCmd())
	root.AddCommand(newWebCmd())
	root.AddCommand(newExecCmd())
	if err := root.ExecuteContext(context.Background()); err != nil {
		os.Exit(1)
	}
}

func makeSandboxConfig(cfg *config.Config) (sandboxpkg.Config, error) {
	vmDir, err := cfg.VMDir()
	if err != nil {
		return sandboxpkg.Config{}, err
	}
	qemuDir, err := cfg.QEMUDir()
	if err != nil {
		return sandboxpkg.Config{}, err
	}
	return sandboxpkg.Config{
		QEMUExe: cfg.QEMUPath,
		QEMUDir: qemuDir,
		VMDir:   vmDir,
		SSHPort: cfg.SSHPort,
		QMPPort: cfg.QMPPort,
	}, nil
}

func newExecCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "exec <cmd>",
		Short: "Execute a shell command inside the VM via SSH",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			sbCfg, err := makeSandboxConfig(cfg)
			if err != nil {
				return err
			}
			run, close, err := sandboxpkg.DirectDial("127.0.0.1", sbCfg.SSHPort, "root",
				filepath.Join(sbCfg.VMDir, "id_rsa"))
			if err != nil {
				return err
			}
			defer close() //nolint:errcheck
			stdout, stderr, err := run(args[0])
			if stdout != "" {
				fmt.Print(stdout)
			}
			if stderr != "" {
				fmt.Fprint(os.Stderr, stderr)
			}
			return err
		},
	}
}

func newWebCmd() *cobra.Command {
	var addrFlag string
	var timeoutFlag int
	cmd := &cobra.Command{
		Use:   "web",
		Short: "Start the web shell (terminal SSH + file explorer SFTP)",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer cancel()

			cfg, err := config.Load()
			if err != nil {
				return fmt.Errorf("config: %w", err)
			}

			sbCfg, err := makeSandboxConfig(cfg)
			if err != nil {
				return err
			}

			if timeoutFlag > 0 {
				sandboxpkg.StartTimeout = time.Duration(timeoutFlag) * time.Second
			}

			fmt.Println("Starting VM...")
			vm, err := sandboxpkg.NewVM(sbCfg)
			if err != nil {
				return fmt.Errorf("start VM: %w", err)
			}
			defer vm.Stop() //nolint:errcheck

			srv := web.New(vm)

			url := "http://localhost" + addrFlag
			fmt.Printf("Web shell: %s\n", url)
			openBrowser(url)

			go func() {
				if err := srv.Start(ctx, addrFlag); err != nil {
					slog.Error("web server error", "error", err)
				}
			}()
			<-ctx.Done()
			fmt.Println("Web server stopped.")
			return nil
		},
	}
	cmd.Flags().StringVar(&addrFlag, "addr", ":8080", "HTTP listen address")
	cmd.Flags().IntVar(&timeoutFlag, "timeout", 0, "VM start timeout in seconds (default 15)")
	return cmd
}

func newResetCmd() *cobra.Command {
	var hard, yes bool
	cmd := &cobra.Command{
		Use:   "reset",
		Short: "Reset VM state (delete base.img + cidata so next init rebuilds the VM)",
		RunE: func(cmd *cobra.Command, args []string) error {
			bd, err := config.BaseDir()
			if err != nil {
				return err
			}
			if _, err := os.Stat(bd); os.IsNotExist(err) {
				fmt.Printf("Nothing to reset — %s does not exist.\n", bd)
				return nil
			}

			if hard {
				fmt.Printf("⚠  This will DELETE %s entirely:\n", bd)
				fmt.Println("    - config.toml (LLM provider, API keys, GitHub token)")
				fmt.Println("    - SSH keypair")
				fmt.Println("    - VM disk, memory, logs, runs, skills")
				if !yes && !confirm("Type 'yes' to confirm hard reset: ", "yes") {
					fmt.Println("Aborted.")
					return nil
				}
				fmt.Printf("Removing %s...\n", bd)
				if err := os.RemoveAll(bd); err != nil {
					return fmt.Errorf("hard reset: %w", err)
				}
				fmt.Println("Done. Run 'golovebox init' to start over.")
				return nil
			}

			fmt.Println("This will delete the VM disk and cloud-init data:")
			fmt.Println("    - vm/base.img            (the VM disk; ~164 MB, re-extracted from binary)")
			fmt.Println("    - vm/cidata.iso          (cloud-init seed)")
			fmt.Println("    - vm/.alpine-image-size  (idempotency marker)")
			fmt.Println("    - vm/qemu.log, install.log")
			fmt.Println("Config (config.toml) and SSH keys are PRESERVED.")
			if !yes && !confirm("Proceed? [y/N]: ", "y", "yes") {
				fmt.Println("Aborted.")
				return nil
			}

			vmDir := filepath.Join(bd, "vm")
			targets := []string{
				filepath.Join(vmDir, "base.img"),
				filepath.Join(vmDir, ".alpine-image-size"),
				filepath.Join(vmDir, "cidata.iso"),
				filepath.Join(vmDir, "qemu.log"),
				filepath.Join(vmDir, "install.log"),
			}
			for _, p := range targets {
				if err := os.Remove(p); err == nil {
					fmt.Printf("  removed %s\n", p)
				}
			}
			fmt.Println("VM reset. Run 'golovebox init' to rebuild base.img and re-run cloud-init.")
			return nil
		},
	}
	cmd.Flags().BoolVar(&hard, "hard", false, "remove the entire .golovebox/ directory (config, keys, everything)")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "skip confirmation prompt")
	return cmd
}

func initLogger(format, level string) {
	var lvl slog.Level
	switch strings.ToLower(level) {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}
	opts := &slog.HandlerOptions{Level: lvl}
	var h slog.Handler
	if strings.ToLower(format) == "json" {
		h = slog.NewJSONHandler(os.Stderr, opts)
	} else {
		h = slog.NewTextHandler(os.Stderr, opts)
	}
	slog.SetDefault(slog.New(h))
}

func openBrowser(url string) {
	var c *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		c = exec.Command("cmd", "/c", "start", url)
	case "darwin":
		c = exec.Command("open", url)
	default:
		c = exec.Command("xdg-open", url)
	}
	_ = c.Start()
}

func confirm(prompt string, accept ...string) bool {
	fmt.Print(prompt)
	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		return false
	}
	got := strings.ToLower(strings.TrimSpace(scanner.Text()))
	for _, a := range accept {
		if got == strings.ToLower(a) {
			return true
		}
	}
	return false
}
