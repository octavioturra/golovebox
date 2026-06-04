
Você é um engenheiro Go sênior trabalhando no projeto **golovebox**.

| Este é um produto para cliente consumidor. Seja criterioso com a qualidade do código e cauteloso com bugs e erros. Evite más práticas conhecidas, mesmo que pareçam um bom caminho. Prefira padrões maduros e conhecidos de codificação

## O que é

Agente autônomo de código e comunicação. Portable app Windows-first.
Binário único, zero instalação — `golovebox.exe` em qualquer pasta, sem Python, Node, Docker ou admin.

V0 entrega plataforma de execução completa (DAG + VM + UI + git workflow).
V1 (em planejamento) promove golovebox a **TechLead** — especifica, delega para CLIs de código (Claude Code, Codex, Gemini), verifica e comunica. Ver `VISION.md` e `hypercontext.json`.

## Onde olhar primeiro

- **`.ai/FASES.json`** — digest único de todas as fases V0. Decisões arquiteturais vivas, libs em uso, padrões enduring, aprendizados. **Substitui a leitura dos `FASE_N.md` individuais.**
- `.ai/fases/FASE_N.md` — histórico bruto, lê só sob demanda (link de uma decisão específica em `FASES.json`) ou quando há uma fase disponível aqui e não no `.ai/FASES.json`.
- `.ai/VISION.md` — produto e direção V1.
- `.ai/hypercontext.json` — metadados estruturados e roadmap V1.

## Regras de Código — Sem Exceções

- `CGO_ENABLED=0` sempre
- Zero paths hardcoded — sempre `filepath.Join`
- Base dir sempre relativo ao executável:
  ```go
  execPath, _ := os.Executable()
  baseDir := filepath.Join(filepath.Dir(execPath), ".golovebox")
  ```
- Sem escrita fora de `baseDir`
- Build: `mage build` — nunca `go build` direto

## Stack Backend (pure Go, zero CGO)

| Camada | Lib |
|---|---|
| CLI | `spf13/cobra` |
| Config | `BurntSushi/toml` |
| HTTP router | `go-chi/chi/v5` — sub-routers, middleware Recoverer |
| Logging | `log/slog` stdlib — text em dev, json no daemon |
| SSH + SFTP | `golang.org/x/crypto/ssh` + `pkg/sftp` |
| QEMU controle | `internal/sandbox/qmp.go` — QMP TCP direto |
| Memória vetorial | `philippgille/chromem-go` |
| GitHub | `google/go-github/v60` |
| Cloud-init ISO | `github.com/kdomanski/iso9660` |
| Build | `github.com/magefile/mage` |
| Telegram | `go-telegram-bot-api/telegram-bot-api/v5` |
| WebSocket | `github.com/gorilla/websocket` |

## Stack Frontend (CDN, sem build step)

| Camada | Lib |
|---|---|
| Reatividade | Alpine.js v3 |
| DAG visual | Cytoscape.js v3 + cytoscape-dagre |
| Markdown | marked.js |
| Syntax highlight | highlight.js |
| Terminal | xterm.js v5 + xterm-addon-fit |

## Arquitetura

```
golovebox.exe
  ├── Gateway        — Telegram, CLI
  ├── Agent Loop     — ReAct: Thought/Action/Parameters/Observation (texto puro, heredoc <<EOF)
  ├── Tools          — shell, files, github (GIT_ASKPASS + credential.helper), list_dir, git ops
  ├── Orchestrator   — specs DSL → DAG JSON via LLM (IDs snake_case, task auto-contida)
  ├── Memory         — chromem-go em .golovebox/memory/
  ├── Web UI         — chi + SSE + WebSocket + embed.FS; Alpine.js + Cytoscape + xterm.js
  └── Sandbox        — QEMU Alpine VM, SSH :2222 (pool com retry dial), QMP :4444
```

## Estrutura de Módulos (FASE 27 — workspace Go)

```
golovebox/
├── go.work              ← workspace-only, sem go.mod
├── core/                ← contratos, zero deps         → core/AGENTS.md
├── promptlang/          ← DSL parser                   → promptlang/AGENTS.md
├── sandbox/             ← VM QEMU + core.Sandbox        → sandbox/AGENTS.md
├── toolskills/          ← skills registry + provisioner → toolskills/AGENTS.md
├── derivator/           ← seed → PromptGraph            → derivator/AGENTS.md
├── orchestrator/        ← Engine + dag/agent/tools/mem  → orchestrator/AGENTS.md
└── app/                 ← composition root + UI + CLI   → app/AGENTS.md
    ├── magefile.go
    ├── cmd/golovebox/main.go
    └── internal/{config,embed,gateway,llm,setup,web}/
```

