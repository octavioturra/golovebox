# golovebox — Guia de Debug

Modos de falha comuns, causa raiz e fix.

---

## VM / QEMU

### `golovebox init` trava no "aguardando SSH"

**Sintoma:** poll SSH fica em loop, nunca retorna `echo ok`.

**Causa mais comum:** cloud-init ainda rodando. No primeiro boot leva ~1m42s no Windows (TCG sem KVM).

**Fix:**
```
# verificar se a VM está viva
cat .golovebox/vm/qemu.log

# se aparecer "login:" a VM bootou mas SSH não está pronto ainda — aguardar
# se aparecer "Kernel panic" ou nada — problema no disco ou QEMU args
```

**Causa alternativa:** `base.img` corrompido ou incompleto (extração interrompida).
```
# verificar magic bytes do qcow2
xxd .golovebox/vm/base.img | head -1
# deve começar com: 51 46 49 fb (QFI\xfb)

# se corrompido: apagar e reinicializar
rm .golovebox/vm/base.img
golovebox init --repair
```

---

### SSH timeout após boot normal (não primeiro boot)

**Sintoma:** VM boota (aparece no qemu.log), mas `ssh -p 2222 root@localhost` recusa.

**Causa:** sshd não iniciou — cloud-init reescreveu `/etc/ssh/sshd_config` no segundo boot
(cidata.iso ainda presente) e reverteu `PermitRootLogin`.

**Fix:**
```
golovebox reset
# recria cidata.iso e reinicializa a VM
```

**Causa alternativa:** porta 2222 ocupada por outro processo.
```
netstat -ano | findstr :2222   # Windows
lsof -i :2222                  # Linux/macOS
```

---

### QEMU não inicia — "cannot find qemu-system-x86_64.exe"

**Causa:** extração do QEMU incompleta ou `mage build` rodou sem `mage fetch`.

**Fix:**
```
mage fetch
mage build
golovebox init --repair
```

**Verificar:**
```
dir .golovebox\qemu\qemu-system-x86_64.exe   # Windows
ls .golovebox/qemu/qemu-system-x86_64        # Linux/macOS
```

---

### VM inicia mas fica lenta demais

**Causa:** Windows sem KVM — QEMU usa TCG (emulação por software). Normal.
Linux com KVM é ~10x mais rápido.

**Não tem fix para Windows no V0.** É comportamento esperado.

---

## LLM

### Health check LLM fica vermelho

**Verificar em ordem:**

1. `config.toml` tem `llm_base_url` e `llm_model` corretos
2. API key válida (não expirada, tem saldo)
3. URL acessível da máquina (testar no browser ou curl)

```
# testar manualmente
curl -X POST https://api.anthropic.com/v1/messages \
  -H "x-api-key: SUA_KEY" \
  -H "anthropic-version: 2023-06-01" \
  -H "content-type: application/json" \
  -d '{"model":"claude-opus-4-5","max_tokens":1,"messages":[{"role":"user","content":"ping"}]}'
```

**Causa frequente:** `llm_base_url` com `/v1` no final duplicando o path.
Deve ser `https://api.anthropic.com`, não `https://api.anthropic.com/v1`.

---

### Agente fica em loop sem terminar

**Sintoma:** iterações avançam mas nunca chegam em `Action: done`.

**Causa:** model não está seguindo o formato ReAct (Thought/Action/Parameters).
Geralmente acontece com modelos menores ou mal configurados.

**Fix:** trocar para modelo mais capaz no `config.toml` (`llm_model`).

**Debug:** ver o log raw do node no painel da web. Se as linhas não seguem
`Thought:` / `Action:` / `Parameters:` / `Observation:`, o parser está falhando.

---

### `github_clone_repo` falha com "authentication failed"

**Causa:** GIT_ASKPASS script não foi criado ou a VM não encontra o binário.

**Verificar:**
```
# dentro da VM via exec
golovebox exec "ls /tmp/askpass-*.sh"
golovebox exec "cat /tmp/askpass-*.sh"
```

**Causa alternativa:** token GitHub sem permissão `repo` (só `read:user`).
Verificar em `github.com/settings/tokens`.

---

## Web UI

### DAG não aparece após iniciar run

**Sintoma:** run iniciou (aparece no chat), mas canvas fica vazio.

**Causa:** `GET /api/runs/{id}` retornou erro ou DAG com nodes vazio.

**Debug:**
```
# abrir DevTools → Network → filtrar /api/runs
# verificar resposta do endpoint
```

**Causa frequente:** orquestrador gerou JSON inválido (LLM retornou markdown em volta do JSON).
Ver log do servidor:
```
golovebox web --log-level debug
```

---

### SSE desconecta e não reconecta

**Sintoma:** log do node para de atualizar mid-run.

**Causa:** browser mata conexões SSE ociosas após ~30s por default no Chrome.

**Fix no servidor:** enviar comentário keepalive a cada 15s:
```go
// em server.go, no SSE handler
ticker := time.NewTicker(15 * time.Second)
defer ticker.Stop()
for {
    select {
    case <-ticker.C:
        fmt.Fprintf(w, ": keepalive\n\n")
        flusher.Flush()
    case event := <-ch:
        // ...
    }
}
```

---

### Health dots sempre cinza (não verificam)

**Causa:** `checkHealth()` não foi chamado. Verificar se Alpine.js carregou (CDN).

**Debug:** DevTools → Console → digitar `checkHealth()` manualmente.

**Causa alternativa:** CDN bloqueado por firewall corporativo.
```
# verificar no browser
fetch('https://cdn.jsdelivr.net/npm/alpinejs@3/dist/cdn.min.js')
```

---

### Página em branco após `golovebox web`

**Causa:** `embed.FS` não encontra `static/index.html`.

**Fix:** garantir que `mage build` foi rodado após mudanças no HTML.
O `go:embed` embute na compilação — mudanças no HTML exigem recompilação.

---

## Build

### `mage build` falha com "missing assets"

```
mage fetch   # baixa QEMU + Alpine qcow2
mage build   # compila com assets
```

### `mage fetch` falha no download do QEMU Windows

**Causa:** `qemu.weilnetz.de` fora do ar ou URL do installer mudou.

**Fix:** verificar manualmente em `https://qemu.weilnetz.de/w64/` o nome do .exe mais recente
e atualizar `parseLatestQEMUURL()` em `magefile.go`.

### Binário compila mas tem tamanho errado (< 100MB)

**Causa:** assets não foram embutidos — `go:embed` falhou silenciosamente
porque o diretório `internal/embed/assets/` estava vazio.

```
mage check   # vai reportar o problema
mage fetch
mage build
```

---

## Reset de emergência

Quando tudo falha e precisa começar do zero:

```bash
# Windows
rmdir /s /q .golovebox
golovebox init

# Linux/macOS
rm -rf .golovebox
./golovebox init
```

`golovebox reset --hard` faz o mesmo sem apagar `config.toml`.
