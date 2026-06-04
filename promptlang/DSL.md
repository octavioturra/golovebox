# golovebox DSL — Especificação

Esta é a **fonte canônica** da DSL de intenção do golovebox. Outros documentos linkam aqui.

Implementado em: `promptlang/` (módulo `github.com/user/golovebox/promptlang`).

---

## Formato

Arquivos `.md` em linguagem natural. Keywords opcionais em MAIÚSCULAS controlam o comportamento do DAG.

```markdown
# Título da feature (opcional)

Prosa descrevendo o trabalho. O orchestrator gera tasks auto-contidas a partir disso.

NEW BRANCH feature/minha-feature

Mais prosa. Cada parágrafo pode virar um node task no DAG.

ATTENTION_HERE: validar com o time de segurança antes de continuar.

RUN_TEST: go test ./internal/auth/...

PUSH
PR "feat: minha feature"

NOT_TODO: OAuth2 social login — registrar como débito técnico.
```

---

## Keywords

### Workflow Git

| Keyword | Efeito | Node type |
|---|---|---|
| `NEW BRANCH <name>` | Cria/checkout branch; persiste em `run_meta.json` | `branch` |
| `PUSH` | Push com `--set-upstream`; usa `credential.helper=store` | `push` |
| `PR "title"` | Cria PR ou atualiza body se já existe (detecção de duplicata) | `pr` |

**Ordem garantida** (FASE 19): `sync_repo → branch → edits → push → pr`. O `enforceWorkflowOrdering` no orchestrator impõe a cadeia independente de como o LLM gera o DAG.

**sync_repo** é injetado automaticamente quando `workflow.default_repo` está configurado — não declarar no spec.

### Controle de Fluxo

| Keyword | Efeito | Node type |
|---|---|---|
| `ATTENTION_HERE: <msg>` | Pausa DAG, aguarda aprovação humana no web UI (node laranja) | `checkpoint` |
| `PAUSE_TO_REVIEW` | Alias de `ATTENTION_HERE` | `checkpoint` |
| `RUN_TEST: <cmd>` | Gate — DAG só avança se `<cmd>` termina com exit 0 na VM | `gate` |
| `TRY <texto> OR_ELSE <fallback>` | Tenta o bloco; se falhar, executa o fallback | `try_else` |
| `WHEN <evento> DO <ação>` | Aguarda evento externo antes de executar ação | `wait_event` |

### Comunicação

| Keyword | Efeito | Node type |
|---|---|---|
| `NOTIFY_ME: <msg>` | Envia notificação (Telegram/Slack) e continua | `notify` |

### Exclusão

| Keyword | Efeito |
|---|---|
| `NOT_TODO: <texto>` | Grava em `artifacts/tech_debt.md`; exclui do DAG completamente |

---

## Exemplo Completo

```markdown
# Autenticação JWT

NEW BRANCH feature/jwt-auth

Implementar login com email/senha em /api/auth.
Token JWT com expiração em 24h.

ATTENTION_HERE: validar estratégia de expiração com o time de segurança.

RUN_TEST: go test ./internal/auth/...

Adicionar suporte a refresh token.

NOTIFY_ME: avisar quando os testes passarem.

PUSH
PR "feat: autenticação JWT"

NOT_TODO: OAuth2 social login — fora do escopo deste sprint.
```

DAG gerado (com `workflow.default_repo` configurado):

```
sync_repo → branch(feature/jwt-auth) → impl-login → ATTENTION_HERE
                                     ↗              ↓
                                                   RUN_TEST → impl-refresh → push → pr
```

---

## Notas de Implementação

- **Tokenização**: `ParseContent(src string) (*ParsedSpec, error)` — lê linha a linha
- **Semântica**: `ToIntent(spec *ParsedSpec) core.Intent` — converte para `core.Intent` com `[]core.Step`
- **Cada parágrafo de prosa** entre keywords vira um `Step{Kind: KindEdit}` → node `task` no DAG
- **IDs de nodes** gerados como `snake_case` (FASE 9) pelo orchestrator
- **Tasks auto-contidas**: cada node recebe objetivo completo do run + detalhe específico — o ReAct loop não assume contexto externo (FASE 17)

Para a implementação: `promptlang/parser.go`, `promptlang/adapter.go`, `promptlang/keywords.go`.
