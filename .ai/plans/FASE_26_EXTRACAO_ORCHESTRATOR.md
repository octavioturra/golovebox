# FASE 26 — Extração: orchestrator

**Tipo:** Refactor arquitetural (sexta execução da FASE 21)
**Depende de:** FASE 25 (`derivator` extraído, `core.Completer` formalizado, `llm.NewCompleter` adapter)
**Escopo:** Módulo `orchestrator` — o mais pesado. Engine de execução: intent→DAG + executor + agent loop + tools.
**Marco:** Penúltimo passo. Consome 3 contratos do core por injeção. Depois sobra só isolar `app`.

---

## Objetivo

Mover o engine de execução para módulo `orchestrator/`, atrás de injeção de `core.*`. O legado deixa de conter a lógica de planejamento, DAG e ReAct — vira só `app` (wiring + Web UI + gateway + config + setup).

```
ANTES                          DEPOIS
golovebox/                     golovebox/
├── go.work (6)                ├── go.work (7)
├── core/ promptlang/          ├── core/ promptlang/ sandbox/
│   sandbox/ toolskills/       │   toolskills/ derivator/
│   derivator/                 ├── orchestrator/go.mod ← engine extraído
└── go.mod (legado)            └── go.mod (legado = app)
   ├── internal/orchestrator/     └── internal/{web,gateway,config,setup,memory}/
   ├── internal/dag/
   ├── internal/agent/
   └── internal/tools/
```

## Regras absolutas
- `CGO_ENABLED=0`, build via `mage build`
- `orchestrator` importa só `core` (+ deps externas: go-github, x/crypto já transitiva)
- **Nunca** importa `sandbox`/`toolskills`/`derivator` concretos — recebe `core.Sandbox`, `core.Completer`, `core.Deriver`, `core.Provisioner` por injeção
- Não conhece `web`, `gateway`, `config` — recebe valores e callbacks por parâmetro
- core continua zero-dep de projeto

---

## 0. Mapear o que move

O engine hoje está espalhado em 4 pacotes acoplados. Movem juntos (são um produto só — o workflow engine):

| Pacote legado | Papel |
|---|---|
| `internal/orchestrator/` | intent → DAG via LLM, enforce workflow ordering, fallback repo |
| `internal/dag/` | tipos de node, executor paralelo, checkpoint pre-buffering, resume |
| `internal/agent/` | ReAct loop (task nodes), ProgressFunc |
| `internal/tools/` | shell, files (via core.Sandbox), git, github |

Já usam `core.Intent` (desde FASE 22) e `core.Sandbox` nas tools (desde FASE 23). A extração é mover + cortar deps de `config`/`web`.

---

## 1. Contratos no core

A maior parte já existe. Adicionar só o necessário para tirar o orchestrator das deps de `config`/`web`:

```go
package core

// Progress — callback de observabilidade por iteração ReAct.
// app implementa (NodeLog + SSE); orchestrator só chama.
type Progress func(nodeID string, iter int, action, params, obs, prompt, reply string)

// RunConfig — parâmetros de execução injetados pelo app (sem importar internal/config).
type RunConfig struct {
	Repo          string
	DefaultBranch string
	GitHubToken   string
	WorkDir       string // dentro de .golovebox/runs/<id>/
}
```

`Progress` substitui o acoplamento direto do agent loop com `web.NodeLog`. CodingAgent (V1) **não** entra agora — YAGNI; o orchestrator V0 coordena o ReAct loop, não CLIs externos.

> Nota: se `RunConfig` crescer, manter mínimo. Tokens e paths vêm do composition root.

---

## 2. Módulo `orchestrator`

`orchestrator/go.mod`:
```
module github.com/user/golovebox/orchestrator

go 1.22

require (
	github.com/user/golovebox/core v0.0.0
	github.com/google/go-github/v60 v...
	golang.org/x/crypto v...   // se tools/git precisar
)
```
+ `replace ... core => ../core`.

Estrutura interna (sub-pacotes do módulo, não mais `internal/` do legado):
```
orchestrator/
├── go.mod
├── orchestrator.go     # Plan(intents []core.Intent) → DAG; enforce ordering; sync_repo inject
├── dag/                # tipos, executor paralelo, checkpoint, resume
├── agent/              # ReAct loop; usa core.Sandbox + core.Progress
└── tools/              # shell/files (core.Sandbox), git, github (token via RunConfig)
```

API pública que o `app` chama:
```go
func New(sb core.Sandbox, llm core.Completer, deriver core.Deriver,
         prov core.Provisioner) *Engine

func (e *Engine) Plan(intents []core.Intent, rc core.RunConfig) (*dag.DAG, error)
func (e *Engine) Run(ctx context.Context, d *dag.DAG, rc core.RunConfig,
                     prog core.Progress) error
func (e *Engine) Resume(ctx context.Context, runDir string, rc core.RunConfig,
                        prog core.Progress) error
```

- `Plan` injeta `sync_repo` como root, força `branch→edits→push→pr`, pós-processamento defensivo (FASE 19).
- `Run` executa paralelo (máx 3), checkpoint pre-buffering race-free, persiste atômico via `os.Rename`.
- Workflow nodes (sync_repo/branch/push/pr) despachados direto via tools/git + tools/github — fora do ReAct loop.
- Task nodes entram no agent loop com task auto-contida, emitindo `core.Progress`.

