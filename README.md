# golovebox

Box (QEMU) + luva (Canvas + mediação). Binário único, zero instalação — `golovebox.exe` em qualquer pasta, sem Python, Node, Docker ou admin.

O agente de trabalho é o **PicoClaw**, rodando selado dentro da VM. A luva opera o PicoClaw: injeta prompts, recebe respostas, gerencia recursos e audita cada tool-call. A API key fica na luva — nunca na box.

## Módulos

| Módulo | O quê | Doc |
|---|---|---|
| `core` | Contratos e tipos de domínio. Zero deps. | [core/AGENTS.md](core/AGENTS.md) |
| `sandbox` | VM QEMU + SSH/SFTP pool | [sandbox/AGENTS.md](sandbox/AGENTS.md) |
| `toolskills` | Registry de skills + provisionamento (resource manager — FASE 38) | [toolskills/AGENTS.md](toolskills/AGENTS.md) |
| `app` | Composition root + CLI + Web shell | [app/AGENTS.md](app/AGENTS.md) |

## Regra de dependência

```
app → sandbox    → core
app → toolskills → core
```

Módulos de produto importam **só** `core`. `app` importa todos. Ninguém importa `app`.

## Build

```bash
# binário completo (via go.work)
cd app && CGO_ENABLED=0 mage build

# compilação rápida
cd app && go build ./...

# teste isolado de um módulo
cd <modulo> && GOWORK=off go test ./...

# verificação de fronteiras
! grep -rq 'golovebox/app' core/ sandbox/ toolskills/
```

---

```
golovebox init   →   extrai QEMU, sobe Alpine VM, gera SSH keys, configura
golovebox web    →   abre localhost:8080  (terminal SSH + file explorer SFTP)
```

> **Self-contained**: binários QEMU e imagem Alpine cloud embutidos no binário em build time. Sem downloads manuais. Sem pré-requisitos além do executável.

> **Roadmap**: ver [`.ai/roadmap.yaml`](.ai/roadmap.yaml) — fases 33–39 de build do glovebox (box + luva).

> **Contexto para agentes**: [`.ai/AGENTS.md`](.ai/AGENTS.md) (instruções), [`.ai/VISION.md`](.ai/VISION.md) (narrativa), [`.ai/FASES.json`](.ai/FASES.json) (digest das fases — leia este em vez dos `FASE_N.md` individuais).

---

## Como funciona

```
golovebox web
      │
      ▼
  sandbox (QEMU VM Alpine)
      │  SSH PTY / SFTP
      ▼
  PicoClaw (FASE 34+)
      │  stdin/stdout via luva
      ▼
  Canvas (FASE 37)
   ┌──────────────────────────────────────┐
   │  prompt zone   → stdin PicoClaw      │
   │  response zone ← stdout PicoClaw     │
   │  terminal      → SSH PTY             │
   │  files         → SFTP explorer       │
   │  claw panel    → gateway :18800 proxy│
   └──────────────────────────────────────┘
```

Todo `shell`, `read_file`, `write_file` do PicoClaw roda dentro da VM Alpine via SSH/SFTP. O host nunca é tocado.

---

## Quick start

### 1. Inicializar (uma vez)

```
golovebox init
```

Setup automatizado em 7 passos:

| Passo | O que acontece |
|---|---|
| 1 | Extrai binários QEMU embutidos para `.golovebox/qemu/` |
| 2 | Extrai Alpine cloud qcow2 para `.golovebox/vm/base.img` |
| 3 | Gera par de chaves RSA 4096 em `.golovebox/vm/` |
| 4 | Cria CIDATA ISO com SSH key + sshd drop-in |
| 5 | Wizard interativo: configuração (reutiliza `config.toml` existente — só Enter) |
| 6 | Boot QEMU silencioso (`stdout → vm/qemu.log`) |
| 7 | Smoke test: poll SSH até `echo ok` |

Todos os passos são idempotentes — seguro re-executar.

```
golovebox init --repair   # refaz só os passos incompletos, sem wizard
```

### 2. Reset da VM

```
golovebox reset            # apaga base.img + cidata.iso; mantém config + SSH keys
golovebox reset --hard     # apaga todo .golovebox/
```

### 3. Web shell

```
golovebox web
golovebox web --addr :9090
golovebox web --timeout 30
```

Sobe a VM e abre servidor em `localhost:8080` com terminal SSH e file explorer SFTP.

### 4. Exec direto

```
golovebox exec "echo ok"
```

Executa comando na VM via SSH sem abrir o web shell.

---

## Web UI (pós-FASE 33)

`localhost:8080` via `golovebox web`.

```
┌─────────────────────────────────────────────────────────────┐
│  ● VM                                                       │
├─────────────────────────────────────────────────────────────┤
│  [⌨ Terminal]  [📁 Files]                                   │
│  root@alpine:~# _                                           │
└─────────────────────────────────────────────────────────────┘
```

**Health:** check de VM ready (auto-poll 5s).

