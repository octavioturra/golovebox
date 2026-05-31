# golovebox — Product Requirements Document

**Versão:** 0.1 — V0  
**Status:** Em planejamento

---

## 1. Problema

Ferramentas de agente autônomo existentes (OpenClaw, Hermes, OpenHands, Manus) têm um problema em comum: dependências pesadas. Python com 300 bibliotecas onde sempre tem uma que quebra em alguma versão. Node.js. Docker. Daemons rodando em background. Instaladores que escrevem em `C:\` sem pedir licença.

O desenvolvedor quer focar em resolver issues, não em manter ambiente.

---

## 2. Solução

`golovebox.exe` — um único arquivo executável, portable, que você copia para qualquer pasta em qualquer drive e roda. Sem instalação. Sem Python. Sem Node. Sem Docker. Sem admin.

Na primeira execução ele mesmo configura tudo que precisa, dentro da própria pasta.

---

## 3. Usuário Alvo

Desenvolvedor individual ou pequeno time que:
- Trabalha com repositórios GitHub
- Quer automação de issues e PRs sem supervisão constante
- Usa Telegram/Slack para comunicação
- Valoriza controle sobre o ambiente (self-hosted, dados locais)
- Desenvolve no Windows

---

## 4. Proposta de Valor

| Problema atual | golovebox |
|---|---|
| Python + pip + venv = inferno de versões | Binário único, zero dependências |
| Instala coisa em `C:\` sem perguntar | Tudo em `.golovebox/` ao lado do `.exe` |
| Docker obrigatório para sandbox | QEMU embutido, instalado automaticamente |
| Configuração complexa | `golovebox init` interativo resolve tudo |
| Dados na nuvem de terceiros | Self-hosted, dados ficam na máquina |

---

## 5. Casos de Uso V0

### UC1 — Resolver Issue via Telegram
```
Usuário envia: "@golovebox issue #42"
→ Agente lê a issue no GitHub
→ Clona o repo na VM isolada
→ Implementa a solução
→ Roda os testes
→ Abre PR com descrição gerada
→ Responde no Telegram: "PR #87 aberta: [link]"
```

### UC2 — Trigger direto via CLI
```
golovebox github --repo owner/repo --issue 42
→ mesmo fluxo acima no terminal
```

### UC3 — Monitoramento contínuo
```
golovebox daemon
→ fica escutando Telegram/Slack/Email
→ age conforme triggers chegam
→ reporta progresso no canal de origem
```

---

## 6. Arquitetura

```
┌─────────────────────────────────────────────┐
│                golovebox.exe                │
│                                             │
│  ┌──────────┐    ┌─────────────────────┐   │
│  │ Gateway  │    │    Agent Loop       │   │
│  │          │───▶│  (ReAct pattern)    │   │
│  │ Telegram │    │                     │   │
│  │ Slack    │    │  think → plan → act │   │
│  │ Email    │    └──────────┬──────────┘   │
│  │ CLI      │               │               │
│  └──────────┘         tools │               │
│                             ▼               │
│  ┌──────────┐    ┌─────────────────────┐   │
│  │ Memory   │◀──▶│       Tools         │   │
│  │          │    │  shell / files      │   │
│  │ chromem  │    │  github / browser   │   │
│  │ vectors  │    │  email / slack      │   │
│  └──────────┘    └──────────┬──────────┘   │
│                             │ SSH/SFTP      │
└─────────────────────────────┼───────────────┘
                              │
                    ┌─────────▼──────────┐
                    │   QEMU Alpine VM   │
                    │                   │
                    │  git  go  python  │
                    │  node  gcc  make  │
                    └───────────────────┘
```

---

## 7. Estrutura de Arquivos em Runtime

```
D:\qualquer\pasta\
├── golovebox.exe
└── .golovebox\
    ├── config.toml          # configuração do usuário
    ├── qemu\                # QEMU instalado aqui
    ├── vm\
    │   └── base.img         # Alpine Linux VM
    ├── memory\              # embeddings vetoriais
    └── logs\                # logs por sessão
