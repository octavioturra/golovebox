# FASE 26 — Extração: orchestrator

> Este arquivo é **histórico**. Já vive em `.ai/fases/FASE_26.md`.
> Digerido em `.ai/FASES.json` (v0_done id 26 + `go_workspace_modular` atualizado p/ 7 módulos).

## Contexto

- O engine de execução (intent→DAG, executor, ReAct loop, tools) ainda morava no legado, espalhado em `internal/{orchestrator,dag,agent,tools,memory}`.
- `orchestrator` importava `internal/config` e `internal/llm` concretos — acoplado ao app.
- A lógica de dispatch dos workflow nodes (sync_repo/branch/push/pr) e a entrada do agent loop estavam **fora** do engine, replicadas em `web/server.go`, `gateway/gateway.go` e `main.go`.
- `agent.Loop` falava `[]llm.Message` (multi-turn), incompatível com o contrato mínimo `core.Completer`.
- Sem isso isolado, trocar o substrato de LLM/VM/deriver obrigava a mexer no engine.

## Solução

Mover o engine para o módulo `orchestrator/` atrás de injeção `core.*`, consolidando o dispatch espalhado num `Engine`. Comportamento de runtime idêntico ao anterior (mesmo DAG, mesma ordenação de workflow, mesma UI/SSE). Sem novas dependências externas — o módulo reusa go-github/uuid/chromem já presentes.

---

## Regras absolutas

- `CGO_ENABLED=0` sem exceções
- Build via `mage build`
- Zero escrita fora de `.golovebox/`
- `orchestrator` importa **só** `core` (+ go-github/uuid/chromem) — nunca `internal/*`, `sandbox`, `toolskills`, `derivator`, `config`, `web`

---

## 1. Contratos no core

### O que é

Dois novos contratos em `core/contracts.go` para tirar o engine das deps de `config`/`web`.

### Por que dois canais de observabilidade

O plano original previa só `core.Progress`. Na prática há **dois** canais distintos: estado de node (transições pending→running→done) e iteração ReAct (action/obs por passo). O primeiro fica em `dag.NotifyFunc(*dag.Node)` (exportado do módulo, consumido por web/CLI); o segundo vira `core.Progress`.

### Implementação

```go
// Progress — callback de observabilidade por iteração ReAct (task nodes).
type Progress func(nodeID string, iter int, action, params, obs, prompt, reply string)

// RunConfig — valores projetados pelo app; o app resolve a precedência repo antes.
type RunConfig struct {
	Repo, DefaultBranch, GitHubToken, WorkDir, ClonePath, RunMode string
}
```

### Comportamento esperado

- `core` segue zero-dep de projeto (só `context`).
- `RunConfig` carrega tudo que o engine lê de config; nenhum `*config.Config` cruza a fronteira do módulo.

---

## 2. Módulo orchestrator + Engine

### O que é

Módulo `orchestrator/` com sub-pacotes `dag/`, `agent/`, `tools/`, `memory/` e o tipo `Engine` (`orchestrator.go` = Plan; `engine.go` = execução).

### Por que `New(sb, completer, mem)` e não `(sb, llm, deriver, prov)`

O mapeamento do código mostrou que nenhum pacote do engine consome `core.Deriver` nem `core.Provisioner` — injetá-los seria especulativo (viola o YAGNI que o próprio plano prega). `*memory.Memory` é o 3º colaborador real, construído pelo app e injetado já pronto (nil-tolerante).

### Por que transcript de string no loop

`core.Completer.Complete(ctx, prompt string)` é single-string, e `llm.NewCompleter` colapsa para 1 mensagem `user`. O loop acumula a conversa como transcript de texto (systemPrompt+task, depois `reply` + `"Observation: ..."` por iteração) — consistente com `react_loop_text_format`.

### Implementação