Cortes de dependência:
- `tools/shell`, `tools/files` → `core.Sandbox` (já feito FASE 23, confirmar).
- `tools/github` → token via `core.RunConfig`, não `internal/config`.
- `agent` → emite `core.Progress` em vez de chamar `web.NodeLog` direto.
- Persistência de `dag.json`/`node_states.json` → recebe `runDir` (string), não conhece `.golovebox/` layout.

`go.work` → adicionar `./orchestrator`.

---

## 3. Memory (decisão)

`internal/memory/` (chromem-go) é usado pela tool `search_memory` do agent. Opções:
- **(a)** mover `memory` para dentro de `orchestrator/` (consumidor único hoje).
- **(b)** definir `core.Memory` interface e injetar.

Escolher **(a)** por YAGNI — consumidor único, sem segundo cliente. Se `app` precisar de memory depois, promove para `core.Memory` (mesma regra do `Completer` na FASE 25). Mover `internal/memory/` → `orchestrator/memory/`.

---

## 4. Adaptar o legado (= app)

- `internal/web/server.go` — substitui chamadas diretas a `orchestrator`/`dag` por `orchestrator.New(...)` + `Plan`/`Run`. Implementa `core.Progress` que escreve no `NodeLog` + faz SSE broadcast. Monta `core.RunConfig` a partir de `internal/config`.
- `internal/gateway/gateway.go` — idem para runs via Telegram.
- `cmd/golovebox/main.go` — composition root: injeta `sandbox.VM` (core.Sandbox), `llm.NewCompleter` (core.Completer), `derivator.New` (core.Deriver), `toolskills.NewProvisioner` (core.Provisioner) no `orchestrator.New`. Comandos `run`/`resume`/`github` chamam o Engine.
- Apagar `internal/orchestrator/`, `internal/dag/`, `internal/agent/`, `internal/tools/`, `internal/memory/`.

---

## 5. Build

`magefile.go`: cobrir `orchestrator` no workspace. `CGO_ENABLED=0` preservado.

---

## Arquivos

| Ação | Arquivo |
|---|---|
| Editar | `core/contracts.go` (+ Progress, RunConfig) |
| Criar | `orchestrator/go.mod` |
| Mover | `internal/orchestrator/` → `orchestrator/orchestrator.go` |
| Mover | `internal/dag/` → `orchestrator/dag/` |
| Mover | `internal/agent/` → `orchestrator/agent/` (ProgressFunc → core.Progress) |
| Mover | `internal/tools/` → `orchestrator/tools/` (github token via RunConfig) |
| Mover | `internal/memory/` → `orchestrator/memory/` |
| Editar | `internal/web/server.go` (Engine + core.Progress + RunConfig) |
| Editar | `internal/gateway/gateway.go` |
| Editar | `cmd/golovebox/main.go` (wiring completo) |
| Editar | `go.work` (+ ./orchestrator), `magefile.go` |
| Apagar | `internal/{orchestrator,dag,agent,tools,memory}/` |

---

## Verificação

```bash
cd orchestrator && GOWORK=off go test ./... && go vet ./...  # importa só core + go-github
cd .. && mage build
build/golovebox.exe web   # spec NEW BRANCH/PUSH/PR → DAG correto, ReAct no node panel
```

1. `orchestrator` compila isolado; grep nos imports não acha `internal/*` nem `sandbox`/`toolskills`/`derivator`/`config`/`web`. Só `core` + libs externas.
2. `Plan` gera `sync_repo → branch → edits → push → pr` (invariantes FASE 19/20 preservadas).
3. `Run` paralelo, checkpoint pausa/retoma, `Resume` de run interrompido funciona.
4. `core.Progress` injetado pelo app preenche NodeLog + SSE — prompt/reply por iter visíveis no painel.
5. PR criado contra `main` com `head=branch` (workflow node, não agent).
6. `internal/{orchestrator,dag,agent,tools,memory}/` não existem mais.

---

## Definição de Pronto

> `go.work` com 7 módulos. `orchestrator` é o engine atrás de injeção de `core.Sandbox`/`core.Completer`/`core.Deriver`/`core.Provisioner`, emite `core.Progress`, recebe `core.RunConfig`. Não conhece web/gateway/config. O legado encolheu para `app`. Trocar qualquer substrato (VM, LLM, deriver) não toca no engine.

---

## Anti-padrões (desta fase)

- **Big-bang.** É a maior fase: mover 5 pacotes. Ainda assim, um módulo só. Compilar e testar antes de seguir.
- **`orchestrator` importar `config`/`web`.** Valores via `RunConfig`, observabilidade via `core.Progress`.
- **CodingAgent adapters agora.** É V1 (v1_fase1). YAGNI nesta fase.
- **`core.Memory` especulativo.** Memory move para dentro do orchestrator; só promove se 2º consumidor surgir.
- **Agent chamar `web.NodeLog`.** Só `core.Progress`. O app traduz para NodeLog + SSE.

---

## Próxima Fase (FASE 27)

Isolar `app` — último módulo. `cmd/golovebox` + `internal/{web,gateway,config,setup,embed}` viram `app/go.mod`. Raiz fica só com `go.work`. Composition root puro: injeta todos os `core.*`. Trocar implementação de qualquer módulo = mudança de um arquivo no app.