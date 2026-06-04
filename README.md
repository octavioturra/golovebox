# golovebox

Agente de coding autônomo com sandbox QEMU. Workspace Go — 7 módulos independentes. Setas de dependência só apontam para `core`.

## Módulos

| Módulo | O quê | Doc |
|---|---|---|
| `core` | Contratos e tipos de domínio. Zero deps. | [core/AGENTS.md](core/AGENTS.md) |
| `promptlang` | DSL spec → `core.Intent` | [promptlang/AGENTS.md](promptlang/AGENTS.md) |
| `sandbox` | Execução isolada em VM QEMU | [sandbox/AGENTS.md](sandbox/AGENTS.md) |
| `toolskills` | Registry de skills + provisionamento | [toolskills/AGENTS.md](toolskills/AGENTS.md) |
| `derivator` | Seed → grafo de prompts priorizados | [derivator/AGENTS.md](derivator/AGENTS.md) |
| `orchestrator` | Engine: Intent→DAG, executor, ReAct loop | [orchestrator/AGENTS.md](orchestrator/AGENTS.md) |
| `app` | Composition root + CLI + Web UI + gateway | [app/AGENTS.md](app/AGENTS.md) |

## Regra de dependência

```
app → orchestrator → core
app → sandbox      → core
app → toolskills   → core
app → derivator    → core
app → promptlang   → core
```

Módulos de produto importam **só** `core`. `app` importa todos. Ninguém importa `app`.

## Mapa de contratos

Ver [core/CONTRACTS.md](core/CONTRACTS.md) — lista cada interface, quem implementa, quem consome.

## Build

```bash
# binário completo (via go.work)
cd app && CGO_ENABLED=0 mage build

# compilação rápida
cd app && go build ./...

# teste isolado de um módulo
cd <modulo> && GOWORK=off go test ./...

# verificação de fronteiras
! grep -rq 'golovebox/app' core/ promptlang/ sandbox/ toolskills/ derivator/ orchestrator/
```

## Swarm

Cada módulo é uma unidade de trabalho autossuficiente. Um agente trabalha com contexto mínimo: `<modulo>/AGENTS.md` + `core/contracts.go` + arquivos do módulo.

Mudança de contrato (`core`) segue protocolo aditivo — ver [core/AGENTS.md](core/AGENTS.md).

---



```
golovebox init   →   extracts QEMU, boots Alpine VM, generates SSH keys, configures
golovebox web    →   opens localhost:8080
```

> **Self-contained since Phase 5**: QEMU binaries and a ready-to-boot Alpine cloud image are embedded inside the binary at build time. No manual downloads. No prerequisites beyond the binary itself.

