# FASE 19 — Bugfix UX residual + Orchestrator workflow-aware

## Status: ✅ Entregue

## Contexto

Após FASE 18, novo round de uso identificou 9 atritos. Esta fase será a base que pessoas externas começarão a testar — prioridade total em correção, sem regressão.

---

## Bug 1 — PR falha com `422 Validation Failed: Field:base Code:invalid`

**Sintoma:** Spec contém `PR "teste"`, orquestrador gerou `open_pull_request` como `task` genérico. O agent loop dentro do node chamou `github_open_pr` passando `base: teste` (a feature branch) em vez de `main`. Spec usa as keywords PUSH/PR mas viraram tasks soltas.

**Fix:**

- **`internal/orchestrator/orchestrator.go`** — prompt endurecido com mapeamento explícito `PUSH→push`, `PR→pr`, `NEW BRANCH→branch` (nunca `task`) + BAD/GOOD examples mostrando o erro real visto em produção.
- **`internal/orchestrator/orchestrator.go`** — nova função `enforceWorkflowOrdering(d, specs)` executa após o parse JSON do LLM e:
  1. Detecta keywords presentes nos specs (`KwNewBranch`, `KwPush`, `KwPR`).
  2. Promove nodes `task` com IDs reconhecíveis (`*branch*`, `*push*`, `*pull_request*`, `*open_pr*`, `*_pr`) para o tipo workflow correto.
  3. Se ainda faltar algum, injeta nodes `create_branch`/`push_branch`/`open_pr` mínimos.
  4. Enforce dependências: todos os non-workflow nodes dependem de `branch`; `push` depende de todos os non-workflow + `branch`; `pr` depende de `push` (ou de `branch` se não houver push).

Resultado: workflow nodes vão direto pro dispatch em `server.go`, que usa `s.cfg.Workflow.DefaultBranch` (default `main`) como `base`. Agent loop tirado da equação na criação de PR — sem confusão entre `base` e `head`.

---

## Bug 2 — DAG topologicamente errado (edits paralelos com branch)

**Sintoma:** Spec com `NEW BRANCH teste / criar div / PR`. DAG mostrou `create_feature_branch` em **paralelo** com `create_centered_black_div` (em vez de dependência). Arquivo foi criado fora da branch nova.

**Fix:** Mesma `enforceWorkflowOrdering` acima. Cobre o caso: depois de detectar/injetar o branch node, força todos os non-workflow a depender dele.

---

## Bug 3 — Chat perde mensagens ao refresh

**Sintoma:** `chatPanel.messages` é in-memory. Refresh F5 zera o histórico (incluindo erro vermelho que o usuário queria reler).

**Fix:**

- **`internal/web/static/index.html`** — `chatPanel`:
  - `addMsg()` cap em **200 mensagens** + `_persist()` grava em `localStorage['glb_chat']` após cada push.
  - `init()` lê e restaura `glb_chat` antes da mensagem "Bem-vindo...". Se vazio, mostra welcome.
  - Botão `✕` no header do chat chama `clearChat()` — limpa estado + localStorage.

---

## Bug 4 — VM ainda demora ~1m40s no Windows TCG sem feedback

**Sintoma:** Dot VM fica vermelho/cinza por minutos no boot frio. Usuário não sabe se travou ou só está aquecendo.

**Fix:**

- **`internal/web/server.go`** — `Server` ganha campo `vmBootStartedAt time.Time` + mutex. `handleHealth`, após coletar checks, marca `vmBootStartedAt` no primeiro `vm.ok=false` e zera quando `vm.ok=true`. Inclui `boot_elapsed_secs` no JSON da chave `vm` enquanto está aquecendo.
- **`healthResult`** ganha campo `BootElapsedSecs int` (`omitempty`). Todas as 17 ocorrências de `healthResult{false, msg}` / `healthResult{true, msg}` migradas pra struct literal com nome de campo (`{OK: ..., Msg: ...}`) — backwards-compatible para o JSON existente, e tolera o novo campo opcional sem quebrar callers.
- **`internal/web/static/index.html`** — `healthPanel.vmTimer()` formata `boot_elapsed_secs` em `m:ss` (ex: `1:38`). Template exibe ao lado do label `VM` em amarelo monospace; some quando `vm.ok=true`.

---

## Bug 5 — Terminal SSH ainda exige refresh (3ª tentativa)

**Sintoma:** FASE 16 adicionou wait `IsVMReady` antes do upgrade WS; FASE 18 adicionou retry no pool dial. Ainda flicka `conectado→desconectado` na primeira aba aberta antes do boot completar.

**Diagnóstico:** o frontend chama `mount()` no `x-init`, que dispara antes do healthcheck ter chance de virar `vm.ok=true`. O backend espera, mas o browser já abriu o WS.

**Fix:**

- **`internal/web/static/index.html`** — `healthPanel` agora **despacha evento `vm-ready`** na transição `false→true` do check da VM.
- **`terminalPanel._waitForVMAndConnect()`** — novo: escreve `● aguardando VM...` no terminal, faz um probe `/api/health`; se ok já conecta, senão registra listener `window.addEventListener('vm-ready', ...)` que dispara `_connect()` quando a VM ficar pronta. `mount()` chama esta função em vez de `_connect()` direto.

Resultado: aba terminal aberta em boot frio mostra `● aguardando VM...` em vez do ciclo conectado/desconectado, e conecta sozinha quando pronta.

---

## Bug 6 — Run só vira `error` no badge depois de clicar

