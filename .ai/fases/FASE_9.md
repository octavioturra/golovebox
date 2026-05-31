# FASE 9 — Ajustes UX e Bugs

## Status: ✅ Entregue

## Arquivos Modificados

| Arquivo | Mudança |
|---|---|
| `internal/agent/loop.go` | `ProgressFunc` recebe `params string`; `formatParams()` prioriza chaves legíveis |
| `internal/gateway/telegram.go` | Atualizado para nova assinatura 4-arg do ProgressFunc |
| `cmd/golovebox/main.go` | Atualizado para nova assinatura 4-arg do ProgressFunc |
| `internal/web/nodelog.go` | `NodeLogEntry.Params` adicionado; `Append` recebe 4 args |
| `internal/web/store.go` | `cancelMap` + `RegisterRun/UnregisterRun/CancelRun`; `GetNodeLog` com fallback disk; `RunMeta.Task`; `ReadTask` |
| `internal/web/server.go` | `POST /api/runs/{id}/stop`; task.txt salvo; cancelable context em launchRun/launchSingleTask; `healthHTTP` com 4s timeout; task em GET responses |
| `internal/orchestrator/orchestrator.go` | Prompt exige snake_case descritivo nos IDs |
| `internal/web/static/index.html` | Todos os fixes abaixo |

## Bugs Corrigidos

### Bug 1 — Task invisível
- `task.txt` gravado em `startRunFromTask`
- Chat mostra `📋 "texto da task"` após "Run iniciada"
- `GET /api/runs/{id}` inclui `task` field
- Node panel mostra "Objetivo do run" no topo

### Bug 2 — Health dots lentos
- `healthHTTP = &http.Client{Timeout: 4s}` — não bloqueia indefinidamente
- CSS `@keyframes pulse` + classe `.checking` nos dots durante verificação

### Bug 3 — Sem como parar run
- `RunStore.cancelMap` guarda `context.CancelFunc` por run ativo
- `POST /api/runs/{id}/stop` chama `CancelRun`, retorna `{"ok": true/false}`
- Botão "⏹ Parar run" no header do DAG, some quando run conclui

### Bug 4 — Erro apaga histórico
- Chat usa `appendChild` incremental — nunca limpa mensagens existentes
- Evento `state === 'error'` mostra `addMsg([error] id, 'err', data.error)`
- Node panel mostra caixa vermelha com a mensagem de erro

## UX Melhorias

### UX 1 — Nomes descritivos
- Orchestrator prompt: "NUNCA use step-1, step-2 — use snake_case que descreve a ação"

### UX 2 — Painel vazio após run concluído
- `GetNodeLog` tenta buffer em memória; se vazio, lê o `.jsonl` do disco

### UX 3 — Params visíveis no log
- `ProgressFunc(iter, action, params, obs)` — params é o conteúdo formatado do bloco Parameters
- Log do node mostra `▶ cmd: git clone ...` em vez de `▶ shell`

### UX 4 — Canvas centralizado
- Calcula bounding box do DAG e `offsetX/offsetY` para centrar no canvas
- Node único fica exatamente no centro

### UX 5 — Tooltip no canvas
- `mousemove` mostra `<div id="canvas-tooltip">` com ID completo + erro truncado

### UX 6 — Contador de iteração
- `iterCounts[nodeId]` atualizado a cada `node_log` SSE
- Sub-label do node running: `running [7/20]`

### UX 7 — Preview da task na lista de runs
- `RunMeta.Task` truncado em 60 chars
- Renderizado como segunda linha no card do run
