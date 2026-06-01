# FASE 21 — Decomposição Modular (Arquitetura)

**Tipo:** Decisão arquitetural / design  
**Entrega:** Princípio diretor + plano de extração. Sem código.

---

## Diagnóstico

golovebox V0 acumulou 6 produtos distintos num único `go.mod`:

| Produto | Responsabilidade |
|---|---|
| sandbox (QEMU) | Execução isolada em VM |
| toolskills | Provisionar recursos Linux via skill metadata |
| derivator | Prompt → grafo de prompts priorizados |
| promptlang | Metalinguagem / keywords DSL |
| orchestrator | Orquestrar CLIs de código |
| app (UI) | Wiring, UI, gateway |

Sintoma: mudar `promptlang` pode quebrar `orchestrator`. Testar `sandbox` exige subir tudo. Impossível evoluir os produtos em paralelo.

---

## Princípio Diretor

**Regra de dependência única:** setas só apontam para `core` ou para camada inferior.

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
                      │
                  ┌───┴───┐
                  │  app  │  composition root
                  └───────┘
```

`core` não importa nada do projeto. Módulos de produto importam só `core`. `app` faz o wiring.

---

## Estrutura Alvo (monorepo, Go workspace)

```
golovebox/
├── go.work
├── core/          go.mod — interfaces puras + tipos de domínio
├── sandbox/       go.mod — QEMU, SSH pool, QMP
├── toolskills/    go.mod — skills registry
├── derivator/     go.mod — prompt → PromptGraph
├── promptlang/    go.mod — parser DSL → AST de intenção
├── orchestrator/  go.mod — CodingAgent adapters, DAG executor
└── app/           go.mod — composition root + Web UI + Gateway (o que existe hoje)
```

---

## Contratos (rascunho inicial para FASE 22+)

```go
package core

type Sandbox interface {
    Exec(ctx context.Context, cmd string) (Output, error)
    PutFile(ctx context.Context, path string, data []byte) error
    Ready(ctx context.Context) bool
}
type Provisioner interface { Provision(ctx context.Context, skill SkillMeta) error }
type Deriver interface     { Derive(ctx context.Context, seed string) (PromptGraph, error) }
type Parser interface      { Parse(src string) (Intent, error) }
type CodingAgent interface { Delegate(ctx context.Context, spec Spec, repoPath string) (Report, error) }
```

Tipos de domínio (`Output`, `SkillMeta`, `PromptGraph`, `Intent`, `Spec`, `Report`) vivem em `core` — sem nenhuma dependência externa.

---

## Ordem de Extração (strangler fig)

Extrair folha primeiro. Nunca dois módulos ao mesmo tempo.

| # | Módulo | Motivo |
|---|---|---|
| 1 | `core` | Pré-requisito de tudo. |
| 2 | `promptlang` | Parser puro, zero deps externas. Prova o padrão. |
| 3 | `derivator` | Depende só de LLM client + core. |
| 4 | `sandbox` | Substrate bem delimitado (QEMU/SSH). |
| 5 | `toolskills` | Consome sandbox via interface core.Sandbox. |
| 6 | `orchestrator` | Consome todos via interfaces. |
| 7 | `app` | O que sobra. Wiring + UI + gateway. |

Regra: extrair = mover código + criar `go.mod` + compilar atrás da interface + testes passando. Só então o próximo.

---

## Anti-padrões a Evitar

- **Big-bang rewrite.** Extrair tudo de uma vez = quebrar tudo de uma vez.
- **`core` importar implementação.** core só interfaces + tipos.
- **Import cruzado entre módulos.** `sandbox` nunca importa `orchestrator`.
- **Interface vazando detalhe.** `Sandbox` não expõe `*ssh.Client`.
- **YAGNI.** Só contratos que existem hoje.

---

## Definição de Pronto

> `go.work` com 7 módulos. `core` zero-dep. Cada módulo compila e testa isolado. `app` faz wiring por injeção de `core.*`. Trocar implementação (ex: sandbox QEMU → Docker) não toca em nenhum outro módulo.
