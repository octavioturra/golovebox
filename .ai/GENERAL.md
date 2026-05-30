# golovebox — Visão Geral

**Tagline**: Devin + n8n + Temporal, em um único `.exe`. Self-hosted, intention-DSL agentic workflow platform.

**Status**: Phase 7 completa. Binário ~450MB embute QEMU + Alpine cloud qcow2 + web UI.

---

## O que é

Um agente autônomo de codificação distribuído como **um único `.exe`**. Você copia para qualquer pasta, roda `golovebox init`, e tem:

- VM Alpine isolada para execução segura de código (QEMU + cloud-init no first boot)
- ReAct agent loop com ferramentas (`shell`, `read_file`, `write_file`, `github_*`, `search_memory`)
- Orquestrador que converte specs Markdown em linguagem natural em DAG executável
- Web UI embutida (HTML+JS via `embed.FS`, sem Electron, sem build step)
- Memória vetorial persistente (chromem-go, pure Go, zero CGO)
- Skills plugáveis (templates `.md` com TOML frontmatter)
- Gateway Telegram (opcional, secundário)

Zero Python, zero Docker, zero admin, zero conta em nuvem.

---

## Como funciona

```
spec.md ──▶ orchestrator (LLM) ──▶ DAG ──▶ executor (paralelo, máx 3)
                                              │
                                              ├─ task         → ReAct loop dentro da VM
                                              ├─ checkpoint   → orange node no canvas, aguarda humano
                                              ├─ gate         → roda testes na VM
                                              └─ notify       → Telegram/email
                                              │
                                              ▼
                                      run_summary.md + PR URLs + artifacts
```

Toda execução (`shell`, `read_file`, `write_file`) acontece **dentro da VM** via SSH/SFTP. O host nunca é tocado.

---

## Comandos

| Comando | Função |
|---|---|
| `golovebox init` | Setup (config + extract QEMU/Alpine + boot + cloud-init). Reusa `config.toml` se já existir. |
| `golovebox init --repair` | Reusa config sem perguntar; refaz apenas o que falta. |
| `golovebox reset` | Apaga `base.img` + `cidata.iso` (mantém config + keys). Pede confirmação. |
| `golovebox reset --hard` | Apaga `.golovebox/` inteiro. Exige digitar `yes`. |
| `golovebox web` | Sobe VM + web UI em `localhost:8080`. `--timeout N` ajusta start timeout. |
| `golovebox run <spec>` | Executa specs via DAG, com progresso no terminal. |
| `golovebox resume <run-id>` | Retoma run interrompido. |
| `golovebox status [run-id]` | Status da VM/config, ou DAG de um run específico. |
| `golovebox exec "<cmd>"` | Shell direto na VM via SSH. |
| `golovebox github --repo o/r --issue N` | Resolve uma issue (one-shot). |
| `golovebox daemon` | Telegram bot gateway. |
| `golovebox skill list / generate "<desc>"` | Gerencia skills. |

---

## Estrutura em runtime

Tudo fica em `.golovebox/` ao lado do `.exe`:

```
.golovebox/
├── config.toml
├── qemu/             ← extraído do binário (flat layout: exe + DLLs + share/qemu/)
├── vm/
│   ├── base.img             ← Alpine cloud qcow2 (~164MB)
│   ├── .alpine-image-size   ← marker de idempotência
│   ├── cidata.iso           ← cloud-init NoCloud seed
│   ├── id_rsa, id_rsa.pub   ← SSH keypair
│   └── qemu.log             ← console serial da VM
├── memory/           ← chromem-go embeddings
├── skills/           ← .md skills
└── runs/<run-id>/    ← dag.json, node_states.json, specs/, logs/, artifacts/, run_summary.md
```

---

## DSL de Intenção

Prosa Markdown legível com keywords opcionais em MAIÚSCULAS:

```markdown
# Feature: autenticação JWT

Implementar login com email/senha em /api/auth.

ATTENTION_HERE: validar token expiry com o time de segurança
RUN_TEST: go test ./internal/auth/...

Adicionar refresh token.

NOTIFY_ME: avisar quando merge entrar em main

NOT_TODO: OAuth2 social login (fora do escopo)
```

| Keyword | Efeito |
|---|---|
| `ATTENTION_HERE` / `PAUSE_TO_REVIEW` | pausa o DAG, aguarda aprovação no web UI |
| `RUN_TEST` | gate — DAG só avança se passar |
| `NOTIFY_ME` | notifica e continua |
| `NOT_TODO` | grava em tech_debt.md, exclui do DAG |
| `TRY ... OR_ELSE ...` | try/fallback |
| `WHEN ... DO ...` | espera evento externo |

