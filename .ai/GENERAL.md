# golovebox — Visão Geral

**Tagline:** Devin + n8n + Temporal, em um único .exe. Self-hosted, intention-DSL agentic workflow platform.

**Status:** Phase 9 completa. Phases 10-11 em spec. Binário ~450MB embute QEMU + Alpine cloud qcow2 + web UI.

---

## O que é

Um agente autônomo de codificação distribuído como um único .exe. Você copia para qualquer pasta, roda `golovebox init`, e tem:

- VM Alpine isolada para execução segura de código (QEMU + cloud-init no first boot)
- ReAct agent loop com ferramentas (shell, read_file, write_file, github_*, search_memory)
- Orquestrador que converte specs Markdown em linguagem natural em DAG executável
- Web UI embutida com health dashboard, stream ReAct por node e sessão persistente
- Memória vetorial persistente (chromem-go, pure Go, zero CGO)
- Skills plugáveis (templates .md com TOML frontmatter)
- Gateway Telegram (opcional, secundário)

Zero Python, zero Docker, zero admin, zero conta em nuvem.

---

## Como funciona

```
spec.md ──▶ orchestrator (LLM) ──▶ DAG ──▶ executor (paralelo, máx 3)
                                              │
                                              ├─ task         → ReAct loop dentro da VM
                                              ├─ checkpoint   → orange node no canvas, aguarda humano
                                              ├─ gate         → roda testes na VM
                                              └─ notify       → Telegram/email
                                              │
                                              ▼
                                      TASK_ID.md + PR URLs + artifacts
```

Toda execução (shell, read_file, write_file) acontece dentro da VM via SSH/SFTP. O host nunca é tocado.

---

## Comandos

| Comando | Função |
|---|---|
| `golovebox init` | Setup (config + extract QEMU/Alpine + boot + cloud-init). Reusa config.toml se já existir. |
| `golovebox init --repair` | Reusa config sem perguntar; refaz apenas o que falta. |
| `golovebox reset` | Apaga base.img + cidata.iso (mantém config + keys). Pede confirmação. |
| `golovebox reset --hard` | Apaga .golovebox/ inteiro. Exige digitar yes. |
| `golovebox web` | Sobe VM + web UI em localhost:8080. `--timeout N` ajusta start timeout. |
| `golovebox run <spec>` | Executa specs via DAG, com progresso no terminal. |
| `golovebox resume <run-id>` | Retoma run interrompido. |
| `golovebox exec "<cmd>"` | Shell direto na VM via SSH. |
| `golovebox github --repo o/r --issue N` | Resolve uma issue (one-shot). |
| `golovebox daemon` | Telegram bot gateway. |
| `golovebox skill list / generate "<desc>"` | Gerencia skills. |

---

## Estrutura em runtime

Tudo fica em `.golovebox/` ao lado do `.exe`:

```
.golovebox/
├── config.toml
├── qemu/                    ← extraído do binário (flat: exe + DLLs + share/qemu/)
├── vm/
│   ├── base.img             ← Alpine cloud qcow2 (~164MB)
│   ├── .alpine-image-size   ← marker de idempotência
│   ├── cidata.iso           ← cloud-init NoCloud seed
│   ├── id_rsa, id_rsa.pub   ← SSH keypair RSA 4096
│   └── qemu.log             ← console serial da VM
├── memory/                  ← chromem-go embeddings
├── skills/                  ← .md skills com frontmatter TOML
└── runs/<run-id>/
    ├── dag.json
    ├── node_states.json
    ├── task.txt
    ├── logs/<nodeID>.jsonl
    └── artifacts/
```

---

## DSL de Intenção

Prosa Markdown legível com keywords opcionais em MAIÚSCULAS:

```markdown
# Feature: autenticação JWT

Implementar login com email/senha em /api/auth.

ATTENTION_HERE: validar token expiry com o time de segurança
RUN_TEST: go test ./internal/auth/...

Adicionar refresh token.

NOTIFY_ME: avisar quando merge entrar em main
NOT_TODO: OAuth2 social login (fora do escopo)
```

| Keyword | Efeito |
|---|---|
| `ATTENTION_HERE` / `PAUSE_TO_REVIEW` | pausa o DAG, aguarda aprovação no web UI (node laranja) |
| `RUN_TEST` | gate — DAG só avança se os testes passarem |
| `NOTIFY_ME` | notifica e continua |
| `NOT_TODO` | grava em tech_debt.md, exclui do DAG |
| `TRY ... OR_ELSE ...` | try/fallback |
| `WHEN ... DO ...` | espera evento externo |

---

## Web UI

Interface primária. `localhost:8080` via `golovebox web`.

**Painel esquerdo:**
- Health dashboard — 4 dots (VM, LLM, GitHub, Repo) com check em paralelo, timeout 5s
- Chat — input de task, histórico de eventos, botão ⏹ Parar run ativo
- Lista de runs anteriores com preview da task e badge de status

**Canvas central:**
- DAG em Cytoscape.js com layout dagre automático
- Nodes atualizam cor em tempo real via SSE sem re-render do grafo
- Click no node abre painel lateral
- Tooltip no hover com nome completo e erro se houver
- Counter `running [7/20]` sobreposto ao node em execução

**Painel direito (node):**
- Objetivo do run + descrição do node
- Caixa de erro quando `state === error`
- Log de iterações ReAct em tempo real: `iter N — action ▶ params ◀ obs`
- Syntax highlight em Go/bash/JSON via highlight.js

**Sessão persistente:** `localStorage` restaura run ativo e node selecionado após reload.

---

## Stack

### Backend (pure Go, CGO_ENABLED=0)

