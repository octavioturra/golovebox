# FASE 17 — Bugfix Execução (git push, arquivos vazios, task rica)

## Status: ✅ Entregue

---

## Bug 1 — `git push` falha sempre por falta de credenciais persistentes

**Sintoma:** Node `push_to_remote` aborta com `git push: Process exited with status 2 — needs human review`. Token GitHub existe nos checks, mas `git` na VM não tem como autenticar.

**Root cause:** O agent gerou um node genérico tipo `task` (não `dag.TypePush`), que executa via shell tool — sem `GIT_ASKPASS`. As helpers `ExecPush`/`SyncRepo` injetam `GIT_ASKPASS` e removem o script logo após, então pushes feitos diretamente via `shell` ficavam sem autenticação.

**Fix:**

- **`internal/embed/cloudinit.go`** — `.gitconfig` ganhou `[credential] helper = store`. Cloud-init persiste no primeiro boot.
- **`internal/tools/git.go`** — nova helper `writeGitCredentials(client, token)` grava `/root/.git-credentials` (`https://x-token:<TOKEN>@github.com`, perm `0600` via `chmod`). Chamada automaticamente em `SyncRepo` após o clone.
- **`internal/tools/github.go`** — `CloneRepo` (caminho de issues) também grava as credenciais após o clone bem-sucedido.

Resultado: qualquer `git push/pull/fetch` posterior — seja via `ExecPush`, seja via o agent fazendo `git push` na shell — autentica transparentemente via o credential helper.

> Usuários com VM existente precisam de `golovebox reset && golovebox init` para o cloud-init reaplicar o `.gitconfig`.

Token nunca aparece em `ps`/history: escrita via SFTP (`sandbox.WriteFile`), não `echo` em shell.

---

## Bug 2 — Arquivos criados ficam vazios (só `<!DOCTYPE html>`)

**Sintoma:** Agent reporta sucesso em `write_file`, mas `cat /root/repo/index.html` mostra só a primeira linha.

**Root cause:** `parseAction` em [internal/agent/loop.go](internal/agent/loop.go) iterava linha-a-linha e fazia `SplitN(line, ":", 2)` — qualquer `content:` multilinha tinha só a 1ª linha capturada como valor; as linhas seguintes (sem `:`) eram descartadas, ou pior, viravam keys-fantasma sem valor.

**Fix:**

- **`internal/agent/loop.go`** — `parseAction` agora suporta heredoc:
  ```
  Parameters:
    path: /root/repo/index.html
    content: <<EOF
    <!DOCTYPE html>
    <html>...
    </html>
    EOF
  ```
  Quando `value` começa com `<<`, lê todas as linhas até encontrar o tag (`EOF` no exemplo) numa linha própria, com indent inicial de 2 espaços tolerado.
- **`internal/agent/loop.go`** — `systemPromptTemplate` documenta a sintaxe heredoc com exemplo explícito de HTML.

Compatibilidade: valores single-line (`key: value`) continuam funcionando idênticos.

---

## Bug 3 — Node tem `task` genérica sem o objetivo do usuário

**Sintoma:** Node `push_to_remote` tem `task: "Push the branch to the remote repository octavioturra/octavioturra"`. Nada do objetivo real do usuário ("Crie index.html com div preta centralizada") aparece no prompt que o agent recebe — então o agent dos nodes de desenvolvimento (`create_index_html`) tampouco tem contexto rico, e gera arquivos incompletos.

**Fix:**

- **`internal/orchestrator/orchestrator.go`** — `buildPlanningPrompt` ganhou regra explícita instruindo o LLM a tornar o `task` de cada node **auto-contido**, com restatement do objetivo do usuário + detalhes concretos (paths, conteúdos, estilo, comportamento). Exemplos BAD/GOOD direto no prompt:
  ```
  BAD:  "Push the branch to the remote repository"
  GOOD: "Push the current feature branch to origin. Context: this is part of
         delivering the user's request 'Crie /root/repo/index.html com...'."
  ```

Como o nó passa por `agent.Loop.Run(ctx, node.Task, progress)`, o agent agora recebe o prompt completo e — combinado com a fixagem do heredoc — consegue produzir HTML/CSS reais.

---

## Arquivos Modificados

| Arquivo | Mudança |
|---|---|
| `internal/embed/cloudinit.go` | `.gitconfig` ganha `[credential] helper = store` |
| `internal/tools/git.go` | Nova `writeGitCredentials()`; `SyncRepo` chama após clone |
| `internal/tools/github.go` | `CloneRepo` chama `writeGitCredentials` após clone bem-sucedido |
| `internal/agent/loop.go` | `parseAction` suporta heredoc `<<EOF…EOF`; `systemPromptTemplate` documenta a sintaxe |
| `internal/orchestrator/orchestrator.go` | Prompt instrui task auto-contida com BAD/GOOD examples |

## Arquivos Criados

Nenhum.

---

## Entregável

```bash
mage build
golovebox reset && golovebox init   # aplica .gitconfig novo
build\golovebox.exe serve
```

Cenários a validar:

1. **Git push funcional**: Submeter "Crie /root/octavioturra/index.html com div preta centralizada". Node de push completa; branch aparece no GitHub remoto.
2. **Arquivo real**: SSH na VM → `cat /root/octavioturra/index.html` mostra HTML completo (DOCTYPE + html + body + div + CSS), não só DOCTYPE.
3. **Task rica visível**: No painel direito do node, "Descrição do node" + bloco "Prompt enviado" (FASE 16) mostram o objetivo do usuário restated.
4. **Credentials persistentes**: SSH na VM → `cat /root/.git-credentials` mostra `https://x-token:...@github.com` (perm 0600). `cat /root/.gitconfig` contém `[credential]\n\thelper = store`.
5. **Backwards compat**: Parâmetros single-line antigos (`cmd: ls /tmp`) continuam funcionando sem heredoc.
