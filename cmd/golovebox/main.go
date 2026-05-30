package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/user/golovebox/internal/config"
	"github.com/user/golovebox/internal/dag"
	"github.com/user/golovebox/internal/dsl"
	"github.com/user/golovebox/internal/gateway"
	"github.com/user/golovebox/internal/llm"
	"github.com/user/golovebox/internal/orchestrator"
	"github.com/user/golovebox/internal/sandbox"
	"github.com/user/golovebox/internal/setup"
	"github.com/user/golovebox/internal/skills"
	"github.com/user/golovebox/internal/web"
)

func main() {
	root := &cobra.Command{
		Use:   "golovebox",
		Short: "Autonomous Go agent with QEMU sandbox",
	}
	root.AddCommand(setup.NewInitCmd())
	root.AddCommand(newExecCmd())     // renamed from "run" — executes a raw shell cmd in VM
	root.AddCommand(newStatusCmd())
	root.AddCommand(newGitHubCmd())
	root.AddCommand(newDaemonCmd())
	root.AddCommand(newRunCmd())      // new: executes spec files through orchestrator
	root.AddCommand(newWebCmd())
	root.AddCommand(newSkillCmd())
	root.AddCommand(newResumeCmd())
	root.AddCommand(newResetCmd())
	if err := root.ExecuteContext(context.Background()); err != nil {
		os.Exit(1)
	}
}

// newExecCmd executes a raw shell command inside the VM.
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
		Use:   "status [run-id]",
		Short: "Show VM/config status, or details of a specific run",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return fmt.Errorf("config: %w", err)
			}

			if len(args) == 1 {
				return printRunStatus(cfg, args[0])
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

func printRunStatus(cfg *config.Config, runID string) error {
	runsDir, err := cfg.RunsDir()
	if err != nil {
		return err
	}
	dagPath := filepath.Join(runsDir, runID, "dag.json")
	data, err := os.ReadFile(dagPath)
	if err != nil {
		return fmt.Errorf("run %q not found: %w", runID, err)
	}
	d, err := dag.FromJSON(data)
	if err != nil {
		return err
	}
	fmt.Printf("Run: %s\n\n", runID)
	fmt.Printf("%-20s %-12s %-12s %s\n", "NODE", "TYPE", "STATE", "RESULT")
	for _, n := range d.Nodes {
		result := n.Result
		if n.Error != "" {
			result = "ERR: " + n.Error
		}
		if len(result) > 50 {
			result = result[:50] + "..."
		}
		fmt.Printf("%-20s %-12s %-12s %s\n", n.ID, n.Type, n.State, result)
	}
	return nil
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

// newRunCmd executes spec files through the orchestrator DAG.
func newRunCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "run <dir-or-file>",
		Short: "Execute spec files through the orchestrator DAG",
		Args:  cobra.ExactArgs(1),
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
			orch := orchestrator.New(llmClient, nil)

			// Parse specs.
			var specs []*dsl.ParsedSpec
			info, err := os.Stat(args[0])
			if err != nil {
				return fmt.Errorf("stat %s: %w", args[0], err)
			}
			if info.IsDir() {
				specs, err = dsl.ParseDir(args[0])
			} else {
				var s *dsl.ParsedSpec
				s, err = dsl.ParseFile(args[0])
				if err == nil {
					specs = []*dsl.ParsedSpec{s}
				}
			}
			if err != nil {
				return fmt.Errorf("parse: %w", err)
			}

			runsDir, err := cfg.RunsDir()
			if err != nil {
				return err
			}
			store, err := web.NewRunStore(runsDir)
			if err != nil {
				return err
			}
			runID, err := store.NewRun()
			if err != nil {
				return err
			}
			runDir := store.RunDir(runID)
			fmt.Printf("Run: %s\n", runID)

			d, err := orch.Plan(ctx, runID, specs, runDir)
			if err != nil {
				return fmt.Errorf("plan: %w", err)
			}

			cm := dag.NewCheckpointManager()
			scanner := bufio.NewScanner(os.Stdin)

			notify := func(node *dag.Node) {
				fmt.Printf("[%-14s] %s\n", node.State, node.ID)
			}
			dispatch := func(dCtx context.Context, node *dag.Node) (string, error) {
				return gw.RunSpecTask(dCtx, node.Task, func(iter int, action, obs string) {
					fmt.Printf("  [%d] %s: %s\n", iter, action, truncate(obs, 80))
				})
			}

			// Run checkpoints interactively from stdin.
			go func() {
				for {
					select {
					case <-ctx.Done():
						return
					default:
					}
					// Checkpoints are handled inline in the notify goroutine
					// by prompting on waiting_human nodes.
				}
			}()

			exec := dag.NewExecutor(d, dag.ExecutorConfig{}, dispatch, func(node *dag.Node) {
				notify(node)
				if node.State == dag.StateWaitingHuman {
					fmt.Printf("  ↳ Checkpoint: %s\n", node.Annotation)
					fmt.Print("  approve / reject: ")
					if scanner.Scan() {
						input := strings.TrimSpace(scanner.Text())
						if strings.HasPrefix(input, "reject") {
							reason := strings.TrimSpace(strings.TrimPrefix(input, "reject"))
							if reason == "" {
								reason = "rejeitado manualmente"
							}
							cm.Reject(node.ID, reason)
						} else {
							cm.Approve(node.ID)
						}
					}
				}
			}, cm, runDir)

			if err := exec.Run(ctx); err != nil {
				return fmt.Errorf("run: %w", err)
			}
			fmt.Printf("\n✓ Run %s completo. Relatório: %s\n", runID, filepath.Join(runDir, "run_summary.md"))
			return nil
		},
	}
}

