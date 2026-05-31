# FASE 16 — Bugfix UX & Observabilidade

## Status: ✅ Entregue

---

## Bug 1 — Stream do LLM não aparece no detalhe do node

**Sintoma:** Painel direito do node mostra só `objetivo do run` + `descrição do node`. A atividade do ReAct loop (prompt enviado, reply do modelo, ações) aparece efêmera no chat e some ao trocar de run / refresh.

**Root cause:** `ProgressFunc` capturava só `(iter, action, params, obs)`. O prompt enviado ao LLM e a reply crua nunca eram persistidos no `NodeLog`.

**Fix:**

- **`internal/web/nodelog.go`** — `NodeLogEntry` ganhou campos `Prompt` e `Reply` (`omitempty`). `Append()` recebe os dois argumentos extras.
- **`internal/agent/loop.go`** — `ProgressFunc` agora é `func(iter, action, params, obs, prompt, reply string)`. O loop captura `lastPrompt` (último `messages[].Content` antes do `Complete`) e a `reply` crua antes do `parseAction`.
- **`internal/web/server.go`** — `broadcastNodeLog` propaga `prompt` + `reply` no payload SSE. Os 2 call sites (`launchSingleTask`, `launchRun.dispatch`) repassam os novos argumentos para `nodelog.Append`.
- **`internal/web/static/index.html`** — `nodePanel` ganhou blocos colapsáveis "Prompt enviado" (border azul) e "Resposta do LLM" (border verde) por entry, persistidos no `.jsonl` e recarregados via `/api/runs/{id}/nodes/{nodeId}/log`.

Backwards-compatible: entries antigos sem `prompt`/`reply` simplesmente omitem os blocos.

Call sites secundários atualizados para a nova assinatura: `internal/gateway/telegram.go`, `cmd/golovebox/main.go` (resume + dispatch).

---

## Bug 2 — Health indicators demoram pra ficar verdes

**Sintoma:** Dots VM/LLM/GITHUB/REPO ficam vermelhos por minutos mesmo com tudo no ar. Só atualizam ao clicar `↻`.

**Fix:**

- **`internal/web/static/index.html`** — `healthPanel.init()` agora dispara `check()` imediatamente **e** `setInterval(check, 5000)`. Mudança de estado em ≤5s sem ação manual.
- **`internal/web/server.go`** — `handleHealth` reduz timeout total de 5s → 3s, evitando que um check lento bloqueie o ciclo.

---

## Bug 3 — Branch atual não aparece em lugar nenhum

**Sintoma:** Após `create_feature_branch` rodar, branch fica em `run_meta.json` mas o UI não exibe.

**Fix:**

- **`internal/web/server.go`** — `handleGetRun` adiciona `current_branch` na resposta (lê via `store.GetRunMeta`).
- **`internal/web/static/index.html`** — `nodePanel` mostra linha `Branch: <name>` em amarelo monospace abaixo da descrição. Omite quando ausente.

---

## Bug 4 — Terminal desconecta na 1ª conexão

**Sintoma:** Ao abrir o app, primeira aba do terminal mostra "conectado à VM" → "desconectado" em ~1s. Refresh resolve.

**Root cause:** WS upgrade acontecia antes do pool SSH estar pronto. A FASE 15 já tinha retry de dial, mas o upgrade prematuro fecha rápido demais para o retry ajudar no primeiro carregamento.

**Fix:**

- **`internal/web/terminal.go`** — `handleTerminal` espera `IsVMReady()` por até ~5s (backoff 0/250/500/1000/2000ms) **antes** de fazer o WS upgrade.
- **`internal/web/static/index.html`** — `terminalPanel._connect()` faz **1 reconnect automático** se o WS fechar em menos de 2s após abrir (com aviso amarelo "● reconectando..."). Falha persistente exibe "● desconectado" normalmente.

---

## Arquivos Modificados

| Arquivo | Mudança |
|---|---|
| `internal/web/nodelog.go` | `NodeLogEntry.Prompt/Reply` opcionais; `Append` recebe os 2 novos args |
| `internal/agent/loop.go` | `ProgressFunc` 6-arg (adiciona `prompt`, `reply`); captura `lastPrompt` + `reply` cru |
| `internal/gateway/telegram.go` | Adapta callback para nova assinatura (ignora prompt/reply) |
| `cmd/golovebox/main.go` | Idem nos dois dispatchers (resume / specs run) |
| `internal/web/server.go` | `broadcastNodeLog` inclui prompt/reply; `handleGetRun` retorna `current_branch`; health timeout 3s |
| `internal/web/terminal.go` | `handleTerminal` aguarda `IsVMReady()` com backoff antes do WS upgrade |
| `internal/web/static/index.html` | `nodePanel`: blocos colapsáveis prompt/reply + branch; `healthPanel`: auto-poll 5s; `terminalPanel`: reconnect 1× em close <2s |

## Arquivos Criados

Nenhum.

---

## Entregável

```bash
mage build && build\golovebox.exe serve
```

Validar no UI:
1. Submeter "Crie index.html com div preta centralizada" → selecionar node `create_index_html` → painel direito mostra blocos colapsáveis com prompt e resposta crua do LLM por iter.
2. Recarregar página com VM up → dots verdes em ≤5s sem clicar `↻`.
3. Após `create_feature_branch` rodar, selecionar qualquer node mostra `Branch: feature/...`.
4. Abrir terminal logo após boot do app → conecta de primeira (sem ciclo conectado/desconectado).
5. Runs antigos (`.jsonl` sem `prompt`/`reply`) abrem sem erros — blocos colapsáveis simplesmente não aparecem.
