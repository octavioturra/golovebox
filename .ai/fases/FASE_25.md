# FASE 25 — Extração: derivator (+ promoção core.Completer)

**Status:** concluída  
**Data:** 2026-06-01

## Objetivo

Extrair a responsabilidade de derivação de prompts (`seed → PromptGraph priorizado`) para módulo próprio `derivator`, atrás de `core.Deriver`. Formalizar o contrato LLM no core como `core.Completer` — disparado pela regra da FASE 24 ("se um 2º consumidor aparecer, promove").

## Análise prévia

O orchestrator já converte `intents → DAG` via LLM — essa é orquestração. Derivação (`seed → grafo de prompts priorizados`) é responsabilidade distinta, ainda não implementada. FASE 25 cria a impl mínima sem inventar features além do contrato especificado.

## O que foi feito

### `core/contracts.go` — adicionados

```go
// Completer — abstração mínima de LLM (prompt → reply).
type Completer interface {
    Complete(ctx context.Context, prompt string) (string, error)
}

type PromptPriority string
const (
    PriorityCritical  PromptPriority = "critical"
    PriorityEnabling  PromptPriority = "enabling"
    PriorityAccessory PromptPriority = "accessory"
)

type PromptNode struct { ID, Text string; Priority PromptPriority; DependsOn []string }
type PromptGraph struct { Nodes []PromptNode }

type Deriver interface {
    Derive(ctx context.Context, seed string) (PromptGraph, error)
}
```

### Módulo `derivator/`

| Arquivo | Conteúdo |
|---|---|
| `go.mod` | `module github.com/user/golovebox/derivator`, deps: só core |
| `derivator.go` | `deriver{llm core.Completer}` implementa `core.Deriver`. 1 chamada LLM → JSON → `PromptGraph`. Strips markdown fences. Valida prioridades. |
| `derivator_test.go` | 4 testes: parse correto de prioridades+deps, strip de fences, erro em JSON malformado, erro em prioridade desconhecida. Mock `core.Completer`. |

### `internal/llm/completer.go` — novo

```go
func NewCompleter(c *Client) core.Completer
```

Adapter que wraps `*llm.Client` para `core.Completer`. Cada prompt vira um `[]Message{{Role: "user", Content: prompt}}`.

### `toolskills/generator.go` — migrado

`CompleteFn func(ctx, prompt) (string, error)` → `core.Completer`. Callers passam `llm.NewCompleter(llmClient)`.

### Consumers atualizados

- `internal/web/server.go`: `completeFn` inline removido → `llm.NewCompleter(s.llmClient)`
- `cmd/golovebox/main.go`: idem

## Decisões técnicas

**`Completer` mínimo**: só `Complete(ctx, prompt) (string, error)`. Sem vazar `[]Message`, HTTP, model name. Trocar provider = trocar `internal/llm` + adapter.

**Derivação ≠ Orquestração**: o orchestrator transforma `[]Intent → DAG de execução`. O deriver transforma `seed text → grafo de prompts priorizados` — escala diferente, consumidor diferente. Não fundidos.

**Prompt JSON para PromptGraph**: estrutura simples `[{id, text, priority, depends_on}]` — parseável de forma determinista, sem ambiguidade de frontmatter ou delimitadores custom.

## Invariantes preservados

- `derivator` importa só `core` (verificado via grep — nenhum `internal/` nos imports).
- `toolskills` não regrediu — 4 testes passam com `GOWORK=off`.
- `CGO_ENABLED=0` preservado.
- `go.work` com 6 módulos: core, promptlang, sandbox, toolskills, derivator, app.
