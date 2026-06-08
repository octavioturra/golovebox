# FASE 29 — Estratégia de Testes

**Status:** concluída  
**Data:** 2026-06-04

## Objetivo

Confiança mínima de que cada módulo cumpre seu contrato. Sem virar super-suíte. Sair do "qualidade = teste manual final".

## Princípio

Testar o **contrato** (o mínimo esperado), não cobertura. Mock nas fronteiras `core.*`. Recurso real (QEMU) e IA ficam fora do CI bloqueante.

## O que foi feito

### `core/coretest/` — test doubles compartilhados

| Arquivo | Conteúdo |
|---|---|
| `sandbox.go` | `FakeSandbox` — `core.Sandbox` configurável. Scripts de resposta por comando, filesystem in-memory, `Calls []string` para asserções de ordem. |
| `completer.go` | `FakeCompleter` — `core.Completer` com replies scriptadas em sequência, `Prompts []string` para asserção do transcript. |
| `sandbox_contract.go` | `SandboxContract(t, factory)` — suíte comportamental que mock E real devem passar (Ready=true, Exec exitCode 0, PutFile/GetFile roundtrip, GetFile missing → error). |
| `fixtures/derivator_plan.json` | JSON fixture de resposta LLM para derivator |
| `fixtures/dag_plan.json` | JSON fixture de resposta LLM para plano de DAG |

### Asserções de compile-time (por módulo)

`var _ core.X = (*impl)(nil)` em cada módulo — garante que a interface é cumprida sem rodar testes.

| Arquivo | Asserta |
|---|---|
| `sandbox/contract_test.go` | `*VM` satisfaz `core.Sandbox` |
| `promptlang/contract_test.go` | `*impl` satisfaz `core.Parser` |
| `toolskills/contract_test.go` | `*provisioner` satisfaz `core.Provisioner` |
| `derivator/contract_test.go` | `*deriver` satisfaz `core.Deriver` |
| `app/internal/llm/contract_test.go` | `*completer` satisfaz `core.Completer` |

### Novos testes unitários

**`sandbox/validation_test.go`:**
- `TestStart_rejectsCorruptImage` — qcow2 magic check rejeita arquivo corrompido (sem QEMU)
- `TestStart_rejectsMissingImage` — erro claro quando base.img ausente
- `TestConfig_pathsRetainDir` — Config não mutaciona fields

**`sandbox/pool_test.go`:**
- Pool armazena config corretamente
- `Close` em pool vazio é no-op
- Close concorrente é thread-safe

**`sandbox/coretest_contract_test.go`:**
- `TestSandboxContract_FakeSandbox` — executa `SandboxContract` contra `FakeSandbox`, provando que o mock é honesto

**`orchestrator/dag/checkpoint_test.go`:**
- Approve-before-Wait entrega imediatamente
- Wait-then-Approve desbloqueia
- Reject carrega reason
- Double-approve não panic

**`orchestrator/ordering_test.go`:**
- `enforceWorkflowOrdering` garante que edits dependem de branch
- Push depende de todos os task nodes
- No-op quando sem branch/push/pr (FASE 20 dedup preservado)

**`orchestrator/tools/github_test.go`:**
- `ExecPRWithBase` cria PR quando nenhum existe (httptest.Server, sem rede real)
- Detecta PR open existente e atualiza body

**`app/internal/web/server_test.go` + `handlers_export_test.go`:**
- `HandleListRuns` retorna array vazio (não null) quando store vazio
- `HandleGetRun` retorna 404 para run inexistente
- `HandleGetRun` retorna 200 com task quando dag.json ausente (degrada gracefully)

### `ExecPRWithBase` em `orchestrator/tools/git.go`

Extrai a lógica de `ExecPR` para variante testável que aceita `apiBaseURL` para apontar para `httptest.Server`. `ExecPR` delega para ela com string vazia (produção).

### CI `.github/workflows/ci.yml` — dois trilhos

**Trilho fast (bloqueia merge):**
- `module` (matriz 7×): `GOWORK=off go test -short ./...` por módulo, `CGO_ENABLED=0`
- `boundary`: invariante de dependência (grep de imports proibidos)
- `integration-build`: `go build ./...` via workspace

**Trilho heavy (`continue-on-error: true`, não bloqueia):**
- `integration-sandbox`: QEMU real em Linux+KVM, `//go:build integration`, `TestMain` boota 1× (estrutura pronta, testes a adicionar em FASE futura)
- `integration-e2e`: e2e nightly com `//go:build e2e`
- Ativado por: `schedule` (nightly), label `test:heavy`, ou push em `sandbox/`

## Arquivos

| Ação | Arquivo |
|---|---|
| Criar | `core/coretest/{sandbox,completer,sandbox_contract}.go`, `fixtures/*.json` |
| Criar | `sandbox/{contract,validation,pool,coretest_contract}_test.go` |
| Criar | `promptlang/contract_test.go` |
| Criar | `toolskills/contract_test.go` |
| Criar | `derivator/contract_test.go` |
| Criar | `orchestrator/dag/checkpoint_test.go`, `orchestrator/ordering_test.go` |
| Criar | `orchestrator/tools/github_test.go` |
| Criar | `app/internal/llm/contract_test.go`, `app/internal/web/server_test.go`, `app/internal/web/handlers_export_test.go` |
| Editar | `orchestrator/tools/git.go` — adiciona `ExecPRWithBase` |
| Editar | `.github/workflows/ci.yml` — dois trilhos (fast/heavy) |

## Verificação local

```bash
# fast — todo módulo, sem recurso pesado
for m in core promptlang sandbox toolskills derivator orchestrator app; do
  (cd $m && GOWORK=off CGO_ENABLED=0 go test -short ./...)
done

# boundary
! grep -rq 'golovebox/app' core/ promptlang/ sandbox/ toolskills/ derivator/ orchestrator/

# heavy — local (requer .golovebox/ inicializado)
cd sandbox && go test -tags integration ./...
```

Todos passaram.

## Definição de Pronto

Cada módulo tem teste do objetivo mínimo. `SandboxContract` única para mock e real. `go test -short` verde em segundos por módulo. CI fast bloqueia; heavy alerta. Compile-time assertions: quebrar interface → não compila.
