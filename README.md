# golovebox

Agentic workflow platform with a self-hosted web UI and VM sandbox. Describe work in plain-language Markdown specs, get a visual DAG execution plan, approve checkpoints in the browser, and receive Pull Requests — all from a single 14 MB Windows binary with zero installation.

```
golovebox web   →   opens localhost:8080
```

---

## How it works

```
Spec files (.md)
      │  uploaded via web chat or CLI
      ▼
 Orchestrator  (LLM → JSON plan)
      │
      ▼
 DAG Executor  (parallel nodes, up to 3 simultaneous)
   ┌─────────────────────────────────┐
   │  task node    → ReAct loop      │
   │  checkpoint   → wait for human  │  ← orange node in canvas
   │  gate         → run tests in VM │
   │  notify       → send alert      │
   └─────────────────────────────────┘
      │
      ▼
 run_summary.md  +  PR URLs  +  artifacts
```

Every `shell`, `read_file`, `write_file` call runs inside an **Alpine Linux QEMU VM** via SSH/SFTP. The host machine is never touched.

---

## Prerequisites

### QEMU

Download the Windows installer from **https://www.qemu.org/download/#windows** or the nightly builds from **https://qemu.weilnetz.de/w64/**.

After installing, copy `qemu-system-x86_64.exe` to `.golovebox/qemu/`:

```
.golovebox/qemu/qemu-system-x86_64.exe
```

### Alpine VM image

You need a qcow2 disk image with Alpine Linux, SSH server enabled, and root login via key. Place it at:

```
.golovebox/vm/base.img
```

Build one with:

```bash
# On any Linux machine with qemu-img + virt-customize:
qemu-img create -f qcow2 base.img 4G
# ... install Alpine, enable sshd, authorise id_rsa.pub for root
```

Or use any pre-built Alpine qcow2 that allows root SSH login.

### SSH key pair

Generate a key pair for host↔VM communication (one-liner, no passphrase):

```bash
ssh-keygen -t rsa -b 4096 -f .golovebox/vm/id_rsa -N "" -C "golovebox-vm"
```

Copy `.golovebox/vm/id_rsa.pub` into the VM's `/root/.ssh/authorized_keys`.

---

## Quick start

### 1. Initialize

```
golovebox init
```

Creates `.golovebox/` next to the binary and runs an interactive wizard for all API keys. Ends with a smoke test (`echo ok` via SSH into the VM).

```
golovebox init --repair   # skips steps whose artifacts already exist
```

### 2. Open the web UI

```
golovebox web
```

Boots the VM, starts a server on `localhost:8080`, and opens the browser automatically.

```
golovebox web --addr :9090   # custom port
```

The UI has two panels:
- **Left — chat**: upload `.md` spec files and watch live execution logs
- **Right — DAG canvas**: nodes coloured by state; click orange checkpoints to approve or reject

### 3. Write a spec file

Create a Markdown file describing the work. Use keywords to annotate special behaviour:

```markdown
# Feature: user authentication

Implement JWT-based login for the /api/auth endpoint.

ATTENTION_HERE: review the token expiry strategy before continuing
RUN_TEST: go test ./internal/auth/...

Add refresh token support.

NOTIFY_ME: send summary when done

NOT_TODO: OAuth2 social login (out of scope for this sprint)
```

| Keyword | Effect |
|---|---|
| `ATTENTION_HERE:` | Pauses execution — orange node in canvas, click to approve |
| `PAUSE_TO_REVIEW:` | Same as ATTENTION_HERE |
| `RUN_TEST:` | Creates a gate node that runs tests in the VM |
| `NOTIFY_ME:` | Creates a notify node |
| `NOT_TODO:` | Logged to `tech_debt.md`, excluded from DAG |
| `TRY ... OR_ELSE ...` | Creates a try/fallback node |
| `WHEN ... DO ...` | Creates a wait-for-event node |

### 4. Run specs from the CLI

```bash
golovebox run ./specs/
# or a single file:
golovebox run auth-feature.md
```

Parses the specs, asks the LLM for an execution plan, and runs the DAG with terminal progress. Checkpoints prompt for `approve` / `reject` on stdin.

