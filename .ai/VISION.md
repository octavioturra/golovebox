# golovebox — Visão de Produto

## A frase

> **Devin + n8n + Temporal, em um binário, self-hosted, orientado a intenção natural.**

V1 reposiciona: **um TechLead autônomo que especifica, delega para CLIs de código, verifica e comunica** — sem escrever código.

---

## O problema que todo mundo ignora

As ferramentas de agente existentes resolvem o problema errado.

Devin é poderoso mas vive na nuvem — seus dados, seu código, seu contexto, tudo passa por servidores de terceiros. OpenHands e SWE-agent são pesados, cheios de dependências, precisam de Python, Docker, configuração. n8n e Zapier automatizam fluxos mas exigem que *você* monte o grafo. Temporal e Cadence são plataformas de engenharia, não ferramentas de desenvolvedor individual.

E uma camada acima: Claude Code, Codex e Gemini CLI já escrevem código melhor do que qualquer ReAct loop genérico. A lacuna real **não é gerar código** — é orquestrar, manter contexto e comunicar.

Nenhum dos existentes é portable. Nenhum é self-hosted de verdade. Nenhum fala linguagem natural como interface primária. Nenhum coordena CLIs de código sem ser ele mesmo um SaaS.

---

## O que o golovebox é (V0 — entregue)

Um ambiente de execução autônomo completo, distribuído como um único `.exe`.

Você copia para qualquer pasta, roda, e tem:
- VM Alpine isolada para execução segura de código (QEMU embutido)
- Agente que lê suas intenções em linguagem natural (DSL em Markdown)
- Orquestrador que gera e executa DAGs de trabalho via LLM
- Interface web com DAG ao vivo (Cytoscape), terminal SSH no browser (xterm.js) e file explorer SFTP
- Memória vetorial persistente entre sessões (chromem-go)
- Skills plugáveis por domínio
- Workflow Git completo: clone, branch, commit, push, PR — token nunca em logs

Sem Python. Sem Docker. Sem conta em nuvem. Sem admin. Sem instalação.

---

## O que o golovebox vai ser (V1 — TechLead)

Em vez de implementar, **coordena CLIs de código**. Em vez de tentar competir com Claude Code, **usa Claude Code**.

```
Issue no Jira ──▶ golovebox lê
                  └─ escreve .ai/specs/feature.md
                     └─ delega ao Claude Code (ou Codex, ou Gemini)
                        └─ CLI implementa em src/ + escreve .ai/reports/feature.md
                           └─ golovebox verifica diff vs spec + roda testes
                              └─ push, abre PR, atualiza Jira, manda Slack
```

Golovebox **entende o projeto** o suficiente para especificar e verificar. Os CLIs implementam. Os humanos aprovam checkpoints e recebem updates.

---

## A DSL de Intenção

O diferencial que nenhum outro tem: você não configura o workflow — você *descreve o que quer* e o orquestrador monta o DAG.

A linguagem é prosa. Com anotações opcionais quando você precisa de controle:

```
Construa a tela de login com email e senha.
ATTENTION_HERE: validar com o time de segurança antes de prosseguir.

NEW BRANCH feature/login-jwt

O token JWT deve expirar em 24h.
RUN_TEST: go test ./internal/auth/...

TRY autenticação via OAuth OR_ELSE fallback para senha local.

NOTIFY_ME quando os testes passarem.
PUSH
PR "feat: tela de login com JWT"

NOT_TODO: internacionalização — registrar como débito técnico.
```

Isso não é YAML. Não é JSON. Não é código. É intenção — legível por humano, executável por agente.

O orquestrador lê, gera um DAG de execução com tasks auto-contidas (cada node sabe o objetivo original), e despacha. Tudo persiste em `.golovebox/runs/`.

---

## A Arquitetura

