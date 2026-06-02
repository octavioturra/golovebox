# FASE 26 — Extração: orchestrator (REFINADO)

> Refinamento do plano original após mapeamento do código real (8 unidades).
> Preenche 3 lacunas entre o plano e o estado atual do código:
> 1. A API `Engine.Run/Resume` **não existe** — a lógica de dispatch está espalhada em
>    `web/server.go`, `gateway/gateway.go` e `main.go`. Precisa ser **içada** para o módulo.
> 2. O motor **não consome** `core.Deriver` nem `core.Provisioner` (nenhum pacote do engine os importa).
> 3. Há **dois** canais de observabilidade, não um: estado de node (`NotifyFunc(*Node)`) e
>    iteração ReAct (`core.Progress`). O plano só citou o segundo.

---

## Decisões confirmadas com o usuário

- **Execução:** refinar o plano primeiro (este doc), aprovar, depois executar em etapas compiláveis num só branch/PR.
- **Persistência:** **o Engine é dono do `dag.json`/`node_states.json`/`run_meta.json`** dentro de `rc.WorkDir`
  (o `dag.Executor` já faz escrita atômica via `os.Rename`). O `RunStore` (web) continua dono de
  specs/logs/listagem e **lê** esses arquivos para exibição.

---

## Desvios do plano original (com justificativa)

| Plano original | Refinado | Porquê |
|---|---|---|
| `New(sb, llm, deriver, prov)` | `New(sb core.Sandbox, llm core.Completer, mem *memory.Memory)` | grep confirma: nenhum pacote do engine importa `derivator`/`Provisioner`. Injetar dep não-consumida viola o YAGNI que o próprio plano prega. `memory` é o 3º colaborador real (injetado já construído pelo app). |
| Observabilidade só via `core.Progress` | `core.Progress` (iteração ReAct) **+** `dag.NotifyFunc(*dag.Node)` (transição de estado de node) | São canais distintos. web faz SSE do node inteiro; `main run` usa o estado p/ prompt de checkpoint no stdin. `core.Progress` cobre só o log por-iteração. |
| `RunConfig{Repo, DefaultBranch, GitHubToken, WorkDir}` | + `ClonePath`, `RunMode` | o engine lê esses 2 valores hoje (clone path na VM, restrição `build_only` do prompt). Embedding/memDir saem via injeção de `*memory.Memory` (não poluem `RunConfig`). |
| memory via `core.Memory` (rejeitado) ou dentro do módulo | dentro do módulo, **injetado já pronto** (`orchestrator/memory.New` chamado pelo app) | mantém `core` sem `core.Memory` especulativo; embedding config (provider/url/key) fica no composition root, não no `RunConfig`. |

---

## 1. core/contracts.go (✅ parcialmente feito)

Já adicionado nesta sessão: `Progress` e `RunConfig` (4 campos). **Expandir `RunConfig`** para 6 campos:

```go
// Progress — callback de observabilidade por iteração ReAct (task nodes).
type Progress func(nodeID string, iter int, action, params, obs, prompt, reply string)

// RunConfig — valores de execução projetados pelo app a partir de internal/config.
// O app resolve a precedência repo (workflow.default_repo → default_repo) ANTES de montar isto.
type RunConfig struct {
	Repo          string // owner/repo já resolvido
	DefaultBranch string // base do PR (Workflow.DefaultBranch)
	GitHubToken   string
	WorkDir       string // .golovebox/runs/<id>/ — dag.json/node_states/run_meta vivem aqui
	ClonePath     string // path do repo DENTRO da VM (Workflow.ClonePath, ex. /root/repo)
	RunMode       string // Workflow.RunMode ("build_only" restringe o prompt de planejamento)
}
```

`core` segue zero-dep (só `context`). `NotifyFunc`/`*dag.Node` **não** vão para o core — saem do módulo `orchestrator/dag`.

---

## 2. Módulo `orchestrator/`

### `orchestrator/go.mod`
```
module github.com/user/golovebox/orchestrator
go 1.26
require (
	github.com/user/golovebox/core v0.0.0-00010101000000-000000000000
	github.com/google/go-github/v60 v60.0.0
)
replace github.com/user/golovebox/core => ../core
```
> `x/crypto` **não** é necessário: as tools usam `core.Sandbox.Exec`, não SSH direto. (Confirmar no build; se `go vet` reclamar, adicionar.)
> `require v0.0.0-... + replace` é obrigatório mesmo com `go.work` (Go 1.26 resolve via rede sem ele — learning FASE 22).

### Estrutura
```
orchestrator/
├── go.mod
├── orchestrator.go     # Engine: New, Plan, Run, Resume, RunSingleTask, RunIssueTask, Approve, Reject
├── dag/                # checkpoint.go, dag.go, executor.go (MOVE puro — zero deps de projeto)
├── agent/              # loop.go (→ core.Completer), planner.go (→ core.Completer), tools.go, agent.go
├── tools/              # files.go, git.go, github.go, shell.go (já usam core.Sandbox)
└── memory/             # embed.go, memory.go (MOVE puro)
```

