# golovebox — Exemplos de Uso

Histórias reais de como o golovebox funciona na prática.

---

## 1. Construindo um sistema do zero

Você tem um sistema de empacotamento seguro de arquivos para construir.
Escreve um arquivo por área:

**`spec_ui.md`**
```
Construa três interfaces: admin, enviador e recebedor.
A UI do admin deve listar todos os pacotes enviados com status e metadados.
A UI do enviador deve permitir upload, seleção de destinatário e criptografia do pacote.
A UI do recebedor deve mostrar pacotes recebidos e permitir download com descriptografia.

ATTENTION_HERE: revisar fluxo de UX com o time antes de implementar.
NOT_TODO: internacionalização — registrar como débito técnico.
```

**`spec_security.md`**
```
Implemente a camada de segurança com criptografia AES-256 para os pacotes.
Gestão de chaves via envelope encryption — chave do pacote criptografada com chave do destinatário.
NO_RETRY se a lib de crypto falhar durante testes — parar e notificar imediatamente.

TRY integração com HSM OR_ELSE armazenamento local de chaves em keystore criptografado.

ATTENTION_HERE: esta camada precisa de revisão de segurança antes do deploy.
NOTIFY_ME quando os testes de segurança passarem.
```

**`spec_dx.md`**
```
Testes unitários para todas as funções de criptografia.
Testes e2e cobrindo o fluxo completo: upload → criptografia → envio → recepção → descriptografia.
RUN_TEST após cada módulo concluído — não avançar com testes quebrando.
Documentação da API gerada automaticamente.
Revisão de vulnerabilidades OWASP top 10.

NOTIFY_ME quando a cobertura de testes ultrapassar 80%.
```

Você roda:
```
golovebox run specs/
```

O orquestrador lê os três arquivos, gera um DAG com 23 nodes, abre o web chat.
Você vê a árvore nascendo. Três sub-agentes trabalhando em paralelo onde possível.
Quando chega no `ATTENTION_HERE` da UI, o node fica laranja — você revisa no canvas, aprova.
Quando os testes de segurança passam, você recebe notificação e o agente avança.

---

## 2. Checkpoint de revisão humana

Você está construindo os termos de uso do sistema.
O agente termina de gerar o documento legal e para:

```
⏸ ATTENTION_HERE — "spec_legal.md" linha 12
"Quando os termos estiverem redigidos, enviar para o Jurandir validar antes de prosseguir."

Aguardando aprovação para continuar.
```

No web chat você vê o documento gerado. Você copia, manda pro Jurandir.
Quando ele responde, você volta no chat e digita `aprovado` — o DAG continua.

---

## 3. TRY / OR_ELSE em ação

O spec dizia:
```
TRY implementar autenticação OAuth com Google OR_ELSE fallback para JWT local.
```

O agente tenta OAuth. A lib tem um conflito de versão na VM.
Sem intervenção sua, ele executa o OR_ELSE: implementa JWT local, registra no log:

```
[node: auth_layer] TRY falhou: oauth2 lib conflict
[node: auth_layer] Executando OR_ELSE: JWT local
[node: auth_layer] ✅ JWT implementado, testes passando
```

---

## 4. Gate de testes

O spec tinha:
```
RUN_TEST após implementar cada módulo de criptografia.
```

O agente implementa o módulo de chaves. Roda os testes. Dois falham.
Ele não avança — entra em loop de correção até verde:

```
[node: crypto_keys] RUN_TEST → 2 falhas
[node: crypto_keys] Corrigindo: key derivation function incorreta
[node: crypto_keys] RUN_TEST → passou ✅
[node: crypto_server] iniciando...
```

---

## 5. Deploy + notificação

O spec de infraestrutura dizia:
```
Quando o módulo de servidor estiver testado e aprovado, fazer deploy no ambiente de staging.
O script de deploy está em scripts/deploy-staging.sh.
NOTIFY_ME quando o deploy terminar com a URL do ambiente.
```

O agente termina os testes, executa o script de deploy dentro da VM, e te manda:

```
✅ Deploy concluído
Ambiente: https://staging.seuapp.com
Build: #47 — 3m12s
Próximo node: revisão de segurança
```

---

## 6. Integração com mundo externo

Spec com triggers externos:
```
Quando chegar email do Jurandir com assunto "termos aprovados",
embuta os termos na tela de aceite e faça deploy.

WHEN email from jurandir@empresa.com subject "termos aprovados"
DO embed_terms AND deploy_production
```

O agente fica em `wait_event`. Quando o email chega, o node dispara automaticamente.
Você não precisou fazer nada — só escrever a intenção.

---

## 7. Débitos técnicos automáticos

Durante a execução, o agente encontra vários `NOT_TODO` nos specs:
```
NOT_TODO: internacionalização
NOT_TODO: dark mode
NOT_TODO: exportação para PDF
```

Ao final da run, o relatório inclui:

```
📋 Débitos técnicos registrados:
- [ ] Internacionalização (spec_ui.md:34)
- [ ] Dark mode (spec_ui.md:67)  
- [ ] Exportação para PDF (spec_ui.md:89)

Arquivo gerado: .golovebox/runs/run-042/tech_debt.md
```

---

## 8. Retomada após interrupção

Você estava rodando uma build longa. Acabou a luz.
Na manhã seguinte:

```
golovebox resume run-042
```

O DAG retoma do último checkpoint salvo. Nodes já concluídos não reexecutam.
O agente continua de onde parou.

---

## 9. Skill especializada

Você trabalha muito com APIs REST. Cria uma skill:

**`.golovebox/skills/api_review.md`**
```
---
name: api_review
description: Revisa uma API REST para consistência, boas práticas e segurança
tools: [read_file, shell, search_memory]
---

Revise os endpoints da API verificando:
- Nomenclatura consistente (substantivos, plural, versionamento)
- Códigos HTTP corretos por operação
- Autenticação e autorização em todas as rotas protegidas
- Rate limiting documentado
- Tratamento de erros padronizado
```

No próximo spec você escreve simplesmente:
```
Revise a API do módulo de upload usando api_review.
```

O agente encontra a skill, sabe exatamente como proceder.

---

## 10. Visão geral de uma run

```
golovebox status run-042

Run: run-042
Iniciada: 2h 34m atrás
Specs: 8 arquivos
Nodes: 31 total

✅ Concluídos: 18
🔄 Em execução: 3 (paralelo)
⏸ Aguardando: 1 (ATTENTION_HERE — revisão de segurança)
⏳ Pendentes: 9

Agentes ativos:
  → agent-frontend: implementando tela de recebedor
  → agent-backend: escrevendo testes de integração  
  → agent-security: em revisão

Próximo checkpoint: RUN_TEST completo (estimado: ~12 min)
```
