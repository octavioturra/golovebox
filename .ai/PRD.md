# golovebox — Product Requirements Document

**Versão:** 0.3 — V0 completo (29 fases) + V1 planejado
**Status:** V0 completo (fases 1-29). Ver [FASES.json](FASES.json) para digest completo. V1 em planejamento.

---

## 1. Problema

Ferramentas de agente autônomo existentes (OpenHands, Devin, Manus) têm dois problemas:

**Problema 1 — Dependências pesadas:**
Python com 300 bibliotecas onde sempre tem uma que quebra. Node.js. Docker. Daemons rodando em background. Instaladores que escrevem em `C:\` sem pedir licença.

**Problema 2 — Escopo estreito:**
Escrevem código. Só isso. Não sabem comunicar o time, atualizar o Jira, mandar o relatório pro cliente, agendar o teste com o QA. O desenvolvedor ainda precisa fazer toda a coordenação manualmente.

---

## 2. Solução

`golovebox.exe` — um único arquivo executável, portable, que você copia para qualquer pasta e roda. Sem instalação. Sem Python. Sem Node. Sem Docker. Sem admin.

**V0:** Agente de codificação portable. Escreve código, cria branches, abre PRs.

**V1:** TechLead autônomo. Para de escrever código — especifica, delega para CLIs de código (Claude Code, Codex, Gemini CLI), verifica o resultado, e mantém todo mundo informado.

---

## 3. Usuário Alvo

Desenvolvedor individual ou pequeno time (3-8 pessoas) que:
- Trabalha com repositórios GitHub
- Quer features virarem PRs sem supervisão constante
- Quer clientes e stakeholders atualizados sem overhead manual
- Valoriza controle sobre o ambiente (self-hosted, dados locais)
- Desenvolve no Windows

---

## 4. Proposta de Valor

| Problema atual | golovebox |
|---|---|
| Python + pip + venv = inferno de versões | Binário único, zero dependências |
| Instala coisa em `C:\` sem perguntar | Tudo em `.golovebox/` ao lado do `.exe` |
| Docker obrigatório para sandbox | QEMU embutido, instalado automaticamente |
| Agentes escrevem código mas não coordenam | TechLead que especifica, delega, verifica, comunica |
| Contexto do projeto perdido entre sessões | `.ai/` no repo — memória viva, versionável, portável |
| Stakeholders precisam ser atualizados manualmente | Slack, email, calendário automáticos (V1) |

---

## 5. Casos de Uso

### UC1 — Task via web UI (V0)
```
Usuário digita no chat: "implemente autenticação JWT no /api/auth"
→ Orchestrator gera DAG com 4 nodes
→ sync_repo: git pull do repo padrão
→ NEW BRANCH feature/jwt-auth
→ Agent escreve código na VM, roda testes
→ PUSH + PR aberto
→ Canvas mostra DAG verde, log ReAct visível no node panel
```

### UC2 — Spec com 5 fases (V0)
```
Usuário faz upload de spec.md com keywords DSL
→ DAG com nodes paralelos e checkpoints
→ ATTENTION_HERE: canvas pausa em laranja, aguarda aprovação
→ RUN_TEST: gate — só avança se testes passam
→ PR com título e body gerados a partir da task
```

### UC3 — TechLead com CLI delegation (V1)
```
Issue aberta no Jira: "Feature: login social com Google"
→ golovebox lê issue, entende .ai/CONTEXT.md do projeto
→ escreve .ai/specs/google-oauth.md com contexto rico
→ cria branch, sincroniza repo
→ delega: claude code "implemente per .ai/specs/google-oauth.md"
→ claude code implementa, documenta em .ai/reports/google-oauth.md
→ golovebox verifica diff vs spec + testes
→ push, PR aberto e linkado ao ticket Jira
→ Jira: ticket → "in review"
→ Slack: "@time PR #87 pronta para review"
→ Email pro cliente: "login social disponível em staging"
→ .ai/tasks/TASK_042.md documentado com síntese
```

### UC4 — Monitoramento contínuo (V0/V1)
```
golovebox daemon
→ escuta Telegram/Slack
→ age conforme triggers chegam
→ reporta progresso no canal de origem
```

---

## 6. Arquitetura

Workspace Go com 7 módulos independentes. Ver [README](../README.md#módulos) para mapa completo e diagrama de dependências.

---

## 7. Estrutura em Runtime

```
D:\qualquer\pasta\
├── golovebox.exe
└── .golovebox\
    ├── config.toml
    ├── qemu\                    # flat: exe + DLLs + share/qemu/
    ├── vm\
    │   ├── base.img             # Alpine cloud qcow2 (~164MB)
    │   ├── cidata.iso           # cloud-init seed
    │   ├── id_rsa               # keypair SSH
    │   └── qemu.log
    ├── memory\                  # chromem-go vectors
    ├── skills\                  # .md com frontmatter TOML
    └── runs\<id>\
        ├── dag.json
        ├── node_states.json
        ├── task.txt
        └── logs\<nodeID>.jsonl

repo\  (repositório do usuário)
└── .ai\                         # território do golovebox no repo
    ├── CONTEXT.md               # contexto permanente do projeto
    ├── DECISIONS.md             # ADRs
    ├── specs\<feature>.md       # golovebox escreve antes de delegar
    ├── reports\<feature>.md     # CLI escreve após implementar (V1)
    └── tasks\<TASK_ID>.md       # síntese após cada run