### API pública (derivada dos 4 call sites reais)
```go
func New(sb core.Sandbox, llm core.Completer, mem *memory.Memory) *Engine

func (e *Engine) Plan(ctx context.Context, runID string, intents []core.Intent, rc core.RunConfig) (*dag.DAG, error)
func (e *Engine) Run(ctx context.Context, d *dag.DAG, rc core.RunConfig, notify dag.NotifyFunc, prog core.Progress) error
func (e *Engine) Resume(ctx context.Context, rc core.RunConfig, notify dag.NotifyFunc, prog core.Progress) error
func (e *Engine) RunSingleTask(ctx context.Context, task string, rc core.RunConfig, prog core.Progress) (string, error)
func (e *Engine) RunIssueTask(ctx context.Context, owner, repo string, issueNum int, rc core.RunConfig, prog core.Progress) (string, error)
func (e *Engine) Approve(nodeID string)        // delega p/ o CheckpointManager interno
func (e *Engine) Reject(nodeID, reason string)
```

- **`Plan`** = `orchestrator.go` atual, com `*config.Config` → `rc`. Mantém injeção de `sync_repo`, filtro de duplicata (FASE 20), `enforceWorkflowOrdering` (FASE 19). `workflowRepo(cfg)` vira leitura de `rc.Repo` (precedência já resolvida pelo app).
- **`Run`** = iça o `dispatch`/`notify`/`NewExecutor` de `web/server.go:272-346` + `main.go:367-394` para dentro do Engine:
  - workflow nodes (`TypeSyncRepo`/`TypeBranch`/`TypePush`/`TypePR`) → `tools.SyncRepo`/`ExecBranch`/`ExecPush`/`ExecPR` com valores de `rc`.
  - `current_branch`: o Engine escreve/lê em `rc.WorkDir/run_meta.json` (formato `map[string]string`, compatível com `RunStore.GetRunMeta`).
  - PR title/body: Engine lê `rc.WorkDir/task.txt` + `d.RunID` (igual ao web hoje).
  - task nodes → `RunSingleTask` interno, emitindo `core.Progress(node.ID, ...)`.
  - `Executor` construído com o `CheckpointManager` interno do Engine + `rc.WorkDir`.
- **`Resume`** = carrega `dag.json` de `rc.WorkDir` (`dag.FromJSON`), constrói Executor, chama `executor.Resume`.
- **`RunSingleTask`** = `gateway.RunSpecTask` atual: monta registry (memory + tools), roda `agent.Loop`. `nodeID` fixo `"task-0"` p/ o `core.Progress`.
- **`RunIssueTask`** = `gateway.RunTask` atual: `GetIssue → CloneRepo → index README → PlanFromIssue → loop`.

### Cortes de dependência dentro do módulo
- **`agent/loop.go`** (maior risco): hoje usa `[]llm.Message` multi-turn. `core.Completer.Complete(ctx, prompt string)` é single-string — e o adapter `llm.NewCompleter` colapsa para 1 mensagem `user` de qualquer forma. ⇒ reescrever o acúmulo de conversa como **transcript de string** (systemPrompt+task, depois append de `reply` + `"Observation: ..."` a cada iter). Consistente com `react_loop_text_format` (digest). `ProgressFunc` interno permanece (func por-iteração); o Engine adapta para `core.Progress` adicionando `nodeID`.
- **`agent/planner.go`**: `PlanFromIssue(ctx, c core.Completer, issue, owner, repo)` — troca `*llm.Client`/`llm.Message` por `core.Completer`.
- **`tools/git.go`** importa `dag` → re-aponta p/ `orchestrator/dag`.
- **`memory`**: move puro; `NewEmbedFnFromConfig(provider, url, key)` já recebe strings (zero dep de config). O app o chama no composition root.
- **`go.work`** → adicionar `./orchestrator` (depois de criar go.mod + mover arquivos, p/ o workspace não quebrar no intervalo).

---

## 3. Adaptar o legado (= app)

### `internal/gateway/gateway.go` → fica fino (continua no app)
- `New(sb core.Sandbox, engine *orchestrator.Engine, rc core.RunConfig) *Gateway`.
- Mantém: `RegisterHandler`, `Run`, `Stop`, `IsVMReady`, `HealthCheckVM` (usam `sb`).
- `RunTask` → `engine.RunIssueTask`; `RunSpecTask` → `engine.RunSingleTask` (assinaturas passam a usar `core.Progress`).
- **Remove** imports: `agent`, `config`, `llm`, `memory`, `tools`. Passa a importar `orchestrator` + `core`.
- **Atenção:** `gateway/telegram.go` (não lido ainda) provavelmente referencia `agent.ProgressFunc` — trocar por `core.Progress`.

