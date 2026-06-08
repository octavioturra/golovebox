# golovebox — Visão de Produto

## A frase

> **O TechLead autônomo. Especifica, delega para CLIs de código, verifica e comunica — sem escrever código.**

---

## O problema que todo mundo ignora

As ferramentas de agente existentes resolvem o problema errado.

Devin é poderoso mas vive na nuvem — seus dados, seu código, seu contexto, tudo passa por servidores de terceiros. OpenHands e SWE-agent são pesados, cheios de dependências, precisam de Python, Docker, configuração. n8n e Zapier automatizam fluxos mas exigem que *você* monte o grafo. Temporal e Cadence são plataformas de engenharia, não ferramentas de desenvolvedor individual.

E uma camada acima: Claude Code, Codex e Gemini CLI já escrevem código melhor do que qualquer ReAct loop genérico. A lacuna real **não é gerar código** — é orquestrar, manter contexto e comunicar.

Nenhum dos existentes é portable. Nenhum é self-hosted de verdade. Nenhum fala linguagem natural como interface primária. Nenhum coordena CLIs de código sem ser ele mesmo um SaaS.

---

## V0 — Dev (entregue)

Um ambiente de execução autônomo completo, distribuído como um único `.exe`. Sem Python. Sem Docker. Sem conta em nuvem. Sem admin. Sem instalação.

Detalhe técnico: decomposição em 7 módulos Go independentes, swarm-habilitado. Ver [README](../README.md) e [FASES.json](FASES.json).

---

## V1 — TechLead (próximo)

Em vez de implementar, **coordena CLIs de código**. Em vez de tentar competir com Claude Code, **usa Claude Code**.

```
Issue no Jira ──▶ golovebox lê
                  └─ escreve .ai/specs/feature.md
                     └─ delega ao Claude Code (ou Codex, ou Gemini)
                        └─ CLI implementa em src/ + escreve .ai/reports/feature.md
                           └─ golovebox verifica diff vs spec + roda testes
                              └─ push, abre PR, atualiza Jira, manda Slack
```

Roadmap detalhado: [`hypercontext.json:v1_roadmap`](hypercontext.json).

---

## A DSL de Intenção

O diferencial: você não configura o workflow — você *descreve o que quer*.

```
Construa a tela de login com email e senha.
ATTENTION_HERE: validar com o time de segurança antes de prosseguir.
NEW BRANCH feature/login-jwt
RUN_TEST: go test ./internal/auth/...
PUSH
PR "feat: tela de login com JWT"
NOT_TODO: internacionalização — registrar como débito técnico.
```

Spec completa da DSL: [`promptlang/DSL.md`](../promptlang/DSL.md).

---

## A Arquitetura

7 módulos independentes. Setas de dependência só apontam para `core`. Ver [README](../README.md#módulos).

Princípio: **Go** como aposta de portabilidade. Binário único, zero runtime, CGO desabilitado, VM embutida no init, interface via `embed.FS`. A complexidade toda fica dentro do `.exe`.

---

## O Usuário

Desenvolvedor individual ou time pequeno que:
- Quer automatizar construção de software sem supervisão constante
- Valoriza controle e privacidade — dados ficam na máquina
- Não quer administrar infraestrutura para usar uma ferramenta de desenvolvimento
- Pensa em sistemas, não em prompts
- (V1) Já usa Claude Code / Codex / Gemini e quer um orquestrador acima

---

## Definição de Pronto

### V0 (entregue, 28+ fases)

> Jogar arquivos de spec em DSL natural no golovebox, ver o orquestrador gerar um DAG, agente executar dentro da VM isolada, checkpoints pausarem para revisão no web chat, e o resultado ser código no branch, PR aberta, e notificação enviada — tudo a partir de um único `.exe`, sem instalar nada no sistema.

### V1 (alvo)

> Abrir uma issue no Jira. Golovebox lê, escreve `.ai/specs/feature.md`, delega ao Claude Code, Claude Code implementa em `src/` e documenta em `.ai/reports/feature.md`, golovebox verifica diff vs spec + roda testes, push, abre PR, atualiza Jira, manda Slack pro time e email pro cliente — **sem tocar em uma linha de código**.
