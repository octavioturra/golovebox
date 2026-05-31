# FASE 12 — VM no Browser: Terminal SSH + File Explorer

## Status: ✅ Entregue

---

## Arquivos Criados

| Arquivo | Função |
|---|---|
| `internal/web/terminal.go` | WebSocket↔SSH PTY bridge + SFTP file explorer handlers |

## Arquivos Modificados

| Arquivo | Mudança |
|---|---|
| `go.mod` | `github.com/gorilla/websocket v1.5.3` adicionado |
| `internal/gateway/gateway.go` | `AcquireSSH()` e `ReleaseSSH()` expostos para uso direto do pool |
| `internal/web/server.go` | Rotas `/ws/terminal`, `/api/vm/files`, `/api/vm/file` adicionadas |
| `internal/web/static/index.html` | CDN xterm.js; componentes `terminalPanel` e `filePanel`; painel inferior com tabs |

---

## Terminal SSH (WebSocket↔PTY)

**Rota:** `GET /ws/terminal` (WebSocket upgrade)

`internal/web/terminal.go` — `handleTerminal`:
- Faz upgrade WebSocket via `gorilla/websocket`
- Adquire `*ssh.Client` do pool da gateway
- Abre sessão SSH com PTY `xterm-256color` 24×80
- Dois goroutines: stdout→WS e stderr→WS
- Loop principal: WS→stdin; mensagens JSON `{"type":"resize","cols":N,"rows":N}` chamam `session.WindowChange()`
- Ao fechar: cancela contexto, fecha sessão, devolve conexão ao pool

**Frontend:** `terminalPanel` Alpine component
- `mount()`: instancia `Terminal` (xterm.js) + `FitAddon`, abre WebSocket
- `ResizeObserver` no container → `fitAddon.fit()` + envia resize event
- `destroy()`: limpa recursos ao esconder painel
- Montado automaticamente ao init; re-fit ao trocar de tab

---

## File Explorer (SFTP read-only)

**Rotas:**
- `GET /api/vm/files?path=/root` → JSON `[]FileEntry`
- `GET /api/vm/file?path=/root/main.go` → conteúdo do arquivo inline

`FileEntry`:
```go
type FileEntry struct {
    Name  string `json:"name"`
    Path  string `json:"path"`
    IsDir bool   `json:"is_dir"`
    Size  int64  `json:"size"`
    Mode  string `json:"mode"`
}
```

Ordenação: diretórios primeiro, depois arquivos, ambos alfabéticos.

Content-Type por extensão: `.go`, `.sh`, `.md`, `.json`, `.toml`, `.log`, `.yaml` → `text/plain`; demais → `application/octet-stream` (download).

**Frontend:** `filePanel` Alpine component
- `cd(path)` empilha histórico; `back()` desempilha
- `open(entry)`: dir → `cd()`; arquivo → `window.open()` nova aba
- `icon(entry)`: emojis por extensão
- `fmt(bytes)`: formata tamanho (B/KB/MB)

---

## Layout — Painel Inferior com Tabs

```
┌─────────────────────────────────────────────────┐
│  DAG (Cytoscape)          flex:1 min-height:0   │
└─────────────────────────────────────────────────┘
┌─────────────────────────────────────────────────┐
│  [⌨ Terminal]  [📁 Arquivos]    height: 260px    │
│  ─────────────────────────────────────────────  │
│  (conteúdo da tab ativa)                         │
└─────────────────────────────────────────────────┘
```

Tab ativa persiste em `localStorage('glb_bottom_tab')`.

---

## Gateway — AcquireSSH / ReleaseSSH

```go
func (g *Gateway) AcquireSSH(ctx context.Context) (*ssh.Client, error)
func (g *Gateway) ReleaseSSH(c *ssh.Client)
```

Delegam para o `sandbox.Pool` existente. Sem novo estado no Gateway.

---

## CDN Adicionados

```html
<link  rel="stylesheet" href="https://cdn.jsdelivr.net/npm/xterm@5/css/xterm.css">
<script src="https://cdn.jsdelivr.net/npm/xterm@5/lib/xterm.js"></script>
<script src="https://cdn.jsdelivr.net/npm/xterm-addon-fit@0.8/lib/xterm-addon-fit.js"></script>
```

---

## Limitações Conhecidas (V0)

1. Uma sessão terminal por WebSocket connection — sem multiplexing
2. Sem autenticação no `/ws/terminal` — aceitável para uso local
3. File explorer read-only — upload e edição são feature de V1
4. Terminal fecha (WebSocket cai) ao trocar de tab — shell continua na VM; reconectar abre novo shell
