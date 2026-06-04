# FASE 30 — Deduplicação de Documentação

**Status:** concluída  
**Data:** 2026-06-04

## Objetivo

Uma fonte de verdade por preocupação. Eliminar duplicação entre docs. Leitores seguem links em vez de encontrar informação repetida e possivelmente divergente.

## Princípio

Mesma regra do código: cada fato vive em um único lugar. Outros documentos linkam, nunca copiam.

| Tipo de informação | Dono |
|---|---|
| Fato vivo (decisão, lib, aprendizado) | `FASES.json` |
| Direção de produto e V1 | `hypercontext.json` |
| Spec de módulo (contrato, API) | `<módulo>/AGENTS.md` |
| DSL keywords e semântica | `promptlang/DSL.md` |
| Arquitetura de módulos | `README.md` |
| Narrativa de produto | `VISION.md` |
| Requisitos funcionais | `PRD.md` |

## O que foi feito

### Criado

**`promptlang/DSL.md`** — spec canônica da DSL de intenção.
- Todas as keywords: NEW BRANCH, PUSH, PR, ATTENTION_HERE, PAUSE_TO_REVIEW, RUN_TEST, TRY/OR_ELSE, WHEN/DO, NOTIFY_ME, NOT_TODO
- Invariante de ordenação de workflow (FASE 19)
- Notas de implementação linkando para parser.go/adapter.go/keywords.go
- DAG de exemplo com sync_repo

### Editado

**`.ai/VISION.md`** — reduzido a pitch + links.
- Removido: tabelas de stack, diagrama de arquitetura monolítica, spec completa da DSL
- Adicionado: links para README.md (arquitetura), promptlang/DSL.md (DSL), hypercontext.json (roadmap V1)

**`.ai/PRD.md`** — requisitos limpos, sem duplicação.
- Corrigido status stale: "phases 1-9" → "29 fases completas, link para FASES.json"
- Removido: diagrama de arquitetura monolítica (V0 e V1) — substituído por link para README.md
- Removido: tabela de stack técnica (~25 linhas) — substituída por link para AGENTS.md
- Removido: tabela de keywords DSL — substituída por link para promptlang/DSL.md
- Removido: roadmap V0 com status stale (phases 10-13 "🔜") — substituído por "29 fases, ver FASES.json"
- Removido: roadmap V1 com fases — substituído por link para hypercontext.json

**`.ai/AGENTS.md`** — transformado em router puro.
- Removido: tabelas de stack backend e frontend (~25 linhas duplicadas de PRD.md)
- Removido: diagrama de arquitetura monolítica (`golovebox.exe` com `├──`)
- Removido: seção "Decisões Vivas" com lista de 10 bullets — substituída por link para FASES.json
- Corrigido: status "V0 completo (1-18)" → "V0 completo (1-29)"
- Adicionado: seção "Regra de Documentação" com tabela de ownership

**`.ai/hypercontext.json`**
- `v0_status.phases_done`: 18 → 29
- `v0_status.summary`: "18 fases" → "29 fases"
- `ai_folder_convention.structure`: adicionado `promptlang/DSL.md`
- AGENTS.md description atualizado para "router para os demais docs"

**`README.md`**
- "Project structure": substituída seção com layout monolítico antigo (`internal/`) pelo workspace de 7 módulos
- "V0 — All 18 phases delivered" → "All 29 phases delivered"

### Deletado

**`.ai/GENERAL.md`** — conteúdo stale (status phase 9, roadmap V1 antigo, referências a CONTEXT_v5.json e hypercontext_v2.json inexistentes). Todo conteúdo único (comandos, build steps, web UI) já estava no README.md com mais detalhe.

## Arquivos

| Ação | Arquivo |
|---|---|
| Criar | `promptlang/DSL.md` |
| Editar | `.ai/VISION.md` |
| Editar | `.ai/PRD.md` |
| Editar | `.ai/AGENTS.md` |
| Editar | `.ai/hypercontext.json` |
| Editar | `README.md` |
| Deletar | `.ai/GENERAL.md` |

## Definição de Pronto

Cada preocupação tem um único dono. Outros documentos linkam em vez de copiar. GENERAL.md eliminado. Status e contagem de fases consistentes em todos os docs.
