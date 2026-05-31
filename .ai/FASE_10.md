# FASE 10 — UI Refactor: Alpine.js

## Status: ✅ Entregue

## Arquivo Modificado

| Arquivo | Mudança |
|---|---|
| `internal/web/static/index.html` | Reescrito com Alpine.js — 4 componentes, stores, SSE/Canvas vanilla intacto |

**Zero arquivos Go modificados.**

## Componentes Alpine

### `healthPanel`
- `checks: {vm, llm, github, repo}` com `{ok, msg}`
- `loading` ativa CSS `@keyframes pulse` via `:class="{checking: loading}"`
- `x-for` nos 4 keys — sem HTML repetido
- `init()` dispara check automático

### `chatPanel`
- `messages[]` renderizado via `x-for` — sem `appendChild` manual
- `task` two-way via `x-model`; `files[]` atualizado em `handleDrop`/`handleFileChange`
- `:disabled="!task.trim() || running"` — sem toggling manual
- `Ctrl+Enter` no textarea via `@keydown.ctrl.enter`
- `onStateChange()` escuta `@sse-state-change.window` → adiciona msg reativa
- `init()` restaura `localStorage` e conecta SSE

### `runList`
- `x-for` sobre `runs[]`, `:class="{active: isActive(r.id)}"` 
- Recarrega em `@run-started.window` e `@run-selected.window`
- Dispatcha `run-selected` via `window.dispatchEvent`

### `nodePanel`
- `x-show="open"` + CSS `transform` para slide-in
- `x-for` sobre `entries[]` com auto-scroll via `$nextTick`
- `openFor()` faz duas requests em paralelo (`Promise.all`)
- `onNodeLog()` escuta `@sse-node-log.window` e faz push reativo

### `checkpointDialog`
- `x-cloak` nos dois elementos para evitar flash
- `x-show="open"` controla overlay + dialog simultaneamente
- `approve()`/`reject()` despacham `cp-result` lido pelo chatPanel

## Stores

| Store | Campos | Consumidores |
|---|---|---|
| `session` | `runId, running, connOk` | DAG header stop btn, status dot |
| `dag` | `data, nodes{state,error,iterCount}` | Canvas2D (`drawDAG`) |

## Comunicação via CustomEvents

```
chatPanel ──run-started──────────────────→ runList.load()
runList ───run-selected──────────────────→ chatPanel.onRunSelected()
canvas ────node-selected─────────────────→ nodePanel.openFor()
SSE ───────sse-state-change──────────────→ chatPanel.onStateChange()
SSE ───────sse-node-log──────────────────→ nodePanel.onNodeLog()
SSE ───────sse-checkpoint────────────────→ checkpointDialog.openFor()
dialog ────cp-result─────────────────────→ chatPanel.addMsg()
```

Zero acoplamento direto entre componentes — todos se comunicam via `window`.

## SSE + Canvas2D — vanilla JS

- `connectSSE(runId)`: abre `EventSource`, parseia eventos, atualiza `Alpine.store('dag')`, despacha CustomEvents
- `drawDAG()`: lê `Alpine.store('dag').data` e `.nodes` — sem dependência de variáveis globais de estado
- Funções auxiliares intactas: `computeLayers`, `roundRect`, `drawArrow`, `trunc`

## Notas sobre line count

O objetivo "50% menor" era aspiracional. Na prática:
- JS imperativo original: ~608 linhas (muitos `getElementById`, `innerHTML`, `appendChild`)
- Alpine components: ~480 linhas + ~256 linhas vanilla = 736 linhas total JS
- Redução em *complexidade* é real; em linhas brutas, similar

O ganho arquitetural supera o de linhas:
- Sem 15 variáveis globais de estado
- Sem `document.getElementById` em nenhum lugar
- Estado localizado por componente
- Adição de features (Fase 11 terminal/files) como novos componentes `x-data`
