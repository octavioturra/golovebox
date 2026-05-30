# golovebox

Agentic workflow platform with a self-hosted web UI and VM sandbox. Describe work in plain-language Markdown specs, get a visual DAG execution plan, approve checkpoints in the browser, and receive Pull Requests — all from a **single self-contained Windows binary** with zero installation.

```
golovebox init   →   extracts QEMU, installs Alpine VM, generates SSH keys, configures
golovebox web    →   opens localhost:8080
```

> **Self-contained since Phase 5**: QEMU binaries and a ready-to-boot Alpine cloud image are embedded inside the binary at build time. No manual downloads. No prerequisites beyond the binary itself.

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

## Quick start

### 1. Initialize (one-time setup)

```
golovebox init
```

This runs a fully automated 7-step setup:

| Step | What happens |
|---|---|
| 1 | Create `.golovebox/` directory tree |
| 2 | Interactive wizard: LLM provider, API keys, GitHub token (reuses existing `config.toml` if present — just press Enter) |
| 3 | Extract embedded QEMU binaries to `.golovebox/qemu/` |
| 4 | Extract embedded Alpine cloud qcow2 to `.golovebox/vm/base.img` (already a bootable Alpine install — no separate install step needed) |
| 5 | Generate RSA 4096 SSH key pair in `.golovebox/vm/` |
| 6 | Build cloud-init CIDATA disk with SSH key + sshd config |
| 7 | Boot VM — cloud-init applies SSH key and starts sshd on first boot; host SSHs in to run `echo ok` |

All steps are idempotent — safe to re-run. To redo only incomplete steps and skip the config wizard:

```
golovebox init --repair
```

If you ever need to wipe the VM and re-run cloud-init from scratch (e.g. after changing the cloud-init template):

```
golovebox reset            # delete base.img + cidata; keep config + SSH keys
golovebox reset --hard     # delete everything (config, keys, VM)
```

Both prompt for confirmation; add `-y` to skip the prompt.

### 2. Open the web UI

```
golovebox web
```

Boots the VM, starts a server on `localhost:8080`, and opens the browser automatically.

```
golovebox web --addr :9090       # custom port
golovebox web --timeout 30       # increase VM start timeout (default 15s)
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

```
golovebox skill list
golovebox skill generate "review a Go REST API for naming conventions and security"
```

The orchestrator automatically includes skill descriptions in its planning context.

**Skill file format** (TOML frontmatter + freeform prompt):
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

## Building from source

The build pipeline is managed by [Mage](https://magefile.org/). **Do not run `go build` directly** — embedded assets must be populated first.

### Prerequisites

- Go 1.22+, no C toolchain, `CGO_ENABLED=0`
- Mage: `go install github.com/magefile/mage@latest`
- macOS only: `brew install qemu` (for `mage fetchDarwin`)
- Linux only: `apt install qemu-system-x86` (for `mage fetchLinux`)
- Windows only: no extra tools needed — `mage fetchWindows` runs the QEMU installer silently

### Build steps

```bash
git clone https://github.com/octavioturra/golovebox
cd golovebox

# 1. Download QEMU + Alpine ISO (all assets go to internal/embed/assets/)
mage fetch          # fetches for current host OS

# For cross-compiling Windows binary from Linux:
mage fetchWindows   # downloads weilnetz.de installer, runs it silently, filters files
mage fetchAlpine    # downloads Alpine NoCloud qcow2 (~164 MB)

# 2. Verify assets
mage check
# → checks: qemu-system-x86_64.exe present (>10 MB), DLL count >50,
#   bios-256k.bin present, Alpine cloud qcow2 present (>40 MB)

