package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/user/golovebox/internal/config"
	"github.com/user/golovebox/internal/gateway"
	"github.com/user/golovebox/internal/llm"
	"github.com/user/golovebox/internal/sandbox"
	"github.com/user/golovebox/internal/setup"
)

func main() {
	root := &cobra.Command{
		Use:   "golovebox",
		Short: "Autonomous Go agent with QEMU sandbox",
	}
	root.AddCommand(setup.NewInitCmd())
	root.AddCommand(newRunCmd())
	root.AddCommand(newStatusCmd())
	root.AddCommand(newGitHubCmd())
	root.AddCommand(newDaemonCmd())
	if err := root.ExecuteContext(context.Background()); err != nil {
		os.Exit(1)
	}
}

func newRunCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "run <cmd>",
		Short: "Execute a shell command inside the VM via SSH",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			vmDir, err := cfg.VMDir()
			if err != nil {
				return err
			}
			client, err := sandbox.Dial(
				"127.0.0.1",
				strconv.Itoa(cfg.SSHPort),
				"root",
				filepath.Join(vmDir, "id_rsa"),
			)
			if err != nil {
				return err
			}
			defer client.Close()
			stdout, stderr, err := sandbox.Exec(client, args[0])
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

func newStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show VM and configuration status",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return fmt.Errorf("config: %w", err)
			}
			fmt.Printf("LLM Provider : %s\n", cfg.LLMProvider)
			fmt.Printf("LLM Model    : %s\n", cfg.LLMModel)
			fmt.Printf("SSH Port     : %d\n", cfg.SSHPort)
			fmt.Printf("QMP Port     : %d\n", cfg.QMPPort)

			vmDir, err := cfg.VMDir()
			if err != nil {
				return err
			}
			sshClient, sshErr := sandbox.Dial(
				"127.0.0.1",
				strconv.Itoa(cfg.SSHPort),
				"root",
				filepath.Join(vmDir, "id_rsa"),
			)
			if sshErr != nil {
				fmt.Printf("VM SSH       : DOWN (%v)\n", sshErr)
			} else {
				sshClient.Close()
				fmt.Printf("VM SSH       : OK\n")
			}
			return nil
		},
	}
}

func newGitHubCmd() *cobra.Command {
	var repoFlag string
	var issueFlag int

	cmd := &cobra.Command{
		Use:   "github",
		Short: "Resolve a GitHub issue using the autonomous agent",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			cfg, err := config.Load()
			if err != nil {
				return fmt.Errorf("config: %w", err)
			}

			parts := strings.SplitN(repoFlag, "/", 2)
			if len(parts) != 2 {
				return fmt.Errorf("--repo must be in owner/repo format")
			}
			owner, repo := parts[0], parts[1]

			fmt.Println("Starting VM...")
			mgr, err := sandbox.Start(*cfg)
			if err != nil {
				return fmt.Errorf("start VM: %w", err)
			}
			defer mgr.Stop() //nolint:errcheck

			vmDir, err := cfg.VMDir()
			if err != nil {
				return err
			}
			pool := sandbox.NewPool(sandbox.PoolConfig{
				Host:    "127.0.0.1",
				Port:    cfg.SSHPort,
				User:    "root",
				KeyPath: filepath.Join(vmDir, "id_rsa"),
			})
			defer pool.Close()

			llmClient := llm.New(llm.Config{
				BaseURL: cfg.LLMBaseURL,
				APIKey:  cfg.APIKey,
				Model:   cfg.LLMModel,
			})

			gw := gateway.New(pool, llmClient, cfg)
			fmt.Printf("Resolving issue #%d in %s/%s...\n", issueFlag, owner, repo)
			result, err := gw.RunTask(ctx, owner, repo, issueFlag, nil)
			if err != nil {
				return fmt.Errorf("agent: %w", err)
			}
			fmt.Println("\n✓ Done:", result)
			return nil
		},
	}

	cmd.Flags().StringVar(&repoFlag, "repo", "", "GitHub repository in owner/repo format")
	cmd.Flags().IntVar(&issueFlag, "issue", 0, "Issue number to resolve")
	_ = cmd.MarkFlagRequired("repo")
	_ = cmd.MarkFlagRequired("issue")
	return cmd
}

func newDaemonCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "daemon",
		Short: "Run the gateway daemon (Telegram; Slack and Email are stubs)",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer cancel()

			cfg, err := config.Load()
			if err != nil {
				return fmt.Errorf("config: %w", err)
			}

			fmt.Println("Starting VM...")
			mgr, err := sandbox.Start(*cfg)
			if err != nil {
				return fmt.Errorf("start VM: %w", err)
			}
			defer mgr.Stop() //nolint:errcheck

			vmDir, err := cfg.VMDir()
			if err != nil {
				return err
			}
			pool := sandbox.NewPool(sandbox.PoolConfig{
				Host:    "127.0.0.1",
				Port:    cfg.SSHPort,
				User:    "root",
				KeyPath: filepath.Join(vmDir, "id_rsa"),
			})
			defer pool.Close()

			llmClient := llm.New(llm.Config{
				BaseURL: cfg.LLMBaseURL,
				APIKey:  cfg.APIKey,
				Model:   cfg.LLMModel,
			})

			gw := gateway.New(pool, llmClient, cfg)

			if cfg.TelegramToken != "" {
				th, err := gateway.NewTelegramHandler(cfg.TelegramToken, gw)
				if err != nil {
					return fmt.Errorf("telegram: %w", err)
				}
				gw.RegisterHandler(th)
				fmt.Println("Telegram gateway registered.")
			} else {
				fmt.Println("No TelegramToken in config — daemon idle (no handlers active).")
			}

			fmt.Println("Daemon running. Press Ctrl+C to stop.")
			if err := gw.Run(ctx); err != nil {
				return fmt.Errorf("gateway: %w", err)
			}

			fmt.Println("Daemon stopped.")
			return nil
		},
	}
}