// newWebCmd starts the web server and opens the browser.
func newWebCmd() *cobra.Command {
	var addrFlag string
	var timeoutFlag int
	cmd := &cobra.Command{
		Use:   "web",
		Short: "Start the web chat server and open browser",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer cancel()

			cfg, err := config.Load()
			if err != nil {
				return fmt.Errorf("config: %w", err)
			}

			if timeoutFlag > 0 {
				sandbox.StartTimeout = time.Duration(timeoutFlag) * time.Second
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
			orch := orchestrator.New(llmClient, nil)

			skillsDir, err := cfg.SkillsDir()
			if err != nil {
				return err
			}
			reg, err := skills.NewRegistry(skillsDir)
			if err != nil {
				return err
			}

			runsDir, err := cfg.RunsDir()
			if err != nil {
				return err
			}
			cm := dag.NewCheckpointManager()

			srv, err := web.New(gw, orch, cm, reg, llmClient, cfg, runsDir)
			if err != nil {
				return err
			}

			url := "http://localhost" + addrFlag
			fmt.Printf("Web server: %s\n", url)
			openBrowser(url)

			go func() {
				if err := srv.Start(ctx, addrFlag); err != nil {
					fmt.Fprintf(os.Stderr, "web: %v\n", err)
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

// newSkillCmd returns the parent "skill" command with sub-commands.
func newSkillCmd() *cobra.Command {
	parent := &cobra.Command{
		Use:   "skill",
		Short: "Manage reusable agent skills",
	}

	parent.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List available skills",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			skillsDir, err := cfg.SkillsDir()
			if err != nil {
				return err
			}
			reg, err := skills.NewRegistry(skillsDir)
			if err != nil {
				return err
			}
			list := reg.List()
			if len(list) == 0 {
				fmt.Println("No skills found. Use 'golovebox skill generate' to create one.")
				return nil
			}
			for _, s := range list {
				fmt.Printf("  %-20s %s\n", s.Name, s.Description)
			}
			return nil
		},
	})

	parent.AddCommand(&cobra.Command{
		Use:   "generate <description>",
		Short: "Generate a new skill from a plain-language description",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			skillsDir, err := cfg.SkillsDir()
			if err != nil {
				return err
			}
			llmClient := llm.New(llm.Config{
				BaseURL: cfg.LLMBaseURL,
				APIKey:  cfg.APIKey,
				Model:   cfg.LLMModel,
			})
			sk, err := skills.Generate(cmd.Context(), llmClient, args[0], skillsDir)
			if err != nil {
				return err
			}
			fmt.Printf("✓ Skill '%s' gerada em %s\n", sk.Name, sk.FilePath)
			return nil
		},
	})

	return parent
}

// newResumeCmd resumes an interrupted run by its ID.
func newResumeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "resume <run-id>",
		Short: "Resume an interrupted run",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer cancel()

			cfg, err := config.Load()
			if err != nil {
				return fmt.Errorf("config: %w", err)
			}

			runsDir, err := cfg.RunsDir()
			if err != nil {
				return err
			}
			store, err := web.NewRunStore(runsDir)
			if err != nil {
				return err
			}
			runID := args[0]
			d, err := store.LoadDAG(runID)
			if err != nil {
				return fmt.Errorf("load run %s: %w", runID, err)
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
			cm := dag.NewCheckpointManager()
			runDir := store.RunDir(runID)

			dispatch := func(dCtx context.Context, node *dag.Node) (string, error) {
				return gw.RunSpecTask(dCtx, node.Task, func(iter int, action, obs string) {
					fmt.Printf("  [%d] %s: %s\n", iter, action, truncate(obs, 80))
				})
			}
			notify := func(node *dag.Node) {
				fmt.Printf("[%-14s] %s\n", node.State, node.ID)
			}
			exec := dag.NewExecutor(d, dag.ExecutorConfig{}, dispatch, notify, cm, runDir)
			if err := exec.Resume(ctx); err != nil {
				return fmt.Errorf("resume: %w", err)
			}
			fmt.Printf("✓ Run %s completo.\n", runID)
			return nil
		},
	}
}

// newResetCmd wipes VM state so the next `init` re-extracts base.img and
// re-runs cloud-init from scratch. With --hard, removes the entire .golovebox/
// directory (including config.toml and SSH keys).
func newResetCmd() *cobra.Command {
	var hard bool
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
				fmt.Printf("Removing %s entirely...\n", bd)
				if err := os.RemoveAll(bd); err != nil {
					return fmt.Errorf("hard reset: %w", err)
				}
				fmt.Println("Done. Run 'golovebox init' to start over.")
				return nil
			}

			// Soft reset: drop everything that ties the VM to its prior first boot.
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
	return cmd
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
