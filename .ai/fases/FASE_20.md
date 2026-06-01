# FASE 20 — Bugfix sync_repo duplicate

## Contexto

App não iniciava nenhum run. Erro imediato ao planejar qualquer tarefa:

```
Erro: plan: orchestrator: add node "sync_repo": dag: node "sync_repo" already exists
```

## Causa Raiz

`buildPlanningPrompt()` mencionava `sync_repo →` no bloco CRITICAL ORDERING. O LLM interpretava isso como instrução para incluir um node `sync_repo` no JSON de saída. O orchestrator, porém, já injeta `sync_repo` programaticamente antes de processar os nodes do LLM. Ao tentar adicionar o node LLM com `id == "sync_repo"`, `dag.AddNode` retornava o erro de duplicata.

## Solução

### 1. Filtro nos loops de AddNode e AddEdge

`internal/orchestrator/orchestrator.go` — nos dois loops que processam os nodes LLM:
- Se `nd.ID == "sync_repo"` ou `dag.NodeType(nd.Type) == dag.TypeSyncRepo` → `continue` (skip).
- Nas edges: também skip edges onde `dep == "sync_repo"` (o orchestrator já cria essas edges via root-node logic).

### 2. Prompt corrigido

Substituído:
```
  sync_repo → branch → <edits> → push → pr
```
Por:
```
  branch → <edits> → push → pr
  (a sync_repo step is auto-prepended by the system — do NOT include it in your plan)
```

## Arquivos Modificados

- `internal/orchestrator/orchestrator.go`
- `.ai/FASES.json`
- `.ai/fases/FASE_19.md` (movido de plans/)

## Verificação

1. Submeter qualquer tarefa com repo configurado → DAG abre sem erro
2. Tarefa com `NEW BRANCH / PUSH / PR` → DAG: `sync_repo → branch → edits → push → pr`
3. LLM não gera node `sync_repo` no JSON (verificar NodeLog do orchestrator)
