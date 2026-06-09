
Você é um engenheiro Go sênior trabalhando no projeto **golovebox**.

| Este é um produto para cliente consumidor. Seja criterioso com a qualidade do código e cauteloso com bugs e erros. Evite más práticas conhecidas, mesmo que pareçam um bom caminho. Prefira padrões maduros e conhecidos de codificação

## O que é

Box (QEMU) + luva (Canvas + mediação). Portable app Windows-first.
Binário único, zero instalação — `golovebox.exe` em qualquer pasta, sem Python, Node, Docker ou admin.

O agente de trabalho é o **PicoClaw**, selado dentro da VM. A luva opera o PicoClaw via stdin/stdout, proxy do modelo (key nunca na box), resource manager e hook de mediação/audit.

V0: box + luva funcionando (fases 33–39). Ver `.ai/roadmap.yaml`.
V1 (planejamento futuro): receita de agente, multi-agente, secretary, memória .ai/.

## Onde olhar primeiro

- **`.ai/roadmap.yaml`** — plano V0 completo: fases 31–39, status, caminho crítico.
- **`.ai/FASES.json`** — digest das fases já entregues. Decisões arquiteturais vivas, libs em uso, padrões enduring.
- `.ai/fases/FASE_N.md` — histórico bruto, lê só sob demanda.
- `.ai/VISION.md` — narrativa de produto.
- `.ai/hypercontext.json` — metadados estruturados.

## Regras de Código — Sem Exceções

- `CGO_ENABLED=0` sempre
- Zero paths hardcoded — sempre `filepath.Join`
- Base dir sempre relativo ao executável:
  ```go
  execPath, _ := os.Executable()
  baseDir := filepath.Join(filepath.Dir(execPath), ".golovebox")
  ```
- Sem escrita fora de `baseDir`
- Build: `mage build` — nunca `go build` direto

## Estrutura de Módulos (workspace Go)

```
golovebox/
├── go.work              ← workspace-only, sem go.mod
├── core/                ← contratos, zero deps         → core/AGENTS.md
├── sandbox/             ← VM QEMU + core.Sandbox        → sandbox/AGENTS.md
├── toolskills/          ← skills registry + provisioner → toolskills/AGENTS.md
└── app/                 ← composition root + CLI + web  → app/AGENTS.md
    ├── magefile.go
    ├── cmd/golovebox/main.go
    └── internal/{config,embed,llm,setup,web}/
```

**Regra de dependência**: setas só apontam para `core`. `app` importa todos. Ninguém importa `app`.

> `promptlang/`, `derivator/`, `orchestrator/` foram deletados na FASE 33 — PicoClaw assume orquestração/DSL/derivação.
**Contexto por módulo**: leia `<modulo>/AGENTS.md` + `core/contracts.go`. Não precisa do resto.
**Mapa de contratos**: `core/CONTRACTS.md` — interface → implementador → consumidores.

## Runtime — .golovebox/

Detalhes completos em `FASES.json:runtime_layout`. Resumo:

```
.golovebox/
├── config.toml          # llm/github/telegram + [workflow] + (V1) [agents]
├── qemu/                # flat: qemu-system-x86_64.exe + DLLs + share/qemu/
├── vm/                  # base.img, cidata.iso, id_rsa, qemu.log
├── memory/, skills/
└── runs/<id>/
    ├── dag.json, node_states.json, task.txt, run_meta.json
    ├── logs/<nodeID>.jsonl   # iter/action/params/obs/prompt/reply/ts
    └── artifacts/
```

## Primeiro Boot

`golovebox init` — 7 steps idempotentes. Detalhes em `FASES.json` (FASE 5/7).
Resumo: extrai QEMU + Alpine qcow2 → gera keypair → CIDATA ISO com SSH key + `.gitconfig` (com `credential.helper=store`) + sshd drop-in → config wizard → boot silencioso → smoke test SSH.

## Web UI

`golovebox web` em `localhost:8080`. Layout em duas colunas + bottom panel:
- **Esquerda**: chat (status + 4 health dots auto-poll 5s + messages + textarea + runs)
- **Direita**: DAG canvas (Cytoscape dagre) + node panel deslizável + bottom panel (terminal/files com tabs)

**Node panel** (FASE 16/18):
- Objetivo do run + descrição do node + branch atual
- Por iter ReAct: blocos colapsáveis `Prompt enviado` (azul) e `Resposta do LLM` (verde), params, obs
- Box vermelho com erro; auto-abre quando node falha
- Persistido — recarregar página mantém histórico

## Agent Loop

```go
type ProgressFunc func(iter int, action, params, obs, prompt, reply string)
```

- Texto puro — portável entre todos os LLM providers
- Heredoc para valores multilinha: `content: <<EOF ... EOF`
- max 20 iterações por node
- Output → NodeLog buffer + .jsonl + SSE broadcast

## Decisões Vivas

Promovidas para `FASES.json:enduring_decisions`. Leia diretamente de lá.

## Status das Fases

V0 completo (1-29). Detalhes em `FASES.json:phases.v0_done`.
V1 em planejamento (5 fases). Detalhes em `FASES.json:phases.v1_planned`.

## Workflow de Documentação

1. **Implementando uma fase**: cria `.ai/fases/FASE_N.md` seguindo `FASE_TEMPLATE.md`. Bug fixes pontuais (15/16/17/18) seguem o mesmo formato.
2. **Ao final da fase**: arquivo permanece em `.ai/fases/` enquanto for "recente".
3. **A cada 3-5 fases**: digestão — releia os recentes, atualize `FASES.json`. Mantenha em `FASES.json` apenas o que ainda informa o presente.
4. **Lendo o projeto pela primeira vez**: leia `FASES.json` + `VISION.md` + `hypercontext.json` + este arquivo. Os `FASE_N.md` históricos só sob demanda.

## Estilo de Resposta

- Direto, sem preâmbulo
- Código limpo, DRY, KISS, YAGNI
- Quando criar arquivo: cria, não explica
- Quando editar: edita, não narra

## Regra de Documentação

Cada fato vive em **um único lugar**. Outros documentos linkam, nunca copiam.

| Tipo de informação | Dono |
|---|---|
| Fato vivo (decisão, lib, aprendizado) | `FASES.json` |
| Direção de produto e V1 | `hypercontext.json` |
| Spec de módulo (contrato, API) | `<módulo>/AGENTS.md` |
| Roadmap de fases (V0) | `.ai/roadmap.yaml` |
| Arquitetura de módulos | `README.md` |
| Narrativa de produto | `VISION.md` |
| Requisitos funcionais | `PRD.md` |
