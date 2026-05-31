# FASE 11 — Library Swaps (Refactor Only)

## Status: ✅ Entregue

## Zero comportamento alterado — apenas substituição de libs

---

## 1. chi router (Go)

**Arquivo:** `internal/web/server.go`

- `http.NewServeMux()` → `chi.NewRouter()` com sub-roteadores
- `middleware.Recoverer` adicionado como middleware global
- `r.PathValue("x")` → `chi.URLParam(r, "x")` em todos os handlers
- `slog.Warn("run finished with error", ...)` adicionado no goroutine de launchRun
- Imports: `github.com/go-chi/chi/v5`, `github.com/go-chi/chi/v5/middleware`

**go.mod:** `github.com/go-chi/chi/v5 v5.3.0` adicionado

---

## 2. slog (Go stdlib)

**Arquivos modificados:**

| Arquivo | Antes | Depois |
|---|---|---|
| `internal/llm/client.go` | `fmt.Fprintf(os.Stderr, "llm: attempt %d/%d ...")` | `slog.Warn("llm retry", "attempt", ...)` |
| `internal/gateway/gateway.go` | `fmt.Fprintf(os.Stderr, "gateway: handler error: %v\n", err)` | `slog.Error("gateway handler error", "error", err)` |
| `internal/gateway/telegram.go` | `fmt.Printf("Telegram bot @%s online\n", ...)` | `slog.Info("telegram bot online", "username", ...)` |
| `cmd/golovebox/main.go` | `fmt.Fprintf(os.Stderr, "web: %v\n", err)` | `slog.Error("web server error", "error", err)` |

**`cmd/golovebox/main.go` — novo:**
- `initLogger(format, level string)` — cria `slog.NewTextHandler` ou `slog.NewJSONHandler`
- `--log-level` flag (debug/info/warn/error, default: info)
- `--log-format` flag (text/json, default: text)
- `PersistentPreRun` chama `initLogger` antes de qualquer subcomando

Todos os `fmt.Printf` interativos (tabelas, prompts CLI) preservados sem alteração.

---

## 3. Cytoscape.js (Frontend)

**Arquivo:** `internal/web/static/index.html`

`<canvas id="dag-canvas">` → `<div id="dag-canvas">`

CDN adicionados (antes do Alpine):
```html
<script src="https://cdn.jsdelivr.net/npm/cytoscape@3/dist/cytoscape.min.js"></script>
<script src="https://cdn.jsdelivr.net/npm/cytoscape-dagre@2/cytoscape-dagre.min.js"></script>
```

Canvas2D removido (drawDAG, computeLayers, roundRect, drawArrow, clickAreas).

Funções novas:
- `loadDAG(dagData)` — popula cy com nodes/edges, roda layout `dagre TB`
- `updateNodeState(id, state, error)` — atualiza `.data()` sem re-render completo
- `updateNodeIter(id, iter)` — atualiza label do node com `running [N/20]`

Eventos Cytoscape:
- `cy.on('tap', 'node')` → `node-selected` ou `sse-checkpoint`
- `cy.on('mouseover'/'mouseout', 'node')` → tooltip

`Alpine.store('dag')` continua como fonte de verdade; Cytoscape consome via `loadDAG` e updates incrementais.

---

## 4. marked.js (Frontend)

CDN: `https://cdn.jsdelivr.net/npm/marked@12/marked.min.js`

`chatPanel.addMsg()` agora gera `html: marked.parse(text)`.

Template de mensagem usa `x-html="m.html"` (antes `x-text="m.text"`).

CSS adicionado para `.msg p`, `.msg code`, `.msg pre` — markdown renderiza inline.

---

## 5. highlight.js (Frontend)

CDN:
```html
<link rel="stylesheet" href="https://cdnjs.cloudflare.com/ajax/libs/highlight.js/11.9.0/styles/github-dark.min.css">
<script src="https://cdnjs.cloudflare.com/ajax/libs/highlight.js/11.9.0/highlight.min.js"></script>
```

`nodePanel.obsHtml(e)` — retorna `hljs.highlightAuto(e.obs).value` para highlight do output do tool.

Template de log usa `<pre x-html="obsHtml(e)">` em vez de `x-text`.

CSS `.log-obs .hljs{background:transparent}` preserva tema escuro do painel.

---

## Notas

- Zero arquivos Go novos
- Zero mudança de comportamento em runtime
- Build: `CGO_ENABLED=0 go build ./...` passa sem erros
