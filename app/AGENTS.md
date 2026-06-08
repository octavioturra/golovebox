# app — AGENTS.md

## Responsabilidade única
Composition root: instancia todos os módulos concretos, monta o binário `golovebox`. Contém CLI (cobra), Web UI (chi + Alpine.js), gateway (Telegram/Slack/email), setup wizard e assets embed (QEMU + Alpine qcow2).

## Este módulo NÃO tem contrato próprio
É o único módulo que importa todos os outros. Nenhum módulo importa `app`.

## Sub-pacotes internos
| Pacote | Função |
|---|---|
| `cmd/golovebox/` | Entry point. `main.go` é o único lugar que conhece todos os tipos concretos. |
| `internal/config/` | Parse de `config.toml`. Só usado dentro de `app`. |
| `internal/embed/` | `//go:embed` de QEMU + Alpine qcow2 + static/. Extração para `.golovebox/`. |
| `internal/gateway/` | Telegram bot, Slack, email. Delegador fino para `orchestrator.Engine`. |
| `internal/llm/` | HTTP client OpenAI-compat. `NewCompleter(*Client) core.Completer` — adapter para injeção. |
| `internal/setup/` | Wizard de inicialização (`golovebox init`). |
| `internal/web/` | Servidor HTTP chi: SSE, WebSocket terminal, API runs, `RunStore`. |

## Composition root (`main.go`)
Responsável por:
- Resolver `sandbox.Config` a partir de `config.Config`
- Construir `orchestrator.Engine` via `buildEngine(sb, llmClient, cfg)`
- Montar `core.RunConfig` via `buildRunConfig(cfg)`
- Injetar `llm.NewCompleter(llmClient)` onde `core.Completer` é esperado
- Injetar `*sandbox.VM` onde `core.Sandbox` é esperado

## Dependências permitidas
- Todos os módulos do workspace (`core`, `promptlang`, `sandbox`, `toolskills`, `derivator`, `orchestrator`)
- Todas as libs externas listadas em `app/go.mod`

## Build
```bash
cd app && CGO_ENABLED=0 mage build   # binário em build/
go build ./...                        # verifica compilação
```

## Verificação local
```bash
cd app && GOWORK=off go vet ./...
```

## Regras absolutas
- CGO_ENABLED=0 (magefile.go aplica automaticamente)
- Zero paths hardcoded — `baseDir = filepath.Join(filepath.Dir(execPath), ".golovebox")`
- Zero escrita fora de `baseDir`
- `main.go` é o único ponto de acoplamento de concretos — gateway/web nunca importam `*sandbox.VM` diretamente
