# toolskills — AGENTS.md

## Responsabilidade única
Registry de skills (TOML frontmatter + prompt livre) e provisionamento de recursos Linux na VM a partir de skill metadata. Não conhece agentes, DAG ou LLM de orquestração.

## Contratos que cumpre
- `core.Provisioner` (`Provision(ctx, core.SkillMeta) error`) — executa comandos de setup na VM

## Dependências permitidas
- `core`
- `github.com/BurntSushi/toml` — parse de frontmatter
- NUNCA importar: `sandbox`, `derivator`, `orchestrator`, `app/internal/*`

## Recebe por injeção
- `core.Sandbox` — para provisionar (`Provisioner`); não importa `*sandbox.VM`
- `core.Completer` — para gerar skill via LLM (`Generate`); não importa `*llm.Client`

## Verificação local
```bash
cd toolskills && GOWORK=off go test ./... && go vet ./...
```

## Arquivos
| Arquivo | Função |
|---|---|
| `registry.go` | `Registry` — carrega, indexa e recarrega skills de um diretório |
| `provisioner.go` | `provisioner` — implementa `core.Provisioner` sobre `core.Sandbox` |
| `generator.go` | `Generate` — usa `core.Completer` para gerar skill a partir de descrição |
| `registry_test.go` | 4 testes com `mockSandbox` implementando `core.Sandbox` |

## Regras absolutas
- CGO_ENABLED=0
- Zero paths hardcoded — diretório de skills passado no construtor
- Skills residem em `.golovebox/skills/` — responsabilidade de path do app, não do módulo
