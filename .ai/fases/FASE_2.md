# Fase 2 — Agent Loop: Achados e Aprendizados

## Status
Build: `GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build ./cmd/golovebox` ✓  
Binário: `golovebox.exe` ~13 MB (redução de 5 MB vs Fase 1 ao remover anthropic-sdk-go)

---

## Arquivos criados

| Arquivo | Função |
|---|---|
| `internal/llm/client.go` | HTTP client OpenAI-compatible, sem SDK |
| `internal/tools/shell.go` | Exec SSH na VM, output combinado |
| `internal/tools/files.go` | ReadFile / WriteFile / ListDir via SFTP |
| `internal/tools/github.go` | ListIssues / GetIssue / CloneRepo / OpenPR |
| `internal/agent/tools.go` | Registry de tools com descrições para system prompt |
| `internal/agent/loop.go` | ReAct loop (Thought/Action/Parameters, max 20 iter) |
| `internal/agent/planner.go` | PlanFromIssue: issue → task string para o loop |
| `internal/memory/memory.go` | Wrapper chromem-go (PersistentDB + Collection) |
| `internal/memory/embed.go` | Helper para EmbeddingFunc OpenAI-compat |

## Arquivos modificados

| Arquivo | Mudança |
|---|---|
| `internal/config/config.go` | Adicionado `LLMBaseURL`, `LLMModel`, `DefaultBaseURL()` |
| `internal/setup/init.go` | Wizard prompta agora BaseURL e Model |
| `cmd/golovebox/main.go` | Substituído `agent` stub por comando `github` completo |
| `internal/agent/agent.go` | Esvaziado (removida dep anthropic-sdk-go) |

---

## Decisões de Design

### LLM Client customizado (sem anthropic-sdk-go)
- `anthropic-sdk-go` removido — package era heavy e com typings que travam o provider lock-in
- Cliente próprio em `net/http`: POST `/v1/messages`, header `anthropic-version` condicional por URL
- Response struct unificada: `content[0].text` (Anthropic) + `choices[0].message.content` (OpenAI-compat) — zero código condicional por provider no parsing
- Resultado: qualquer provider OpenAI-compatible funciona mudando apenas BaseURL e Model no config

### ReAct Loop — formato texto simples
- Formato `Thought / Action / Parameters` em texto puro em vez de tool_use JSON nativo do Claude
- Motivo: portabilidade — funciona com qualquer LLM (OpenAI, Gemini, Ollama) sem nenhuma dependência de features específicas do provider
- Parser: `strings.Split` por linha, sem regex — simples e resistente a variações de whitespace
- Max 20 iterações; `Action: done` → resultado; `Action: error` → erro explícito

### Tools: conexão SSH por chamada
- Cada tool abre e fecha sua própria conexão SSH (via `sandbox.Dial`)
- Trade-off: overhead de handshake por chamada vs. simplicidade e ausência de state compartilhado
- Aceitável para Phase 2; Phase 3 pode introduzir pool de conexões

### Memory / Embeddings
- `chromem.NewPersistentDB` persiste embeddings em `.golovebox/memory/`
- Para Anthropic: `embFn = nil` → `NewEmbeddingFuncDefault()` do chromem-go (usa OPENAI_API_KEY env)
- Para OpenAI/Ollama: `NewEmbeddingFuncOpenAICompat(baseURL, apiKey, model)` com o mesmo endpoint do LLM
- Fallback: se `collection.Count() == 0`, `Search` retorna nil sem erro (agente ignora memória vazia)

### goclaw descartado (decisão da Fase 1)
- `github.com/sausheong/goclaw` v0.1.7: todos os packages são `internal/`, não importáveis como biblioteca
- Substituído por `anthropics/anthropic-sdk-go` na Fase 1, depois por HTTP client próprio na Fase 2
- Documentado em `CONTEXT.json`: `"why_not_goclaw": "application framework — all packages are internal/"`

---

## APIs de Libs Usadas

### chromem-go v0.7.0
```go
// Criação
chromem.NewPersistentDB(path string, compress bool) (*DB, error)
db.GetOrCreateCollection(name string, metadata map[string]string, embFn EmbeddingFunc) (*Collection, error)

// Embedding functions
chromem.NewEmbeddingFuncOpenAICompat(baseURL, apiKey, model string, normalized *bool) EmbeddingFunc
chromem.NewEmbeddingFuncDefault() EmbeddingFunc  // requer OPENAI_API_KEY

// Uso
collection.AddDocument(ctx, chromem.Document{ID: id, Content: text}) error
collection.Query(ctx, queryText string, nResults int, where, whereDocument map[string]string) ([]Result, error)
collection.Count() int
```

### go-github v60.0.0
```go
// Auth
github.NewClient(nil).WithAuthToken(token string) *Client

// Issues
client.Issues.ListByRepo(ctx, owner, repo string, opts *IssueListByRepoOptions) ([]*Issue, *Response, error)
client.Issues.Get(ctx, owner, repo string, number int) (*Issue, *Response, error)

// Pull Requests
client.PullRequests.Create(ctx, owner, repo string, pull *NewPullRequest) (*PullRequest, *Response, error)
// NewPullRequest{Title, Head, Base, Body *string}
// PullRequest.GetHTMLURL() string
```

---

## Limitações Conhecidas (Phase 2)

1. **SSH por tool call**: cada ferramenta abre nova conexão — lento para loops longos
2. **Embeddings Anthropic**: sem endpoint nativo; usa `NewEmbeddingFuncDefault()` que requer `OPENAI_API_KEY` em env
3. **Formato ReAct**: LLM pode não seguir o formato exato — parser tolera variações simples mas pode falhar em respostas mal formatadas
4. **CloneRepo**: token no URL de clone — ok para repos privados, mas deixa credencial em `git log --remotes`; Phase 3 deve usar `GIT_ASKPASS` ou deploy key
5. **Sem retry**: falhas de rede no LLM ou GitHub não são retentadas
6. **Sem streaming**: LLM responde completo antes de processar — timeout 120s pode ser curto para modelos lentos

---

## O que fica para Phase 3 (Gateway)

- [ ] Gateway Telegram: `go-telegram-bot-api/v5` — recebe `@golovebox issue #42`, retorna link da PR
- [ ] Gateway Slack: `slack-go/slack`
- [ ] Gateway Email: `emersion/go-imap`
- [ ] Pool de conexões SSH para o loop (evitar handshake por tool call)
- [ ] Retry com exponential backoff para LLM e GitHub API
- [ ] Streaming de progresso via Telegram durante o loop
- [ ] Deploy key para `git clone` (não colocar token no URL)
- [ ] Substituir `search_memory` por embedding de Anthropic quando disponível
- [ ] Comando `golovebox daemon` — escuta múltiplos canais simultaneamente
