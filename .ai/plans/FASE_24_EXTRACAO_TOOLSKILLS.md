# FASE 24 — Extração: toolskills

**Tipo:** Refactor arquitetural (quarta execução da FASE 21)
**Depende de:** FASE 23 (`sandbox` extraído, `core.Sandbox` = Exec/PutFile/GetFile/Ready, composition root único em main.go)
**Escopo:** Módulo 5 da ordem de extração — `toolskills`. Nada mais.
**Marco:** Primeiro módulo que **consome** `core.Sandbox`. Prova injeção entre módulos extraídos.

---

## Objetivo

Mover `internal/skills/` para módulo próprio `toolskills/`, atrás de `core.Provisioner`. O módulo recebe `core.Sandbox` por injeção e provisiona recursos Linux na VM a partir de metadados de skill — sem conhecer o tipo concreto `sandbox.VM`.

```
ANTES                          DEPOIS
golovebox/                     golovebox/
├── go.work (4)                ├── go.work (5)
├── core/                      ├── core/          ← ganha Provisioner + SkillMeta
├── promptlang/                ├── promptlang/
├── sandbox/                   ├── sandbox/
└── go.mod (legado)            ├── toolskills/go.mod ← extraído de internal/skills
   └── internal/skills/        └── go.mod (legado) ← consome core.Provisioner
```

## Regras absolutas
- `CGO_ENABLED=0`, build via `mage build`
- `toolskills` importa só `core` (+ BurntSushi/toml para frontmatter)
- `toolskills` **nunca** importa `sandbox` — recebe `core.Sandbox` por injeção
- core continua zero-dep de projeto (só stdlib)

---

## 1. Contrato no core

`core/contracts.go` — adicionar:

```go
package core

import "context"

// SkillMeta — frontmatter TOML de uma skill (.golovebox/skills/<x>.md).
type SkillMeta struct {
	Name        string
	Description string
	Tools       []string // ex: read_file, shell, search_memory
	Provision   []string // comandos de setup a rodar na VM (apk add, etc.)
}

// Provisioner — prepara o substrate para uma skill.
// Implementado por toolskills; recebe um Sandbox por injeção.
type Provisioner interface {
	Provision(ctx context.Context, skill SkillMeta) error
}
```

`Provision []string` cobre o caso real (provisionar recursos Linux via skill metadata). Sem campos especulativos (YAGNI).

---

## 2. Módulo `toolskills`

`toolskills/go.mod`:
```
module github.com/user/golovebox/toolskills

go 1.22

require (
	github.com/user/golovebox/core v0.0.0
	github.com/BurntSushi/toml v...
)
```
+ `replace github.com/user/golovebox/core => ../core` (mesmo padrão FASE 22/23 — `v0.0.0` sem tag exige replace mesmo com go.work no Go 1.22+).

Mover de `internal/skills/`: `registry.go`, `generator.go`.

`toolskills/registry.go`:
- `Registry` — carrega `.md` com frontmatter TOML de um diretório, parseia para `core.SkillMeta`.
- Path do dir de skills vem por parâmetro (não hardcoded) — composition root injeta `.golovebox/skills/`.

`toolskills/provisioner.go` — **novo.** Implementa `core.Provisioner`:
```go
type provisioner struct{ sb core.Sandbox }

func NewProvisioner(sb core.Sandbox) core.Provisioner { return &provisioner{sb} }

func (p *provisioner) Provision(ctx context.Context, skill core.SkillMeta) error {
	for _, cmd := range skill.Provision {
		out, err := p.sb.Exec(ctx, cmd)
		if err != nil { return err }
		if out.ExitCode != 0 { /* erro com out.Stderr */ }
	}
	return nil
}
```
`toolskills` depende de `core.Sandbox` — não de `sandbox.VM`. Cumpre a regra de dependência da FASE 21.

`toolskills/generator.go` — gera skill via LLM. Recebe o LLM client por interface (ainda no legado nesta fase; passar por parâmetro/func, não importar `internal/llm`). Se precisar de contrato, adicionar `core.SkillGenerator` — mas só se houver consumidor real (YAGNI: provavelmente basta uma func injetada).

