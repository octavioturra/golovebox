# FASE 31 — Git Determinístico

**Status:** concluída  
**Data:** 2026-06-05

## Objetivo

Git nunca funcionou de forma confiável no V0 porque a responsabilidade de branch/push/PR estava distribuída entre DSL, orchestrator, LLM e agent loop. Decisão: **tirar git do domínio cognitivo**. Branch, commit, push e PR são operações de infraestrutura — o código faz, sempre, deterministicamente.

## Decisão de Arquitetura

| Antes | Depois |
|---|---|
| `NEW BRANCH`, `PUSH`, `PR` na DSL | Removidos da DSL |
| LLM decide quando criar branch | Engine cria branch no início de todo run |
| LLM decide quando fazer push | Engine faz commit+push ao final de todo run |
| PR aberto via node do DAG | Botão na UI — usuário decide quando abrir |
| `enforceWorkflowOrdering` como paliativo | Deletado — problema resolvido na raiz |

## O que foi feito

### Removido da DSL

**`promptlang/keywords.go`**: Remove `KwNewBranch`, `KwPush`, `KwPR` e entradas em `allKeywords`.

**`promptlang/parser.go`**: Remove `KwPush` de `noArgKeywords`, remove `KwNewBranch`/`KwPR` de `spaceArgKeywords`.

**`promptlang/adapter.go`**: Remove cases `KwNewBranch`, `KwPush`, `KwPR` do `annotationToStep`.

### Removido dos contratos

**`core/contracts.go`**: Remove `KindBranch`, `KindPush`, `KindPR` de `StepKind`. Adiciona `CurrentBranch string` em `RunConfig`.

**`orchestrator/dag/dag.go`**: Remove `TypeBranch`, `TypePush`, `TypePR`. Mantém `TypeSyncRepo`, `TypeCommit`.

### Engine — git determinístico

**`orchestrator/branch.go`** (novo): `GenerateBranchName()` — gera `glb-<adj>-<noun>-<4hex>`. 40 adjetivos × 40 substantivos.

**`orchestrator/branch_test.go`** (novo): Testa formato (4 partes, prefixo `glb`, hex de 4 chars) e ausência de colisão em 1000 chamadas.

**`orchestrator/engine.go`** — `Run`:
- Pré-DAG: `GenerateBranchName()` + `tools.ExecBranch` → salva em `run_meta.json`
- Pós-DAG (só se sucesso): `tools.ExecCommitPush` → `run_state=pushed`
- Falha no commit+push: `slog.Error` + `run_state=git_error`, sem propagar (artefatos preservados)

**`orchestrator/tools/git.go`**: Adiciona `ExecCommitPush(ctx, sb, token, clonePath, branch, msg)` — chama `ExecCommit` + `ExecPush` em sequência.

### Orchestrator — LLM simplificado

**`orchestrator/orchestrator.go`**:
- Remove `enforceWorkflowOrdering` (~200 linhas) — razão de existir eram os nodes branch/push/pr
- Remove dispatch de `TypeBranch`, `TypeCommit`, `TypePush`, `TypePR` do `engine.go`
- Atualiza `validNodeType` — remove branch/push/pr do conjunto válido
- Reescreve `buildPlanningPrompt` — instrui LLM a não gerar nodes de git; tipos válidos: `task|checkpoint|gate|notify|wait_event|try_else`
- Remove output de anotações KindBranch/KindPR no prompt de planejamento

**`orchestrator/ordering_test.go`**: Substituído por comentário — testava `enforceWorkflowOrdering` deletado.

### Web — endpoint PR + UI

**`app/internal/web/server.go`**:
- `POST /api/runs/{id}/pr` — `handleOpenPR`: lê branch de `run_meta.json`, chama `ExecPR`, salva `pr_url`
- `GET /api/runs/{id}/pr/suggest` — `handleSuggestPR`: uma call LLM sugere título+body; fallback a task text se falhar
- `handleGetRun` expõe `run_state` e `pr_url`

**`app/internal/web/static/index.html`**:
- `loadDAG`: filtra nodes `sync_repo` e edges envolvendo `sync_repo` do canvas Cytoscape — infraestrutura não aparece
- `nodePanel`: novos campos `runState`/`prUrl`; badge verde `pushed` / vermelho `git_error`; botão "Abrir PR →" quando `pushed && !prUrl`; link "Ver PR ↗" quando `prUrl` existe
- `prModal` (novo Alpine component): abre com sugestão LLM pré-carregada; submit chama `POST /api/runs/{id}/pr`; evento `pr-opened` atualiza nodePanel

## Arquivos

| Ação | Arquivo |
|---|---|
| Criar | `orchestrator/branch.go`, `orchestrator/branch_test.go` |
| Editar | `core/contracts.go` |
| Editar | `promptlang/keywords.go`, `promptlang/parser.go`, `promptlang/adapter.go`, `promptlang/parser_test.go` |
| Editar | `orchestrator/dag/dag.go` |
| Editar | `orchestrator/engine.go` |
| Editar | `orchestrator/orchestrator.go` |
| Editar | `orchestrator/ordering_test.go` (esvaziado) |
| Editar | `orchestrator/tools/git.go` |
| Editar | `app/internal/web/server.go` |
| Editar | `app/internal/web/static/index.html` |

## Limitações Conhecidas

1. **Autenticação git na VM**: `ExecCommitPush` precisa de `credential.helper=store` + identidade git funcionais na VM. As FASEs 14/17 tentaram estabilizar isso — push ainda pode falhar em cenários reais. Estado vira `git_error` sem perder artefatos. Fase futura: auditoria completa do caminho git.
2. **Branch órfã em erro**: se o run falhar antes de produzir mudanças, a branch existe no remote sem commits. Aceitável.
3. **Commit message genérica**: primeiros 72 chars de `task.txt`. Sem LLM no caminho crítico — intencional.

## Definição de Pronto

Branch `glb-*` criada automaticamente antes do DAG. Commit+push automáticos ao final. LLM nunca decide sobre git. `enforceWorkflowOrdering` deletado. Botão "Abrir PR" aparece quando `run_state=pushed`. Modal com sugestão LLM. Canvas não mostra `sync_repo`. Todos os módulos compilam e testam verde.
