# FASE 24 — Extração: toolskills

**Status:** concluída  
**Data:** 2026-06-01

## Objetivo

Mover `internal/skills/` para módulo próprio `toolskills/`, implementando `core.Provisioner`. Marco: primeiro módulo que **consome** `core.Sandbox` por injeção — sem importar o tipo concreto `sandbox.VM`. Prova de cross-module injection via `core`.

## O que foi feito

### `core/contracts.go` — adicionados

```go
type SkillMeta struct {
    Name        string
    Description string
    Tools       []string
    Provision   []string // shell commands to run in the VM
}

type Provisioner interface {
    Provision(ctx context.Context, skill SkillMeta) error
}
```

### Módulo `toolskills/`

| Arquivo | Conteúdo |
|---|---|
| `go.mod` | `module github.com/user/golovebox/toolskills`, deps: core + BurntSushi/toml |
| `registry.go` | Movido de `internal/skills/registry.go`. `Skill` embute `core.SkillMeta` e adiciona `Examples`, `Prompt`, `FilePath` (campos de runtime não pertencentes ao core). |
| `provisioner.go` | **Novo.** Implementa `core.Provisioner` via `provisioner{sb core.Sandbox}`. `NewProvisioner(sb core.Sandbox) core.Provisioner`. |
| `generator.go` | Movido de `internal/skills/generator.go`. Aceita `CompleteFn func(ctx, prompt) (string, error)` em vez de `*llm.Client` — desacopla toolskills de internal/llm. |
| `registry_test.go` | **Novo.** 4 testes: parse frontmatter TOML → `core.SkillMeta`, provisioner executa comandos em ordem, falha em ExitCode ≠ 0, não chama Exec se Provision vazio. Mock `mockSandbox` implementa `core.Sandbox` sem VM real. |

### Consumidores migrados

- **`internal/web/server.go`** — `*skills.Registry` → `*toolskills.Registry`; `skills.Generate(ctx, llmClient, ...)` → adapter `CompleteFn` inline + `toolskills.Generate`.
- **`cmd/golovebox/main.go`** — `skills.NewRegistry` → `toolskills.NewRegistry`; `skills.Generate` → `CompleteFn` adapter + `toolskills.Generate`.
- `go.work` → 5 módulos; `go.mod` raiz → `require + replace toolskills`.

### Limpeza

`internal/skills/` deletado após migração.

## Decisões técnicas

**`Skill` embute `core.SkillMeta`**: o core define o contrato mínimo (Name/Description/Tools/Provision). `toolskills.Skill` adiciona campos de runtime (Prompt/FilePath/Examples) que são detalhes de carregamento, não contratos de domínio.

**`CompleteFn` em vez de interface LLM**: `generator.go` não tem consumidor que precise de `core.LLMClient` ainda — uma func injetada é suficiente e evita criar contrato especulativo no core (YAGNI). Se um segundo consumidor aparecer, promove para `core.Completer`.

**Mock sandbox no teste**: `mockSandbox` implementa `core.Sandbox` com 4 métodos. Confirma que `toolskills` testa sem VM real e sem importar `sandbox`.

## Invariantes preservados

- `toolskills` não importa `sandbox` nem `internal/*` (verificado via grep).
- `CGO_ENABLED=0` — sem CGO no módulo.
- `go test ./...` passa isolado com `GOWORK=off`.