```go
func New(sb core.Sandbox, completer core.Completer, mem *memory.Memory) *Engine

func (e *Engine) Plan(ctx, runID, intents []core.Intent, rc core.RunConfig) (*dag.DAG, error)
func (e *Engine) Run(ctx, d *dag.DAG, rc core.RunConfig, notify dag.NotifyFunc, prog core.Progress) error
func (e *Engine) Resume(ctx, rc core.RunConfig, notify dag.NotifyFunc, prog core.Progress) error
func (e *Engine) RunSingleTask(ctx, task string, rc core.RunConfig, prog core.Progress) (string, error)
func (e *Engine) RunIssueTask(ctx, owner, repo string, issueNum int, rc core.RunConfig, prog core.Progress) (string, error)
func (e *Engine) Approve(nodeID string)        // CheckpointManager interno
func (e *Engine) Reject(nodeID, reason string)
```

- `Run` constrói o `DispatchFunc`: workflow nodes → `tools.SyncRepo/ExecBranch/ExecPush/ExecPR` com valores de `rc`; task nodes → ReAct loop emitindo `core.Progress(node.ID, …)`.
- `current_branch` persistido em `rc.WorkDir/run_meta.json` (formato `map[string]string`, compatível com `web.RunStore.GetRunMeta`).
- `Resume` lê `dag.json` de `rc.WorkDir` e re-roda nodes interrompidos.

### Comportamento esperado

- Módulo compila isolado: `cd orchestrator && GOWORK=off go build/vet/test` limpo.
- `Plan` gera `sync_repo → branch → edits → push → pr` (invariantes FASE 19/20 preservadas: dedup de sync_repo + `enforceWorkflowOrdering`).
- Engine é dono de `dag.json`/`node_states.json`/`run_meta.json`; `web.RunStore` só lê para exibição.

---

## 3. Memory (decisão (a))

### O que é

`internal/memory/` → `orchestrator/memory/`, consumidor único (tool `search_memory`).

### Por que não `core.Memory`

YAGNI — um consumidor só. O embed-fn (`NewEmbedFnFromConfig(provider, url, key)`) já recebe strings; o composition root constrói o `*memory.Memory` e injeta no `New`. Sem interface especulativa no core (mesma regra do `Completer` na FASE 25, em reverso).

### Comportamento esperado

- Embeddings continuam vindo do endpoint OpenAI-compat; Anthropic → mem nil (search vira no-op).

---

## 4. Adaptar o legado (= app)

### O que é

`gateway` vira delegador fino; `web` e `main` montam `RunConfig` e chamam o Engine.

### Implementação

- `gateway.New(sb core.Sandbox, engine *orchestrator.Engine, rc core.RunConfig)` — mantém health/handlers, delega `RunTask`→`RunIssueTask` e `RunSpecTask`→`RunSingleTask`. Perde imports `agent`/`llm`/`memory`/`tools`/`config`.
- `web/server.go` — `engine.Plan` + `engine.Run` com closures `notify` (SSE do node) e `prog` (NodeLog+SSE por iteração); approve/reject via `engine.*`. O dispatch closure inteiro sumiu.
- `cmd/golovebox/main.go` — composition root: `buildEngine` (memory + `llm.NewCompleter`), `buildRunConfig` (resolve `workflow.default_repo → default_repo`). Comandos `run`/`resume`/`github`/`daemon`/`web` injetam o Engine.

### Comportamento esperado

- Telegram, web chat e CLI `run`/`resume`/`github` funcionam idênticos; checkpoint interativo no `run` via stdin → `engine.Approve/Reject`.

---

## Arquivos Criados

| Arquivo | Função |
|---|---|
| `orchestrator/go.mod` | Módulo do engine (require core + go-github/uuid/chromem, replace core local) |
| `orchestrator/engine.go` | `Engine`: Run/Resume/RunSingleTask/RunIssueTask/Approve/Reject + dispatch + buildRegistry + run_meta |
| `orchestrator/agent/loop_test.go` | Cobre o transcript do ReAct loop (tool→done, error, heredoc) com mock `core.Completer` |

## Arquivos Modificados

