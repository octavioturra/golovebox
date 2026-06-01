# FASE 19 — Bugfix UX residual + Orchestrator workflow-aware

## Contexto

Após FASE 18 entregue, novo round de uso revelou 9 bugs/atritos:

1. **PR falha com `422 Validation Failed: Field:base Code:invalid`** — orquestrador gerou `open_pull_request` como `task` genérico (não `TypePR`). O agent dentro do node chamou `github_open_pr` provavelmente passando `base: teste` (a feature branch) em vez de `main`. As keywords PUSH/PR no spec não viraram nodes de workflow.
2. **DAG topologicamente errado** — `create_feature_branch` rodou **em paralelo** com `create_centered_black_div` (em vez de ser dependência). Arquivo foi criado fora da branch nova. O orchestrator não está garantindo ordem branch→edição→push→PR.
3. **Chat perde mensagens ao refresh** — `chatPanel.messages` é só in-memory. Histórico do run somente no DAG/node panel; chat zera.
4. **VM ainda demora ~1m40s no Windows TCG sem indicação** — usuário fica sem saber se travou ou só está aquecendo. Health dot vermelho não comunica "está aquecendo".
5. **Terminal SSH ainda exige refresh na primeira conexão** — FASE 16/18 mitigou parcialmente. Provavelmente o WS abre antes do `IsVMReady` virar `true` ou o reconnect 1× não está acionando.
6. **Run só vira `error` no badge depois de clicar nele** — `runList` recarrega via `@run-selected`/`@run-started`, não escuta SSE state changes ao vivo.
7. **Nodes sobrepostos no render inicial** — Cytoscape monta em `(0,0)` antes do layout dagre rodar; visualmente todos colam.
8. **Nodes nascem no canto** — `cy.fit()` é chamado mas a câmera não centraliza imediatamente.
9. **Posição dos nodes não persiste** — usuário arrasta para organizar, refresh zera.

Bug "upload engasgou" foi descartado pelo usuário ("faz parte").

Decisões de abordagem:
- **Workflow nodes obrigatórios** quando spec tem PUSH/PR (não confiar no agent loop).
- **Persistência de posições** por `runID` em `localStorage`.
- **Timer de aquecimento** ao lado do dot VM no health panel.

---

## Solução

Workflow correto end-to-end: orquestrador gera tipos `branch`/`push`/`pr` com dependências lineares; UI persiste chat + posições de nodes; sinais visuais explícitos de aquecimento da VM; sidebar reflete erro ao vivo.

## Regras absolutas
- `CGO_ENABLED=0`, build via `mage build`
- Zero escrita fora de `.golovebox/` (server) / localStorage (frontend)
- Não introduzir polling além do já existente (health 5s, SSE)

---

## 1. Orquestrador — força workflow nodes e ordem correta

### Problema raiz
LLM ignora as keywords PUSH/PR do spec e gera tudo como `task`. Mesmo quando gera `branch`, o file-creation node não declara dependência, então roda em paralelo.

### Implementação

**`internal/orchestrator/orchestrator.go`** — endurecer `buildPlanningPrompt`:
- Mapeamento estrito: spec com `NEW BRANCH X` → **exatamente um** node tipo `branch` (id `create_feature_branch`) como dependência de todos os nodes de edição/teste.
- Spec com `PUSH` → **exatamente um** node tipo `push` que depende de todos os nodes de edição.
- Spec com `PR "..."` → **exatamente um** node tipo `pr` que depende do node `push`.
- Regra de ordem: `sync_repo → branch → edits (paralelo entre si) → push → pr`.
- BAD/GOOD examples no prompt mostrando o erro recém-observado (edits paralelos com branch).

**`internal/orchestrator/orchestrator.go`** — pós-processamento defensivo após `parseNodeDescriptors`:
- Se existir node tipo `branch` mas algum node não-workflow não o tiver como dependência (direta ou transitiva), inserir a edge automaticamente.
- Se existir node tipo `pr` sem dependência de algum node `push`, inserir a edge.
- Garantia mínima mesmo se o LLM falhar.

### Por que esta abordagem
Workflow nodes vão direto pro dispatch em [internal/web/server.go:261-321](internal/web/server.go) com `base = s.cfg.Workflow.DefaultBranch` (já protegido por `applyDefaults` → `"main"`). O agent loop é tirado da equação na criação do PR — sem oportunidade pra confusão entre `base` e `head`.

### Comportamento esperado
- Spec `BRANCH teste / ... PR "teste"` gera DAG: `sync_repo → create_branch(teste) → <edits> → push → pr(base=main, head=teste)`.
- PR é criado contra `main` com `head=teste`, retorna 201.
- Edição de `index.html` só roda após branch existir.