# 3. Compile
mage buildWindows   # CGO_ENABLED=0, GOOS=windows, → build/golovebox.exe (~450 MB)
mage build          # current OS, → build/golovebox
```

### Available Mage targets

| Target | Description |
|---|---|
| `mage fetch` | Download all assets for the current host OS |
| `mage fetchAlpine` | Download only the Alpine NoCloud cloud qcow2 |
| `mage fetchWindows` | Download Windows QEMU from qemu.weilnetz.de via silent NSIS install (no UAC) |
| `mage fetchLinux` | Copy Linux QEMU from system (`apt install qemu-system-x86` first) |
| `mage fetchDarwin` | Copy macOS QEMU from Homebrew (`brew install qemu` first) |
| `mage build` | Compile for current OS/arch |
| `mage buildWindows` | Cross-compile for Windows amd64 |
| `mage check` | Verify all embedded assets are present and correctly sized |
| `mage clean` | Remove downloaded assets and scratch dirs (keeps `placeholder.txt`) |

### How the embedded assets work

```
internal/embed/assets/             ← go:embed source (gitignored except placeholder.txt)
├── alpine/
│   ├── placeholder.txt            ← committed — lets go:embed compile without real image
│   └── alpine-cloud-x86_64.qcow2  ← populated by mage fetchAlpine (~164 MB, bootable Alpine)
└── qemu/
    ├── windows-amd64/
    │   ├── placeholder.txt
    │   ├── qemu-system-x86_64.exe ← extracted from weilnetz.de NSIS installer
    │   ├── qemu-img.exe
    │   ├── *.dll                  ← ~100 DLLs, all needed at runtime
    │   └── share/qemu/            ← firmware: bios-256k.bin, efi-virtio.rom…
    ├── linux-amd64/
    │   ├── placeholder.txt
    │   └── qemu-system-x86_64     ← copied from system by mage fetchLinux
    └── darwin-arm64/
        ├── placeholder.txt
        └── qemu-system-x86_64     ← copied from Homebrew by mage fetchDarwin

build/tmp/                         ← installer + silent-install scratch (gitignored)
```

**How `mage fetchWindows` works:**
1. Scrapes `qemu.weilnetz.de/w64/` to find the latest `qemu-w64-setup-YYYYMMDD.exe`
2. Downloads the installer and verifies its SHA512
3. Runs `installer.exe /S /D=<build/tmp/qemu-raw>` with `__COMPAT_LAYER=RunAsInvoker` (no UAC, no admin rights)
4. Filters: keeps `qemu-system-x86_64.exe`, `qemu-img.exe`, all `*.dll`, `share/qemu/`
5. Copies to `internal/embed/assets/qemu/windows-amd64/`

A **placeholder build** (`go build` without assets) compiles cleanly — `golovebox init` will fail with a clear message:

```
QEMU/Alpine assets not embedded: run 'mage fetch' before 'mage build'
```

`sandbox.Start()` automatically passes `-L .golovebox/qemu/share/qemu` to QEMU so it finds the embedded firmware (BIOS, VirtIO ROMs) after extraction, and validates the qcow2 magic of `base.img` before booting — so stale/empty disks fail with a clear error instead of SeaBIOS's generic "could not read boot disk".

### Why Alpine NoCloud instead of the Virt ISO

Earlier phases used `alpine-virt-*.iso` (the Alpine live installer) and attempted to drive a first-boot install via cloud-init. That ISO does not ship cloud-init enabled on the boot path — it stops at `localhost login:` waiting for `setup-alpine`. Phase 7 switched to the **Alpine NoCloud cloud image** (`nocloud_alpine-*-x86_64-bios-cloudinit-r0.qcow2`), which is a ready-to-boot Alpine install with cloud-init wired into OpenRC. The CIDATA disk is detected on first boot and applied; no install step is needed.

---

## Testing

### Unit / vet

```bash
mage check        # verify embedded assets (size, DLL count, firmware presence)
go vet ./...      # static analysis (no VM needed)
go test ./...     # run tests (no VM needed)
```

### Smoke test (end-to-end)

The `golovebox init` smoke test (step 9) is the canonical end-to-end test. After a full init:

```
golovebox status           # shows VM running + SSH connectivity
golovebox exec "uname -a"  # direct shell command in VM
```

### Testing the web UI

```bash
# Start web server in dev mode (system QEMU, no embedded assets needed)
golovebox web

# Then open http://localhost:8080 and:
# 1. Upload a .md spec file
# 2. Watch the DAG canvas populate with pending nodes
# 3. Observe nodes go yellow (running) → green (done)
# 4. Click any orange node to approve a checkpoint
```

### Testing a single DAG run from CLI

```bash
cat > /tmp/test-spec.md << 'EOF'
# Test run
Create a file /tmp/hello.txt with content "golovebox ok".
RUN_TEST: test -f /tmp/hello.txt
EOF