`toolskills/registry_test.go`:
- Tabela: frontmatter TOML → `core.SkillMeta` esperado.
- `Provision` com Sandbox mock (fake `core.Sandbox` que registra comandos) → verifica ordem e propagação de erro em ExitCode != 0.
- `go test ./...` passa isolado, sem legado e sem sandbox real.

`go.work` → adicionar `./toolskills`.

---

## 3. Adaptar o legado

Consumidores de `internal/skills`:
- `internal/agent/` (loop/tools) — usa Registry para resolver skill por nome. Trocar import por `toolskills`; tipos viram `core.SkillMeta`.
- `internal/gateway/gateway.go` — `buildRegistry()` passa a usar `toolskills.NewRegistry(skillsDir)`; se provisiona, injeta `toolskills.NewProvisioner(g.sb)` (o `g.sb core.Sandbox` já existe desde FASE 23).
- `cmd/golovebox/main.go` — composition root: constrói `toolskills.NewRegistry(...)` + `toolskills.NewProvisioner(vm)` (onde `vm` é o `*sandbox.VM` injetado como `core.Sandbox`). Único lugar que liga sandbox concreto ↔ toolskills.
- Comando CLI `skill list / generate` — migrar para `toolskills`.

Apagar `internal/skills/` após migração.

---

## 4. Build

`magefile.go`:
- Confirmar que `mage build` cobre `toolskills` no workspace.
- `CGO_ENABLED=0` preservado.

---

## Arquivos

| Ação | Arquivo |
|---|---|
| Editar | `core/contracts.go` (+ Provisioner, SkillMeta) |
| Criar | `toolskills/go.mod`, `toolskills/provisioner.go`, `toolskills/registry_test.go` |
| Mover | `internal/skills/{registry,generator}.go` → `toolskills/` |
| Editar | `internal/agent/` (import + `core.SkillMeta`) |
| Editar | `internal/gateway/gateway.go` (buildRegistry + NewProvisioner) |
| Editar | `cmd/golovebox/main.go` (wiring sandbox→toolskills) |
| Editar | `cmd/golovebox/main.go` (CLI skill list/generate) |
| Editar | `go.work` (+ ./toolskills), `magefile.go` |
| Apagar | `internal/skills/` |

---

## Verificação

```bash
cd toolskills && go test ./... && go vet ./...   # importa core + toml; NÃO importa sandbox
cd .. && mage build
build/golovebox.exe skill list                   # registry lê .golovebox/skills/
build/golovebox.exe web                          # run usando skill que provisiona
```

1. `toolskills` compila isolado; grep nos imports não acha `internal/*` **nem** `sandbox`.
2. `Provision` roda comandos via `core.Sandbox` mock no teste (sem VM).
3. `skill list` lê e parseia frontmatter TOML para `core.SkillMeta`.
4. Run real: skill com `Provision = ["apk add ..."]` instala na VM via Sandbox injetado.
5. `internal/skills/` não existe mais.

---

## Definição de Pronto

> `go.work` com 5 módulos. `toolskills` implementa `core.Provisioner`, consome `core.Sandbox` por injeção, testa isolado com mock. main.go liga `sandbox.VM` → `toolskills.NewProvisioner`. Trocar o substrate não toca em toolskills. Cross-module injection (`sandbox` → `toolskills` via `core`) provado.

---

## Anti-padrões (desta fase)

- **`toolskills` importar `sandbox`.** Recebe `core.Sandbox`, nunca o concreto. Se importou sandbox, errou.
- **Path de skills hardcoded.** Injetar `skillsDir` no construtor.
- **`core.SkillGenerator` especulativo.** Só criar contrato se houver consumidor — senão func injetada.
- **Extrair derivator/orchestrator junto.** FASE 25+.

---

## Próxima Fase (FASE 25)

Extrair `derivator` (prompt → grafo de prompts priorizados) atrás de `core.Deriver`. Depois `orchestrator`, e por fim isolar `app` (composition root + Web UI + gateway).
