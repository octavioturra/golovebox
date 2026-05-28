# golovebox — Visão de Produto

## A frase

> **Devin + n8n + Temporal, em um binário, self-hosted, orientado a intenção natural.**

---

## O problema que todo mundo ignora

As ferramentas de agente existentes resolvem o problema errado.

Devin é poderoso mas vive na nuvem — seus dados, seu código, seu contexto, tudo passa por servidores de terceiros. OpenHands e SWE-agent são pesados, cheios de dependências, precisam de Python, Docker, configuração. n8n e Zapier automatizam fluxos mas exigem que *você* monte o grafo — você é o arquiteto, a ferramenta só executa. Temporal e Cadence são plataformas de engenharia, não ferramentas de desenvolvedor individual.

Nenhum deles é portable. Nenhum é self-hosted de verdade. Nenhum fala linguagem natural como interface primária.

---

## O que o golovebox é

Um ambiente de execução autônomo completo, distribuído como um único `.exe`.

Você copia para qualquer pasta, roda, e tem:
- Uma VM Alpine isolada para execução segura de código
- Um agente que lê suas intenções em linguagem natural
- Um orquestrador que gera e executa grafos de trabalho
- Uma interface web para acompanhar e intervir
- Memória vetorial persistente entre sessões
- Skills plugáveis por domínio

Sem Python. Sem Docker. Sem conta em nuvem. Sem admin. Sem instalação.

---

## A DSL de Intenção

O diferencial que nenhum outro tem: você não configura o workflow — você *descreve o que quer* e o agente monta o workflow.

A linguagem é prosa. Com anotações opcionais quando você precisa de controle:

```
Construa a tela de login com email e senha.
ATTENTION_HERE: validar com o time de segurança antes de prosseguir.

O token JWT deve expirar em 24h.
NO_RETRY se a lib de crypto falhar — parar e notificar.

TRY autenticação via OAuth OR_ELSE fallback para senha local.

NOTIFY_ME quando os testes de integração passarem.

NOT_TODO: internacionalização — registrar como débito técnico.
```

Isso não é YAML. Não é JSON. Não é código. É intenção — legível por humano, executável por agente.

O agente lê isso, entende o que precisa ser feito, gera um DAG de execução, despacha sub-agentes especializados, e te notifica nos checkpoints.

---

## A Arquitetura em Três Camadas

### Camada 1 — Tools
Funções puras. Input, output. `shell`, `read_file`, `write_file`, `github_open_pr`, `send_email`, `http_request`. Building blocks.

### Camada 2 — Skills
Composição de tools com contexto de domínio. Uma skill de "revisar segurança de API" sabe quais tools usar, qual prompt aplicar, quais padrões checar. Skills vivem em `.golovebox/skills/` e são descobertas automaticamente.

### Camada 3 — Agents
Sub-agentes especializados com identidade, memória isolada e objetivo fixo. O agente de frontend não sabe nada de infraestrutura. O agente de segurança só faz revisão de segurança. Cada um com seu loop ReAct, suas skills, sua memória.

O orquestrador lê os specs, monta o DAG, despacha para os agentes certos, agrega os resultados.

---

## O DAG que o Agente Gera

Você escreve 10 arquivos de spec. O orquestrador gera 30-40 arquivos de plano — um DAG onde cada node é uma task com agente responsável, critério de saída e dependências.

```
[spec_ui_admin]──────────┐
                         ▼
[spec_auth]────────► [plan_login_screen] ──► [plan_jwt_layer] ──► RUN_TEST ──► [plan_deploy]
                         ▲                                              │
[spec_security]──────────┘                                    ATTENTION_HERE
```

Nodes executam em paralelo onde não há dependência. Gates bloqueiam até condição satisfeita. Checkpoints esperam você.

Estado do DAG persiste em `.golovebox/runs/` — se o processo morrer, você retoma de onde parou.

---

## A Interface

Web chat embutido. Sem Electron. Sem framework. HTML+JS servido pelo próprio binário.

Você vê o canvas do DAG nascendo em tempo real:
- **cinza** — pendente
- **amarelo** — executando
- **verde** — concluído
- **vermelho** — erro
- **laranja** — aguardando você

Clica num node laranja, vê o contexto, aprova ou ajusta. A execução continua.

---

## Por que isso ainda não existe

Porque é difícil fazer tudo isso *sem* criar uma plataforma pesada.

O golovebox resolve isso com uma aposta: Go. Binário único, zero runtime, CGO desabilitado, portable por design. A VM vem embutida no init, não no binário. A interface vem no binário via `embed.FS`. O workflow engine é Go puro, sem Temporal, sem Redis, sem broker.

A complexidade toda fica dentro do `.exe`. O usuário vê só a pasta.

---

## O Usuário

Desenvolvedor individual ou time pequeno que:
- Quer automatizar construção de software sem supervisão constante
- Valoriza controle e privacidade — dados ficam na máquina
- Não quer administrar infraestrutura para usar uma ferramenta de desenvolvimento
- Pensa em sistemas, não em prompts

---

## Definição de Pronto (V1)

> Jogar 10 arquivos de spec em DSL natural no golovebox, ver o orquestrador gerar um DAG,
> sub-agentes especializados construírem a aplicação dentro da VM isolada,
> checkpoints pausarem para revisão no web chat,
> e o resultado ser código testado, PR aberta, e notificação enviada —
> tudo a partir de um único `.exe`, sem instalar nada no sistema.
