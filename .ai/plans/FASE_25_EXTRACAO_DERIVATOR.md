# FASE 25 — Extração: derivator (+ promoção core.Completer)

**Tipo:** Refactor arquitetural (quinta execução da FASE 21)
**Depende de:** FASE 24 (`toolskills` extraído, `CompleteFn` adapter, padrão de mock + `GOWORK=off`)
**Escopo:** Módulo `derivator`. + Promoção de `CompleteFn` → `core.Completer` (2º consumidor de LLM).
**Marco:** Primeiro contrato de LLM no core, disparado por regra da FASE 24 ("se um 2º consumidor aparecer, promove").

---

## Objetivo

Extrair a responsabilidade de **derivação de prompts** (seed → grafo de prompts priorizados: críticos, habilitantes, acessórios) para módulo próprio `derivator`, atrás de `core.Deriver`. Formalizar o contrato de LLM no core, agora que há 2 consumidores (`toolskills.generator` + `derivator`).

```
ANTES                          DEPOIS
golovebox/                     golovebox/
├── go.work (5)                ├── go.work (6)
├── core/                      ├── core/           ← + Deriver, PromptGraph, Completer
├── promptlang/                ├── promptlang/
├── sandbox/                   ├── sandbox/
├── toolskills/                ├── toolskills/     ← generator usa core.Completer
└── go.mod (legado)            ├── derivator/go.mod ← novo
   └── (lógica de derivação    └── go.mod (legado) ← consome core.Deriver
        hoje difusa)
```

## Regras absolutas
- `CGO_ENABLED=0`, build via `mage build`
- `derivator` importa só `core`
- core continua zero-dep de projeto (só stdlib)
- `go test` isolado passa com `GOWORK=off` (padrão FASE 24)

---

## 0. Localizar a lógica de derivação

Diferente de sandbox/skills, `derivator` pode **não existir** como pacote dedicado. A derivação prompt→grafo hoje está difusa (provavelmente embrionária ou parcialmente dentro de `internal/orchestrator` no passo specs→DAG via LLM).

Antes de extrair, mapear:
- Se há código de "derivar prompts priorizados" no orchestrator → extrair essa fatia.
- Se não há → FASE 25 **formaliza o contrato + impl mínima**, sem inventar features (YAGNI). O módulo nasce com `Deriver` + uma implementação simples (1 chamada LLM → parse para `PromptGraph`), pronta para o orchestrator consumir.

Não misturar com a geração de DAG do orchestrator — derivação é prompt→grafo de prompts; orquestração é intent→DAG de execução. Fronteiras distintas.

---

## 1. Contratos no core

`core/contracts.go` — adicionar:

```go
package core

import "context"

// Completer — abstração mínima de LLM. Promovido de toolskills.CompleteFn
// agora que derivator é o 2º consumidor (regra YAGNI da FASE 24).
type Completer interface {
	Complete(ctx context.Context, prompt string) (string, error)
}

// PromptNode — prompt priorizado dentro do grafo.
type PromptNode struct {
	ID       string
	Text     string
	Priority PromptPriority
	DependsOn []string // habilitação: este prompt depende destes
}

type PromptPriority string

const (
	PriorityCritical  PromptPriority = "critical"  // crítico
	PriorityEnabling  PromptPriority = "enabling"   // habilitante
	PriorityAccessory PromptPriority = "accessory"  // acessório
)

// PromptGraph — saída do Deriver.
type PromptGraph struct {
	Nodes []PromptNode
}

// Deriver — seed → grafo de prompts priorizados.
type Deriver interface {
	Derive(ctx context.Context, seed string) (PromptGraph, error)
}
```

`Completer` é a abstração mínima (prompt entra, texto sai) — sem vazar HTTP/provider.

---

## 2. Módulo `derivator`

`derivator/go.mod`:
```
module github.com/user/golovebox/derivator

go 1.22

require github.com/user/golovebox/core v0.0.0
```
+ `replace ... core => ../core` (padrão das fases anteriores).