### `internal/web/server.go`
- `New(gw *gateway.Gateway, vm *sandboxpkg.VM, engine *orchestrator.Engine, reg *toolskills.Registry, llmClient *llm.Client, cfg *config.Config, runsDir string)` — **remove** o param `cm` (Engine é dono do checkpoint).
- `launchRun`: `engine.Plan(...)` → monta `notify` (broadcast do `*dag.Node`) + `prog` (`core.Progress`: NodeLog+SSE) → `go engine.Run(...)`. **Some** o dispatch closure inteiro (vai p/ o Engine).
- `launchSingleTask`: `engine.RunSingleTask(ctx, task, rc, prog)`.
- `handleApprove`/`handleReject` → `s.engine.Approve/Reject`.
- Monta `core.RunConfig` por run (helper `s.runConfig(runDir)` resolvendo `resolveRepo` + clonepath + branch + runmode).
- web **continua** importando `config`/`dag` (é app) — mas **não** importa mais `internal/tools` nem `internal/orchestrator` legado; usa `orchestrator` (módulo) + `orchestrator/dag`.
- Health checks (`healthLLM` etc.) seguem lendo `cfg` direto (app layer, ok).

### `cmd/golovebox/main.go` (composition root)
- Constrói: `vm` (core.Sandbox), `completer := llm.NewCompleter(llmClient)`, `mem` via `orchestrator/memory.New(memDir, memory.NewEmbedFnFromConfig(cfg.LLMProvider, cfg.LLMBaseURL, cfg.APIKey))` (nil-tolerante).
- `engine := orchestrator.New(vm, completer, mem)`.
- helper `buildRunConfig(cfg, workDir) core.RunConfig`.
- `gw := gateway.New(vm, engine, rc)`.
- `github` cmd → `engine.RunIssueTask`. `run` cmd → `engine.Plan` + `engine.Run` (notify interativo p/ checkpoint via `engine.Approve/Reject`). `resume` cmd → `engine.Resume`. `web` cmd → `web.New(...)` sem `cm`.
- `run`/`resume` continuam usando `web.NewRunStore` p/ `NewRun`/`RunDir` (app importando app — ok).
- Mantém imports concretos `sandboxpkg`, `toolskills`, `llm` (é o único lugar que conhece concretos — padrão FASE 23).

### Apagar
`internal/{orchestrator,dag,agent,tools,memory}/` (após o módulo compilar e os call sites migrarem).

---

## 4. Build
`magefile.go` **não muda**: `Build` compila `./cmd/golovebox` e o `go.work` resolve os módulos. `CGO_ENABLED=0` preservado. Só `go.work` ganha `./orchestrator`.

---

## 5. Ordem de execução (compilável a cada passo)

1. ✅ `core`: `Progress` + `RunConfig` (expandir p/ 6 campos).
2. Criar `orchestrator/go.mod` + mover `dag/`, `memory/`, `tools/`, `agent/` (git mv) e `orchestrator.go`; ajustar `package`/imports internos. **Ainda fora do go.work** → compilar isolado com `cd orchestrator && GOWORK=off go build ./...` falhará até cortar deps.
3. Cortar deps no módulo: `loop.go`/`planner.go` → `core.Completer` (transcript), `orchestrator.go` → `rc`, escrever `Engine` (New/Plan/Run/Resume/RunSingleTask/RunIssueTask/Approve/Reject). `GOWORK=off go build ./... && go vet ./...` limpo.
4. `go.work` += `./orchestrator`.
5. Migrar `gateway` → fino. Compilar legado.
6. Migrar `web/server.go`. Compilar.
7. Migrar `main.go`. `mage build`.
8. Apagar `internal/{orchestrator,dag,agent,tools,memory}/`. `mage build` + testes.

---

## Verificação
```bash
cd orchestrator && GOWORK=off go build ./... && go test ./... && go vet ./...
# imports: só core + go-github. grep NÃO acha internal/* nem sandbox/toolskills/derivator/config/web.
cd .. && mage build
build/golovebox.exe run <spec NEW BRANCH/PUSH/PR>   # DAG sync_repo→branch→edits→push→pr; ReAct emite Progress
build/golovebox.exe resume <run-id>                  # retoma run interrompido
```
1. Módulo compila isolado; grep nos imports do módulo → só `core` + `go-github`.
2. `Plan`: `sync_repo → branch → edits → push → pr` (FASE 19/20 preservadas).
3. `Run` paralelo (máx 3), checkpoint pausa/retoma; `Resume` funciona.
4. `core.Progress` injetado preenche NodeLog + SSE (prompt/reply por iter no painel).
5. PR contra `main` com `head=branch` (workflow node).
6. `internal/{orchestrator,dag,agent,tools,memory}/` não existem mais.

## Riscos
- **`loop.go` transcript** — único rewrite comportamental. Mitigar com teste do loop usando mock `core.Completer` (heredoc + done/error + max-iter).
- **`gateway/telegram.go`** ainda não lido — confirmar referências a `agent.ProgressFunc`.
- **Big-bang** — 5 pacotes + 3 consumidores. Mitigado pela ordem compilável (passos 2-8).
