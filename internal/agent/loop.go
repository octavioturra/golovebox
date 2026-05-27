package agent

import (
	"context"
	"fmt"
	"strings"

	"golang.org/x/crypto/ssh"

	"github.com/user/golovebox/internal/llm"
	"github.com/user/golovebox/internal/memory"
)

const maxIterations = 20

const systemPromptTemplate = `You are an autonomous coding agent. Solve the given task using the available tools.

%s
Respond ONLY in this exact format:
Thought: <your reasoning>
Action: <tool_name>
Parameters:
  <key>: <value>

To finish successfully:
Action: done
Parameters:
  result: <summary of what was accomplished, including PR URL if applicable>

To signal an unrecoverable error:
Action: error
Parameters:
  reason: <explanation>`

type Loop struct {
	llm      *llm.Client
	registry *Registry
	memory   *memory.Memory
	ssh      *ssh.Client
}

func New(llmClient *llm.Client, registry *Registry, mem *memory.Memory, sshClient *ssh.Client) *Loop {
	return &Loop{
		llm:      llmClient,
		registry: registry,
		memory:   mem,
		ssh:      sshClient,
	}
}

func (l *Loop) Run(ctx context.Context, task string) (string, error) {
	systemPrompt := fmt.Sprintf(systemPromptTemplate, l.registry.Descriptions())
	messages := []llm.Message{
		{Role: "user", Content: systemPrompt + "\n\nTask:\n" + task},
	}

	for i := range maxIterations {
		reply, err := l.llm.Complete(ctx, messages)
		if err != nil {
			return "", fmt.Errorf("llm complete (iter %d): %w", i, err)
		}

		_, action, params := parseAction(reply)

		switch action {
		case "done":
			return params["result"], nil
		case "error":
			return "", fmt.Errorf("agent: %s", params["reason"])
		case "":
			return "", fmt.Errorf("agent: no action found in reply: %.200s", reply)
		}

		tool, ok := l.registry.Get(action)
		if !ok {
			observation := fmt.Sprintf("Error: unknown tool %q. Use one of the available tools.", action)
			messages = appendObservation(messages, reply, observation)
			continue
		}

		observation, execErr := tool.Execute(ctx, params)
		if execErr != nil {
			if observation != "" {
				observation = fmt.Sprintf("Error: %v\nOutput: %s", execErr, observation)
			} else {
				observation = fmt.Sprintf("Error: %v", execErr)
			}
		}

		messages = appendObservation(messages, reply, observation)
	}

	return "", fmt.Errorf("agent: reached max iterations (%d) without completing task", maxIterations)
}

func appendObservation(messages []llm.Message, reply, observation string) []llm.Message {
	return append(messages,
		llm.Message{Role: "assistant", Content: reply},
		llm.Message{Role: "user", Content: "Observation: " + observation},
	)
}

// parseAction extracts thought, action name, and parameters from an LLM reply.
func parseAction(reply string) (thought, action string, params map[string]string) {
	params = make(map[string]string)
	inParams := false

	for _, line := range strings.Split(reply, "\n") {
		switch {
		case strings.HasPrefix(line, "Thought:"):
			thought = strings.TrimSpace(strings.TrimPrefix(line, "Thought:"))
			inParams = false
		case strings.HasPrefix(line, "Action:"):
			action = strings.TrimSpace(strings.TrimPrefix(line, "Action:"))
			inParams = false
		case strings.TrimSpace(line) == "Parameters:":
			inParams = true
		case inParams && strings.HasPrefix(line, "  "):
			parts := strings.SplitN(strings.TrimPrefix(line, "  "), ":", 2)
			if len(parts) == 2 {
				params[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
			}
		}
	}
	return
}