```

---

## 8. Fluxo de Primeiro Boot

`golovebox init` (executado automaticamente se `.golovebox/` não existir):

1. Cria `.golovebox/` ao lado do `.exe`
2. Baixa QEMU installer oficial (binário assinado) → instala em `.golovebox/qemu/`
3. Baixa Alpine VM image (~300MB) → extrai em `.golovebox/vm/`
4. Config interativo:
   - Provider LLM (Claude / GPT / Gemini / Ollama)
   - API key
   - GitHub token (escopo: `repo`)
   - Telegram bot token (opcional)
5. Smoke test: sobe VM → `echo ok` via SSH → imprime "golovebox pronto ✓"

Requer internet. Reexecutável via `golovebox init --repair`.

---

## 9. Stack Técnica

| Camada | Tecnologia | Justificativa |
|---|---|---|
| Linguagem | Go | Single binary, cross-platform, zero runtime |
| CGO | Desabilitado | Sem dependência de gcc no Windows |
| Gateway | sausheong/goclaw | Pure Go, Telegram/CLI, skill system |
| Sandbox | QEMU | Único sandbox cross-platform real |
| Controle VM | digitalocean/go-qemu (QMP) | Pure Go, JSON API sobre TCP |
| Exec na VM | golang.org/x/crypto/ssh | Pure Go, SSH + SFTP |
| Memória | philippgille/chromem-go | Pure Go, zero CGO, embedded |
| GitHub | google/go-github | Client oficial |
| Browser | go-rod/rod | Headless Chromium, auto-download |
| Config | BurntSushi/toml | Simples, pure Go |
| CLI | spf13/cobra | Subcomandos padrão Go |

---

## 10. Requisitos Funcionais

### Coding Agent
- Conectar repositório GitHub via personal access token
- Listar e ler issues abertas
- Clonar repo dentro da VM sandbox
- Executar ciclo ReAct até solução ou timeout
- Rodar testes dentro da VM
- Abrir Pull Request com título e descrição gerados pelo agente

### Comms Gateway
- Receber comandos via Telegram bot
- Receber menções em canais Slack
- Ler email via IMAP com trigger por subject pattern
- Reportar progresso e resultado no canal de origem

### Memória
- Indexar estrutura e README do repo na primeira clonagem
- Recuperar contexto relevante por similaridade antes de cada ciclo
- Persistir entre sessões em `.golovebox/memory/`

### Sandbox
- Instalar QEMU no primeiro boot em `.golovebox/qemu/`
- Manter VM Alpine com toolchain: git, go, python, node, gcc, make, curl
- Boot da VM no startup do daemon
- Shutdown graceful da VM ao encerrar

---

## 11. Requisitos Não-Funcionais

- **Portabilidade:** zero escrita fora de `.golovebox/` ao lado do `.exe`
- **Distribuição:** `.exe` único sem dependências pré-instaladas
- **CGO:** proibido — `CGO_ENABLED=0` no build e CI
- **Tamanho do `.exe`:** alvo ~30MB (VM image baixada no init, não embutida)
- **LLM:** interface OpenAI-compatible — sem lock-in de provider
- **Privacidade:** dados locais, sem telemetria

---

## 12. Fora do Escopo V0

- Multi-agent paralelo (um agente por repo simultâneo)
- VS Code Server integrado
- Interface web
- Linux e macOS validados
- Auto-update do binário
- Assinatura de código (Windows Defender)
- Execução sem internet no primeiro boot

---

## 13. Plano de Entrega V0

| Fase | Semanas | Entregável |
|---|---|---|
| 1 — Fundação | 1-2 | `.exe` portable que instala QEMU, sobe VM, executa shell |
| 2 — Agent Loop | 3-4 | Agente resolve issue simples e abre PR |
| 3 — Gateway | 5-6 | Trigger e resposta via Telegram |
| 4 — Polish | 7 | Usável por humano real no Windows |

---

## 14. Definição de Pronto

> Copiar `golovebox.exe` para qualquer pasta em qualquer drive Windows,
> executar `golovebox init`, enviar `@golovebox issue #42` no Telegram,
> e receber o link da PR aberta em resposta —
> sem instalar nada no sistema além do que o próprio `init` baixa na pasta local.
