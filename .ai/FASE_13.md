# FASE 13 — Workflow DSL: Branch, Push, PR e Repo Sync

## Status: ✅ Entregue

---

## Arquivos Criados

| Arquivo | Função |
|---|---|
| `internal/tools/git.go` | `SyncRepo`, `ExecBranch`, `ExecPush`, `ExecPR`, `isGitConflict`, `writeAskpass` |

## Arquivos Modificados

| Arquivo | Mudança |
|---|---|
| `internal/config/config.go` | `WorkflowConfig` struct + `applyDefaults()` |
| `internal/dsl/keywords.go` | `KwNewBranch`, `KwPush`, `KwPR` adicionados |
| `internal/dsl/parser.go` | `noArgKeywords`, `spaceArgKeywords` — suporte a `PUSH` (sem arg) e `NEW BRANCH`/`PR` (arg sem colon) |
| `internal/dag/dag.go` | `TypeSyncRepo`, `TypeBranch`, `TypePush`, `TypePR`; `ErrNeedsHuman` sentinel |
| `internal/dag/executor.go` | `errors.Is(err, ErrNeedsHuman)` → `setStateWaiting` + espera aprovação |
| `internal/orchestrator/orchestrator.go` | Campo `cfg`; `sync_repo` injetado; `run_mode` no prompt; novos tipos no `validNodeType` |
| `internal/web/server.go` | Dispatch para `sync_repo`/`branch`/`push`/`pr`; import `tools` |
| `internal/web/store.go` | `SetRunMeta` / `GetRunMeta` via `run_meta.json` |
| `cmd/golovebox/main.go` | `orchestrator.New(llmClient, nil, cfg)` |

---

## Config — `[workflow]` section

```toml
[workflow]
default_repo    = "owner/repo"
clone_path      = "/root/repo"
run_mode        = "build_only"   # "build_only" | "full"
default_branch  = "main"
```

Defaults aplicados por `applyDefaults()` se ausentes:
- `clone_path` → `/root/repo`
- `run_mode` → `build_only`
- `default_branch` → `main`

---

## Novos Keywords DSL

| Keyword | Sintaxe | Node type |
|---|---|---|
| `NEW BRANCH` | `NEW BRANCH feature/x` | `branch` |
| `PUSH` | `PUSH` (linha só) | `push` |
| `PR` | `PR` ou `PR "título"` | `pr` |

Parsing: `PUSH` é `noArgKeywords` (sem colon, sem argumento). `NEW BRANCH` e `PR` são `spaceArgKeywords` (argumento após espaço).

---

## sync_repo — Pre-step Automático

Se `workflow.default_repo != ""`, o orchestrador injeta `sync_repo` como primeiro node.
Todos os nodes sem dependências passam a depender de `sync_repo`.

`tools.SyncRepo`:
- Se `/root/repo/.git` não existe → `git clone https://github.com/owner/repo`
- Se existe → `git pull origin main`
- Conflito detectado → retorna `fmt.Errorf("...: %w", dag.ErrNeedsHuman)`

---

## ErrNeedsHuman → waiting_human

Definido em `dag/dag.go`. Qualquer tool que retorne um erro wrappando `ErrNeedsHuman`
faz o executor:
1. Guardar o erro em `node.Error`
2. `setStateWaiting(n)` (SSE notifica frontend)
3. Aguardar `cm.Wait(n.ID)` (botão Aprovar/Rejeitar no canvas)
4. Se aprovado → `done`; se rejeitado → `error`

Usado em: conflito de merge (`SyncRepo`), push falho (`ExecPush`).

---

## run_mode = "build_only"

Quando ativo, o planning prompt inclui:
> "Não inclua nenhum node que execute servidores, processos em background..."

Bloco aplicado ao LLM; NOT_TODO é gerado para specs que peçam servidores.

---

## SetRunMeta / GetRunMeta

Persiste par chave-valor em `<runDir>/run_meta.json`.
Usado para propagar `current_branch` entre nodes:
- `TypeBranch` dispatch → `SetRunMeta(runID, "current_branch", branchName)`
- `TypePush`/`TypePR` dispatch → `GetRunMeta(runID, "current_branch")`

---

## Spec de Exemplo (após Fase 13)

```markdown
NEW BRANCH feature/landing-page

## Fase 1 — Estrutura
Crie src/index.html com estrutura semântica HTML5.

## Fase 2 — Estilos
Crie src/styles/main.css.
ATTENTION_HERE revisar com o time antes de continuar

## Fase 3 — Publicar
PUSH
PR "feat: landing page inicial"
```

DAG gerado:
```
sync_repo → create_branch → create_html → create_css → checkpoint → push_branch → create_pr
```

---

## Limitações Conhecidas (Fase 13)

1. NEW BRANCH sem namespace automático — nome literal da spec
2. PR sem reviewers/labels — feature de V1
3. Stall detection genérico não implementado — comandos git têm timeout via context (120s)
4. run_mode block via prompt LLM — não enforced tecnicamente
5. Resolução de conflito sempre manual — agente não tenta auto-resolver
