package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/spf13/cobra"
	"github.com/user/golovebox/internal/agent"
	"github.com/user/golovebox/internal/config"
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
	root.AddCommand(newAgentCmd())
	if err := root.Execute(); err != nil {
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

func newAgentCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "agent",
		Short: "Run autonomous agent (Phase 2)",
		Run: func(cmd *cobra.Command, args []string) {
			_ = agent.New()
			fmt.Println("Agent mode coming in Phase 2")
		},
	}
}
