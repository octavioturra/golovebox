# Fase 3 — Gateway: Achados e Aprendizados

## Status
Build: `GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build ./cmd/golovebox` ✓
Binário: `golovebox.exe` ~13 MB (sem crescimento — Telegram é HTTP puro)

---

## Arquivos criados

| Arquivo | Função |
|---|---|
| `internal/sandbox/pool.go` | Pool de conexões SSH — Acquire/Release/Close |
| `internal/gateway/gateway.go` | Router central: Handler interface, RunTask, buildRegistry |
| `internal/gateway/telegram.go` | Bot Telegram completo: parsing, dispatch, progress updates |
| `internal/gateway/slack.go` | Stub — `errors.New("not implemented in V0")` |
| `internal/gateway/email.go` | Stub — `errors.New("not implemented in V0")` |

## Arquivos modificados

| Arquivo | Mudança |
|---|---|
| `internal/llm/client.go` | Retry com backoff: `statusError`, `isRetryable`, `retryDelay`, `doComplete` extraído |
| `internal/agent/loop.go` | `ProgressFunc`, `MaxIterations` exportado, campo `ssh` removido |
| `internal/tools/github.go` | `CloneRepo` usa GIT_ASKPASS — token fora da URL e do `ps aux` |
| `internal/config/config.go` | Campo `DefaultRepo string` adicionado |
| `internal/setup/init.go` | Wizard prompta `DefaultRepo` |
| `cmd/golovebox/main.go` | `buildRegistry` removido (movido para gateway), `newDaemonCmd` adicionado |

---

## Decisões de Design

### SSH Pool — sem limite máximo

- Pool sem `MaxConns` — Phase 3 tem at most uma tarefa ativa por vez
- `Acquire` pops da slice idle (LIFO) para reutilizar conexões mais recentes (mais provável estarem vivas)
- `Release` push na mesma slice — sem validação de liveness (simplidade > overhead de keepalive)
- Trade-off: conexão stale fica no pool silenciosamente; na Phase 4, checar `client.SendRequest("keepalive@openssh.com", true, nil)` antes de retornar

### GIT_ASKPASS — token fora da URL

- Script `#!/bin/sh\necho '<token>'\n` escrito via SFTP em `/tmp/.golovebox_askpass.sh`
- `chmod +x` antes do clone; `rm -f` em `defer` após
- Token aparece em `/tmp/` na VM (mas não em `git log --remotes` nem em `ps aux` do host)
- Single-quote escaping: `strings.ReplaceAll(token, "'", "'\\''")` — cobre GitHub tokens (alfanumérico + `_`)
- Para Phase 4: escrever em `/tmp/.golovebox_askpass_<uuid>.sh` para evitar conflitos em tarefas paralelas

### LLM Retry — backoff com jitter

- `statusError` type permite `errors.As` sem parsear strings de erro
- Retryable: 429 (rate limit), 500/502/503/504 (infra), `net.Error` (rede)
- Não-retryable: 400/401/403/404 — retorna imediato sem retry (erro permanente)
- Delay: `base * 2^attempt ± 20%` capped em `MaxDelay`
- Jitter via `rand.Int63n` — Go 1.20+ auto-seed, sem `rand.Seed()` necessário
- Log via `fmt.Fprintf(os.Stderr, ...)` — sem logger estruturado (Phase 4 pode adicionar `slog`)

### ProgressFunc — callback nil-safe

- `type ProgressFunc func(iteration int, action, observation string)` no package `agent`
- `agent.MaxIterations` exportado para que `telegram.go` possa formatar `[3/20]` sem duplicar a constante
- Observation truncada em 120 chars antes de chamar progress — Telegram tem limite de 4096 por mensagem, mas mensagens curtas são mais legíveis no mobile
- `nil` é aceito — callers CLI passam `nil` e o loop funciona como antes

### Gateway — RunTask centraliza o fluxo