### 5. Manage skills

Skills are reusable agent behaviour templates stored as `.md` files in `.golovebox/skills/`.

**List available skills:**
```
golovebox skill list
```

**Generate a skill from a description:**
```
golovebox skill generate "review a Go REST API for naming conventions and security"
```

This calls the LLM, produces a structured `.md` skill file, and saves it to `.golovebox/skills/`. The orchestrator automatically includes skill descriptions in its planning context.

**Skill file format** (TOML frontmatter):
```markdown
---
name = "go_api_review"
description = "Reviews a Go REST API for conventions and security"
tools = ["shell", "read_file", "search_memory"]
examples = ["Review the endpoints in /internal/api"]
---

Analyse each route for:
- Consistent naming (plural nouns, versioned paths)
- Correct HTTP status codes per operation
- Authentication on protected routes
- Missing input validation
```

### 6. Resume an interrupted run

```
golovebox resume run-20250601-a3f9c2
```

Resets any `running` nodes back to `pending` and re-executes from the current state.

---

## Web UI walkthrough

```
┌──────────────────────┬──────────────────────────────────────────┐
│  chat panel          │  DAG canvas                              │
│                      │                                          │
│  Upload spec files   │   [parse-specs]──▶[impl-auth]           │
│  ─────────────────   │        ↓                                 │
│  [run-20250601-a3f9] │   [ATTENTION_HERE]  ◀── orange, click   │
│  [run-20250528-c12a] │        ↓                                 │
│                      │   [run-tests]──▶[open-pr]               │
│  > run-20250601-a3f9 │                                          │
│  [impl-auth] running │                                          │
│  [ATTENTION] waiting │                                          │
└──────────────────────┴──────────────────────────────────────────┘
```

**Node colours:**

| Colour | State |
|---|---|
| Gray `#9ca3af` | pending |
| Yellow `#fbbf24` | running |
| Green `#34d399` | done |
| Red `#f87171` | error |
| Orange `#fb923c` | waiting_human — click to review |

**SSE real-time updates** — the canvas updates without polling. Each state change is pushed over a Server-Sent Events stream at `/api/runs/{id}/stream`.

**REST API** (for scripting or CI integration):

```
POST /api/run                          upload spec files, returns {run_id}
GET  /api/runs                         list all runs
GET  /api/runs/{id}                    dag.json + node_states
GET  /api/runs/{id}/stream             SSE stream of node events
POST /api/runs/{id}/approve/{nodeID}   approve a checkpoint
POST /api/runs/{id}/reject/{nodeID}    reject a checkpoint (body: {"reason":"..."})
GET  /api/skills                       list skills
POST /api/skills/generate              body: {"description":"..."}, returns Skill
```

---

## Configuration

`.golovebox/config.toml` (created by `golovebox init`):

```toml
llm_provider   = "anthropic"           # anthropic | openai | ollama | gemini
llm_base_url   = "https://api.anthropic.com"
llm_model      = "claude-opus-4-5"
api_key        = "sk-ant-..."
github_token   = "ghp_..."
telegram_token = "123456:ABC-..."      # optional — enables Telegram daemon
default_repo   = "owner/repo"          # fallback repo for Telegram commands
ssh_port       = 2222
qmp_port       = 4444
qemu_path      = ".golovebox/qemu/qemu-system-x86_64.exe"
```

### Supported LLM providers

| `llm_provider` | `llm_base_url` | Notes |
|---|---|---|
| `anthropic` | `https://api.anthropic.com` | Adds `anthropic-version` header automatically |
| `openai` | `https://api.openai.com` | Standard Bearer auth |
| `ollama` | `http://localhost:11434` | Local models, no API key needed |
| `gemini` | `https://generativelanguage.googleapis.com` | OpenAI-compat endpoint |

---

## Runtime data layout

Everything lives in `.golovebox/` next to the binary:

```
.golovebox/
├── config.toml
├── qemu/
│   └── qemu-system-x86_64.exe
├── vm/
│   ├── base.img          ← Alpine qcow2
│   ├── id_rsa            ← SSH private key (host → VM)
│   └── id_rsa.pub
├── memory/               ← chromem-go vector embeddings
├── logs/
├── skills/               ← reusable skill .md files
│   └── go_api_review.md
└── runs/
    └── run-20250601-a3f9c2/
        ├── dag.json           ← full DAG plan
        ├── node_states.json   ← live state snapshot
        ├── specs/             ← copy of uploaded spec files
        ├── logs/
        │   └── node-1.log
        ├── artifacts/
        │   └── tech_debt.md   ← NOT_TODO items
        └── run_summary.md     ← generated on completion
```

---

## CLI reference

```
golovebox init [--repair]              Initialize environment
golovebox web [--addr :8080]          Start web UI (primary interface)

golovebox run <dir-or-file>           Execute specs through DAG orchestrator
golovebox resume <run-id>             Resume an interrupted run
golovebox status [run-id]             VM/config status, or details of a run

golovebox skill list                  List available skills
golovebox skill generate "<desc>"     Generate a skill via LLM

golovebox github --repo o/r --issue N Resolve a GitHub issue (one-shot)
golovebox exec "<cmd>"                Execute a shell command in the VM
golovebox daemon                      Telegram bot gateway (secondary interface)
```

---

## Telegram (optional, secondary interface)

Set `telegram_token` in config and run `golovebox daemon`. The bot responds to:

| Message | Effect |
|---|---|
| `issue #42 repo owner/repo` | Resolve issue #42 in the specified repo |
| `issue #42` | Resolve issue #42 in `default_repo` |
| `status` | VM health check |
| `help` | List commands |

Progress updates are sent as the agent works:
```
⚙️ Processando issue #42 em owner/repo...
🔄 [3/20] shell: go test ./...
✅ PR aberta: https://github.com/owner/repo/pull/7
```

---

## Building from source

```bash
git clone https://github.com/octavioturra/golovebox
cd golovebox
go mod tidy
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build ./cmd/golovebox
# → golovebox.exe (~14 MB)
```

Requirements: Go 1.22+. No C toolchain. `CGO_ENABLED=0`.

---

## Security notes

- **Token isolation**: `CloneRepo` uses `GIT_ASKPASS` with a UUID-named temp script — the token never appears in `git log` or `ps aux`.
- **VM sandbox**: all code execution is isolated inside the QEMU VM; the host filesystem is not mounted.
- **SSH pool liveness**: idle connections are validated with a keepalive before reuse — stale connections are discarded automatically.
- **LLM retry**: transient failures (429, 5xx, network) are retried with exponential backoff (2 s base, 30 s cap, 3 attempts).

---

## Project structure

```
golovebox/
├── cmd/golovebox/main.go       CLI entry point
└── internal/
    ├── agent/                  ReAct loop + tool registry + planner
    ├── config/                 Portable paths (relative to executable)
    ├── dag/                    DAG, Executor (parallel), CheckpointManager
    ├── dsl/                    Keyword parser for spec .md files
    ├── gateway/                Handler interface, Gateway, TelegramHandler
    ├── llm/                    HTTP client (OpenAI-compat + Anthropic, retry)
    ├── memory/                 chromem-go vector store + embeddings
    ├── orchestrator/           Specs → DAG via LLM planning
    ├── sandbox/                QEMU manager, SSH pool, SSH/SFTP helpers
    ├── setup/                  Init wizard (5 steps)
    ├── skills/                 Skill registry + LLM generator
    ├── tools/                  Shell, file, GitHub primitives
    └── web/                    HTTP server, SSE broker, embedded UI
```

---

## Phase roadmap

| Phase | Status | Summary |
|---|---|---|
| 1 | ✅ | Scaffold — CLI, config, QEMU sandbox, SSH/SFTP, init wizard |
| 2 | ✅ | Agent — ReAct loop, LLM client, GitHub tools, vector memory |
| 3 | ✅ | Gateway — Telegram bot, SSH pool, GIT_ASKPASS, LLM retry |
| 4 | ✅ | Platform — DAG orchestrator, web chat, skills, parallel execution |
| 5 | planned | Auth, HTTPS, multi-tenant, skill marketplace, streaming output |