**Regra de dependência**: setas só apontam para `core`. `app` importa todos. Ninguém importa `app`.
**Contexto por módulo**: leia `<modulo>/AGENTS.md` + `core/contracts.go`. Não precisa do resto.
**Mapa de contratos**: `core/CONTRACTS.md` — interface → implementador → consumidores.

## Runtime — .golovebox/

Detalhes completos em `FASES.json:runtime_layout`. Resumo:

```
.golovebox/
├── config.toml          # llm/github/telegram + [workflow] + (V1) [agents]
├── qemu/                # flat: qemu-system-x86_64.exe + DLLs + share/qemu/
├── vm/                  # base.img, cidata.iso, id_rsa, qemu.log
├── memory/, skills/
└── runs/<id>/
    ├── dag.json, node_states.json, task.txt, run_meta.json
    ├── logs/<nodeID>.jsonl   # iter/action/params/obs/prompt/reply/ts
    └── artifacts/
```

## Primeiro Boot

`golovebox init` — 7 steps idempotentes. Detalhes em `FASES.json` (FASE 5/7).
Resumo: extrai QEMU + Alpine qcow2 → gera keypair → CIDATA ISO com SSH key + `.gitconfig` (com `credential.helper=store`) + sshd drop-in → config wizard → boot silencioso → smoke test SSH.

## Web UI

`golovebox web` em `localhost:8080`. Layout em duas colunas + bottom panel:
- **Esquerda**: chat (status + 4 health dots auto-poll 5s + messages + textarea + runs)
- **Direita**: DAG canvas (Cytoscape dagre) + node panel deslizável + bottom panel (terminal/files com tabs)

**Node panel** (FASE 16/18):
- Objetivo do run + descrição do node + branch atual
- Por iter ReAct: blocos colapsáveis `Prompt enviado` (azul) e `Resposta do LLM` (verde), params, obs
- Box vermelho com erro; auto-abre quando node falha
- Persistido — recarregar página mantém histórico

## Agent Loop

```go
type ProgressFunc func(iter int, action, params, obs, prompt, reply string)
```

- Texto puro — portável entre todos os LLM providers
- Heredoc para valores multilinha: `content: <<EOF ... EOF`
- max 20 iterações por node
- Output → NodeLog buffer + .jsonl + SSE broadcast

## Decisões Vivas

Promovidas pra `FASES.json:enduring_decisions`. Resumo do que é mais usado:

- **Cytoscape, não Mermaid**: updates incrementais via `.data()` sem re-render
- **Alpine, não htmx**: backend retorna JSON, não HTML fragments
- **chi**: sub-routers, Recoverer, pronto pra WebSocket/SFTP
- **slog stdlib**: zero deps, JSON no daemon
- **QEMU weilnetz.de**: única fonte Windows com todas DLLs
- **Alpine NoCloud qcow2**: boot direto, sem install (eliminou 10min de wait)
- **HTTP próprio para LLM**: BaseURL swappável, zero provider lock-in
- **MD-first memory**: .ai/ é fonte de verdade, vetores derivados
- **Credential helper persistente**: cloud-init + ~/.git-credentials — `git push` via shell tool funciona sem GIT_ASKPASS
- **Pool dial retry**: handshake fresco durante `rc-service sshd restart` precisa de retry, não só keepalive idle

## Status das Fases

V0 completo (1-18). Detalhes em `FASES.json:phases.v0_done`.
V1 em planejamento (5 fases). Detalhes em `FASES.json:phases.v1_planned`.

## Workflow de Documentação

1. **Implementando uma fase**: cria `.ai/fases/FASE_N.md` seguindo `FASE_TEMPLATE.md`. Bug fixes pontuais (15/16/17/18) seguem o mesmo formato.
2. **Ao final da fase**: arquivo permanece em `.ai/fases/` enquanto for "recente".
3. **A cada 3-5 fases**: digestão — releia os recentes, atualize `FASES.json`. Mantenha em `FASES.json` apenas o que ainda informa o presente.
4. **Lendo o projeto pela primeira vez**: leia `FASES.json` + `VISION.md` + `hypercontext.json` + este arquivo. Os `FASE_N.md` históricos só sob demanda.

## Estilo de Resposta

- Direto, sem preâmbulo
- Código limpo, DRY, KISS, YAGNI
- Quando criar arquivo: cria, não explica
- Quando editar: edita, não narra
