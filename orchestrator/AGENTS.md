# orchestrator — AGENTS.md

## Responsabilidade única
Engine de execução: converte `[]core.Intent` em DAG, executa nodes em paralelo, roda o ReAct loop para task nodes, despacha workflow nodes (sync_repo/branch/push/pr). É dono da persistência de estado de run (`dag.json`, `run_meta.json`). Não conhece HTTP, WebSocket, Telegram ou embed.

## API pública
`Engine` — instanciado pelo composition root via `New(sb, completer, mem)`:

```go
func New(sb core.Sandbox, completer core.Completer, mem *memory.Memory) *Engine

func (e *Engine) Plan(ctx, runID string, intents []core.Intent, rc core.RunConfig) (*dag.DAG, error)
func (e *Engine) Run(ctx context.Context, d *dag.DAG, rc core.RunConfig, notify dag.NotifyFunc, prog core.Progress) error
func (e *Engine) Resume(ctx context.Context, rc core.RunConfig, notify dag.NotifyFunc, prog core.Progress) error
func (e *Engine) RunSingleTask(ctx context.Context, task string, rc core.RunConfig, prog core.Progress) (string, error)
func (e *Engine) RunIssueTask(ctx context.Context, owner, repo string, issueNum int, rc core.RunConfig, prog core.Progress) (string, error)
func (e *Engine) Approve(nodeID string)
func (e *Engine) Reject(nodeID, reason string)
```

## Dependências permitidas
- `core`
- `github.com/google/go-github/v60` — issue tasks
- `github.com/google/uuid` — run IDs
- `github.com/philippgille/chromem-go` — memory (sub-pacote `orchestrator/memory/`)
- NUNCA importar: `sandbox`, `toolskills`, `derivator`, `app/internal/*`

## Recebe por injeção
- `core.Sandbox` — execução de comandos na VM
- `core.Completer` — ReAct loop + planner
- `*memory.Memory` — busca vetorial (nil-tolerante: search vira no-op)

## Sub-pacotes
| Pacote | Função |
|---|---|
| `dag/` | Tipos `DAG`/`Node`, executor paralelo, `CheckpointManager`, persistência atômica |
| `agent/` | ReAct loop (transcript string), planner de issues, registry de tools |
| `tools/` | Implementações de tools: shell, files, git, github |
| `memory/` | Wrapper chromem-go, embed-fn OpenAI-compat |

## Dois canais de observabilidade
- `dag.NotifyFunc(*dag.Node)` — transições de estado de node (pending→running→done/error). Consumido por `web.RunStore` para SSE.
- `core.Progress` — callback por iteração ReAct (action/params/obs/prompt/reply). Consumido por gateway e web para NodeLog.

## Verificação local
```bash
cd orchestrator && GOWORK=off go test ./... && go vet ./...
```

## Regras absolutas
- CGO_ENABLED=0
- Engine é dono de `dag.json`/`node_states.json`/`run_meta.json` em `rc.WorkDir` — somente ele escreve
- `web.RunStore` lê esses arquivos, nunca escreve
- Invariante FASE 19/20: `sync_repo` sempre primeiro node; `enforceWorkflowOrdering` (branch→edits→push→pr)
