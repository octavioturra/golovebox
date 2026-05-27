package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/user/golovebox/internal/agent"
	"github.com/user/golovebox/internal/config"
	"github.com/user/golovebox/internal/llm"
	"github.com/user/golovebox/internal/memory"
	"github.com/user/golovebox/internal/sandbox"
	"github.com/user/golovebox/internal/setup"
	"github.com/user/golovebox/internal/tools"
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
			sshClient, err := sandbox.Dial(
				"127.0.0.1",
				strconv.Itoa(cfg.SSHPort),
				"root",
				filepath.Join(vmDir, "id_rsa"),
			)
			if err != nil {
				return fmt.Errorf("ssh: %w", err)
			}
			defer sshClient.Close()

			fmt.Printf("Fetching issue #%d from %s/%s...\n", issueFlag, owner, repo)
			issue, err := tools.GetIssue(ctx, cfg.GitHubToken, owner, repo, issueFlag)
			if err != nil {
				return fmt.Errorf("get issue: %w", err)
			}
			fmt.Printf("Issue: %s\n", issue.GetTitle())

			destPath := "/root/" + repo
			fmt.Printf("Cloning %s/%s into VM...\n", owner, repo)
			if err := tools.CloneRepo(sshClient, cfg.GitHubToken, owner, repo, destPath); err != nil {
				return fmt.Errorf("clone repo: %w", err)
			}

			memDir, err := cfg.MemoryDir()
			if err != nil {
				return err
			}
			embFn := memory.NewEmbedFnFromConfig(cfg.LLMProvider, cfg.LLMBaseURL, cfg.APIKey)
			mem, err := memory.New(memDir, embFn)
			if err != nil {
				return fmt.Errorf("memory: %w", err)
			}
			if readme, readErr := tools.ReadFile(sshClient, destPath+"/README.md"); readErr == nil {
				_ = mem.Index(ctx, "readme", string(readme))
			}

			llmClient := llm.New(llm.Config{
				BaseURL: cfg.LLMBaseURL,
				APIKey:  cfg.APIKey,
				Model:   cfg.LLMModel,
			})

			registry := buildRegistry(cfg, owner, repo, mem)

			fmt.Println("Planning task...")
			task, err := agent.PlanFromIssue(ctx, llmClient, issue, owner, repo)
			if err != nil {
				return fmt.Errorf("plan: %w", err)
			}

			fmt.Println("Running agent loop...")
			loop := agent.New(llmClient, registry, mem, sshClient)
			result, err := loop.Run(ctx, task)
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

// buildRegistry wires all tools with their runtime dependencies via closures.
// Each tool opens its own SSH connection so they remain independent across loop iterations.
func buildRegistry(cfg *config.Config, owner, repo string, mem *memory.Memory) *agent.Registry {
	r := agent.NewRegistry()

	r.Register(agent.Tool{
		Name:        "shell",
		Description: "Execute a shell command in the VM",
		Parameters:  map[string]string{"cmd": "shell command to execute"},
		Execute: func(fCtx context.Context, params map[string]string) (string, error) {
			vmDir, _ := cfg.VMDir()
			sc, err := sandbox.Dial("127.0.0.1", strconv.Itoa(cfg.SSHPort),
				"root", filepath.Join(vmDir, "id_rsa"))
			if err != nil {
				return "", err
			}
			defer sc.Close()
			return tools.Shell(sc, params["cmd"]), nil
		},
	})

	r.Register(agent.Tool{
		Name:        "read_file",
		Description: "Read a file from the VM",
		Parameters:  map[string]string{"path": "absolute path to the file"},
		Execute: func(fCtx context.Context, params map[string]string) (string, error) {
			vmDir, _ := cfg.VMDir()
			sc, err := sandbox.Dial("127.0.0.1", strconv.Itoa(cfg.SSHPort),
				"root", filepath.Join(vmDir, "id_rsa"))
			if err != nil {
				return "", err
			}
			defer sc.Close()
			data, err := tools.ReadFile(sc, params["path"])
			return string(data), err
		},
	})

	r.Register(agent.Tool{
		Name:        "write_file",
		Description: "Write content to a file in the VM",
		Parameters: map[string]string{
			"path":    "absolute path to the file",
			"content": "file content",
		},
		Execute: func(fCtx context.Context, params map[string]string) (string, error) {
			vmDir, _ := cfg.VMDir()
			sc, err := sandbox.Dial("127.0.0.1", strconv.Itoa(cfg.SSHPort),
				"root", filepath.Join(vmDir, "id_rsa"))
			if err != nil {
				return "", err
			}
			defer sc.Close()
			if err := tools.WriteFile(sc, params["path"], []byte(params["content"])); err != nil {
				return "", err
			}
			return "file written", nil
		},
	})

	r.Register(agent.Tool{
		Name:        "list_dir",
		Description: "List files in a directory in the VM",
		Parameters:  map[string]string{"path": "absolute path to the directory"},
		Execute: func(fCtx context.Context, params map[string]string) (string, error) {
			vmDir, _ := cfg.VMDir()
			sc, err := sandbox.Dial("127.0.0.1", strconv.Itoa(cfg.SSHPort),
				"root", filepath.Join(vmDir, "id_rsa"))
			if err != nil {
				return "", err
			}
			defer sc.Close()
			entries, err := tools.ListDir(sc, params["path"])
			return strings.Join(entries, "\n"), err
		},
	})

	r.Register(agent.Tool{
		Name:        "search_memory",
		Description: "Search the memory store for relevant context",
		Parameters:  map[string]string{"query": "search query text"},
		Execute: func(fCtx context.Context, params map[string]string) (string, error) {
			results, err := mem.Search(fCtx, params["query"], 3)
			return strings.Join(results, "\n---\n"), err
		},
	})

	r.Register(agent.Tool{
		Name:        "github_list_issues",
		Description: "List open issues in the GitHub repository",
		Parameters:  map[string]string{},
		Execute: func(fCtx context.Context, params map[string]string) (string, error) {
			issues, err := tools.ListIssues(fCtx, cfg.GitHubToken, owner, repo)
			if err != nil {
				return "", err
			}
			var sb strings.Builder
			for _, iss := range issues {
				sb.WriteString(fmt.Sprintf("#%d: %s\n", iss.GetNumber(), iss.GetTitle()))
			}
			return sb.String(), nil
		},
	})

	r.Register(agent.Tool{
		Name:        "github_get_issue",
		Description: "Get a specific GitHub issue by number",
		Parameters:  map[string]string{"number": "issue number"},
		Execute: func(fCtx context.Context, params map[string]string) (string, error) {
			num, err := strconv.Atoi(params["number"])
			if err != nil {
				return "", fmt.Errorf("invalid issue number: %s", params["number"])
			}
			iss, err := tools.GetIssue(fCtx, cfg.GitHubToken, owner, repo, num)
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("Title: %s\nBody:\n%s", iss.GetTitle(), iss.GetBody()), nil
		},
	})

	r.Register(agent.Tool{
		Name:        "github_open_pr",
		Description: "Open a Pull Request on GitHub",
		Parameters: map[string]string{
			"head":  "source branch name",
			"base":  "target branch (usually main)",
			"title": "PR title",
			"body":  "PR description",
		},
		Execute: func(fCtx context.Context, params map[string]string) (string, error) {
			base := params["base"]
			if base == "" {
				base = "main"
			}
			return tools.OpenPR(fCtx, cfg.GitHubToken, owner, repo,
				params["head"], base, params["title"], params["body"])
		},
	})

	return r
}