| Arquivo | Mudança |
|---|---|
| `core/contracts.go` | + `Progress`, + `RunConfig` (6 campos) |
| `orchestrator/orchestrator.go` | `Orchestrator`→`Engine`; `Plan` lê `rc` (sem `*config.Config`/`*llm.Client`) |
| `orchestrator/agent/loop.go` | `[]llm.Message`→transcript string; `core.Completer` injetado |
| `orchestrator/agent/planner.go` | `PlanFromIssue` recebe `core.Completer` |
| `orchestrator/tools/git.go` | import `internal/dag`→`orchestrator/dag` |
| `internal/gateway/{gateway,telegram}.go` | delegador fino; `core.Progress`; `orchestrator.MaxIterations` |
| `internal/web/{server,store}.go` | Engine + `RunConfig` + `core.Progress`; dispatch removido |
| `cmd/golovebox/main.go` | composition root: `buildEngine`/`buildRunConfig`, Engine em todos os comandos |
| `go.work` / `go.mod` | + `./orchestrator` (use + require/replace) |

> Movidos via `git mv` (renames preservam histórico): `internal/{orchestrator,dag,agent,tools,memory}/` → `orchestrator/`. Originais deletados.

---

## Decisões de Design

### Engine dono da persistência de run

`Run`/`Resume` recebem `rc.WorkDir` e o executor já grava `dag.json`/`node_states.json` atomicamente (`os.Rename`); `current_branch` vai em `run_meta.json`. `web.RunStore` permanece no app e só **lê** esses arquivos (specs/logs/listagem). Alternativa descartada: passar `*dag.DAG` carregado + delegar save/load ao app — deixaria a lógica de Resume rachada entre app e módulo.

### Dois canais de observabilidade

`dag.NotifyFunc(*dag.Node)` (estado, p/ SSE e prompt de checkpoint no CLI) + `core.Progress` (iteração ReAct, p/ NodeLog). Colapsar tudo em `core.Progress` perderia o `*Node` que web/CLI precisam.

### Deriver/Provisioner fora do `New`

Não consumidos pelo engine hoje. Promovem-se a injeção quando (e se) o engine os consumir — coerente com `inject_dont_import`.

---

## Aprendizados

- Plano de refactor presume layout; o **mapeamento do código real** revelou que `Engine.Run/Resume` não existia — o dispatch estava içado em 3 consumidores. Mapear antes de mover evitou um `git mv` que não compilaria.
- `core.Completer` single-string força o ReAct loop a transcript de texto; como o adapter `llm.NewCompleter` já colapsava multi-turn para 1 mensagem, não houve perda semântica — só explicitou o que já acontecia.
- Repo é CRLF; arquivos novos criados como LF destoam. Normalizar p/ CRLF mantém o diff limpo.

---

## Limitações Conhecidas (Fase 26)

1. **`CheckpointManager` único por Engine**: runs concorrentes compartilham o mesmo CM (chave = nodeID), igual ao comportamento pré-fase no web. Sem colisão observada; revisitar se nodeIDs colidirem entre runs paralelos.
2. **Smoke test de runtime não executado**: o caminho `golovebox web` com VM real (spec NEW BRANCH/PUSH/PR) exige QEMU/Alpine configurados — validação manual pendente. Build, unit e checagem de fronteira passam.

---

## Entregável

```bash
cd orchestrator && GOWORK=off go build ./... && go test ./... && go vet ./...
cd .. && mage build
build/golovebox.exe run <spec NEW BRANCH/PUSH/PR>
build/golovebox.exe resume <run-id>
```

Deve funcionar ao final:
- `orchestrator` compila isolado; imports só `core` + libs externas (grep não acha `internal/*`/`sandbox`/`toolskills`/`derivator`/`config`/`web`).
- `Plan` gera `sync_repo → branch → edits → push → pr`.
- `Run` paralelo (máx 3), checkpoint pausa/retoma, `Resume` de run interrompido.
- `core.Progress` injetado preenche NodeLog + SSE (prompt/reply por iter no painel).
- PR contra `main` com `head=branch` (workflow node, não agent).
- `internal/{orchestrator,dag,agent,tools,memory}/` não existem mais.
