# FASE 8 — Observabilidade: Health Dashboard + Input Manual + Stream ReAct

## Status: ✅ Entregue

## Arquivos Criados
- `internal/web/nodelog.go` — `NodeLog`, `NodeLogEntry`, `loadNodeLogFromDisk`

## Arquivos Modificados
- `internal/web/store.go` — `NodeLogFor(runID, nodeID)` adicionado ao `RunStore`
- `internal/web/server.go` — health endpoint, node log endpoint, JSON POST /api/run, broadcastNodeLog
- `internal/web/static/index.html` — health panel, textarea, localStorage, node log panel
- `internal/gateway/gateway.go` — `HealthCheckVM(ctx)` exposto

## O que foi implementado

### 1. Health Dashboard (`GET /api/health`)
- Checks em paralelo com `context.WithTimeout(5s)`: VM, LLM, GitHub, Repo
- VM: `Gateway.HealthCheckVM` → adquire pool, executa `echo ok`
- LLM: POST mínimo para `cfg.LLMBaseURL/v1/messages` com max_tokens=1
- GitHub: `GET https://api.github.com/user` com Bearer token
- Repo: `GET https://api.github.com/repos/{defaultRepo}` (skip se vazio)
- Painel no topo do chat com 4 dots coloridos (verde/vermelho/cinza) + botão ↻

### 2. Input Manual (`POST /api/run` JSON)
- Se `Content-Type: application/json` com `{"task":"..."}`, cria run sem arquivo .md
- Fallback DSL: escreve `task.md` e tenta parse; se falhar, executa como single-node direto
- Textarea `rows=5` no chat panel com botão "Executar"

### 3. Sessão Persistente (localStorage)
- `glb_run_id` → restaura run ativo ao recarregar
- `glb_node_id` → reabre painel lateral do node
- Salvo em `selectRun` e `openNodePanel`, limpo em `closeNodePanel`

### 4. Stream ReAct por Node
- `NodeLog`: buffer em memória + `.jsonl` append-only em `<runDir>/logs/<nodeID>.jsonl`
- `RunStore.NodeLogFor(runID, nodeID)` cria/retorna NodeLog existente
- `dispatch` em `launchRun` conecta `ProgressFunc` → `nodelog.Append` + `broadcastNodeLog`
- SSE evento `{type:"node_log", node_id, iter, action, obs, ts}` no canal existente por run
- `GET /api/runs/{id}/nodes/{nodeID}/log` retorna array JSON (memória ou disk)
- Painel lateral: abre ao clicar em qualquer node, carrega histórico + recebe eventos ao vivo
- Auto-scroll para o final a cada nova entrada

## Decisões de implementação
- SSE único por run (não por node) — cliente filtra por `node_id`
- `loadNodeLogFromDisk` usado quando processo reinicia (buffer vazio, disk tem .jsonl)
- `launchSingleTask` para tasks sem estrutura DSL válida — single node direto
- Click em node: `waiting_human` → checkpoint dialog; outros estados → node log panel
