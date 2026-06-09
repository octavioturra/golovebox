# app — AGENTS.md

## Responsabilidade única
Composition root: instancia VM + web shell. Contém CLI (cobra), Web UI (chi + Alpine.js + xterm.js), setup wizard e assets embed (QEMU + Alpine qcow2).

## Este módulo NÃO tem contrato próprio
É o único módulo que importa todos os outros. Nenhum módulo importa `app`.

## Sub-pacotes internos
| Pacote | Função |
|---|---|
| `cmd/golovebox/` | Entry point. `main.go` é o único lugar que conhece todos os tipos concretos. |
| `internal/config/` | Parse de `config.toml`. Só usado dentro de `app`. |
| `internal/embed/` | `//go:embed` de QEMU + Alpine qcow2 + static/. Extração para `.golovebox/`. |
| `internal/llm/` | HTTP client OpenAI-compat. `NewCompleter(*Client) core.Completer` — adapter para injeção. |
| `internal/setup/` | Wizard de inicialização (`golovebox init`). |
| `internal/web/` | Servidor HTTP chi: health (VM), WebSocket terminal SSH, SFTP file explorer. |

## Composition root (`main.go`)
Responsável por:
- Resolver `sandbox.Config` a partir de `config.Config`
- Injetar `*sandbox.VM` onde `core.Sandbox` é esperado
- Bootar a VM e servir terminal SSH + SFTP via web

## Comandos CLI
| Comando | Função |
|---|---|
| `init` | Setup wizard (7 steps idempotentes) |
| `web` | Inicia VM + servidor web |
| `exec <cmd>` | Executa comando shell na VM via SSH |
| `reset` | Remove VM disk/cidata para reinicialização limpa |

## Dependências permitidas
- `core`, `sandbox`, `toolskills` (workspace)
- Todas as libs externas listadas em `app/go.mod`

## Build
```bash
cd app && CGO_ENABLED=0 mage build   # binário em build/
```

## Verificação local
```bash
cd app && GOWORK=off go vet ./...
```

## Regras absolutas
- CGO_ENABLED=0 (magefile.go aplica automaticamente)
- Zero paths hardcoded — `baseDir = filepath.Join(filepath.Dir(execPath), ".golovebox")`
- Zero escrita fora de `baseDir`
