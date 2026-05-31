# FASE N — Nome da Fase

## Contexto

O que existe hoje e por que não é suficiente.
Uma frase por problema. Máximo 5 problemas.

## Solução

Uma frase descrevendo o que esta fase entrega.
Comportamento idêntico ao anterior? Dizer explicitamente.
Novas dependências? Listar aqui.

---

## Regras absolutas

- `CGO_ENABLED=0` sem exceções
- Build via `mage build`
- Zero escrita fora de `.golovebox/`
- [regra específica desta fase se houver]

---

## 1. [Primeira Feature ou Componente]

### O que substitui / O que é

Uma linha descrevendo o que esta seção resolve.

### Por que [escolha técnica]

Justificativa de design. Se houve alternativa descartada, explicar em uma linha.

### Implementação

```go
// código Go relevante
// só o suficiente para deixar a intenção clara
// não precisa ser completo — Claude Code preenche os detalhes
```

```js
// ou JS/HTML se for frontend
```

### Comportamento esperado

- bullet por comportamento observável
- testável pelo entregável final

---

## 2. [Segunda Feature ou Componente]

[mesma estrutura]

---

## N. [Última Feature ou Componente]

[mesma estrutura]

---

## Arquivos Criados

| Arquivo | Função |
|---|---|
| `internal/pacote/arquivo.go` | Uma linha descrevendo o que o arquivo faz |

## Arquivos Modificados

| Arquivo | Mudança |
|---|---|
| `internal/pacote/arquivo.go` | O que muda — uma linha |

---

## Decisões de Design

### [Nome da Decisão]

Por que foi feito assim. Qual alternativa foi descartada e por quê.
Uma decisão por subseção. Só as não-óbvias.

---

## Limitações Conhecidas (Fase N)

1. **Nome da limitação**: descrição. Mitigação futura se houver.
2. [só listar o que realmente importa — não encher de caveats]

---

## Entregável

```bash
mage build && ./golovebox [comando relevante]
```

Lista do que deve funcionar ao final:
- comportamento 1
- comportamento 2
- comportamento N
