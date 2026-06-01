# FASE 23 — Extração: sandbox

**Status:** concluída  
**Data:** 2026-06-01

## Objetivo

Extrair `internal/sandbox/` para um módulo Go standalone `sandbox/`, tornando-o o 4° módulo no `go.work`. Ao mesmo tempo, introduzir `core.Sandbox` como a interface que abstrai o acesso ao VM para toda a cadeia de consumidores (`tools`, `gateway`, `web`, `main`).

## O que foi feito

### Novos arquivos em `sandbox/`

| Arquivo | Conteúdo |
|---|---|
| `go.mod` | `module github.com/user/golovebox/sandbox`, deps: core, go-qemu, sftp, crypto |
| `qemu.go` | Movido de `internal/sandbox/qemu.go`. `config.Config` → `sandbox.Config{QEMUExe,QEMUDir,VMDir,SSHPort,QMPPort,SSHUser}`. Removida dependência de `internal/config` e `internal/embed`. |
| `qmp.go` | Movido intacto |
| `pool.go` | Movido intacto |
| `ssh.go` | Movido. `exec()` renomeado para `runCmd()` (evita colisão com `os/exec` importado em `qemu.go`). Funções de acesso rebaixadas para unexported. |
| `vm.go` | **Novo.** `VM` struct que implementa `core.Sandbox` + expõe `DirectDial`. `NewVM(cfg)` inicia QEMU e cria pool. `Attach(cfg)` cria só pool (VM já rodando). |
| `interactive.go` | **Novo.** `Interactive` interface + `PTY`/`SFTP` wrappers. `VM.OpenPTY` e `VM.OpenSFTP` — mantém ssh.Session/sftp.Client fora do boundary do módulo web. |

### `core/contracts.go`

Adicionados `Output{Stdout, Stderr, ExitCode}` e `Sandbox` interface:
```go
type Sandbox interface {
    Exec(ctx context.Context, cmd string) (Output, error)
    PutFile(ctx context.Context, path string, data []byte) error
    GetFile(ctx context.Context, path string) ([]byte, error)
    Ready(ctx context.Context) bool
}
```

### Consumidores migrados

- **`internal/tools/`**: todos os parâmetros `*ssh.Client` → `core.Sandbox`. `ctx context.Context` como primeiro argumento.
- **`internal/gateway/gateway.go`**: campo `pool *sandbox.Pool` → `sb core.Sandbox`. Removidos `AcquireSSH`/`ReleaseSSH`. `buildRegistry()` usa `g.sb` diretamente.
- **`internal/web/server.go`**: novo campo `sb core.Sandbox` + `interactive *sandboxpkg.VM`. `New()` recebe `*sandboxpkg.VM` e injeta em ambos os campos. Dispatch do workflow usa `s.sb` diretamente.
- **`internal/web/terminal.go`**: substituiu `AcquireSSH` + criação manual de `ssh.Session` por `s.interactive.OpenPTY()` e `s.interactive.OpenSFTP()`.
- **`internal/setup/init.go`**: substituiu `sandbox.Dial` + `sandbox.Exec` por `sandboxpkg.DirectDial`.
- **`cmd/golovebox/main.go`**: composition root. Constrói `sandboxpkg.Config` a partir de `config.Config`, cria `sandboxpkg.NewVM(sbCfg)` e injeta como `core.Sandbox` no gateway e como `*sandboxpkg.VM` no servidor web.

### Limpeza

- `internal/sandbox/` deletado.
- `go.work` atualizado para 4 módulos: `. ./core ./promptlang ./sandbox`.
- `go.mod` raiz: `require sandbox v0.0.0-00010101000000-000000000000` + `replace sandbox => ./sandbox`.

## Decisões técnicas

**`sandbox.Interactive` fora de `core`**: PTY e SFTP são detalhes de substrato (só o browser precisa deles). `core.Sandbox` define o contrato de agente; `Interactive` é uma segunda interface implementada por `*VM`, consumida apenas pelo layer web.

**`GetFile` em `core.Sandbox`**: mesmo não estando no spec inicial, `GetFile` é necessário para `gateway.RunTask` indexar o README e para a tool `read_file` do agente. Adicionado sem hesitação.

**Composition root único**: `cmd/golovebox/main.go` é o único lugar que conhece o tipo concreto `*sandboxpkg.VM`. Todo o resto usa `core.Sandbox` ou `*sandboxpkg.VM` via interface — o que garante que futuras trocas (mock, WSL, Docker) sejam um change de um arquivo só.

**`sandbox.Config` desacoplada de `config.Config`**: a conversão acontece no composition root (`makeSandboxConfig`). O módulo sandbox não depende de `internal/config`.

## Invariantes preservados

- `CGO_ENABLED=0` — o módulo sandbox não usa CGO.
- `StartTimeout` exportado para `internal/setup/init.go` poder sobreescrevê-lo no smoke test.
- `DirectDial` exportado para os comandos `exec` e `status` da CLI (não criam VM completa).