---

## 2. Persistência de chat (localStorage)

### Implementação

**`internal/web/static/index.html`** — `chatPanel`:
- `addMsg` faz `localStorage.setItem('glb_chat', JSON.stringify(this.messages.slice(-200)))` após cada push (cap em 200 mensagens).
- `init()` lê `localStorage.getItem('glb_chat')` antes de mostrar "Bem-vindo..." e popula `this.messages`.
- Botão pequeno "✕ limpar chat" no header do painel pra reset manual.

### Comportamento esperado
- Refresh F5 mantém o histórico de mensagens (incluindo `[error] open_pull_request` com detail vermelho).
- Cap de 200 evita crescer indefinidamente.

---

## 3. Timer de aquecimento da VM

### Implementação

**`internal/web/server.go`** — `/api/health` resposta da chave `vm` ganha campo opcional `boot_elapsed_secs` (segundos desde primeiro health check com `vm.ok=false`). Calculado em memória no `Server` (campo `vmBootStartedAt time.Time`, resetado quando `vm.ok=true`).

**`internal/web/static/index.html`** — `healthPanel`:
- Quando `checks.vm.ok === false` e `checks.vm.msg` existe, renderiza ao lado do dot VM: `VM 0:23` formatado de `boot_elapsed_secs` (mm:ss).
- Quando `ok === true`, contador some.
- CSS: tipo monospace pequeno, mesma cor do label.

### Por que esta abordagem
- Não polui o UI quando tudo está verde.
- Computa server-side (não acumula no client em refresh).
- Reusa o auto-poll de 5s da FASE 16 — sem novo ciclo.

### Comportamento esperado
- Boot frio Windows: `● VM 0:05 ... 0:42 ... 1:38` → dot fica verde, contador some.
- Comunica "tá aquecendo, não travou".

---

## 4. Terminal SSH na primeira conexão (3ª tentativa)

### Diagnóstico
Após FASE 16 (espera IsVMReady antes do upgrade) + FASE 18 (pool dial retry 4×), terminal ainda flicka. Hipótese: o componente `terminalPanel` faz `mount()` no `x-init`, que dispara antes do `IsVMReady` do backend ter chance de ficar `true` — backend espera com `time.Sleep`, mas o WS já foi enviado pelo browser.

### Implementação

**`internal/web/static/index.html`** — `terminalPanel`:
- `mount()` só dispara `_connect()` quando `Alpine.store('healthPanel').checks.vm.ok === true` (ou refatorar — health panel não é store; usar evento `vm-ready` despachado pelo healthPanel quando vm vira ok).
- Alternativa simples: `mount()` faz um `await fetch('/api/health')` e só prossegue se `vm.ok`; senão aguarda 2s e retenta até 3×.

Como o backend `handleTerminal` já espera, o problema concreto é só o **primeiro upgrade** racing com o boot. Bloquear o `mount()` até VM ok elimina o ciclo `conectado→desconectado` visual.

### Comportamento esperado
- Aba terminal ao abrir o app: mostra "aguardando VM..." em vez de "conectado→desconectado".
- Após VM pronta, conecta de primeira sem refresh.

---

## 5. RunList atualiza ao vivo via SSE

### Implementação

**`internal/web/static/index.html`** — `runList`:
- Adicionar listener `@sse-state-change.window` que, se `data.run_id === r.id` (ou sempre que current run muda de estado terminal), atualiza inline `r.state` no array sem refazer fetch.
- Função `inferRunState(events)` espelha o `inferState` do backend ([internal/web/store.go:253-274](internal/web/store.go)): se algum node `error` e nenhum `running/pending/waiting` → `error`.
- Alternativa pragmática: ao receber `data.state === 'error'`, chamar `this.load()` (refetch) — mais simples e já cobre o caso.

### Comportamento esperado
- Node falha → badge do run no sidebar atualiza pra vermelho `error` sem precisar clicar.

---

## 6. Layout Cytoscape — spread, center, persist

### Implementação

**`internal/web/static/index.html`** — função `loadDAG`:
- Aumentar `nodeSep` 40 → **80** e `rankSep` 60 → **120**.
- Aplicar layout `dagre` **antes** de renderizar (mudar opção `layout: { name: 'preset' }` para `layout: { name: 'dagre', ... }` no `cytoscape({...})` inicial). Hoje preset deixa nodes em (0,0) até o `loadDAG` rodar.
- Após `cy.layout(...).run()`, chamar `cy.center()` em vez de só `cy.fit()` — centraliza geometricamente.

