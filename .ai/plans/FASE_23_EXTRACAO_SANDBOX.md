# FASE 23 — Extração: sandbox

**Tipo:** Refactor arquitetural (terceira execução da FASE 21)
**Depende de:** FASE 22 (core + promptlang extraídos, go.work funcional)
**Escopo:** Módulo 4 da ordem de extração — `sandbox`. Nada mais.
**Estratégia:** Strangler fig. Extrai, compila isolado, testa, só então segue.

---

## Objetivo

Mover `internal/sandbox/` para módulo próprio `sandbox/`, atrás de `core.Sandbox`. O legado deixa de conhecer QEMU, SSH, QMP — fala só com a interface.

```
ANTES                          DEPOIS
golovebox/                     golovebox/
├── go.work (3)                ├── go.work (4)
├── core/                      ├── core/         ← ganha contrato Sandbox
├── promptlang/                ├── promptlang/
└── go.mod (legado)            ├── sandbox/go.mod ← extraído de internal/sandbox
   └── internal/sandbox/       └── go.mod (legado) ← consome core.Sandbox
```

## Regras absolutas
- `CGO_ENABLED=0`, build via `mage build`
- `sandbox` importa só `core` (+ deps externas: x/crypto/ssh, pkg/sftp)
- Interface **não vaza** `*ssh.Client`, `*ssh.Session`, QMP nem QEMU process
- core continua zero-dep de projeto

---

## 1. Contrato no core

`core/contracts.go` — adicionar. `core.Sandbox` cobre o que orchestrator/agent usam:

```go
package core

import "context"

type Output struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

// Sandbox — substrate de execução isolada.
// Exec/PutFile/Ready bastam para o agent loop e workflow nodes.
type Sandbox interface {
	Exec(ctx context.Context, cmd string) (Output, error)
	PutFile(ctx context.Context, path string, data []byte) error
	Ready(ctx context.Context) bool
}
```

`context` é stdlib — permitido. Nenhum import de projeto.

---

## 2. Módulo `sandbox`

`sandbox/go.mod`:
```
module github.com/<owner>/golovebox/sandbox

go 1.22

require (
	github.com/<owner>/golovebox/core v0.0.0
	golang.org/x/crypto v...
	github.com/pkg/sftp v...
)
```

Mover de `internal/sandbox/`: `qemu.go`, `qmp.go`, `ssh.go`, `pool.go`.

`sandbox/sandbox.go` — fachada que implementa `core.Sandbox`:
- `New(cfg Config) *VM` (Config: paths do qcow2/iso/id_rsa, portas SSH 2222 / QMP 4444 — tipos próprios do módulo, não core).
- `Exec` → usa Pool.Acquire (dial retry 4×) e roda comando.
- `PutFile` → SFTP write.
- `Ready` → IsVMReady probe.
- Internos preservados: qcow2 magic validation, boot device explícito, QEMU silencioso (stdout→qemu.log, stdin→DevNull), pool keepalive + dial retry.

`go.work` → adicionar `./sandbox`.

---

## 3. Terminal + SFTP explorer (vazamento controlado)

Problema: `core.Sandbox` (Exec/PutFile/Ready) **não cobre** o terminal PTY interativo do WebSocket nem o file explorer SFTP read-only. Forçar isso na interface poluiria o contrato.

Decisão: o módulo `sandbox` expõe uma **segunda interface rica**, consumida só pelo `app` (web/terminal.go). Ainda esconde `*ssh.Client`.

```go
// sandbox/interactive.go — NÃO vai para core. Vive no módulo sandbox.
type Interactive interface {
	OpenPTY(ctx context.Context, cols, rows int) (PTY, error) // xterm WebSocket
	OpenSFTP(ctx context.Context) (SFTP, error)               // file explorer
}
// PTY e SFTP: tipos do módulo sandbox que encapsulam ssh.Session / sftp.Client.
// Resize via PTY.WindowChange(cols, rows) — sem expor ssh.Session.
```