- `buildRegistry` movido de `main.go` para `gateway.go` — elimina duplicação entre `github` command e Telegram handler
- `RunTask` adquire/libera uma conexão SSH para o setup inicial (clone + README index); as tools do loop adquirem suas próprias conexões via pool
- `Run(ctx)` bloqueia em `wg.Wait()` — quando ctx cancela, `TelegramHandler.Start` retorna via `case <-ctx.Done()`, wg.Done() é chamado, e Run retorna
- Sem handlers registrados: `Run` bloqueia em `<-ctx.Done()` diretamente — daemon funciona mesmo sem token Telegram configurado

### Telegram — goroutine por mensagem

- `handleMessage` roda em goroutine separada para não bloquear o update loop
- `ctx` do `Start` é propagado para `handleMessage` e `RunTask` — shutdown cancela tarefas ativas
- `parseTrigger` usa `regexp.MustCompile` em var de package (compilado uma vez)
- Regex `(?i)issue\s+#?(\d+)(?:\s+repo\s+([0-9A-Za-z._-]+/[0-9A-Za-z._-]+))?` — sem `\w` para evitar ambiguidade com `-` em character classes no RE2 de Go

---

## APIs de Libs Usadas

### telegram-bot-api/v5 v5.5.1
```go
// Init
bot, err := tgbotapi.NewBotAPI(token string) (*BotAPI, error)
bot.Self.UserName  // bot username após init

// Receber updates
u := tgbotapi.NewUpdate(offset int)
u.Timeout = 60  // long poll seconds
updates := bot.GetUpdatesChan(u) // <-chan Update
bot.StopReceivingUpdates()       // fecha o canal

// Enviar mensagem
msg := tgbotapi.NewMessage(chatID int64, text string) MessageConfig
bot.Send(msg) (Message, error)

// Update struct
update.Message.Text     string
update.Message.Chat.ID  int64
```

---

## Limitações Conhecidas (Phase 3)

1. **Pool sem validação de liveness**: conexão stale no idle pool causa erro na primeira tool call da iteração seguinte; o agente observa o erro e pode tentar novamente
2. **Askpass em path fixo**: `/tmp/.golovebox_askpass.sh` — conflito se duas tarefas rodarem simultaneamente (não acontece em V0, uma tarefa por vez)
3. **Telegram: sem rate limit**: mensagens de progresso a cada iteração (até 20) podem ser bloqueadas pelo Telegram se enviadas rápido demais (limite: 1 msg/s por chat)
4. **Daemon: uma tarefa por vez por chat**: sem fila — segunda mensagem enquanto task está rodando inicia nova goroutine e nova task em paralelo (pool tem conexões suficientes mas LLM pode ficar lento)
5. **Retry não cobre SFTP**: falhas de SFTP (read_file, write_file) não têm retry — Phase 4 pode adicionar retry no nível da tool
6. **GIT_ASKPASS em texto plano**: token fica em `/tmp/` na VM até o defer executar; se o processo for morto antes do defer, o arquivo permanece

---

## O que fica para Phase 4 (Polish)

- [ ] Pool: validação de liveness antes de retornar idle connection (`SendRequest keepalive`)
- [ ] Pool: limite máximo de conexões para daemon com múltiplos usuários simultâneos
- [ ] Askpass: path com UUID para paralelismo seguro
- [ ] Telegram: rate limit de mensagens de progresso (bucket de 1 msg/s)
- [ ] Telegram: fila de tarefas por chat (não iniciar nova task se já existe uma ativa)
- [ ] Daemon: `slog` estruturado em vez de `fmt.Fprintf(os.Stderr)`
- [ ] `golovebox daemon --dry-run` — valida config sem subir VM
- [ ] Windows Defender signing para o `.exe`
- [ ] Auto-update via GitHub Releases
- [ ] Linux e macOS: substituir `qemu-system-x86_64.exe` por path dinâmico por OS
