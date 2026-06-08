# FASE 19 — Decomposição Modular

**Tipo:** Refactor arquitetural (não-feature)
**Motivo:** Acoplamento total. 6 conceitos num só módulo. Tudo mexe em tudo. Nada funciona.
**Estratégia:** Strangler fig. Zero reescrita. Extração incremental atrás de contratos.

---

## Diagnóstico

golovebox acumula 6 produtos distintos num único módulo Go:

| Produto | Responsabilidade real |
|---|---|
| sandbox (ex-glovebox QEMU) | Execução isolada em VM |
| toolskills | Provisionar recursos Linux via metadados de skill |
| derivator | Prompt → grafo de prompts priorizados |
| promptlang | Metalinguagem / keywords DSL |
| orchestrator | Orquestrar CLIs de código |
| app (ex-glovebox UI) | Colar tudo, UI, wiring |

Problema: um único `go.mod`, imports cruzados, sem fronteira. Mudar `promptlang` quebra `orchestrator`. Testar `sandbox` exige subir tudo. Impossível evoluir em paralelo.

---

## Princípio Diretor

**Regra de dependência única:** setas só apontam para `core` ou para camada inferior. Ninguém importa `app`. `app` importa todos.

```
              ┌─────────┐
              │  core   │  contratos + tipos. ZERO deps. Ninguém implementa.
              └────▲────┘
                   │ (todos importam core)
   ┌────────┬──────┼──────┬───────────┬────────────┐
   │        │      │      │           │            │
sandbox toolskills derivator promptlang orchestrator
   ▲        ▲      ▲      ▲           ▲
   └────────┴──────┴──┬───┴───────────┘
                      │ (app importa todos)
                  ┌───┴───┐
                  │  app  │  composition root. Único que conhece tudo.
                  └───────┘
```

`core` não importa nada do projeto. Os 5 módulos importam só `core`. `app` faz o wiring.

---

## Estrutura Alvo (monorepo, Go workspace)

```
golovebox/
├── go.work                  # tie dos módulos para dev local
├── core/
│   ├── go.mod
│   └── contracts.go         # interfaces puras + tipos de domínio
├── sandbox/
│   ├── go.mod               # importa core
│   └── ...                  # QEMU, SSH pool, QMP
├── toolskills/
│   ├── go.mod               # importa core
│   └── ...
├── derivator/
│   ├── go.mod               # importa core
│   └── ...                  # prompt → PromptGraph
├── promptlang/
│   ├── go.mod               # importa core (idealmente zero deps externas)
│   └── ...                  # parser DSL → AST de intenção
├── orchestrator/
│   ├── go.mod               # importa core
│   └── ...                  # CodingAgent adapters, DAG executor
└── app/
    ├── go.mod               # importa TODOS
    └── cmd/golovebox/main.go  # composition root + Web UI + Gateway
```

Cada módulo: seu `go.mod`, seus testes, seu `AGENTS.md`. Compila e testa isolado.

---

## Contratos (core/contracts.go)

Interfaces minimalistas. Domínio fala em tipos, não em implementações.

```go
package core

import "context"

// --- Substrate de execução (sandbox) ---
type Sandbox interface {
	Exec(ctx context.Context, cmd string) (Output, error)
	PutFile(ctx context.Context, path string, data []byte) error
	Ready(ctx context.Context) bool
}

// --- Provisionamento (toolskills) ---
type Provisioner interface {
	Provision(ctx context.Context, skill SkillMeta) error
}

// --- Derivação de prompts (derivator) ---
type Deriver interface {
	Derive(ctx context.Context, seed string) (PromptGraph, error)
}

// --- Metalinguagem (promptlang) ---
type Parser interface {
	Parse(src string) (Intent, error) // keywords → AST de intenção
}

// --- Orquestração de CLI (orchestrator) ---
type CodingAgent interface {
	Delegate(ctx context.Context, spec Spec, repoPath string) (Report, error)
}

// Tipos de domínio compartilhados (Output, SkillMeta, PromptGraph,
// Intent, Spec, Report) vivem aqui — sem nenhuma dependência externa.
```

Quem precisa de um sandbox recebe `core.Sandbox` por injeção. Nunca o `*QEMU` concreto. Dependency inversion — o módulo de alto nível não conhece a implementação.

---

## Ordem de Extração (strangler fig)

Extrair folha primeiro (poucas deps), substrate depois, cola por último.

| # | Módulo | Por quê primeiro/depois |
|---|---|---|
| 1 | `core` | Define os contratos. Pré-requisito de tudo. |
| 2 | `promptlang` | Parser puro, zero deps externas. Mais fácil isolar e testar. |
| 3 | `derivator` | Depende só de LLM client + core. |
| 4 | `sandbox` | Substrate. Já bem delimitado (QEMU/SSH). |
| 5 | `toolskills` | Consome sandbox via interface core.Sandbox. |
| 6 | `orchestrator` | Consome todos via interfaces. |
| 7 | `app` | O que sobra. Wiring + UI + gateway. |

Regra: extrair um módulo = mover código + criar `go.mod` + fazer compilar atrás da interface + testes passando. Só então o próximo. **Nunca dois ao mesmo tempo.**

---

## Como Swarmar

Decomposição habilita paralelismo real:

1. **Contrato congelado = fronteira de coordenação.** Mudança em `core` exige acordo. Dentro do módulo, liberdade total.
2. **Um agente/CLI por módulo.** Cada `<modulo>/AGENTS.md` descreve só aquele módulo + a interface que cumpre.
3. **Integração só via `core`.** Módulos nunca importam o concreto um do outro. Mocks de `core.X` para testar isolado.
4. **CI por módulo.** `go test ./...` dentro de cada `go.mod`. Falha localizada, não global.

Isso elimina "tudo mexe em tudo": só `core` é compartilhado, e ele muda raramente.

---

## Documentação — também decompor

O acoplamento de docs espelha o de código. Hoje cada `.md` fala dos 6.

- `core/` → contratos versionados (a fonte de verdade das fronteiras).
- Cada módulo → seu `AGENTS.md`, seu `FASES.json` local.
- Raiz → só visão e mapa dos módulos.

---

## Definição de Pronto

> `go.work` com 7 módulos. `core` sem deps de projeto. Cada módulo compila e testa isolado. `app` faz o wiring por injeção de `core.*`. Trocar a implementação de um módulo (ex: sandbox QEMU → Docker) não toca em nenhum outro módulo, só no composition root.

---

## Anti-padrões a Evitar

- **Big-bang rewrite.** Extrair tudo de uma vez = quebrar tudo de uma vez.
- **core importar implementação.** core só interfaces + tipos. Se importou QEMU, errou.
- **Import cruzado entre módulos.** sandbox nunca importa orchestrator. Só core.
- **Interface vazando detalhe.** `Sandbox` não expõe `*ssh.Client`. Expõe `Exec`.
- **Antecipar (YAGNI).** Só os 5 contratos que existem hoje. Sem `Plugin`, sem `Registry` genérico especulativo.