**Persistência de posições por runID**:
- `cy.on('dragfree', 'node', () => savePositions())` — `savePositions` grava `{nodeId: {x,y}}` em `localStorage[`glb_positions_${currentRunId}`]`.
- `loadDAG` consulta `localStorage` antes de aplicar dagre; se existir layout salvo para `currentRunId`, aplica `node.position(pos)` em vez de rodar dagre.
- Botão pequeno "↻ reorganizar" no `dag-header` que limpa o localStorage daquele run e roda dagre de novo.

### Por que esta abordagem
- Dagre rodando direto resolve "nascem no canto" + "sobrepostos" numa só mudança.
- Persistência por runID respeita o trabalho do usuário sem confundir entre runs.
- Botão de reset evita ficar preso em layout ruim.

### Comportamento esperado
- DAG aparece já organizado, centralizado, com espaçamento confortável.
- Arrastar nodes preserva ao refresh.
- Botão "↻ reorganizar" reverte para dagre.

---

## Arquivos Modificados

| Arquivo | Mudança |
|---|---|
| `internal/orchestrator/orchestrator.go` | Prompt endurecido (workflow nodes obrigatórios + ordem); pós-processamento defensivo inserindo edges branch→edits e push→pr quando ausentes |
| `internal/web/server.go` | `/api/health` retorna `boot_elapsed_secs` no vm check; campo `vmBootStartedAt` no `Server` |
| `internal/web/static/index.html` | chatPanel: persist messages em `glb_chat` (cap 200) + botão limpar; healthPanel: timer mm:ss ao lado de dot VM; terminalPanel: bloqueia mount até vm.ok; runList: refetch on sse-state-change; Cytoscape: layout dagre inicial + nodeSep/rankSep maior + `cy.center()`; persistência de posições em `glb_positions_<runId>` + listener dragfree + botão reorganizar |

## Arquivos Criados

Nenhum.

---

## Decisões de Design

### Workflow nodes via pós-processamento (não só prompt)
Confiar só no prompt do LLM é frágil — FASE 17 já viu isso ("task" gerada genérica). Pós-processamento garante invariantes mínimas mesmo com LLM falho. Custo: ~30 linhas em `Plan()`.

### Posições por runID, não global
Cada run tem DAG potencialmente diferente — reusar posições entre runs gera coordenadas órfãs ou colisões. Por runID é local, simples e previsível.

### Timer no health panel, não overlay
Overlay bloqueia interação com features que **não** dependem da VM (ver runs antigos, navegar UI). Timer inline comunica sem bloquear.

---

## Aprendizados que vão pra FASES.json após digestão

- LLM não obedece consistentemente regras de mapeamento DSL → tipo de node. Pós-processamento defensivo no orchestrator é necessário (não é over-engineering).
- Workflow nodes (`TypePush`/`TypePR`) existem justamente pra tirar o agent loop de operações onde o agent confunde semântica (base vs head). Forçar uso quando há keyword é correto.
- Persistência local (`localStorage`) por escopo natural (runID, glb_chat) escala melhor que central única.

---

## Limitações Conhecidas (Fase 19)

1. **Persistência de chat compartilhada entre runs**: `glb_chat` é único, não por run. Cap em 200 evita estouro. Se usuário quiser histórico por run, FASE futura.
2. **Timer VM client-side**: se usuário recarregar antes da VM ficar pronta, o contador reseta visualmente (servidor mantém estado correto). Aceitável.
3. **`cy.center()` pode mover layout salvo**: ao restaurar posições, não chamar center — só fit no boundingBox dos nodes.

---

## Verificação End-to-End

```bash
mage build && build\golovebox.exe serve
```

1. **PR funcional**: submeter "NEW BRANCH teste / criar div centralizada / PUSH / PR 'teste'". DAG deve mostrar `sync_repo → create_branch → <edits> → push → pr` (todos sequenciais nos workflow points). PR é criado contra `main` com sucesso.
2. **Edit depende da branch**: nodes de edição rodam **depois** de `create_branch` (não em paralelo).
3. **Chat persiste**: rodar uma task, refresh F5 → mensagens do run anterior continuam visíveis.
4. **Timer VM**: `golovebox reset && init && serve`, abrir UI → ver `● VM 0:05 ... 1:30` contando enquanto cinza/vermelho; some quando verde.
5. **Terminal**: abrir aba terminal logo após start → "aguardando VM..." (sem ciclo conectado/desconectado); conecta sozinho quando pronto.
6. **RunList live**: provocar erro num node → badge no sidebar vira vermelho `error` sem precisar clicar.
7. **DAG espaçado e centralizado**: novo run → nodes aparecem espaçados (nodeSep 80) e DAG centralizado no canvas.
8. **Posições persistem**: arrastar 2-3 nodes, refresh F5 → posições mantidas. Clicar "↻ reorganizar" → volta para dagre.
