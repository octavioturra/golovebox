# golovebox — Checklist de Fase

Verificar antes de declarar uma fase concluída.
Claude Code deve passar por este checklist antes de qualquer commit.

---

## Build

- [ ] `mage build` compila sem erro
- [ ] `mage build` com `CGO_ENABLED=0` explícito
- [ ] Sem warnings de `go vet`
- [ ] `mage check` passa (assets presentes e corretos)

## Código Go

- [ ] Zero `CGO_ENABLED=1` em qualquer arquivo
- [ ] Zero paths hardcoded (`/home/`, `C:\`, `~`, strings com `/` ou `\` como separador)
- [ ] Todos os paths via `filepath.Join`
- [ ] `baseDir` via `filepath.Dir(os.Executable())` — nunca `os.Getwd()`
- [ ] Zero escrita fora de `.golovebox/` (verificar todos os `os.Create`, `os.WriteFile`, `os.MkdirAll`)
- [ ] Sem `fmt.Fprintf(os.Stderr, ...)` para logging — usar `slog`
- [ ] Sem `fmt.Printf` para logging — usar `slog`
- [ ] `slog.Debug/Info/Warn/Error` com atributos estruturados (`"key", value`)

## Web / Chi

- [ ] Path params via `chi.URLParam(r, "param")` — nunca `r.PathValue()`
- [ ] Novas rotas registradas no sub-router correto em `server.go`
- [ ] Handlers novos com `middleware.Recoverer` cobrindo panics
- [ ] SSE handlers fecham conexão quando `r.Context()` cancela

## Frontend

- [ ] Sem manipulação direta de DOM onde Alpine pode fazer (`getElementById`, `innerHTML`)
- [ ] Novos estados no Alpine via `x-data` — sem variáveis globais soltas
- [ ] Comunicação entre componentes via `$dispatch` / `@evento.window`
- [ ] Canvas2D (Cytoscape) não re-renderiza o grafo inteiro por SSE event — usa `.data()`
- [ ] `localStorage` atualizado nos handlers de seleção

## Comportamento

- [ ] `golovebox init` completa sem erro em pasta limpa
- [ ] `golovebox init` completa sem erro em pasta com `.golovebox/` existente (idempotente)
- [ ] `golovebox web` sobe sem erro
- [ ] Health dots aparecem após `checkHealth()`
- [ ] SSE reconecta após reload da página (sessão restaurada do `localStorage`)
- [ ] Stop button cancela run ativo

## Regressão

- [ ] Fases anteriores não quebraram:
  - [ ] DAG renderiza e atualiza live
  - [ ] Node panel abre ao clicar node
  - [ ] Log ReAct aparece em tempo real
  - [ ] Task aparece no chat e no node panel
  - [ ] Erro de node aparece com mensagem clara (não apaga histórico)
  - [ ] Run anterior carrega ao reabrir a página

## Código Limpo

- [ ] Sem código comentado deixado para trás
- [ ] Sem `TODO` não documentado (se existir, registrar em Limitações Conhecidas da fase)
- [ ] Funções novas têm nome que descreve o que fazem (não o como)
- [ ] Sem duplicação óbvia — extrair função se mesmo bloco aparece 2+ vezes
