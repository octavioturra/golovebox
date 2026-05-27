package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/user/golovebox/internal/llm"
	"github.com/user/golovebox/internal/memory"
)

// MaxIterations is the maximum number of ReAct loop iterations before giving up.
const MaxIterations = 20

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

// ProgressFunc is called after each tool execution to report loop progress.
// Nil is accepted — callers that don't need progress updates pass nil.
type ProgressFunc func(iteration int, action, observation string)

type Loop struct {
	llm      *llm.Client
	registry *Registry
	memory   *memory.Memory
}

func New(llmClient *llm.Client, registry *Registry, mem *memory.Memory) *Loop {
	return &Loop{
		llm:      llmClient,
		registry: registry,
		memory:   mem,
	}
}

// Run executes the ReAct loop for the given task.
// progress is called after each tool call with the iteration number, action name,
// and a truncated observation (max 120 chars). Pass nil to disable progress reporting.
func (l *Loop) Run(ctx context.Context, task string, progress ProgressFunc) (string, error) {
	systemPrompt := fmt.Sprintf(systemPromptTemplate, l.registry.Descriptions())
	messages := []llm.Message{
		{Role: "user", Content: systemPrompt + "\n\nTask:\n" + task},
	}

	for i := range MaxIterations {
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

		if progress != nil {
			obs := observation
			if len(obs) > 120 {
				obs = obs[:120] + "..."
			}
			progress(i+1, action, obs)
		}

		messages = appendObservation(messages, reply, observation)
	}

	return "", fmt.Errorf("agent: reached max iterations (%d) without completing task", MaxIterations)
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