```

---

## 8. Primeiro Boot

`golovebox init` (idempotente — reusa config.toml se existir):

1. Extrai QEMU embutido → `.golovebox/qemu/` (flat layout)
2. Extrai Alpine cloud qcow2 embutida → `.golovebox/vm/base.img`
3. Gera keypair RSA 4096 → `.golovebox/vm/id_rsa`
4. Constrói CIDATA ISO9660 com cloud-init
5. Config wizard:
   - Provider LLM + API key + modelo
   - GitHub token (escopo: `repo`)
   - Repo padrão (`owner/repo`)
   - Telegram bot token (opcional)
6. Boot QEMU silencioso (`stdout → vm/qemu.log`, `stdin → /dev/null`)
7. Smoke test: poll SSH até `echo ok`

Sem internet após o build. Reexecutável via `golovebox init --repair`.

---

## 9. Stack Técnica

Stack completa em [`.ai/AGENTS.md`](AGENTS.md). Regra absoluta: `CGO_ENABLED=0`.

---

## 10. Requisitos Funcionais

### Orchestration (V0)
- Converter spec Markdown com DSL em DAG executável
- Executar nodes em paralelo (máx 3 simultâneos)
- Pausar em `ATTENTION_HERE` e aguardar aprovação humana
- Retomar run após crash (`golovebox resume <id>`)
- Keywords: `NEW BRANCH`, `PUSH`, `PR`, `RUN_TEST`, `NOTIFY_ME`, `NOT_TODO`, `TRY/OR_ELSE`

### Workflow Git (V0)
- Sync automático do repo padrão a cada run (clone ou pull)
- Criar branch via `NEW BRANCH`
- Push com `--set-upstream` via `PUSH`
- Abrir ou atualizar PR via `PR`
- Detectar conflito de merge → `waiting_human` automático

### Web UI (V0)
- Health dashboard: VM, LLM, GitHub, Repo em tempo real
- Input de task sem criar arquivo
- Canvas DAG com Cytoscape — live update via SSE, sem re-render
- Log ReAct por node em tempo real (iter, action, params, obs)
- Terminal SSH interativo no browser (xterm.js)
- File explorer da VM (SFTP read-only)
- Sessão persistente via `localStorage`
- Botão stop — cancela run ativo via context cancel

### CLI Delegation (V1)
- Interface `CodingAgent` — swap entre Claude Code, Codex, Gemini
- Tool `delegate_code` — spawn CLI com `.ai/specs/` como contexto
- Tool `verify_implementation` — diff vs spec + testes
- CLI instala na VM via cloud-init
- Contrato: golovebox escreve `.ai/specs/`; CLI escreve `.ai/reports/`

### Communication Layer (V1)
- Slack: notifica ao iniciar, concluir, bloquear
- Email: relatórios, updates para cliente
- Calendário: agenda demos, reviews, testes
- Templates configuráveis por projeto

### PM Integration (V1)
- GitHub Issues → input de task
- Jira / Linear — lê e atualiza status conforme DAG avança
- PR linkado ao ticket automaticamente
- Sprint report gerado pelo golovebox

---

## 11. Requisitos Não-Funcionais

- **Portabilidade:** zero escrita fora de `.golovebox/` ao lado do `.exe`
- **Distribuição:** `.exe` único sem dependências pré-instaladas no host
- **CGO:** `CGO_ENABLED=0` obrigatório — sem gcc no host Windows
- **Tamanho do binário:** ~450MB (inclui QEMU + Alpine qcow2 embutidos)
- **LLM:** interface OpenAI-compatible — zero lock-in de provider
- **Privacidade:** dados locais, sem telemetria, sem conta em nuvem
- **Resiliência:** run retomável após crash — `dag.json` + `node_states.json`
- **Observabilidade:** log estruturado via `slog`, `--log-format json` no daemon

---

## 12. DSL de Intenção

Prosa Markdown com keywords opcionais em MAIÚSCULAS. Spec canônica: [`promptlang/DSL.md`](../promptlang/DSL.md).

---

## 13. Fora do Escopo V0

- CLI delegation (v1_fase1)
- Slack / email / calendário
- Jira / Linear
- Linux/macOS production-tested
- Auto-update do binário
- Code signing (Windows Defender)
- Execução de servidores (`run_mode = build_only`)

## Fora do Escopo V1

- Mobile UI
- ClaWHub — skill marketplace remoto
- Multi-tenant / multi-user
- Windows Defender signing

---

## 14. Roadmap de Entrega

### V0 — Portable Coding Workflow ✅ Completo

29 fases entregues. Ver [FASES.json](FASES.json) para digest completo.

### V1 — TechLead Autônomo

Roadmap detalhado: [`hypercontext.json:v1_roadmap`](hypercontext.json).

---

## 15. Definição de Pronto

**V0:**
> Copiar `golovebox.exe` para qualquer pasta Windows, rodar `golovebox init`, abrir `localhost:8080`, digitar uma task com 5 fases incluindo `NEW BRANCH`, `RUN_TEST` e `PR`, ver o DAG executar na VM, ver o log ReAct em tempo real, e receber o PR aberto no GitHub — sem instalar nada.

**V1:**
> Abrir uma issue no Jira. Golovebox lê, escreve `.ai/specs/`, delega ao Claude Code, Claude Code implementa e documenta em `.ai/reports/`, golovebox verifica, push, PR, atualiza Jira, manda Slack pro time e email pro cliente — sem tocar em uma linha de código.
