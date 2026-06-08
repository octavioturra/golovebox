# FASE 28 — Swarm Enablement

**Status:** concluída  
**Data:** 2026-06-04

## Objetivo

Tornar cada módulo evoluível em paralelo por um agente próprio. Cada módulo passa a ser uma unidade de trabalho autossuficiente: doc própria, CI isolado, contrato congelado em `core`. Coordenação só no contrato.

## O que foi feito

### AGENTS.md por módulo (7 arquivos criados)

| Arquivo | Conteúdo |
|---|---|
| `core/AGENTS.md` | Responsabilidade, regra de evolução do contrato, protocolo aditivo |
| `promptlang/AGENTS.md` | DSL parser puro, `core.Parser`, zero I/O |
| `sandbox/AGENTS.md` | VM QEMU, `core.Sandbox` + `Interactive`, `sandbox.Config` |
| `toolskills/AGENTS.md` | Registry + `core.Provisioner`, injeção de `core.Sandbox` e `core.Completer` |
| `derivator/AGENTS.md` | Seed→PromptGraph, `core.Deriver`, 1 chamada LLM por Derive |
| `orchestrator/AGENTS.md` | Engine API completo, 2 canais de observabilidade, sub-pacotes |
| `app/AGENTS.md` | Composition root, sub-pacotes internos, composition root responsibilities |

### `core/CONTRACTS.md`

Lista cada interface (`Sandbox`, `Parser`, `Provisioner`, `Completer`, `Deriver`, `CodingAgent`), quem implementa, quem consome, quem injeta. Fonte de verdade das fronteiras — sincronizado com `contracts.go`.

### `.github/workflows/ci.yml` — 3 jobs

| Job | O quê |
|---|---|
| `module` (matriz 7×) | `GOWORK=off go vet + go test` em cada módulo isolado. `CGO_ENABLED=0`. |
| `boundary` | Verifica que nenhum módulo importa `app`, `core` não importa módulo de projeto, produtos não importam irmãos. |
| `integration` | `cd app && go build ./...` via workspace — garante que a composição monta. |

### `README.md` — mapa na raiz

Tabela de módulos com links para AGENTS.md, regra de dependência, mapa de contratos, comandos de build e resumo do modelo swarm. README continua com a doc completa do produto logo abaixo.

### `.ai/AGENTS.md` — seção de estrutura atualizada

Substitui a estrutura de pastas do monolito (`internal/`) pelo mapa de módulos do workspace, com links para os AGENTS.md por módulo.

## Invariantes verificados

```bash
# nenhum módulo importa app
! grep -rq 'golovebox/app' core/ promptlang/ sandbox/ toolskills/ derivator/ orchestrator/
# core não importa módulo de projeto
! grep -rEq 'golovebox/(promptlang|sandbox|toolskills|derivator|orchestrator|app)' core/
# go build passa
cd app && go build ./...
```

Todos passaram.

## Modelo de swarm habilitado

- **1 agente = 1 módulo.** Contexto mínimo: `<modulo>/AGENTS.md` + `core/contracts.go` + arquivos do módulo.
- **Mudança dentro de X** → liberdade total. CI de `module/X` valida.
- **Mudança em `core`** → PR isolado, aditivo, ≥2 consumidores reais. Módulos afetados adaptam em paralelo.
- **CI de `boundary`** → guardião automático da regra de dependência. PR que viola a seta → CI vermelho.

## Definição de Pronto

Cada módulo é uma unidade de trabalho autossuficiente: doc própria, CI isolado, contrato congelado em `core`. Um agente trabalha em um módulo com contexto mínimo. CI de boundary protege a regra de dependência automaticamente. **Swarm habilitado: N agentes, N módulos, coordenação só no contrato.**