> **V1 direction**: golovebox is being promoted from Dev to TechLead. Instead of writing code itself, it will specify, delegate to coding CLIs (Claude Code, Codex, Gemini CLI), verify the result, and keep everyone informed. See [V1 Roadmap](#v1-roadmap--techlead).

> **For contributors and agents**: project context lives under [`.ai/`](.ai/) — `AGENTS.md` (instructions), `VISION.md` (product narrative), `hypercontext.json` (structured metadata), and **`FASES.json`** (digest of all V0 phases — read this instead of the individual `FASE_N.md` files in `.ai/fases/`).

---

## How it works

```
Spec files (.md)
      │  uploaded via web chat or CLI
      ▼
 Orchestrator  (LLM → JSON plan)
      │
      ▼
 sync_repo  (clone or pull default repo — auto-injected)
      │
      ▼
 DAG Executor  (parallel nodes, up to 3 simultaneous)
   ┌──────────────────────────────────────┐
   │  task node    → ReAct loop in VM     │
   │  checkpoint   → wait for human       │  ← orange node in canvas
   │  gate         → run tests in VM      │
   │  branch       → NEW BRANCH           │
   │  push         → PUSH                 │
   │  pr           → PR (create/update)   │
   │  notify       → send alert           │
   └──────────────────────────────────────┘
      │
      ▼
 .ai/tasks/TASK_ID.md  +  PR URLs  +  artifacts
```

Every `shell`, `read_file`, `write_file` call runs inside an **Alpine Linux QEMU VM** via SSH/SFTP. The host machine is never touched.

---

## Quick start

### 1. Initialize (one-time setup)

```
golovebox init
```

Fully automated 7-step setup:

| Step | What happens |
|---|---|
| 1 | Extract embedded QEMU binaries to `.golovebox/qemu/` (flat layout) |
| 2 | Extract embedded Alpine cloud qcow2 to `.golovebox/vm/base.img` |
| 3 | Generate RSA 4096 SSH key pair in `.golovebox/vm/` |
| 4 | Build cloud-init CIDATA ISO with SSH key + sshd drop-in config |
| 5 | Interactive wizard: LLM provider, API keys, GitHub token, default repo (reuses existing `config.toml` — just press Enter) |
| 6 | Boot QEMU silently (`stdout → vm/qemu.log`, `stdin → /dev/null`) |
| 7 | Smoke test: poll SSH until `echo ok` — cloud-init applies on first boot (~1m42s on Windows TCG) |

All steps are idempotent — safe to re-run. To redo only incomplete steps without the wizard:

```
golovebox init --repair
```

To reset the VM:

```
golovebox reset            # delete base.img + cidata.iso; keep config + SSH keys
golovebox reset --hard     # delete entire .golovebox/ directory
```

Both prompt for confirmation; add `-y` to skip.

### 2. Open the web UI

```
golovebox web
```

Boots the VM and starts a server on `localhost:8080`.

```
golovebox web --addr :9090       # custom port
golovebox web --timeout 30       # increase VM start timeout (default 15s)
```

### 3. Write a spec file

Create a Markdown file describing the work. Use uppercase keywords to annotate special behaviour:

```markdown
# Feature: user authentication

NEW BRANCH feature/jwt-auth

Implement JWT-based login for the /api/auth endpoint.

RUN_TEST: go test ./internal/auth/...
ATTENTION_HERE: review the token expiry strategy with the security team

Add refresh token support.

PUSH
PR "feat: JWT authentication"
NOTIFY_ME: send summary when merged

NOT_TODO: OAuth2 social login (out of scope for this sprint)
```

| Keyword | Effect |
|---|---|
| `NEW BRANCH <name>` | Creates branch — `git checkout -b <name>`, persisted in `run_meta.json` |
| `PUSH` | Pushes branch with `--set-upstream` (auth via persistent `credential.helper=store`) |
| `PR "title"` | Creates or updates open PR for the current branch |
| `ATTENTION_HERE` | Pauses execution — orange node in canvas, click to approve |
| `PAUSE_TO_REVIEW` | Same as ATTENTION_HERE |
| `RUN_TEST` | Gate node — only advances if tests pass |
| `NOTIFY_ME` | Sends notification and continues |
| `NOT_TODO` | Logged to `tech_debt.md`, excluded from DAG |
| `TRY ... OR_ELSE ...` | Try/fallback node |
| `WHEN ... DO ...` | Wait-for-event node |

The orchestrator generates each node's `task` as a **self-contained prompt** — restating the user's original objective plus concrete details (file paths, content, behavior) — so the ReAct loop has the context it needs to produce real output instead of stubs.

### 4. Run specs from the CLI

```bash
golovebox run ./specs/
golovebox run auth-feature.md
```

Parses specs, asks the LLM for an execution plan, runs the DAG with terminal progress. Checkpoints prompt for `approve` / `reject` on stdin.

### 5. Manage skills

Skills are reusable agent behaviour templates stored as `.md` files in `.golovebox/skills/`.

```
golovebox skill list
golovebox skill generate "review a Go REST API for naming conventions and security"
```

**Skill file format** (TOML frontmatter + freeform prompt):

```markdown
---
name = "go_api_review"
description = "Reviews a Go REST API for conventions and security"
tools = ["shell", "read_file", "search_memory"]
---

Analyse each route for consistent naming, correct HTTP status codes,
authentication on protected routes, and missing input validation.
```

### 6. Resume an interrupted run

```
golovebox resume run-20250601-a3f9c2
```

Resets any `running` nodes back to `pending` and re-executes from current state.

---

## Web UI

`localhost:8080` via `golovebox web`.

```
┌─────────────────────────┬────────────────────────────────────────────────┐
│  left panel             │  DAG canvas (Cytoscape.js)                     │
│                         │                                                │
│  ● VM  ● LLM            │   [sync_repo]──▶[impl-auth]──▶[run-tests]     │
│  ● GitHub  ● Repo       │                      ↓                        │
│                         │              [ATTENTION_HERE]  ◀── orange     │
│  Task input:            │                      ↓                        │
│  ┌───────────────────┐  │              [push]──▶[open-pr]               │
│  │                   │  │                                                │
│  └───────────────────┘  │                                                │
│  [▶ Execute]            ├────────────────────────────────────────────────┤
│                         │  node: impl-auth              [×]              │
│  Runs:                  │  ─────────────────────────────────────────────│
│  run-20250601  running  │  📋 "implement JWT authentication"             │
│  run-20250528  done     │                                                │
│                         │  iter 3 — shell                                │
│                         │  ▶ go test ./internal/auth/...                 │
│                         │  ◀ PASS coverage: 87.3%                        │
│                         │                                                │
├─────────────────────────┴────────────────────────────────────────────────┤
│  [⌨ Terminal]  [📁 Files]                                                │
│  root@alpine:~# _                                                        │
└──────────────────────────────────────────────────────────────────────────┘
```

**Left panel:**
- Health dashboard — VM, LLM, GitHub, Repo status with parallel checks (3s timeout, **auto-polled every 5s**). Dots pulse while checking.
- Task input — type a task directly, no `.md` file needed. Sends `POST /api/run` as JSON.
- Run list — shows task preview and status badge. Persists active run across reloads.

**DAG canvas (Cytoscape.js):**
- Automatic dagre layout, centered
- Live node color updates via SSE — **no full re-render**, `cy.getElementById(id).data('color', newColor)`
- Click any node to open the log panel on the right
- Hover for full node name and error tooltip
- Counter `running [7/20]` overlaid on running nodes

**Node panel (right):**
- Shows the run task, node description, and **current branch** at the top
- Red error box with message when `state === error` — panel **auto-opens** on the failed node
- ReAct log stream in real time, with collapsible blocks per iteration:
  - **Prompt sent** to the LLM (blue border)
  - **Raw LLM reply** before action parsing (green border)
  - Action name, parameters, observation with syntax highlighting
- Everything persisted in `<runDir>/logs/<nodeID>.jsonl` — reloading the page restores the full history

**Bottom panel (tabs):**
- **Terminal** — interactive SSH session in the browser via xterm.js + WebSocket. Full PTY: Vim, top, git log work correctly.
- **Files** — browse VM filesystem via SFTP. Dirs first, click to navigate, click file to open inline.

**Node colours:**

| Colour | State |
|---|---|
| Gray `#9ca3af` | pending |
| Yellow `#fbbf24` | running |
| Green `#34d399` | done |
| Red `#f87171` | error |
| Orange `#fb923c` | waiting_human — click to review |

**Session persistence** — `localStorage` saves the active run ID and selected node. Reloading the page reconnects to the SSE stream and restores the panel state.

**Stop button** — visible whenever a run is active. Calls `POST /api/runs/{id}/stop` which cancels the executor's context.

**REST API:**

```
POST /api/run                          upload spec files or JSON {task}, returns {run_id}
GET  /api/runs                         list runs (includes task preview)
GET  /api/runs/{id}                    dag + node_states + task + error_msg
GET  /api/runs/{id}/stream             SSE stream: state_change + node_log events
POST /api/runs/{id}/stop               cancel active run
POST /api/runs/{id}/approve/{nodeID}   approve a checkpoint
POST /api/runs/{id}/reject/{nodeID}    reject a checkpoint
GET  /api/runs/{id}/nodes/{nodeID}/log ReAct log entries (history + live)
GET  /api/health                       parallel health checks: VM, LLM, GitHub, Repo
GET  /api/vm/files?path=               SFTP directory listing (JSON)
GET  /api/vm/file?path=                SFTP file content (streamed)
WS   /ws/terminal                      WebSocket SSH PTY bridge
GET  /api/skills                       list skills
POST /api/skills/generate              body: {"description":"..."}, returns Skill
```

---

## Building from source

Build pipeline managed by [Mage](https://magefile.org/). **Do not run `go build` directly** — embedded assets must be populated first.

### Prerequisites

- Go 1.22+, no C toolchain, `CGO_ENABLED=0`
- Mage: `go install github.com/magefile/mage@latest`
- macOS only: `brew install qemu`
- Linux only: `apt install qemu-system-x86`
- Windows: no extra tools — `mage fetchWindows` runs the installer silently

### Build steps

```bash
git clone https://github.com/octavioturra/golovebox
cd golovebox

# 1. Download QEMU + Alpine cloud qcow2
mage fetch          # current host OS
mage fetchWindows   # cross-compile Windows binary from Linux/macOS
mage fetchAlpine    # Alpine NoCloud qcow2 only (~164 MB)

# 2. Verify
mage check

# 3. Compile
mage buildWindows   # → build/golovebox.exe (~450 MB)
mage build          # current OS
```

### Mage targets

| Target | Description |
|---|---|
| `mage fetch` | Download all assets for the current host OS |
| `mage fetchAlpine` | Download Alpine NoCloud cloud qcow2 |
| `mage fetchWindows` | Download Windows QEMU from qemu.weilnetz.de (silent NSIS install, no UAC) |
| `mage fetchLinux` | Copy QEMU from system (`apt install qemu-system-x86` first) |
| `mage fetchDarwin` | Copy QEMU from Homebrew (`brew install qemu` first) |
| `mage build` | Compile for current OS/arch |
| `mage buildWindows` | Cross-compile for Windows amd64 |
| `mage check` | Verify all embedded assets are present and correctly sized |
| `mage clean` | Remove downloaded assets and scratch dirs |

### Embedded assets layout

```
internal/embed/assets/             ← go:embed source (gitignored except placeholders)
├── alpine/
│   └── alpine-cloud-x86_64.qcow2  ← bootable Alpine 3.21.7 (~164 MB)
└── qemu/
    └── windows-amd64/
        ├── qemu-system-x86_64.exe
        ├── qemu-img.exe
        ├── *.dll                  ← ~100 DLLs
        └── share/qemu/            ← BIOS, VirtIO ROMs
```

**How `mage fetchWindows` works:**
1. Scrapes `qemu.weilnetz.de/w64/` for the latest `qemu-w64-setup-YYYYMMDD.exe`
2. Downloads and verifies SHA512
3. Runs installer silently: `installer.exe /S /D=<build/tmp/qemu-raw>` with `__COMPAT_LAYER=RunAsInvoker` (no UAC)
4. Filters: keeps `qemu-system-x86_64.exe`, `qemu-img.exe`, all `*.dll`, `share/qemu/`

**Why Alpine NoCloud instead of Virt ISO:**
The Virt ISO is an interactive installer that stops at `localhost login:` waiting for `setup-alpine` — cloud-init is not on its boot path. The NoCloud cloud image ships with cloud-init wired into OpenRC. The CIDATA disk is detected on first boot and applied automatically (~1m42s on Windows TCG, ~10s on Linux with KVM). No install step needed.

---

## Configuration

`.golovebox/config.toml` (created by `golovebox init`):

```toml
llm_provider   = "anthropic"
llm_base_url   = "https://api.anthropic.com"
llm_model      = "claude-opus-4-5"
api_key        = "sk-ant-..."
github_token   = "ghp_..."
telegram_token = "123456:ABC-..."      # optional

[workflow]
default_repo   = "owner/repo"          # cloned/pulled at the start of every run
clone_path     = "/root/repo"          # path inside VM
run_mode       = "build_only"          # no servers, no long-running processes
default_branch = "main"

[agents]                               # V1 — coding CLI delegation
default        = "claude-code"
# claude-code.command = "claude"
# codex.command       = "codex"
# gemini.command      = "gemini"
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

```
.golovebox/                             ← everything here, never outside
├── config.toml
├── qemu/                               ← flat: exe + ~100 DLLs + share/qemu/
├── vm/
│   ├── base.img                        ← Alpine cloud qcow2 (~164 MB)
│   ├── .alpine-image-size              ← idempotency marker
│   ├── cidata.iso                      ← cloud-init NoCloud seed
│   ├── id_rsa, id_rsa.pub              ← RSA 4096 keypair
│   └── qemu.log                        ← QEMU serial console
├── memory/                             ← chromem-go vector store
├── skills/                             ← reusable skill .md files
└── runs/
    └── run-20250601-a3f9c2/
        ├── dag.json
        ├── node_states.json
        ├── task.txt                    ← submitted task text
        ├── logs/
        │   └── <nodeID>.jsonl          ← ReAct log, append-only
        └── artifacts/
            └── tech_debt.md            ← NOT_TODO items

repo/  (user's repository)
└── .ai/                                ← golovebox's territory in the repo
    ├── CONTEXT.md                      ← permanent project context
    ├── DECISIONS.md                    ← architecture decision records
    ├── specs/<feature>.md              ← golovebox writes before delegating (V1)
    ├── reports/<feature>.md            ← CLI writes after implementing (V1)
    └── tasks/<TASK_ID>.md              ← synthesis written after each run
```

---

## CLI reference

```
golovebox init [--repair]                  Initialize environment
golovebox reset [--hard] [-y]              Reset VM (--hard removes everything)
golovebox web [--addr :8080] [--timeout]   Start web UI (primary interface)

golovebox run <dir-or-file>                Execute specs through DAG orchestrator
golovebox resume <run-id>                  Resume an interrupted run
golovebox status [run-id]                  VM/config status or run details
golovebox exec "<cmd>"                     Run a shell command in the VM

golovebox skill list                       List available skills
golovebox skill generate "<desc>"          Generate a skill via LLM

golovebox github --repo o/r --issue N      Resolve a GitHub issue (one-shot)
golovebox daemon                           Telegram bot gateway
```

---

## Telegram (optional)

Set `telegram_token` in config and run `golovebox daemon`:

| Message | Effect |
|---|---|
| `issue #42 repo owner/repo` | Resolve issue in specified repo |
| `issue #42` | Resolve in `default_repo` |
| `status` | VM health check |

---

## Security notes

- **Token isolation**: `CloneRepo` uses `GIT_ASKPASS` with a UUID-named temp script — token never appears in `git log` or `ps aux`
- **Persistent git credentials**: cloud-init enables `credential.helper=store` in the VM's `.gitconfig`. After clone, `~/.git-credentials` is written via SFTP (perms `0600`) — subsequent `git push/pull` issued from the agent's shell tool authenticate transparently without exposing the token in process listings
- **VM sandbox**: all code execution isolated inside QEMU VM; host filesystem not mounted
- **SSH pool**: idle connections validated with keepalive before reuse; stale connections discarded. **Fresh dials retry 4× with linear backoff** (~3.7s total) to survive the cloud-init `sshd restart` window
- **LLM retry**: transient failures (429, 5xx, network) retried with exponential backoff (2s base, 30s cap, 3 attempts)
- **WebSocket terminal**: no auth in V0 (localhost-only); token-based auth planned for V1

---

## Project structure

```
golovebox/
├── cmd/golovebox/main.go
├── magefile.go
└── internal/
    ├── agent/          ReAct loop, ProgressFunc(iter, action, params, obs, prompt, reply), heredoc parser, tool registry
    ├── config/         Portable paths (relative to executable), WorkflowConfig
    ├── dag/            DAG types, Executor (parallel), CheckpointManager
    ├── dsl/            Keyword parser: NEW BRANCH, PUSH, PR, ATTENTION_HERE, RUN_TEST…
    ├── embed/          go:embed assets, extract.go, keygen.go, cloudinit.go
    ├── gateway/        Handler interface, Gateway router, TelegramHandler
    ├── llm/            HTTP client, OpenAI-compat + Anthropic header, retry backoff
    ├── memory/         chromem-go wrapper, per-agent isolated collections
    ├── orchestrator/   Specs → DAG via LLM; sync_repo auto-injection (workflowRepo fallback); self-contained tasks; run_mode prompt
    ├── sandbox/        qemu.go, qmp.go, ssh.go, pool.go (keepalive + fresh-dial retry)
    ├── setup/          7-step init wizard (idempotent)
    ├── skills/         Local registry + LLM generator
    ├── tools/          shell.go (stall detection), files.go, git.go (SyncRepo + persistent credentials), github.go
    └── web/
        ├── server.go   chi router, handlers, SSE broker, WebSocket terminal
        ├── store.go    RunStore — cancelMap, NodeLogFor, RunMeta
        ├── nodelog.go  NodeLog — memory buffer + .jsonl append-only
        ├── terminal.go WebSocket ↔ SSH PTY bridge
        └── static/
            └── index.html  Alpine.js components + Cytoscape DAG + xterm.js
```

---

## Phase roadmap

### V0 — Portable Coding Workflow ✅ Complete

All 18 phases delivered. See [`.ai/FASES.json`](.ai/FASES.json) for the full digest of decisions, libraries, learnings, and known open issues.

| Phase | Summary |
|---|---|
| 1 | Foundation — CLI, config, QEMU sandbox, SSH/SFTP, init wizard |
| 2 | Agent — ReAct loop, LLM client, GitHub tools, vector memory |
| 3 | Gateway — Telegram bot, SSH pool, `GIT_ASKPASS`, LLM retry |
| 4 | Platform — DAG orchestrator, web chat, skills, parallel execution |
| 5 | Self-contained — embedded QEMU + Alpine, Mage pipeline |
| 6 | QEMU extraction fix — weilnetz.de installer, silent NSIS, SHA512, flat layout |
| 7 | Alpine NoCloud cloud image + silent boot + reset + sshd drop-in |
| 8 | Observability — health dashboard, manual task input, session persistence, ReAct stream per node |
| 9 | UX + bugs — task visible, stop button, clear errors, canvas polish, params in log |
| 10 | UI Refactor — Alpine.js replaces imperative JS; isolated components + stores |
| 11 | Library swaps — Canvas2D→Cytoscape, `net/http`→chi, `fmt`→slog, marked.js, highlight.js |
| 12 | VM in browser — SSH terminal (WebSocket + xterm.js) + SFTP file explorer |
| 13 | Workflow DSL — `NEW BRANCH`, `PUSH`, `PR`, `sync_repo` auto-injection, `ErrNeedsHuman` |
| 14 | Bugfix BRANCH/PR — git identity via cloud-init, `repoCtx` in orchestrator prompt |
| 15 | Bugfix `$HOME` / 404 / 503 — `.gitconfig` via `write_files`, graceful run degradation, `IsVMReady` probe |
| 16 | UX & observability — LLM prompt+reply persisted per iter, health auto-poll, branch in node panel |
| 17 | Execution correctness — persistent `credential.helper=store`, heredoc `<<EOF` parser, self-contained tasks |
| 18 | Residual fixes — pool dial retry, top-level `default_repo` fallback, auto-open node panel on error |

### V1 — TechLead

golovebox stops writing code. It specifies, delegates, verifies, communicates, and learns.

| Phase | Name | What it delivers |
|---|---|---|
| v1_fase1 | CLI Delegation | `CodingAgent` interface, Claude Code/Codex/Gemini adapters, `delegate_code` + `verify_implementation` tools, `.ai/` convention |
| v1_fase2 | Project Memory | `.ai/` as living project memory — specs, reports, tasks, decisions. MD-first, portable, vector-indexed |
| v1_fase3 | Communication Layer | Slack, email, calendar. `notify_slack`, `send_email`, `schedule_meeting` tools |
| v1_fase4 | PM Integration | GitHub Issues, Jira, Linear — read tickets, update status as DAG progresses, auto-close on merge |
| v1_fase5 | Learning Loop | Spec quality improves with use; reflection after each task; learns which CLI works best per task type |

**V1 contract:**

```
golovebox writes:          coding CLI writes:
.ai/specs/feature.md  →   src/ (implementation)
.ai/CONTEXT.md        →   tests/ (passing)
branch, synced repo   →   .ai/reports/feature.md (what it found, decisions made)
```

**V1 done definition:**
> Open a Jira issue. golovebox reads it, writes `.ai/specs/`, delegates to Claude Code, Claude Code implements and documents in `.ai/reports/`, golovebox verifies, pushes, opens PR, updates Jira, sends Slack to the team and email to the client — without touching a single line of code.
