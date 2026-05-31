# FASE 14 — Bugfix: Fluxo BRANCH/PR

## Status: ✅ Entregue

## Root Causes Corrigidos

| # | Arquivo | Problema | Solução |
|---|---|---|---|
| 1 | `internal/embed/cloudinit.go` | `git commit` falha status 128 — VM sem `user.email`/`user.name` | `git config --global` no `runcmd` |
| 2 | `internal/orchestrator/orchestrator.go` | Prompt de planejamento não incluía DefaultRepo — LLM cego ao repositório | `repoCtx` injetado no prompt |
| 3 | `internal/orchestrator/orchestrator.go` | Regras git ausentes — LLM não sabia incluir clone/commit/push/PR | Regras git adicionadas ao prompt |

> Bug 3 (callers de `orchestrator.New` sem `cfg`) já estava resolvido na Fase 13.

---

## Bug 1 — Git sem identidade na VM

**Arquivo:** `internal/embed/cloudinit.go`

Adicionado ao bloco `runcmd` do `buildUserData`:

```yaml
  - git config --global user.email "agent@golovebox.local"
  - git config --global user.name "golovebox-agent"
```

**Nota:** `cidata.iso` é regenerada apenas no próximo `golovebox init`. Usuários com VM existente precisam rodar `golovebox reset && golovebox init` para receber a identidade git.

---

## Bug 2 — Orchestrator cego ao repositório

**Arquivo:** `internal/orchestrator/orchestrator.go`

### `Plan()` — constrói `repoCtx`

```go
var repoCtx string
if o.cfg != nil && o.cfg.DefaultRepo != "" {
    repoCtx = fmt.Sprintf(
        "- DefaultRepo: %s\n- GitHubToken disponível: %v\n- Repositório pode já estar em /root/%s na VM\n",
        o.cfg.DefaultRepo,
        o.cfg.GitHubToken != "",
        repoPathFromRepo(o.cfg.DefaultRepo),
    )
} else if o.cfg != nil && o.cfg.Workflow.DefaultRepo != "" {
    repoCtx = fmt.Sprintf(
        "- DefaultRepo: %s\n- GitHubToken disponível: %v\n- Repositório pode já estar em %s na VM\n",
        o.cfg.Workflow.DefaultRepo,
        o.cfg.GitHubToken != "",
        o.cfg.Workflow.ClonePath,
    )
}
```

Cobre tanto `cfg.DefaultRepo` (Telegram legacy) quanto `cfg.Workflow.DefaultRepo` (Fase 13).

### `buildPlanningPrompt(specs, cfg, repoCtx)` — contexto injetado

```go
if repoCtx != "" {
    sb.WriteString("## Execution Context\n\n")
    sb.WriteString(repoCtx)
    sb.WriteString("\n\n")
}
```

O `GitHubToken` não é exposto — apenas a flag booleana de disponibilidade; o token já está acessível ao agente via GIT_ASKPASS configurado internamente.

### Helper `repoPathFromRepo`

```go
func repoPathFromRepo(ownerRepo string) string {
    parts := strings.SplitN(ownerRepo, "/", 2)
    if len(parts) == 2 { return parts[1] }
    return ownerRepo
}
```

---

## Bug 3 — Regras git ausentes no prompt

**Arquivo:** `internal/orchestrator/orchestrator.go`

Três regras adicionadas ao `buildPlanningPrompt`:

```
- Tasks involving git MUST include: clone the repo (if not already present),
  configure remote with token via GIT_ASKPASS, create branch, commit changes,
  push and open PR
- Use DefaultRepo from execution context when available
- The GitHub token is available via GIT_ASKPASS — the agent shell tool already
  handles authentication
```

---

## Arquivos Modificados

| Arquivo | Mudança |
|---|---|
| `internal/embed/cloudinit.go` | `runcmd` adiciona `git config --global user.email/user.name` |
| `internal/orchestrator/orchestrator.go` | `buildPlanningPrompt` recebe `repoCtx`; `Plan` constrói e injeta contexto; regras git no prompt; `repoPathFromRepo` helper |

## Arquivos Criados

Nenhum.