```
┌──────────────────────────────────────────────────────────────┐
│                       golovebox.exe                           │
│                                                                │
│  Web UI (Alpine + Cytoscape + xterm)  ◀── localhost:8080      │
│  Telegram gateway                                              │
│  CLI (cobra)                                                   │
│             │                                                  │
│  Orchestrator (LLM → DAG JSON, tasks auto-contidas)            │
│             │                                                  │
│  DAG Executor (paralelo, checkpoint pre-buffering, resume)     │
│             │                                                  │
│  ┌──────────┴──────────┐                                       │
│  │ Workflow nodes      │  task nodes → Agent Loop (ReAct)      │
│  │ sync_repo / branch  │                  │                    │
│  │ push / pr           │                  Tools: shell,        │
│  └──────────┬──────────┘                  files, git, github   │
│             │                                  │               │
│             └────────── SSH Pool ──────────────┘               │
│                            │                                    │
│              QEMU Alpine VM (cloud-init NoCloud, sshd)         │
└──────────────────────────────────────────────────────────────┘
```

- **Três camadas conceituais**: Tools (funções puras) → Skills (composição com contexto) → Agents (especializados, V1)
- **Workflow nodes** despachados diretamente (sync_repo, branch, push, pr) — não passam pelo ReAct loop
- **Task nodes** entram no Agent Loop com a task auto-contida do orchestrator
- **Estado persiste atômico** via `os.Rename` — `golovebox resume` retoma exato

---

## A Interface

Web chat embutido. Sem Electron. Sem framework. HTML+JS servido pelo próprio binário.

Você vê o canvas do DAG nascendo em tempo real:
- **cinza** — pendente
- **amarelo** — executando (com contador `[7/20]` overlaid)
- **verde** — concluído
- **vermelho** — erro (painel direito abre automaticamente no node falho)
- **laranja** — aguardando você

Clica em qualquer node, vê:
- Objetivo do run + descrição do node + branch atual
- Por iteração: blocos colapsáveis com prompt enviado ao LLM e resposta crua
- Tool calls com parâmetros e observação (syntax highlight)

Bottom panel com tabs:
- **Terminal** — SSH PTY ao vivo via WebSocket (Vim, top, git log funcionam)
- **Arquivos** — explorer SFTP, click para visualizar inline

Sessão persiste em localStorage. Refresh recupera tudo.

---

## Por que isso ainda não existe

Porque é difícil fazer tudo isso *sem* criar uma plataforma pesada.

A aposta: **Go**. Binário único, zero runtime, CGO desabilitado, portable por design. A VM vem embutida no init. A interface vem no binário via `embed.FS`. O workflow engine é Go puro, sem Temporal, sem Redis, sem broker.

A complexidade toda fica dentro do `.exe`. O usuário vê só a pasta.

---

## O Usuário

Desenvolvedor individual ou time pequeno que:
- Quer automatizar construção de software sem supervisão constante
- Valoriza controle e privacidade — dados ficam na máquina
- Não quer administrar infraestrutura para usar uma ferramenta de desenvolvimento
- Pensa em sistemas, não em prompts
- (V1) Já usa Claude Code / Codex / Gemini e quer um orquestrador acima

---

## Status

**V0 — entregue.** 18 fases. Plataforma completa: DAG executor, Web UI, VM, Git workflow, observabilidade, bugfixes. Detalhes em `FASES.json`.

**V1 — em planejamento.** 5 fases para promover golovebox de Dev a TechLead. Detalhes em `hypercontext.json:v1_roadmap`.

---

## Definição de Pronto

### V0 (entregue)

> Jogar arquivos de spec em DSL natural no golovebox, ver o orquestrador gerar um DAG,
> agente executar dentro da VM isolada, checkpoints pausarem para revisão no web chat,
> e o resultado ser código no branch, PR aberta, e notificação enviada —
> tudo a partir de um único `.exe`, sem instalar nada no sistema.

### V1 (alvo)

> Abrir uma issue no Jira. Golovebox lê, escreve `.ai/specs/feature.md`,
> delega ao Claude Code, Claude Code implementa em `src/` e documenta em `.ai/reports/feature.md`,
> golovebox verifica diff vs spec + roda testes, push, abre PR, atualiza Jira,
> manda Slack pro time e email pro cliente — **sem tocar em uma linha de código**.
