# core/CONTRACTS.md — Mapa de Contratos

Lista cada interface/tipo de `core/contracts.go`, quem implementa e quem consome.
Este arquivo é a fonte de verdade das fronteiras — mantenha sincronizado com `contracts.go`.

---

## Interfaces

### `core.Sandbox`
**O quê:** Execução isolada de comandos (`Exec / PutFile / GetFile / Ready`).

| Papel | Módulo |
|---|---|
| Implementa | `sandbox` (`*sandbox.VM`) |
| Consome | `orchestrator` (engine + tools), `toolskills` (provisioner) |
| Injeta | `app` (composition root, via `*sandbox.VM`) |

---

### `core.Parser`
**O quê:** DSL spec → `core.Intent` (`Parse(src string) (Intent, error)`).

| Papel | Módulo |
|---|---|
| Implementa | `promptlang` |
| Consome | `orchestrator` (Plan) |
| Injeta | `app` |

---

### `core.Provisioner`
**O quê:** Executa comandos de setup na VM para uma skill (`Provision(ctx, SkillMeta) error`).

| Papel | Módulo |
|---|---|
| Implementa | `toolskills` |
| Consome | `app` (setup wizard, skill provisioning) |
| Injeta | `app` |

---

### `core.Completer`
**O quê:** Interface mínima de LLM (`Complete(ctx, prompt string) (string, error)`).

| Papel | Módulo |
|---|---|
| Implementa | `app/internal/llm` (`llm.NewCompleter(*Client) core.Completer`) |
| Consome | `orchestrator` (ReAct loop + planner), `toolskills` (generator), `derivator` |
| Injeta | `app` |

> Promovido de `CompleteFn` na FASE 25 quando `derivator` se tornou o 2º consumidor.

---

### `core.Deriver`
**O quê:** Seed text → grafo de prompts priorizados (`Derive(ctx, seed string) (PromptGraph, error)`).

| Papel | Módulo |
|---|---|
| Implementa | `derivator` |
| Consome | (ainda não consumido por `orchestrator` — YAGNI) |
| Injeta | `app` (disponível para uso futuro) |

---

### `core.CodingAgent`
**O quê:** Delega uma `Spec` de coding a um agente autônomo externo (`Delegate(ctx, Spec, repoPath string) (Report, error)`).

| Papel | Módulo |
|---|---|
| Implementa | (futuro — V1 fase1: adapters Claude Code / Codex / Gemini) |
| Consome | `orchestrator` (futuro) |
| Injeta | `app` (futuro) |

---

## Tipos de domínio chave

| Tipo | Usado por |
|---|---|
| `core.Intent` / `Step` / `StepKind` | `promptlang` (produz), `orchestrator` (consome) |
| `core.SkillMeta` | `toolskills` (registry), `core.Provisioner` |
| `core.PromptNode` / `PromptGraph` / `PromptPriority` | `derivator` (produz) |
| `core.Progress` | `orchestrator` (emite), `app/gateway` + `app/web` (consome) |
| `core.RunConfig` | `orchestrator` (lê), `app` (monta) |
| `core.Spec` / `Report` | `core.CodingAgent` (futuro) |
| `core.Output` | `core.Sandbox` (resultado de Exec) |

---

## Protocolo de mudança

Ver `core/AGENTS.md` §Regra de evolução.
Resumo: PR isolado → aditivo > breaking → ≥2 consumidores reais antes de promover → módulos afetados adaptam em paralelo.
