# FASE 18 — Bugfix Residuais (SSH dial, clone, UX de erro)

## Status: ✅ Entregue

---

## Bug 1 — SSH `handshake failed` ainda derruba primeira execução

**Sintoma:** Mesmo após FASE 15/16, run aborta com `ssh: handshake failed: read tcp 127.0.0.1:xxxxx->127.0.0.1:2222: wsarecv: An existing connection was forcibly closed by the remote host` no primeiro node SSH (ex: `create_branch_div`).

**Root cause:** `pool.Acquire` valida idle connections via keepalive (FASE 15) mas o **dial fresh** não tem retry. Logo após boot, `sshd restart` no cloud-init derruba conexões na faixa de 1-2s — handshake fresco encontra a porta aberta mas a conexão é fechada no meio do KEX.

**Fix:**

- **`internal/sandbox/pool.go`** — `Acquire` agora retenta o `Dial` até 4× com backoff linear (400/800/1200ms entre tentativas, ~3.7s total). Respeita `ctx.Done()` entre tentativas.

Resultado: o dispatch de `TypeBranch`/`TypePush`/etc agora resiste ao janela de boot do sshd sem propagar erro.

---

## Bug 2 — `sync_repo` não é injetado quando só `default_repo` top-level está setado

**Sintoma:** `/root` na VM só tem `.ssh` e `.gitconfig` — repositório nunca foi clonado. DAG não tem node de clone, então o agent tenta `git checkout -b` num diretório inexistente.

**Root cause:** Orchestrator checava apenas `cfg.Workflow.DefaultRepo`. Usuário tinha só o top-level `default_repo` setado (que faz REPO ficar verde no health). `[workflow] default_repo` vazio → sync_repo não injetado.

**Fix:**

- **`internal/orchestrator/orchestrator.go`** — nova helper `workflowRepo(cfg)` faz fallback de `cfg.Workflow.DefaultRepo` → `cfg.DefaultRepo`. Usada em todos os 4 sites que decidiam injeção de sync_repo e edges.
- **`internal/web/server.go`** — mesma helper `resolveRepo(cfg)` aplicada no dispatch de `TypeSyncRepo` e `TypePR`, pra consistência.
- **`internal/tools/git.go`** — `SyncRepo` chama `mkdir -p $(dirname clonePath)` antes do clone, garantindo que paths como `/root/owner/repo` funcionem mesmo sem o parent existir.

---

## Bug 3 — Erro do node se perde no chat e usuário não sabe onde clicar

**Sintoma:** Node falha, mensagem `[error] create_branch_div` aparece brevemente no chat e some entre outras mensagens. Painel direito não abre automaticamente; usuário não vê o stack trace SSH.

**Fix:**

- **`internal/web/static/index.html`** — handler SSE de `sse-state-change` dispara `node-selected` automaticamente quando `data.state === 'error'`. O `nodePanel.openFor` carrega o erro via `dagStore.nodes[id].error` (já populado pela broadcast).

Resultado: assim que um node erra, o painel direito abre nele com o erro destacado em vermelho.

---

## Arquivos Modificados

| Arquivo | Mudança |
|---|---|
| `internal/sandbox/pool.go` | `Acquire` retry dial 4× com backoff linear |
| `internal/orchestrator/orchestrator.go` | `workflowRepo()` fallback; usado em 5 sites |
| `internal/web/server.go` | `resolveRepo()` fallback para dispatch TypeSyncRepo/TypePR |
| `internal/tools/git.go` | `SyncRepo` cria parent dir antes do clone |
| `internal/web/static/index.html` | Auto-abre nodePanel quando state vira `error` |

## Arquivos Criados

Nenhum.

---

## Limitações Conhecidas / Próximos passos

- **SyntaxError em cdn.min.js** observado no console no boot — não reproduzido aqui; provavelmente proveniente de extensão de browser, não do app. Se persistir, abrir uma run limpa e capturar o stack completo.
- **Stale localStorage `glb_run_id`** ainda gera um 404 inicial por sessão (auto-limpo, mas ruidoso no console). Mitigação simples: pré-checar `/api/runs` antes de tentar o GET específico.

---

## Entregável

```bash
mage build && build\golovebox.exe serve
```

Validar:
1. Boot frio + nova run → `sync_repo` aparece como primeiro node mesmo com só `default_repo` (top-level) no config.
2. Primeiro dispatch SSH não aborta com `handshake failed` (pool retenta automaticamente).
3. `/root/<owner>/<repo>/` existe após `sync_repo` mesmo se parent dir não existir.
4. Node erra → painel direito abre nele imediatamente, com erro em vermelho.
