# FASE 22 — Extração: core + promptlang

**Tipo:** Refactor arquitetural (execução da FASE 21)
**Depende de:** FASE 21 (Decomposição Modular — princípio diretor e contratos)
**Escopo:** Primeiros 2 módulos da ordem de extração. Nada mais.
**Estratégia:** Strangler fig. Um módulo de cada vez. Compila e testa isolado antes do próximo.

---

## Objetivo

Sair de 1 `go.mod` monolítico para `go.work` com 2 módulos extraídos (`core`, `promptlang`) + o módulo legado restante ainda intacto. Provar o padrão antes de escalar.

```
ANTES                          DEPOIS (fim da FASE 22)
golovebox/                     golovebox/
└── go.mod (tudo)              ├── go.work
                               ├── core/go.mod        ← novo, zero deps
                               ├── promptlang/go.mod  ← extraído de internal/dsl
                               └── go.mod (legado)    ← resto, importa os 2 acima
```

`internal/dsl` deixa de existir no módulo legado. Vira módulo `promptlang`.

## Regras absolutas
- `CGO_ENABLED=0`, build via `mage build`
- Zero escrita fora de `.golovebox/`
- `core` não importa NADA do projeto (nem stdlib pesada — só tipos + context)
- `promptlang` importa só `core` (idealmente zero deps externas)
- Não mover mais nada além de dsl nesta fase

---

## 1. Criar go.work na raiz

`go.work` amarra os módulos para dev local sem publicar nada.

```
go 1.22

use (
	.
	./core
	./promptlang
)
```

O `.` é o módulo legado atual. `go build`/`go test` na raiz resolve os 3 via workspace.

---

## 2. Módulo `core`

`core/go.mod`:
```
module github.com/<owner>/golovebox/core

go 1.22
```

`core/contracts.go` — só os contratos que a FASE 22 precisa. YAGNI: apenas `Parser` + `Intent` agora. Os outros contratos entram quando seus módulos forem extraídos (FASE 23+).

```go
package core

// Intent — AST de intenção produzido pelo parser da DSL.
// Tipos de domínio compartilhados vivem aqui, sem deps externas.
type Intent struct {
	Steps   []Step
	Repo    string
	Branch  string
}

type Step struct {
	Kind    StepKind // edit | test | push | pr | checkpoint | notify | try_else | not_todo | wait
	Text    string
	Title   string   // PR title, branch name, etc.
	OrElse  string   // fallback do TRY/OR_ELSE
}

type StepKind string

const (
	KindEdit       StepKind = "edit"
	KindTest       StepKind = "test"
	KindPush       StepKind = "push"
	KindPR         StepKind = "pr"
	KindCheckpoint StepKind = "checkpoint"
	KindNotify     StepKind = "notify"
	KindTryElse    StepKind = "try_else"
	KindNotTodo    StepKind = "not_todo"
	KindWait       StepKind = "wait"
)

// Parser — metalinguagem DSL → Intent.
type Parser interface {
	Parse(src string) (Intent, error)
}
```

**Regra:** se `core` ganhar um `import` de qualquer pacote `internal/*`, a extração está errada. Reverter.

---

## 3. Módulo `promptlang`

Origem: `internal/dsl/parser.go` (keywords NEW BRANCH, PUSH, PR, ATTENTION_HERE, RUN_TEST, TRY/OR_ELSE, NOTIFY_ME, NOT_TODO, WHEN/DO).

`promptlang/go.mod`:
```
module github.com/<owner>/golovebox/promptlang

go 1.22

require github.com/<owner>/golovebox/core v0.0.0
```
(versão `v0.0.0` + `go.work` resolve local; sem publicar)

`promptlang/parser.go`:
- Mover o parser de `internal/dsl`.
- Trocar os tipos de retorno internos pelos tipos `core.Intent`/`core.Step`/`core.StepKind`.
- Expor `New() core.Parser` (construtor) implementando `Parse(src) (core.Intent, error)`.
- Sem dependência de `config`, `dag`, `orchestrator`. Parser puro: texto entra, AST sai.

`promptlang/parser_test.go`:
- Mover os testes existentes do dsl.
- Adicionar tabela cobrindo cada keyword → `StepKind` esperado.
- Caso TRY/OR_ELSE preenche `OrElse`. Caso NOT_TODO vira `KindNotTodo`.
- `go test ./...` dentro de `promptlang/` passa isolado.

---

## 4. Adaptar o módulo legado

Quem consumia `internal/dsl` agora consome `promptlang` + `core`:

- `internal/orchestrator/orchestrator.go` — troca import `internal/dsl` por `promptlang`; usa `core.Intent` em vez do tipo antigo do dsl.
- Qualquer outro consumidor do dsl: mesmo ajuste.
- Apagar `internal/dsl/` do módulo legado após confirmar que nada mais importa.

Mapeamento de tipos antigo→novo é mecânico. Se o tipo do dsl tinha campos que `core.Step` não cobre, adicionar em `core` (não criar tipo paralelo no legado).

---

## 5. Ajustar build

`magefile.go`:
- `build` roda na raiz; `go.work` resolve os módulos. Confirmar que `mage build` continua gerando o binário único.
- Se mage chama `go build ./...`, validar que cobre os módulos do workspace (ou iterar `core`, `promptlang`, `.`).
- `CGO_ENABLED=0` preservado em todos.

---

## Arquivos

| Ação | Arquivo |
|---|---|
| Criar | `go.work` |
| Criar | `core/go.mod`, `core/contracts.go` |
| Criar | `promptlang/go.mod`, `promptlang/parser.go`, `promptlang/parser_test.go` |
| Editar | `internal/orchestrator/orchestrator.go` (import + tipos) |
| Editar | `magefile.go` (se necessário p/ workspace) |
| Apagar | `internal/dsl/` (após migração) |

---

## Verificação

```bash
cd core && go test ./... && go vet ./...        # zero import de projeto
cd ../promptlang && go test ./...               # parser isolado passa
cd .. && mage build                             # binário único compila
build/golovebox.exe web                         # smoke: submeter spec com DSL
```

1. `core` compila sem importar nenhum `internal/*` (grep no import block).
2. `promptlang` testa isolado, importa só `core`.
3. Spec com `NEW BRANCH / PUSH / PR` ainda gera o mesmo DAG de antes (parser equivalente).
4. `internal/dsl/` não existe mais.

---

## Definição de Pronto

> `go.work` com `core` + `promptlang` + legado. `core` zero-dep. `promptlang` parseia a DSL atrás de `core.Parser`, testado isolado. Orchestrator consome via interface. Comportamento do parser idêntico ao da FASE 20. Padrão de extração provado e repetível para os próximos módulos.

---

## Anti-padrões (desta fase)

- **Extrair mais que dsl.** Sandbox, derivator, orchestrator ficam para FASE 23+.
- **`core` com lógica.** core é só tipos + interfaces. Lógica de parse vive em `promptlang`.
- **Tipo paralelo no legado.** Se faltou campo, estende `core.Step`, não cria struct duplicada.
- **Publicar módulo.** Tudo local via `go.work`. Sem tag, sem proxy.

---

## Próxima Fase (FASE 23)

Extrair `sandbox` (substrate QEMU/SSH) atrás de `core.Sandbox`. Depois `derivator`, `toolskills`, `orchestrator`, e por fim isolar `app`.