| Camada | Lib |
|---|---|
| CLI | `spf13/cobra` |
| Config | `BurntSushi/toml` |
| HTTP router | `go-chi/chi/v5` — sub-routers, middleware Recoverer |
| Logging | `log/slog` stdlib — text em dev, json no daemon |
| LLM | `internal/llm` — HTTP puro, OpenAI-compat, retry exponencial |
| Telegram | `go-telegram-bot-api/v5` |
| SSH/SFTP | `golang.org/x/crypto/ssh` + `pkg/sftp` |
| QEMU control | `internal/sandbox/qmp.go` — QMP TCP direto |
| Memória vetorial | `philippgille/chromem-go` |
| GitHub | `google/go-github/v60` |
| CIDATA ISO | `github.com/kdomanski/iso9660` |
| Build | `github.com/magefile/mage` |

**Regra absoluta:** `CGO_ENABLED=0`. Sem exceção.

### Frontend (CDN, sem build step)

| Camada | Lib |
|---|---|
| Reatividade | Alpine.js v3 |
| DAG visual | Cytoscape.js v3 + cytoscape-dagre |
| Markdown | marked.js |
| Syntax highlight | highlight.js |

---

## VM Sandbox

- **Imagem:** Alpine NoCloud cloud qcow2 (3.21.7, BIOS, cloud-init habilitado)
- **Por que não Alpine Virt ISO:** ISO é instalador interativo — para em `localhost login:` esperando `setup-alpine`. Cloud-init não está no boot path. Phases 5/6 tentaram e nunca funcionaram.
- **Boot:** QEMU sobe `base.img` (virtio-blk, bootindex=0) + `cidata.iso` (segundo virtio-blk, sem bootindex). SeaBIOS boota o disco principal; cloud-init detecta CIDATA e aplica user-data (~1m42s no first boot em Windows TCG).
- **SSH:** porta 2222 do host → 22 da VM. Key RSA 4096 gerada localmente, publicada via cloud-init.
- **Cloud-init aplica:** SSH key + drop-in `/etc/ssh/sshd_config.d/99-golovebox.conf` (PermitRootLogin) + pacotes (git, curl, bash, openssh, python3, make).
- **QEMU silencioso:** stdout/stderr → `vm/qemu.log`, stdin → `os.DevNull`. Terminal do host intocado.
- **Validação:** `sandbox.Start()` lê magic do qcow2 antes de subir QEMU — erro claro se `base.img` está corrompido.

---

## Build

```bash
go install github.com/magefile/mage@latest

mage fetch          # baixa QEMU + Alpine cloud qcow2 (~244MB total)
mage check          # valida assets
mage buildWindows   # → build/golovebox.exe (~450MB)
mage build          # OS atual
```

Trade-off de tamanho: 450MB é grande, mas elimina qualquer passo de instalação no destino — incluindo os ~10 min de install do Alpine que existiam antes do Phase 7.

---

## Phase Roadmap

### V0 — Portable Coding Agent

| Phase | Status | Resumo |
|---|---|---|
| 1 | ✅ | Foundation — CLI, QEMU sandbox, SSH/SFTP, init wizard |
| 2 | ✅ | Agent — ReAct loop, LLM, GitHub tools, vector memory |
| 3 | ✅ | Gateway — Telegram, SSH pool, GIT_ASKPASS, LLM retry |
| 4 | ✅ | Platform — DAG orchestrator, web chat, skills, parallel exec |
| 5 | ✅ | Self-contained — embedded QEMU + Alpine, Mage pipeline |
| 6 | ✅ | QEMU extraction fix — weilnetz.de installer, silent NSIS |
| 7 | ✅ | Alpine NoCloud + silent boot + reset + config reuse + sshd drop-in |
| 8 | ✅ | Observabilidade — health dashboard, input manual, sessão, stream ReAct |
| 9 | ✅ | UX + Bugs — task visível, stop button, erros claros, canvas polish |
| 10 | 🔜 | UI Refactor — Alpine.js substituindo JS imperativo |
| 11 | 🔜 | Refactor — Canvas2D→Cytoscape, net/http→chi, fmt→slog, marked, highlight |

### V1 — Agentic Workflow Platform

| Fase | Nome | Descrição |
|---|---|---|
| v1_fase1 | Workflow Protocol | Branch-per-task, auto-PR, TASK_ID.md, lifecycle hooks (global_prompt, pre/post_step, pre/post_code), hierarquia de contexto |
| v1_fase2 | Memory Architecture | L1 working → L2 episodic → L3 semantic → L4 hypercontext; MD-first portável |
| v1_fase3 | Learning Loop | Execute → reflect → abstract → skill → store → retrieve |
| v1_fase4 | Multi-agent | Orchestrator (principal) + CodeAgent + TestAgent + GitAgent + ReviewAgent |

---

## Fora do Escopo V0

- Multi-agent paralelo
- VS Code Server
- Linux/macOS production-tested
- Auto-update do binário
- Code signing (Windows Defender)
- CDN assets embedded offline (first load precisa de internet para Alpine/Cytoscape)

---

## Documentos relacionados

| Arquivo | Conteúdo |
|---|---|
| `CLAUDE.md` | Instruções para o Claude Code (regras absolutas, stack, fases) |
| `CONTEXT_v5.json` | Snapshot técnico estruturado — estado atual |
| `hypercontext_v2.json` | Visão, filosofia, decisões de design, roadmap V1 |
| `ROADMAP.md` | Tabela V0 + V1 consolidada |
| `FASE_N.md` | Spec de cada fase (decisões, arquivos, limitações) |
| `FASE_TEMPLATE.md` | Template para novas fases |
| `CHECKLIST.md` | Verificação pré-commit para Claude Code |
| `DEBUG.md` | Modos de falha comuns e fixes |
| `EXAMPLES.md` | Specs de exemplo com DSL |
