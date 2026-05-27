# Fase 4 — Orchestrator + Web Chat: Achados e Aprendizados

## Status

Build: `GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build ./cmd/golovebox` ✓  
Binário: `golovebox.exe` ~14MB (+1MB vs Fase 3 — HTML embutido + novos pacotes)  
`go vet ./...` → zero warnings

---

## Arquivos Criados

| Arquivo | Função |
|---|---|
| `internal/dsl/keywords.go` | Constantes de KeywordType (ATTENTION_HERE, TRY, NOT_TODO…) |
| `internal/dsl/parser.go` | ParseFile / ParseDir — extrai Annotation e TechDebt de .md |
| `internal/dag/dag.go` | DAG, Node, NodeState, NodeType — grafo de execução |
| `internal/dag/checkpoint.go` | CheckpointManager — race-safe approve/reject com pre-buffering |
| `internal/dag/executor.go` | Executor — loop paralelo, persist atômico, run_summary.md |
| `internal/orchestrator/orchestrator.go` | specs → DAG via LLM (JSON plan) |
| `internal/skills/registry.go` | Carrega skills .md com frontmatter TOML |
| `internal/skills/generator.go` | Gera skill via LLM → salva em .golovebox/skills/ |
| `internal/web/store.go` | RunStore — gestão de .golovebox/runs/<id>/ |
| `internal/web/server.go` | HTTP server: /api/* endpoints + SSE fanout |
| `internal/web/static/index.html` | Web chat embed — Canvas2D DAG + SSE client |

## Arquivos Modificados

| Arquivo | Mudança |
|---|---|
| `internal/sandbox/pool.go` | Liveness check via `SendRequest("keepalive@openssh.com")` |
| `internal/tools/github.go` | Askpass path com UUID — evita colisão em execuções paralelas |
| `internal/config/config.go` | `RunsDir()` e `SkillsDir()` adicionados |
| `internal/setup/init.go` | Cria `runs/` e `skills/` no `step1Dirs` |
| `internal/gateway/gateway.go` | `RunSpecTask` — executa agent loop com task arbitrária |
| `cmd/golovebox/main.go` | Novos cmds: `web`, `run`, `skill list/generate`, `resume`; `run` → `exec` |

---

## Dívidas Resolvidas da Fase 3

- ✅ **Pool liveness**: stale connections agora detectadas e descartadas antes de retornar ao caller
- ✅ **Askpass UUID**: `/tmp/.golovebox_askpass_<uuid>.sh` — seguro para execuções paralelas
- ✅ **Dirs faltantes**: `runs/` e `skills/` criados no init

---

## Decisões de Design

### CheckpointManager — pre-buffering race-free

O `setStateWaiting` do executor chama `notify(node)` **sincronamente** antes de chamar `cm.Wait(node.ID)`. Isso cria uma window onde `Approve/Reject` pode ser chamado (pelo terminal ou web UI) antes de `Wait` registrar o channel.

Solução: Approve/Reject e Wait compartilham o mesmo channel via `pending[nodeID]`:
- Se `Approve` chegar antes: cria channel (cap 1) e envia o resultado
- Se `Wait` chegar depois: encontra o channel já preenchido e retorna imediatamente
- Se `Wait` chegar antes: cria o channel vazio, `Approve` o preenche depois

Nenhum sleep, nenhum polling — zero-CPU blocking.

### embed.FS vs `[]byte`

Usamos `//go:embed static` com `embed.FS` (não `[]byte`) para manter a hierarquia de diretórios. Isso permite no futuro servir múltiplos assets (CSS separado, favicon) sem mudar o servidor.

### Persist atômico via rename

O executor escreve `dag.json.tmp` e depois `os.Rename` para `dag.json`. No NTFS e ext4, rename é atômico — `golovebox resume` sempre lê estado consistente mesmo se o processo foi morto a meio write.

### DSL Parser — TRY multi-linha

`TRY <estratégia A>` pode ser seguido por `OR_ELSE <estratégia B>` em linhas subsequentes. O parser acumula o buffer `tryBuffer` até encontrar `OR_ELSE`, então emite dois Annotations: um `KwTry` e um `KwOrElse`. O orquestrador os mapeia para um node `TypeTryElse`.

### Skills frontmatter TOML (não YAML)

`BurntSushi/toml` já é dependência direta (config.toml). Usar TOML para frontmatter de skills evita adicionar `gopkg.in/yaml.v3` que tem histórico de CVEs e aumentaria o binário.

### `RunSpecTask` vs `RunTask`

`RunTask` é específico para GitHub issues (GetIssue → CloneRepo → index README → PlanFromIssue). `RunSpecTask` é o path genérico: apenas cria registry e roda o loop com a task string fornecida. Mantém backward compat para Telegram sem duplicar código.

### Executor paralelo — semáforo + state claim

Para evitar double-dispatch de um mesmo node em ticks consecutivos, o executor **marca o node como StateRunning antes de liberar o mutex** e antes de lançar a goroutine. `dag.Ready()` só retorna `StatePending`, então o node não aparece na próxima iteração.

---

## APIs de Libs Usadas

### `github.com/google/uuid v1.6.0`
```go
uuid.New().String() // gera UUID v4 como string, ex: "550e8400-e29b-41d4-a716-446655440000"
```

### `embed.FS` (stdlib)
```go
//go:embed static
var staticFS embed.FS

data, err := staticFS.ReadFile("static/index.html")
```

### `net/http` pattern routing (Go 1.22+)
```go
mux.HandleFunc("GET /api/runs/{id}/approve/{nodeID}", handler)
r.PathValue("id")  // extrai path variable
```

### SSE (Server-Sent Events) via stdlib
```go
w.Header().Set("Content-Type", "text/event-stream")
fmt.Fprintf(w, "data: %s\n\n", jsonPayload)
flusher.Flush()
```

### Canvas2D (JavaScript client-side)
```js
// No-dependency DAG rendering — topological sort + rounded rects + bezier arrows
const ctx = canvas.getContext('2d');
ctx.beginPath(); roundRect(ctx, x, y, w, h, r); ctx.fill();
```

---

## Limitações Conhecidas (Phase 4)

1. **Orquestrador depende do LLM**: se o LLM não retornar JSON válido, `Plan` falha. Workaround: retry com prompt mais restritivo ou parsing tolerante.

2. **SSE sem autenticação**: o web server em `localhost:8080` não tem auth. OK para uso local, problemático se exposto em rede.

3. **Checkpoint no terminal é síncrono**: o notify do executor é chamado na goroutine do node — `cm.Approve` é chamado inline. Race corrigida pelo pre-buffering, mas o stdin read é bloqueante. Se múltiplos checkpoints chegarem simultaneamente, o segundo espera o primeiro ser respondido.

4. **Skill `examples` não é array TOML em alguns casos**: se o LLM gerar `examples = "..."` (string) em vez de `examples = ["..."]`, o parse falha silenciosamente. Workaround: validação mais tolerante no parser.

5. **DSL parser não suporta TRY/OR_ELSE multi-arquivo**: o bloco TRY é resetado a cada arquivo. Specs que spans múltiplos arquivos precisam ter TRY/OR_ELSE no mesmo arquivo.

6. **Web server não tem HTTPS**: para Phase 5, adicionar TLS com `crypto/tls` e cert auto-gerado.

7. **DAG canvas não renderiza quando nodes == 0**: tela fica em branco. UI deveria mostrar "aguardando plan" enquanto o orquestrador processa.

---

## O que fica para Phase 5

- [ ] Auth básico no web server (token na URL ou header)
- [ ] HTTPS com cert auto-gerado (`crypto/tls`)
- [ ] Multi-tenant: múltiplos usuários com runs isoladas
- [ ] Skill marketplace: pull de skills de um registry remoto
- [ ] DAG canvas interativo: arrastar nodes, editar tasks
- [ ] Hot reload de skills via `fsnotify`
- [ ] Streaming de output do LLM (SSE chunked)
- [ ] Executor com retry automático em nodes `TypeTryElse`
- [ ] WHEN/DO: polling de condições externas (webhook, file watch)
- [ ] Telegram: notificação de checkpoints via bot
