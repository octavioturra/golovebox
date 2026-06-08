# FASE 22 — Extração: core + promptlang

**Tipo:** Refactor arquitetural (primeira execução da FASE 21)  
**Depende de:** FASE 21 (princípio diretor + contratos)  
**Escopo:** Módulos 1 e 2 da ordem de extração. Nada mais.

---

## Objetivo

Sair de 1 `go.mod` monolítico para `go.work` com 2 módulos extraídos (`core`, `promptlang`) + o módulo legado restante.

```
ANTES                          DEPOIS
golovebox/                     golovebox/
└── go.mod (tudo)              ├── go.work
                               ├── core/go.mod        ← novo, zero deps
                               ├── promptlang/go.mod  ← extraído de internal/dsl
                               └── go.mod (legado)    ← resto, importa os 2
```

`internal/dsl` deixa de existir no módulo legado. Vira módulo `promptlang`.

---

## O que foi feito

### 1. `core/contracts.go`

Só os contratos necessários agora (YAGNI):

```go
package core

type Intent struct {
    Source    string
    Raw       string
    Steps     []Step
    TechDebts []string
    Repo      string
    Branch    string
}
type Step struct {
    Kind   StepKind
    Text   string
    Title  string  // branch name, PR title
    OrElse string  // fallback TRY/OR_ELSE
    Line   int
}
type StepKind string
const (
    KindBranch, KindCheckpoint, KindEdit, KindNotTodo,
    KindNotify, KindPR, KindPush, KindTest, KindTryElse, KindWait StepKind = ...
)
type Parser interface { Parse(src string) (Intent, error) }
```

`core` não importa nenhum pacote — nem `context`. Zero deps.  
`KindBranch` adicionado (não estava no rascunho FASE 21) porque `internal/dsl` tinha `KwNewBranch` e o spec diz "se o tipo do dsl tinha campos que `core.Step` não cobre, adicionar em `core`".

### 2. `promptlang/`

Origem: `internal/dsl/keywords.go` + `parser.go`.

- `keywords.go`, `parser.go` — código original, package `promptlang`
- `adapter.go` — `ToIntent(spec *ParsedSpec) core.Intent`: mapeia cada `Annotation` para `core.Step`:
  - `KwNewBranch` → `KindBranch` (Title = branch name)
  - `KwPush` → `KindPush`
  - `KwPR` → `KindPR` (Title = PR title)
  - `KwAttentionHere` / `KwPauseToReview` → `KindCheckpoint`
  - `KwRunTest` → `KindTest`
  - `KwNotifyMe` → `KindNotify`
  - `KwTry` + `KwOrElse` → único `KindTryElse` (Text + OrElse emparelhados)
  - `KwWhen` → `KindWait`
  - `KwNotTodo` → vai para `TechDebts`, não vira Step
- `impl.go` — `New() core.Parser` (construtor que retorna `core.Parser`)
- `parser_test.go` — tabela cobrindo cada keyword → `StepKind` esperado; caso TRY/OR_ELSE emparelha corretamente; NOT_TODO vai para TechDebts

### 3. Módulo legado atualizado

- `internal/orchestrator/orchestrator.go`: troca `[]*dsl.ParsedSpec` por `[]core.Intent`; `enforceWorkflowOrdering` usa `step.Kind == core.KindBranch` etc.; `buildPlanningPrompt` lê `intent.Source`, `intent.Raw`, `intent.Steps`
- `internal/web/server.go`: `ParseDir/ParseFile` via `promptlang`, converte com `ToIntent`, passa `[]core.Intent` para `launchRun`/`Plan`
- `cmd/golovebox/main.go`: mesma migração
- `go.mod`: `require` + `replace` para `core` e `promptlang` (versão placeholder `v0.0.0-00010101000000-000000000000`)
- `internal/dsl/` deletado

### 4. `go.work`

```
go 1.26
use (
    .
    ./core
    ./promptlang
)
```

Nota: `go.work` `use` não dispensa o `replace` em `go.mod` para módulos locais sem tag git publicada — ambos são necessários com Go 1.26 para evitar lookup de rede.

---

## Verificação

```bash
# core: zero imports, compila
cd core && go build ./... && go vet ./...

# promptlang: testa isolado
cd promptlang && go test ./...
# ok  github.com/user/golovebox/promptlang  0.003s

# legado: compila com os novos módulos
cd .. && go build ./...
```

Checklist:
- [x] `core` sem nenhum `import`
- [x] `promptlang` testa isolado (sem legado no `$GOPATH`)
- [x] `internal/dsl/` não existe mais
- [x] `go.work` com 3 entradas (não 7 — módulos prematuros não extraídos)
- [x] `orchestrator` usa `[]core.Intent` (sem referência a `dsl.`)
- [x] Comportamento do parser equivalente ao pre-FASE 22 (mesmo código, só renomeado)

---

## Decisões de Design

### `core.Intent.Steps` vs `core.Intent.Annotations`

O rascunho FASE 21 descrevia `Intent` com `Annotations []Annotation` (espelho sintático do dsl). A FASE 22 adota `Steps []Step` com `StepKind` — modelo semântico. Motivo: `orchestrator.enforceWorkflowOrdering` já tomava decisões baseadas em `keyword type`; com StepKind essa lógica fica explícita e testável sem depender de strings de keyword.

### `replace` no go.mod + go.work

Go workspace deveria dispensar `replace` para módulos locais, mas com Go 1.26 e módulos sem tag publicada (`v0.0.0`), o toolchain ainda tenta resolver via rede antes de consultar o workspace. Solução: `replace` no go.mod aponta para o diretório local. `go.work` provê resolução para os módulos que importam entre si dentro do workspace.

### `KindBranch` adicionado ao core

O rascunho FASE 21 não listava `KindBranch` nos `StepKind`. Adicionado porque `KwNewBranch` existe no DSL e cria um `dag.TypeBranch` no orchestrator. A regra da FASE 21 é: "se o tipo do dsl tinha campos que `core.Step` não cobre, adicionar em `core`".

---

## Aprendizados para FASES.json

- **Workspace Go + módulos locais sem tag**: `replace` no go.mod continua necessário mesmo com `go.work`. `v0.0.0` sem tag causa lookup de rede.
- **Parser puro + adapter limpo**: mover o DSL parser para módulo próprio forçou separar representação sintática (ParsedSpec/Annotation) da semântica (core.Intent/Step). Essa separação torna o orchestrator mais legível.
- **Strangler fig funciona**: extrair só 2 módulos (core+promptlang), compilar, testar, commit — sem tocar em sandbox/skills/orchestrator. Padrão repetível para FASE 23.

---

## Limitações (escopo desta fase)

- `sandbox`, `toolskills`, `derivator`, `orchestrator` ainda em `internal/` — extração em FASE 23+.
- `app` ainda é o módulo raiz (go.mod na raiz). Será isolado por último.
- `core` só tem `Parser`/`Intent` — outros contratos entram quando seus módulos forem extraídos.
