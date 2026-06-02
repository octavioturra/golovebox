# FASE 27 — Isolamento: app (fim da decomposição V0)

**Status:** concluída  
**Data:** 2026-06-02

## Objetivo

Completar a decomposição strangler-fig movendo o código legado restante (`cmd/`, `internal/`, `magefile.go`, `go.mod`) para o módulo `app/`. A raiz do repositório torna-se workspace-only: apenas `go.work` sem `go.mod`.

## O que foi feito

### Movimentos de arquivo

```
cmd/          → app/cmd/
internal/     → app/internal/
magefile.go   → app/magefile.go
go.mod        → app/go.mod   (módulo: github.com/user/golovebox/app)
go.sum        → app/go.sum
```

Executado via `git mv` para preservar histórico.

### `app/go.mod` — recriado

```
module github.com/user/golovebox/app
go 1.26

require (
    github.com/user/golovebox/core        v0.0.0-00010101000000-000000000000
    github.com/user/golovebox/promptlang  v0.0.0-00010101000000-000000000000
    github.com/user/golovebox/sandbox     v0.0.0-00010101000000-000000000000
    github.com/user/golovebox/toolskills  v0.0.0-00010101000000-000000000000
    github.com/user/golovebox/derivator   v0.0.0-00010101000000-000000000000
    github.com/user/golovebox/orchestrator v0.0.0-00010101000000-000000000000
    // + external deps
)

replace (
    github.com/user/golovebox/core        => ../core
    github.com/user/golovebox/promptlang  => ../promptlang
    github.com/user/golovebox/sandbox     => ../sandbox
    github.com/user/golovebox/toolskills  => ../toolskills
    github.com/user/golovebox/derivator   => ../derivator
    github.com/user/golovebox/orchestrator => ../orchestrator
)
```

Note: `replace` usa `../X` (relativo ao diretório do módulo `app/`).

### Import paths atualizados

Todos os imports internos migrados de:

```
"github.com/user/golovebox/internal/..."
```

para:

```
"github.com/user/golovebox/app/internal/..."
```

### `go.work` — raiz do workspace

```
go 1.26

use (
    ./app
    ./core
    ./promptlang
    ./sandbox
    ./toolskills
    ./derivator
    ./orchestrator
)
```

`go.mod` e `go.sum` removidos da raiz.

## Estado final da decomposição

```
golovebox/
├── go.work            ← workspace-only, sem go.mod
├── app/               ← composition root + CLI + internal (config, embed, gateway, llm, setup, web)
│   ├── go.mod
│   ├── magefile.go
│   └── cmd/golovebox/main.go
├── core/              ← contratos, zero deps
├── promptlang/        ← DSL parser
├── sandbox/           ← VM QEMU + core.Sandbox impl
├── toolskills/        ← skills registry + provisioner
├── derivator/         ← seed → PromptGraph
└── orchestrator/      ← Engine + agent/dag/memory/tools sub-pacotes
```

## Regras arquiteturais preservadas

- Setas de dependência nunca apontam para `app/` — todos os módulos produto só importam `core`
- `app/` é o único módulo que importa todos os outros (composition root)
- `CGO_ENABLED=0` preservado em `magefile.go`
- Zero hardcoded paths — `filepath.Join` e baseDir relativo ao executável
- `go build ./app/...` passa sem erros

## Invariante verificado

```
grep -r 'golovebox/app' core/ promptlang/ sandbox/ toolskills/ derivator/ orchestrator/ → zero resultados
```
