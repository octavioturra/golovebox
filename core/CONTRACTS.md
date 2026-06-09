# core/CONTRACTS.md — Mapa de Contratos

Lista cada interface/tipo de `core/contracts.go`, quem implementa e quem consome.
Este arquivo é a fonte de verdade das fronteiras — mantenha sincronizado com `contracts.go`.

---

## Interfaces

### `core.Sandbox`
**O quê:** Execução isolada de comandos (`Exec / PutFile / GetFile / Ready`).

| Papel | Módulo |
|---|---|
| Implementa | `sandbox` (`*sandbox.VM`) |
| Consome | `toolskills` (provisioner), `app/web` (terminal/SFTP) |
| Injeta | `app` (composition root, via `*sandbox.VM`) |

---

### `core.Provisioner`
**O quê:** Executa comandos de setup na VM para uma skill (`Provision(ctx, SkillMeta) error`).

| Papel | Módulo |
|---|---|
| Implementa | `toolskills` |
| Consome | `app` (setup wizard, skill provisioning) |
| Injeta | `app` |

---

### `core.Completer`
**O quê:** Interface mínima de LLM (`Complete(ctx, prompt string) (string, error)`).

| Papel | Módulo |
|---|---|
| Implementa | `app/internal/llm` (`llm.NewCompleter(*Client) core.Completer`) |
| Consome | `toolskills` (generator) |
| Injeta | `app` |

---

## Tipos de domínio chave

| Tipo | Usado por |
|---|---|
| `core.SkillMeta` | `toolskills` (registry), `core.Provisioner` |
| `core.Output` | `core.Sandbox` (resultado de Exec) |

---

## Protocolo de mudança

Ver `core/AGENTS.md` §Regra de evolução.
Resumo: PR isolado → aditivo > breaking → ≥2 consumidores reais antes de promover → módulos afetados adaptam em paralelo.
