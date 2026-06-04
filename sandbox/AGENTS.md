# sandbox — AGENTS.md

## Responsabilidade única
Execução isolada de comandos em VM QEMU via SSH. Gerencia o ciclo de vida da VM (start/attach) e o pool de conexões SSH. Não conhece nada de agentes, skills ou orquestração.

## Contratos que cumpre
- `core.Sandbox` (`Exec / PutFile / GetFile / Ready`) — contrato do agente loop
- `sandbox.Interactive` (`OpenPTY / OpenSFTP`) — interface adicional para web terminal (não está em `core` — consumidor único)

## Dependências permitidas
- `core`
- `github.com/digitalocean/go-qemu` — controle QMP
- `github.com/pkg/sftp` — SFTP
- `golang.org/x/crypto/ssh` — SSH pool
- NUNCA importar: `toolskills`, `derivator`, `orchestrator`, `app/internal/*`

## Recebe por injeção
- `sandbox.Config` — fornecido pelo composition root (`app`). Contém paths, ports, credenciais SSH. Nunca usa `*config.Config` do app.

## Verificação local
```bash
cd sandbox && GOWORK=off go vet ./...
```
(sem testes de unidade — requer QEMU vivo; testes de integração futuros)

## Arquivos
| Arquivo | Função |
|---|---|
| `vm.go` | `VM` struct, `NewVM` (start QEMU), `Attach` (pool only), implementa `core.Sandbox` |
| `pool.go` | Pool de conexões SSH com keepalive + retry dial |
| `qemu.go` | Spawn do processo QEMU, validação magic qcow2 |
| `qmp.go` | Controle QMP (monitor QEMU) |
| `ssh.go` | `DirectDial` helper para CLI exec/status |
| `interactive.go` | `Interactive` interface, `PTY` e `SFTP` structs — web terminal only |

## Regras absolutas
- CGO_ENABLED=0 (sem libvirt, sem bindings C)
- Zero paths hardcoded — todos via `sandbox.Config`
- `*VM` é o único tipo concreto que sai deste módulo; o app o usa via `core.Sandbox`
