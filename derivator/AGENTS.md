# derivator — AGENTS.md

## Responsabilidade única
Transforma um seed text em um grafo de prompts priorizados (`core.PromptGraph`). Uma chamada LLM, parse de JSON, validação de prioridades. Não conhece agentes, DAG, VM ou skills.

## Contrato que cumpre
Implementa `core.Deriver` (`Derive(ctx, seed string) (core.PromptGraph, error)`).
Assinatura congelada — mudar exige protocolo de contrato (ver `core/AGENTS.md`).

## Dependências permitidas
- `core`
- NUNCA importar: `sandbox`, `toolskills`, `orchestrator`, `app/internal/*`

## Recebe por injeção
- `core.Completer` — fornecido pelo composition root; não importa `*llm.Client`

## Verificação local
```bash
cd derivator && GOWORK=off go test ./... && go vet ./...
```

## Arquivos
| Arquivo | Função |
|---|---|
| `derivator.go` | `deriver` struct, `New(core.Completer) core.Deriver`, lógica de parse JSON + strip markdown fences |
| `derivator_test.go` | 4 testes: parse de prioridades+deps, strip de fences, erro JSON malformado, erro prioridade desconhecida |

## Regras absolutas
- CGO_ENABLED=0
- Uma chamada LLM por `Derive` — sem estado, sem retry próprio
- Prioridades válidas: `critical`, `enabling`, `accessory` (definidas em `core`)
