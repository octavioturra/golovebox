# core — AGENTS.md

## Responsabilidade única
Define contratos e tipos de domínio compartilhados. Zero dependências de projeto. Ninguém implementa aqui — só declara.

## O que este módulo contém
- `contracts.go` — todas as interfaces, tipos e constantes do domínio

## Dependências permitidas
- Apenas stdlib (`context`)
- NUNCA importar nenhum outro módulo do monorepo

## Regra de evolução
`core` é o único ponto de coordenação do swarm. Mudanças seguem o **protocolo de contrato**:
1. PR isolado só em `core` (sem impl junto)
2. Aditivo > breaking (adicionar nunca quebra; renomear/remover quebra)
3. Só promover interface quando ≥2 consumidores reais existirem (YAGNI)
4. Após merge, cada módulo afetado adapta no seu próprio PR, em paralelo

## Verificação local
```bash
cd core && GOWORK=off go vet ./...
```
(sem testes — contracts.go é só tipos)

## Regras absolutas
- CGO_ENABLED=0
- Zero paths hardcoded
- Zero deps de projeto

## Ver também
`core/CONTRACTS.md` — lista cada interface, quem implementa, quem consome.