---

## Stack

| Camada | Lib |
|---|---|
| CLI | `spf13/cobra` |
| Config | `BurntSushi/toml` |
| LLM | `internal/llm` (HTTP puro, OpenAI-compat + Anthropic, retry exponencial) |
| Telegram | `go-telegram-bot-api/v5` |
| SSH/SFTP | `golang.org/x/crypto/ssh` + `pkg/sftp` |
| QEMU control | `internal/sandbox/qmp.go` (QMP TCP direto) |
| Memória vetorial | `philippgille/chromem-go` |
| GitHub | `google/go-github/v60` |
| CIDATA ISO | `github.com/kdomanski/iso9660` |
| Build | `github.com/magefile/mage` |

**Regra absoluta**: `CGO_ENABLED=0`. Sem exceção.

---

## VM Sandbox

- **Imagem**: Alpine NoCloud cloud qcow2 (3.21.7, BIOS, cloud-init habilitado)
- **Por que não Alpine Virt ISO**: Virt ISO é instalador interativo — para em `localhost login:` esperando `setup-alpine`. Cloud-init não está no boot path. Phase 5/6 tentaram e nunca funcionaram.
- **Boot**: QEMU sobe `base.img` (virtio-blk, bootindex=0) + `cidata.iso` (segundo virtio-blk, sem bootindex). SeaBIOS boota o disco principal; cloud-init dentro da Alpine detecta CIDATA e aplica user-data (~1m42s no first boot em Windows TCG).
- **SSH**: porta 2222 do host forwardada para 22 da VM. Key RSA 4096 gerada localmente, publicada na VM via cloud-init.
- **Cloud-init aplica**: SSH key em `/root/.ssh/authorized_keys` + drop-in `/etc/ssh/sshd_config.d/99-golovebox.conf` (`PermitRootLogin prohibit-password`) + pacotes (git, curl, bash, openssh, python3, make).
- **QEMU silencioso**: stdout/stderr vão para `vm/qemu.log`, stdin vai para `os.DevNull`. Terminal do host fica intocado.
- **Validação**: `sandbox.Start()` lê o magic do qcow2 antes de subir QEMU — erro claro se `base.img` está corrompido (em vez do críptico "could not read boot disk" do SeaBIOS).

---

## Build

```bash
go install github.com/magefile/mage@latest

mage fetch          # baixa QEMU + Alpine cloud qcow2 (~244MB total)
mage check          # valida assets
mage buildWindows   # → build/golovebox.exe (~450MB)
mage build          # OS atual
```

Trade-off de tamanho: 450MB é grande, mas elimina QUALQUER passo de instalação no destino (incluindo os ~10 min de install do Alpine no first boot, que existia antes).

---

## Phase roadmap

| Phase | Status | Resumo |
|---|---|---|
| 1 | ✅ | Foundation — CLI, QEMU sandbox, SSH/SFTP, init wizard |
| 2 | ✅ | Agent — ReAct loop, LLM, GitHub tools, vector memory |
| 3 | ✅ | Gateway — Telegram, SSH pool, GIT_ASKPASS, LLM retry |
| 4 | ✅ | Platform — DAG orchestrator, web chat, skills, parallel exec |
| 5 | ✅ | Self-contained — embedded QEMU + Alpine, Mage pipeline |
| 6 | ✅ | QEMU extraction fix — weilnetz.de installer, silent NSIS |
| 7 | ✅ | Alpine NoCloud + silent boot + reset + config reuse + sshd drop-in |
| 8 | planned | Auth, HTTPS, multi-tenant, skill marketplace, streaming output |

---

## Out of scope V0

- Multi-agent paralelo (um agente por repo simultâneo)
- VS Code Server
- Linux/macOS production-tested
- Auto-update do binário
- Code signing (Windows Defender)
- First boot 100% offline (mage fetch precisa de internet em build time)

---

## Documentos relacionados

- `.ai/VISION.md` — Visão de produto (manifesto, por que existe)
- `.ai/PRD.md` — Product requirements (V0)
- `.ai/CONTEXT.json` — Snapshot técnico estruturado
- `.ai/FASE_2.md` … `FASE_7.md` — Histórico de cada fase (decisões, arquivos, limitações)
- `.ai/EXAMPLES.md` — Specs de exemplo
- `CLAUDE.md` — Instruções para o Claude Code (regras absolutas, paths, stack)
- `README.md` — Documentação pública (quick start, build, configuração)