**Sintoma:** `runList` só recarrega via eventos `@run-started` / `@run-selected`. Quando um node erra durante run ativo, badge do sidebar fica `running` até o usuário clicar.

**Fix:**

- **`internal/web/static/index.html`** — `runList`:
  - Listener `@sse-state-change.window` chama `onStateChange(detail)`.
  - Update inline imediato do `run.state` (running/error) no array — feedback visual instantâneo.
  - Refetch debounced (600ms) de `/api/runs` pra sincronizar com `inferState` do backend.

---

## Bug 7+8+9 — Layout DAG: sobrepostos, nascem no canto, posição não persiste

**Sintomas:**
- Nodes nascem todos em (0,0) antes do dagre rodar (visualmente colados).
- DAG não centralizado no canvas.
- Arrastar nodes pra organizar — refresh zera.

**Fix:**

- **`internal/web/static/index.html`** — função `loadDAG`:
  - `nodeSep` 40 → **80** e `rankSep` 60 → **120** (mais espaço entre nodes).
  - Quando nenhuma posição salva existe → dagre + `cy.fit(40)` + `cy.center()`.
  - Quando todas posições estão salvas → apenas `cy.fit(40)` (preserva layout do usuário).
  - Quando parcial → roda dagre (cobre nodes novos sem destruir totalmente o trabalho prévio).
- **Helpers novos**: `positionsKey(runId)`, `loadSavedPositions`, `savePositions`, `clearSavedPositions`, `relayoutDAG` — persistem em `localStorage['glb_positions_<runId>']` como `{nodeId: {x, y}}`.
- **`cy.on('dragfree', 'node', () => savePositions())`** — toda vez que o usuário solta um node arrastado, posições do run inteiro são salvas.
- **Botão `↻ reorganizar`** no `dag-header` chama `relayoutDAG()` — limpa o localStorage do run e roda dagre de novo, útil quando o usuário se perde.

---

## Arquivos Modificados

| Arquivo | Mudança |
|---|---|
| `internal/orchestrator/orchestrator.go` | Prompt com BAD/GOOD examples + regras explícitas de tipo; `enforceWorkflowOrdering()` aplicada após `parseNodeDescriptors` |
| `internal/web/server.go` | `Server.vmBootStartedAt` + mutex; `handleHealth` injeta `boot_elapsed_secs`; `healthResult` ganha campo; todos literais migrados para `{OK:, Msg:}` |
| `internal/web/static/index.html` | chatPanel: persist localStorage cap 200 + botão limpar; healthPanel: timer mm:ss + dispatch `vm-ready`; terminalPanel: `_waitForVMAndConnect()`; runList: `onStateChange` + debounced refetch; Cytoscape: `loadDAG` com posições persistidas, `nodeSep`/`rankSep` maior, `cy.center()`, `dragfree` listener, botão `↻ reorganizar` |

## Arquivos Criados

Nenhum.

---

## Aprendizados (promover pra `FASES.json` na próxima digestão)

- **Prompt-only não basta para invariantes críticas.** LLM ignorou regras de mapeamento DSL→tipo já na primeira sessão de teste. Pós-processamento defensivo no orchestrator é o caminho (`enforceWorkflowOrdering`).
- **Workflow nodes existem para tirar o agent loop de operações com semântica delicada** (base vs head em PR). Forçar uso quando há keyword é correto e barato.
- **Eventos custom no `window`** (`vm-ready`) escalam melhor que stores compartilhados quando o componente que precisa do sinal não tem dependência direta do que despacha.
- **Persistência local por escopo natural** (`glb_chat` global, `glb_positions_<runId>` por run) é mais previsível que central única.
- **`cy.center()` complementa `cy.fit()`**: fit ajusta zoom, center ajusta translação. Os dois são necessários para "começa centralizado".

---

## Limitações Conhecidas (Fase 19)

1. **`glb_chat` é global, não por run**: cap em 200 evita estouro. Se quiser histórico por run, fase futura.
2. **Timer VM client-side**: refresh durante boot reseta visualmente (servidor mantém estado correto). Aceitável.
3. **`enforceWorkflowOrdering` usa heurística por substring no ID** (`*branch*`, `*push*`, etc). Se o LLM gerar IDs muito criativos sem essas palavras, o injetor cria nodes novos (não promove os existentes). Sem regressão funcional — apenas DAG maior que o necessário.

---

## Entregável

```bash
mage build && build\golovebox.exe serve
```

Validar:

1. **PR funcional**: spec `NEW BRANCH teste / criar div centralizada / PUSH / PR "teste"` → DAG mostra `sync_repo → create_branch → <edits> → push_branch → open_pr` em sequência. PR aberto contra `main` com sucesso.
2. **Edit depende da branch**: editar arquivo só roda **depois** do `branch` node done.
3. **Chat persiste**: rodar task, F5 → mensagens preservadas. Clicar `✕` → limpa.
4. **Timer VM**: boot frio → `● VM 0:05 ... 1:38` em amarelo ao lado do label; some quando verde.
5. **Terminal**: aba aberta logo após start → `● aguardando VM...` sem flicker; conecta sozinha quando pronta.
6. **RunList live**: node erra → badge no sidebar vira vermelho em ≤1s sem precisar clicar.
7. **DAG**: nodes nascem espaçados e centralizados.
8. **Posições persistem**: arrastar 2-3 nodes, F5 → posições mantidas. `↻ reorganizar` → volta pro dagre.