`*VM` implementa `core.Sandbox` **e** `sandbox.Interactive`. orchestrator/agent recebem `core.Sandbox`. O terminal web recebe `sandbox.Interactive`. Regra FASE 21 mantida: nenhum `*ssh.Client` cruza a fronteira.

---

## 4. Adaptar o legado

Consumidores de `internal/sandbox`:
- `internal/agent/tools.go` (shell, files) → recebe `core.Sandbox` por injeção.
- `internal/gateway/gateway.go` (AcquireSSH/IsVMReady) → `core.Sandbox.Ready` + `Exec`.
- `internal/web/terminal.go` → `sandbox.Interactive`.
- `cmd/golovebox/main.go` → constrói `sandbox.New(...)` e injeta. **Composition root é o único que conhece o concreto.**

Apagar `internal/sandbox/` após migração.

---

## 5. Build

`magefile.go`:
- Confirmar que `mage build` cobre o módulo `sandbox` no workspace.
- `embed.FS` do QEMU/Alpine qcow2: decidir onde mora. Os assets são do substrate → embed vai para o módulo `sandbox` (`sandbox/embed_*.go` + placeholder.txt nos dirs vazios). `mage fetch` baixa para lá.
- `CGO_ENABLED=0` preservado.

---

## Arquivos

| Ação | Arquivo |
|---|---|
| Editar | `core/contracts.go` (+ Sandbox, Output) |
| Criar | `sandbox/go.mod`, `sandbox/sandbox.go`, `sandbox/interactive.go` |
| Mover | `internal/sandbox/{qemu,qmp,ssh,pool}.go` → `sandbox/` |
| Mover | embed do QEMU/Alpine → `sandbox/embed_*.go` |
| Editar | `internal/agent/tools.go`, `internal/gateway/gateway.go`, `internal/web/terminal.go` |
| Editar | `cmd/golovebox/main.go` (wiring) |
| Editar | `go.work` (+ ./sandbox), `magefile.go` |
| Apagar | `internal/sandbox/` |

---

## Verificação

```bash
cd sandbox && go build ./... && go vet ./...   # importa só core + ssh/sftp
cd .. && mage fetch && mage build              # embed resolve no novo módulo
build/golovebox.exe init                       # boot QEMU + smoke SSH
build/golovebox.exe web                        # terminal PTY + file explorer
```

1. `sandbox` compila isolado; grep nos imports não acha `internal/*`.
2. Nenhum `*ssh.Client` aparece em assinatura de função fora do módulo sandbox.
3. `init` faz boot silencioso (qemu.log) + smoke test SSH ok.
4. Terminal WebSocket conecta (PTY); file explorer SFTP lista arquivos.
5. Run com agent loop: shell/files executam via `core.Sandbox` injetado.
6. Dial retry sobrevive ao `rc-service sshd restart` do cloud-init.

---

## Definição de Pronto

> `go.work` com 4 módulos. `sandbox` implementa `core.Sandbox` (+ `sandbox.Interactive` para PTY/SFTP). Trocar QEMU por outra impl (ex: Docker) tocaria só `sandbox/` + o wiring em `main.go`, zero em agent/gateway/web. embed do QEMU mora no módulo dono do substrate.

---

## Anti-padrões (desta fase)

- **`Interactive` no core.** PTY/SFTP são detalhe do substrate, não contrato de domínio. Fica no módulo sandbox.
- **Vazar `ssh.Session`.** Encapsular em tipos `sandbox.PTY`/`sandbox.SFTP`.
- **embed no app.** Asset do QEMU é do substrate. embed mora em `sandbox/`.
- **Extrair junto.** toolskills depende de sandbox mas é FASE 24.

---

## Próxima Fase (FASE 24)

Extrair `toolskills` (provisionamento) — primeiro módulo que **consome** `core.Sandbox`, provando injeção entre módulos extraídos.
