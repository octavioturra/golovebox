# golovebox

Autonomous Go coding agent. Receives a GitHub issue via Telegram (or CLI), clones the repository into an isolated QEMU VM, implements the fix using a ReAct loop driven by any OpenAI-compatible LLM, and opens a Pull Request.

Single Windows binary (~13 MB). No installer. No runtime dependencies. `CGO_ENABLED=0`.

---

## How it works

```
Telegram / CLI
      │
      ▼
 Gateway (daemon)
      │  issue #42 in owner/repo
      ▼
 GetIssue → CloneRepo (GIT_ASKPASS) → index README
      │
      ▼
 PlanFromIssue (LLM single call → task description)
      │
      ▼
 ReAct Loop (max 20 iterations)
   Thought → Action → tool.Execute → Observation → repeat
      │
      ▼  Action: done
 github_open_pr → PR URL → Telegram reply
```

**VM isolation**: every code execution (`shell`, `read_file`, `write_file`) runs inside an Alpine Linux QEMU VM via SSH/SFTP. The host machine is never touched.

**LLM agnostic**: works with Anthropic, OpenAI, Ollama, Gemini — just change `llm_base_url` and `llm_model` in the config.

---

## Quick start

### 1. Prerequisites

| What | Where |
|---|---|
| QEMU for Windows | Place `qemu-system-x86_64.exe` in `.golovebox/qemu/` |
| Alpine VM image | Place `base.img` (qcow2) in `.golovebox/vm/` |
| SSH key pair | Place `id_rsa` + `id_rsa.pub` in `.golovebox/vm/` |

### 2. Initialize

```
golovebox init
```

Interactive wizard creates `.golovebox/` next to the binary, prompts for all API keys, and runs a smoke test (`echo ok` via SSH into the VM).

With `--repair`, skips steps whose artifacts already exist:

```
golovebox init --repair
```

### 3. Run the agent from the CLI

```
golovebox github --repo owner/repo --issue 42
```

Boots the VM, fetches the issue, clones the repo, plans and runs the agent loop, opens a PR, prints the URL.

### 4. Run as a Telegram bot

```
golovebox daemon
```

Starts the VM and listens for Telegram messages. Requires `telegram_token` in config.

**Telegram commands:**

| Message | Effect |
|---|---|
| `issue #42 repo owner/repo` | Resolve issue #42 in the specified repo |
| `issue #42` | Resolve issue #42 in `default_repo` from config |
| `status` | VM health check |
| `help` | List available commands |

**Progress updates** arrive as the agent works:

```
⚙️ Processando issue #42 em owner/repo...
🔄 [1/20] shell: git log --oneline -5
🔄 [3/20] shell: go test ./...
🔄 [7/20] github_open_pr: PR criada
✅ PR aberta: https://github.com/owner/repo/pull/7
```

---

## Configuration

Config lives at `.golovebox/config.toml` (next to the binary):

```toml
llm_provider   = "anthropic"           # anthropic | openai | ollama | gemini
llm_base_url   = "https://api.anthropic.com"
llm_model      = "claude-opus-4-5"
api_key        = "sk-ant-..."
github_token   = "ghp_..."
telegram_token = "123456:ABC-..."      # optional; required for daemon
default_repo   = "owner/repo"          # used when Telegram message omits repo
ssh_port       = 2222
qmp_port       = 4444
qemu_path      = ".golovebox/qemu/qemu-system-x86_64.exe"
```

### Supported LLM providers

| `llm_provider` | `llm_base_url` | Notes |
|---|---|---|
| `anthropic` | `https://api.anthropic.com` | Adds `anthropic-version` header automatically |
| `openai` | `https://api.openai.com` | Standard Bearer auth |
| `ollama` | `http://localhost:11434` | Local model, no API key needed |
| `gemini` | `https://generativelanguage.googleapis.com` | OpenAI-compat endpoint |

---

## Available tools (ReAct loop)

| Tool | Parameters | Description |
|---|---|---|
| `shell` | `cmd` | Execute a shell command in the VM |
| `read_file` | `path` | Read a file from the VM |
| `write_file` | `path`, `content` | Write a file in the VM |
| `list_dir` | `path` | List directory contents in the VM |
| `search_memory` | `query` | Semantic search over indexed context |
| `github_list_issues` | — | List open issues in the target repo |
| `github_get_issue` | `number` | Get issue title and body |
| `github_open_pr` | `head`, `base`, `title`, `body` | Open a Pull Request |

---

## CLI reference

```
golovebox init [--repair]           Initialize environment
golovebox status                    Show config + VM SSH status
golovebox run "<cmd>"               Execute a shell command in the VM
golovebox github --repo o/r --issue N   Resolve an issue (one-shot)
golovebox daemon                    Run gateway daemon (Telegram)
```

---

## Building from source

```bash
git clone https://github.com/user/golovebox
cd golovebox
go mod tidy
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build ./cmd/golovebox
# → golovebox.exe (~13 MB)
```

Requirements: Go 1.22+. No C toolchain needed.

---

## Security notes

- **Token isolation**: `CloneRepo` uses `GIT_ASKPASS` — the GitHub token never appears in `git log --remotes` or `ps aux`.
- **VM sandbox**: all code execution is isolated inside the QEMU VM. The VM image is the only thing that changes.
- **SSH key auth**: host↔VM communication uses RSA key authentication; no passwords.
- **LLM retry**: transient failures (429, 5xx, network errors) are retried with exponential backoff (2 s base, 30 s cap, 3 attempts).

---

## Project structure

```
golovebox/
├── cmd/golovebox/main.go       CLI entry point (cobra commands)
├── internal/
│   ├── agent/                  ReAct loop, tool registry, planner
│   ├── config/                 Portable config (paths relative to executable)
│   ├── gateway/                Handler interface, Gateway, TelegramHandler
│   ├── llm/                    HTTP client (OpenAI-compat + Anthropic)
│   ├── memory/                 chromem-go vector store + embedding helpers
│   ├── sandbox/                QEMU manager, SSH helpers, SSH pool
│   ├── setup/                  Init wizard (5 steps)
│   └── tools/                  Shell, file, GitHub tools
└── .golovebox/                 Runtime data (next to the binary)
    ├── config.toml
    ├── qemu/
    ├── vm/
    ├── memory/
    └── logs/
```

---

## Phase roadmap

| Phase | Status | Summary |
|---|---|---|
| 1 | ✅ | Scaffold — CLI, config, QEMU sandbox, SSH/SFTP, init wizard |
| 2 | ✅ | Agent loop — custom LLM client, ReAct loop, GitHub tools, memory |
| 3 | ✅ | Gateway — Telegram bot, SSH pool, GIT_ASKPASS, LLM retry, daemon |
| 4 | planned | Slack gateway, streaming, Windows signing, auto-update |
