# CLAUDE.md — golovebox

## Claude Code

Ao final do desenvolvimento de um prompt de fase, criar um arquivo ./.ia/FASE_{n}.md

## Projeto
Agente autônomo de código e comunicação em Go.
Portable app Windows-first, binário único, zero instalação.

## Regras Absolutas de Código

- `CGO_ENABLED=0` — sem exceções
- Zero dependências que exijam CGO
- Todos os paths via `filepath.Join` — nunca hardcoded `/` ou `~`
- `baseDir` sempre relativo ao executável: `filepath.Join(filepath.Dir(execPath), ".golovebox")`
- Build: `GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build`

## Stack

| Camada | Lib |
|---|---|
| CLI | `spf13/cobra` |
| Config | `BurntSushi/toml` |
| Gateway | `sausheong/goclaw` |
| SSH + SFTP | `golang.org/x/crypto/ssh` |
| QEMU controle | `digitalocean/go-qemu` |
| Memória vetorial | `philippgille/chromem-go` |
| GitHub | `google/go-github` |
| Browser | `go-rod/rod` |

## Estrutura de Pastas

```
golovebox/
├── cmd/golovebox/main.go
├── internal/
│   ├── agent/       # ReAct loop, planner, tool registry
│   ├── tools/       # shell, files, github, browser, email, slack
│   ├── gateway/     # GoClaw wrapper
│   ├── memory/      # chromem-go
│   ├── sandbox/     # qemu.go, qmp.go, ssh.go
│   ├── setup/       # init.go — primeiro boot
│   └── config/      # config.go — paths portáveis
└── go.mod
```

## Dados em Runtime

Tudo em `.golovebox/` ao lado do executável:
```
.golovebox/
├── config.toml
├── qemu/          # QEMU instalado aqui
├── vm/            # Alpine base.img
├── memory/        # chromem-go embeddings
└── logs/
```

## Primeiro Boot

`golovebox init` (auto se `.golovebox/` não existir):
1. Cria `.golovebox/`
2. Baixa + instala QEMU em `.golovebox/qemu/`
3. Baixa + extrai Alpine VM image em `.golovebox/vm/`
4. Config interativo (LLM provider, API keys, GitHub token, Telegram token)
5. Smoke test: sobe VM, `echo ok` via SSH

## Agent Loop (ReAct)

```
prompt → LLM → parse action → execute tool → observe → repeat
```

Tools disponíveis: `shell`, `read_file`, `write_file`, `search_memory`,
`github_list_issues`, `github_get_issue`, `github_clone_repo`, `github_open_pr`

## Comunicação Host↔VM

- Exec: SSH `localhost:2222`
- Arquivos: SFTP (mesmo canal SSH)
- Controle VM: QMP TCP `localhost:4444` via `digitalocean/go-qemu`

## Fora do Escopo V0

- Multi-agent paralelo
- VS Code Server
- Interface web
- Linux e macOS testados
- Auto-update
- Windows Defender signing