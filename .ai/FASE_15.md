# FASE 15 — Bugfix: $HOME, 404 runs, 503 SFTP

## Status: ✅ Entregue

---

## Bug 1 — `fatal: $HOME not set` no cloud-init runcmd

**Sintoma:** `cloud-final [!!]` no segundo boot — runcmd executa `git config --global` sem `$HOME`.

**Root cause:** `runcmd` do cloud-init não garante `HOME` no ambiente de execução.

**Fix:** `internal/embed/cloudinit.go` — substituído `runcmd git config` por `write_files .gitconfig`:

```yaml
write_files:
  - path: /root/.gitconfig
    permissions: '0644'
    owner: root:root
    content: |
      [user]
        email = golovebox@localhost
        name = golovebox
```

`runcmd` mantém apenas `rc-update add sshd` e `rc-service sshd restart`.

> Usuários com VM existente precisam de `golovebox reset && golovebox init` para aplicar.

---

## Bug 2 — 404 em `/api/runs/{run-id}` após restart

**Sintoma:** Run listada no histórico, GET retorna 404. Browser ainda tem `glb_run_id` do processo anterior.

**Root causes:**
1. `handleGetRun` retornava 404 se `dag.json` ausente (run abortada durante planning)
2. Frontend não limpava `localStorage` ao receber 404

**Fix backend** — `internal/web/server.go` — `handleGetRun` degrada graciosamente:
- 404 real apenas se o **diretório** do run não existe
- Se diretório existe mas `dag.json` ausente → retorna `{dag: null, task: "..."}` com 200

**Fix frontend** — `internal/web/static/index.html` — `selectRunGlobal`:
```javascript
if (resp.status === 404) {
    localStorage.removeItem('glb_run_id');
    localStorage.removeItem('glb_node_id');
    currentRunId = null;
    Alpine.store('session').runId = null;
    if (window._sse) { window._sse.close(); window._sse = null; }
    return;
}
```

**Fix complementar** — `internal/web/store.go` — `indexExistingRuns` escaneia diretórios existentes no startup (documentação/clareza; `RunDir()` computa paths, não usa registry in-memory).

---

## Bug 3 — 503 em `/api/vm/files`

**Sintoma:** File explorer retorna 503 — VM está up mas SFTP falha por race condition entre boot e primeira requisição.

**Fix backend** — `internal/web/terminal.go` — `handleVMFiles` verifica `IsVMReady()` antes de tentar SFTP:
```go
if !s.gw.IsVMReady() {
    http.Error(w, `{"error":"vm_not_ready"}`, 503)
    return
}
```

**`sandbox/qemu.go`** — novo método:
```go
func (m *Manager) IsRunning() bool {
    return m.cmd != nil && m.cmd.Process != nil
}
```

**`internal/gateway/gateway.go`** — novo método:
```go
func (g *Gateway) IsVMReady() bool {
    ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
    defer cancel()
    c, err := g.pool.Acquire(ctx)
    if err != nil { return false }
    g.pool.Release(c)
    return true
}
```

Usa o pool como proxy de saúde — se `Acquire` funciona em 2s, VM está pronta.

**Fix frontend** — `filePanel.load()` trata 503:
```javascript
if (r.status === 503) {
    this.error = 'VM inicializando... tente novamente em alguns segundos.';
    return;
}
```
Mensagem de erro exibida no painel em vermelho.

---

## Bug 4 — Terminal desconecta imediatamente

**Sintoma:** "conectado à VM" → "desconectado" em ~1s — stale connection retornada pelo pool.

**Fix** — `internal/web/terminal.go` — retry SSH dial 3× com backoff 500ms:
```go
for attempt := 1; attempt <= 3; attempt++ {
    sshConn, err = s.gw.AcquireSSH(r.Context())
    if err == nil { break }
    if attempt < 3 { time.Sleep(time.Duration(attempt) * 500 * time.Millisecond) }
}
```

---

## Arquivos Modificados

| Arquivo | Mudança |
|---|---|
| `internal/embed/cloudinit.go` | `.gitconfig` via `write_files`; remove `git config --global` do `runcmd` |
| `internal/web/store.go` | `indexExistingRuns()` no startup |
| `internal/web/server.go` | `handleGetRun` — 404 apenas se diretório ausente; dag.json ausente → 200 com null dag |
| `internal/sandbox/qemu.go` | `IsRunning() bool` |
| `internal/gateway/gateway.go` | `IsVMReady() bool` (pool probe com 2s timeout) |
| `internal/web/terminal.go` | `IsVMReady()` check no handleVMFiles; retry 3× no terminal dial |
| `internal/web/static/index.html` | `selectRunGlobal` limpa localStorage em 404; `filePanel` exibe erro 503 |

## Arquivos Criados

Nenhum.