**Terminal:** sessão SSH interativa no browser via xterm.js + WebSocket. PTY completo: Vim, top, git funcionam.

**Files:** SFTP file explorer. Dirs primeiro, click para navegar, click em arquivo abre inline.

**REST API:**

```
GET  /api/health          VM ready check
GET  /api/vm/files?path=  SFTP directory listing (JSON)
GET  /api/vm/file?path=   SFTP file content (streamed)
WS   /ws/terminal         WebSocket SSH PTY bridge
```

> O Canvas completo (5 zonas: prompt · respostas · terminal · arquivos · painel do Claw) é construído na FASE 37.

---

## Build from source

Pipeline gerenciado pelo [Mage](https://magefile.org/). **Não execute `go build` direto** — assets embutidos precisam ser populados primeiro.

### Pré-requisitos

- Go 1.22+, sem C toolchain, `CGO_ENABLED=0`
- Mage: `go install github.com/magefile/mage@latest`
- macOS: `brew install qemu`
- Linux: `apt install qemu-system-x86`
- Windows: sem ferramentas extras — `mage fetchWindows` roda o installer silenciosamente

### Passos

```bash
git clone https://github.com/octavioturra/golovebox
cd golovebox

# 1. Download QEMU + Alpine cloud qcow2
mage fetch          # OS atual
mage fetchWindows   # cross-compile Windows a partir de Linux/macOS
mage fetchAlpine    # Alpine NoCloud qcow2 (~164 MB)

# 2. Verificar
mage check

# 3. Compilar
mage buildWindows   # → build/golovebox.exe (~450 MB)
mage build          # OS atual
```

### Targets Mage

| Target | Descrição |
|---|---|
| `mage fetch` | Download de todos os assets para o OS atual |
| `mage fetchAlpine` | Alpine NoCloud cloud qcow2 |
| `mage fetchWindows` | QEMU Windows de qemu.weilnetz.de (NSIS silencioso, sem UAC) |
| `mage fetchLinux` | Copia QEMU do sistema (`apt install qemu-system-x86` antes) |
| `mage fetchDarwin` | Copia QEMU do Homebrew (`brew install qemu` antes) |
| `mage build` | Compila para OS/arch atual |
| `mage buildWindows` | Cross-compila para Windows amd64 |
| `mage check` | Verifica assets embutidos (presença + tamanho) |
| `mage clean` | Remove assets e dirs temporários |

---

## Configuração

`.golovebox/config.toml` (criado por `golovebox init`):

```toml
llm_provider   = "anthropic"
llm_base_url   = "https://api.anthropic.com"
llm_model      = "claude-opus-4-5"
api_key        = "sk-ant-..."
github_token   = "ghp_..."
```

> Campos de `[workflow]` (default_repo, clone_path) eram da era de orquestração — removidos na FASE 33. Config será simplificado para picoclaw + proxy na FASE 34/35.

---

## Layout de runtime

```
.golovebox/                    ← tudo aqui, nunca fora
├── config.toml
├── qemu/                      ← flat: exe + ~100 DLLs + share/qemu/
└── vm/
    ├── base.img                ← Alpine cloud qcow2 (~164 MB)
    ├── cidata.iso              ← cloud-init NoCloud seed
    ├── id_rsa, id_rsa.pub      ← RSA 4096
    └── qemu.log               ← console serial do QEMU
```

---

## CLI reference

```
golovebox init [--repair]                  Inicializa o ambiente
golovebox reset [--hard] [-y]             Reset da VM
golovebox web [--addr :8080] [--timeout]  Inicia web shell (interface principal)
golovebox exec "<cmd>"                    Executa comando na VM
```

---

## Segurança

- **VM sandbox**: toda execução isolada dentro da VM QEMU; filesystem do host nunca montado
- **SSH pool**: conexões idle validadas com keepalive antes de reusar; stale descartadas. Fresh dials com retry 4× backoff linear
- **Key isolation (FASE 35+)**: API key fica na luva (host), nunca na box

---

## Roadmap

Ver [`.ai/roadmap.yaml`](.ai/roadmap.yaml) para o plano completo.

### Status atual

| Fase | Nome | Status |
|---|---|---|
| 31 | Git Determinístico | ✅ feito |
| 32 | Estabilização da box | ✅ feito |
| 33 | Amputar o legado | 🔨 em andamento |
| 34 | PicoClaw na box | pending |
| 35 | Proxy do modelo | pending |
| 36 | Canal stdio (cap. 1 e 2) | pending |
| 37 | Canvas — as 5 zonas | pending |
| 38 | Resource manager + manifesto (cap. 3) | pending |
| 39 | Hook de mediação (audit + approval) | pending |

**Caminho crítico:** 33 → 34 → 35 → 36 → 37

**V0 done definition:**
> Dois cliques → box sobe → PicoClaw rodando selado dentro → Canvas opera ele: jogar prompt, receber resposta, mexer recursos da VM, terminal/SFTP, painel do Claw por proxy. Modelo só fala pela luva (key nunca na box). Tudo auditado.