golovebox run /tmp/test-spec.md
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
# qemu_path is auto-derived from .golovebox/qemu/; set only to override:
# qemu_path    = "/custom/path/qemu-system-x86_64"
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

Everything lives in `.golovebox/` next to the binary — never in system paths:

```
.golovebox/
├── config.toml
├── qemu/                        ← extracted at init (embedded in binary)
│   ├── qemu-system-x86_64.exe   ← flat layout (Windows: weilnetz.de installer)
│   ├── qemu-img.exe
│   ├── *.dll                    ← ~100 DLLs on Windows
│   └── share/qemu/              ← BIOS firmware, VirtIO ROMs
├── vm/
│   ├── base.img                 ← Alpine cloud qcow2, extracted from embed
│   ├── .alpine-image-size       ← idempotency marker for base.img
│   ├── cidata.iso               ← cloud-init NoCloud seed (generated at init)
│   ├── id_rsa                   ← SSH private key (generated at init)
│   ├── id_rsa.pub
│   └── qemu.log                 ← QEMU stdout/stderr (serial console of the VM)
├── memory/                      ← chromem-go vector embeddings
├── logs/
├── skills/                      ← reusable skill .md files
│   └── go_api_review.md
└── runs/
    └── run-20250601-a3f9c2/
        ├── dag.json             ← full DAG plan
        ├── node_states.json     ← live state snapshot
        ├── specs/               ← copy of uploaded spec files
        ├── logs/
        │   └── node-1.log
        ├── artifacts/
        │   └── tech_debt.md     ← NOT_TODO items
        └── run_summary.md       ← generated on completion
```

---

## CLI reference

```
golovebox init [--repair]                  Initialize environment (cloud-init runs ~2 min on first boot)
golovebox reset [--hard] [-y]              Reset VM (--hard removes everything; -y skips confirmation)
golovebox web [--addr :8080] [--timeout]   Start web UI (primary interface)

golovebox run <dir-or-file>                Execute specs through DAG orchestrator
golovebox resume <run-id>                  Resume an interrupted run
golovebox status [run-id]                  VM/config status, or details of a run

golovebox skill list                       List available skills
golovebox skill generate "<desc>"          Generate a skill via LLM

golovebox github --repo o/r --issue N      Resolve a GitHub issue (one-shot)
golovebox exec "<cmd>"                     Execute a shell command in the VM
golovebox daemon                           Telegram bot gateway (secondary interface)
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
├── magefile.go                 Build pipeline (mage targets)
└── internal/
    ├── agent/                  ReAct loop + tool registry + planner
    ├── config/                 Portable paths (relative to executable)
    ├── dag/                    DAG, Executor (parallel), CheckpointManager
    ├── dsl/                    Keyword parser for spec .md files
    ├── embed/                  Embedded QEMU + Alpine cloud qcow2 assets
    │   ├── assets/             Populated by "mage fetch" (gitignored)
    │   ├── extract.go          ExtractQEMU, ExtractAlpineImage (writes base.img)
    │   ├── keygen.go           RSA 4096 SSH keypair generation
    │   ├── cloudinit.go        cloud-init NoCloud CIDATA ISO builder (user-data + sshd drop-in)
    │   └── installboot.go      no-op stubs (cloud image is already installed)
    ├── gateway/                Handler interface, Gateway, TelegramHandler
    ├── llm/                    HTTP client (OpenAI-compat + Anthropic, retry)
    ├── memory/                 chromem-go vector store + embeddings
    ├── orchestrator/           Specs → DAG via LLM planning
    ├── sandbox/                QEMU manager, SSH pool, SSH/SFTP helpers
    ├── setup/                  Init wizard (9 automated steps)
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
| 5 | ✅ | Self-contained — embedded QEMU + Alpine assets, Mage build pipeline, automated init |
| 6 | ✅ | QEMU extraction fix — weilnetz.de official installer, silent NSIS install, SHA512 verify, flat layout |
| 7 | ✅ | Alpine NoCloud cloud image + reset command + config reuse on re-init + silent QEMU boot |
| 8 | planned | Auth, HTTPS, multi-tenant, skill marketplace, streaming output |