`derivator/derivator.go` — implementa `core.Deriver`:
```go
type deriver struct{ llm core.Completer }

func New(llm core.Completer) core.Deriver { return &deriver{llm} }

func (d *deriver) Derive(ctx context.Context, seed string) (core.PromptGraph, error) {
	// 1 chamada Complete com prompt de derivação → resposta estruturada
	// (JSON) → parse para core.PromptGraph (nodes priorizados + deps).
}
```
- Recebe `core.Completer` por injeção — não conhece `internal/llm`.
- Prompt de derivação + parser da resposta vivem no módulo.
- Sem dependência de sandbox, dag, orchestrator.

`derivator/derivator_test.go`:
- Mock `core.Completer` retornando JSON canônico → verifica parse para `PromptGraph` (prioridades + deps corretas).
- Caso resposta malformada → erro claro.
- `GOWORK=off go test ./...` passa isolado.

`go.work` → adicionar `./derivator`.

---

## 3. Refatorar toolskills para core.Completer

`toolskills/generator.go` — `CompleteFn func(ctx, prompt) (string, error)` → `core.Completer`. O adapter inline some; passa-se a interface. Atualizar `toolskills/registry_test.go` (mock vira `core.Completer`).

Mudança mínima: `CompleteFn(ctx, p)` → `c.Complete(ctx, p)`.

---

## 4. Adaptar o legado

- `internal/llm/client.go` — passa a satisfazer `core.Completer` (já tem método de completar; alinhar assinatura ou adapter fino no composition root).
- `internal/orchestrator/` — se consumir derivação, injeta `core.Deriver`; senão, sem mudança nesta fase.
- `internal/web/server.go` e `cmd/golovebox/main.go` — wiring: construir `derivator.New(llmClient)` e injetar onde a derivação for usada. Atualizar a construção de `toolskills.Generate` para passar `core.Completer` em vez de `CompleteFn`.

---

## 5. Build

`magefile.go`: confirmar cobertura de `derivator` no workspace. `CGO_ENABLED=0` preservado.

---

## Arquivos

| Ação | Arquivo |
|---|---|
| Editar | `core/contracts.go` (+ Completer, PromptGraph, PromptNode, Deriver) |
| Criar | `derivator/go.mod`, `derivator/derivator.go`, `derivator/derivator_test.go` |
| Editar | `toolskills/generator.go` (CompleteFn → core.Completer) |
| Editar | `toolskills/registry_test.go` (mock → core.Completer) |
| Editar | `internal/llm/client.go` (satisfazer core.Completer) |
| Editar | `internal/web/server.go`, `cmd/golovebox/main.go` (wiring + Completer) |
| Editar | `go.work` (+ ./derivator), `magefile.go` |

---

## Verificação

```bash
cd derivator && GOWORK=off go test ./... && go vet ./...  # importa só core
cd ../toolskills && GOWORK=off go test ./...              # usa core.Completer
cd .. && mage build
```

1. `derivator` compila isolado; grep nos imports não acha `internal/*` nem outro módulo (só core).
2. `Derive` com mock `core.Completer` → `PromptGraph` com prioridades critical/enabling/accessory e deps corretas.
3. `toolskills` segue passando com mock atualizado para `core.Completer`.
4. `internal/llm` satisfaz `core.Completer` (compila como tal).
5. Build do binário único ok.

---

## Definição de Pronto

> `go.work` com 6 módulos. `derivator` implementa `core.Deriver`, consome `core.Completer` por injeção, testa isolado. `core.Completer` promovido (2º consumidor). `toolskills` migrado para `core.Completer`. Trocar provider LLM toca só `internal/llm` + wiring. Derivação separada da orquestração.

---

## Anti-padrões (desta fase)

- **Misturar derivação com geração de DAG.** Deriver: prompt→grafo de prompts. Orchestrator: intent→DAG de execução. Não fundir.
- **`derivator` importar `internal/llm`.** Recebe `core.Completer`. Sempre.
- **`Completer` vazar HTTP/provider.** Só `Complete(ctx, prompt) (string, error)`.
- **Inventar features no derivator.** Se a lógica não existe hoje, impl mínima. YAGNI.
- **Extrair orchestrator junto.** FASE 26.

---

## Próxima Fase (FASE 26)

Extrair `orchestrator` (intent → DAG, CodingAgent adapters, executor) atrás de `core.CodingAgent` — consome `core.Sandbox`, `core.Deriver`, `core.Parser`. Penúltimo passo antes de isolar `app`.
