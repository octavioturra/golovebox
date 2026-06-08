# promptlang — AGENTS.md

## Responsabilidade única
Converte texto DSL (spec files `.md` com keywords golovebox) em `core.Intent`. Parser puro — sem I/O, sem estado, sem dependência além de `core`.

## Contrato que cumpre
Implementa `core.Parser` (`Parse(src string) (core.Intent, error)`).
Assinatura congelada — mudar exige protocolo de contrato (ver `core/AGENTS.md`).

## Dependências permitidas
- `core` (sempre)
- NUNCA importar: `sandbox`, `toolskills`, `derivator`, `orchestrator`, `app/internal/*`

## Recebe por injeção
Nada — parser é stateless. Instanciado diretamente pelo composition root.

## Verificação local
```bash
cd promptlang && GOWORK=off go test ./... && go vet ./...
```

## Arquivos
| Arquivo | Função |
|---|---|
| `parser.go` | Tokenização e extração de keywords |
| `keywords.go` | Constantes das keywords DSL |
| `impl.go` | `Parser` concreto |
| `adapter.go` | Converte parsed spec → `core.Intent` |
| `parser_test.go` | Testes unitários |

## Regras absolutas
- CGO_ENABLED=0
- Zero I/O — recebe string, devolve Intent
- Resultado determinístico: mesma entrada → mesma saída
