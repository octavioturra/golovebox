# FASE 7 — Alpine NoCloud + Boot Silencioso + Reset

## Problema com a Fase 5/6

O design das fases 5 e 6 assumiu que o **Alpine Virt ISO** rodaria cloud-init automaticamente ao detectar um disco com label `CIDATA`. Na prática isso **nunca funcionou**:

1. O Alpine Virt ISO é o **instalador live** — ele faz boot em modo "live" e mostra `localhost login:` esperando `setup-alpine` interativo
2. O pacote `cloud-init` está incluído na ISO desde 3.16, mas **não está habilitado no boot path** do live system
3. O CIDATA ficava montado mas era ignorado
4. O `RunInstallBoot` rodava o QEMU por 10 minutos até timeoutar, e o `step8Config` (que criava `config.toml`) nunca era alcançado

Além disso, o QEMU com `-nographic` mapeava o serial console para `stdio`, **tomando o terminal do usuário** durante todo o boot do Alpine — invadindo a CLI com kernel logs e prompts de login.

## Solução

Duas mudanças estruturais:

### 1. Trocar Alpine Virt ISO por Alpine NoCloud cloud image

A Alpine publica imagens cloud pré-instaladas em `https://dl-cdn.alpinelinux.org/alpine/v3.21/releases/cloud/`. A variante `nocloud_alpine-*-x86_64-bios-cloudinit-r0.qcow2`:

- É um **qcow2 pronto para boot** com Alpine 3.21.7 já instalado
- Tem cloud-init **habilitado em runlevels do OpenRC** (`init-local`, `init`, `config`, `final`)
- Detecta o datasource `NoCloud` em qualquer disco montável com label `CIDATA`
- BIOS variant (não UEFI) — funciona com SeaBIOS padrão do QEMU

Resultado: o disco baixado **é** o `base.img`. Não há mais "install boot" — boot direto.

### 2. Silenciar o QEMU sem perder logs

Em vez de `-nographic` (que = `-display none -serial mon:stdio`), redirecionar stdout/stderr/stdin de forma explícita:

```go
cmd.Stdout = logFile           // → vm/qemu.log
cmd.Stderr = logFile
cmd.Stdin  = os.DevNull        // getty vê EOF, não trava esperando input
```

E manter o `-nographic` para o QEMU produzir o serial em stdio (capturado pelo log file). O terminal do host fica intocado.

## Arquivos Modificados

| Arquivo | Mudança |
|---|---|
| `magefile.go` | `alpineISOURL` aponta para `nocloud_alpine-3.21.7-x86_64-bios-cloudinit-r0.qcow2`; `alpineISODest` muda para `alpine-cloud-x86_64.qcow2` |
| `internal/embed/embed.go` | `AlpineImageName` exportada; `ReadAlpineImage()` substitui `ReadAlpineISO()` |
| `internal/embed/extract.go` | `ExtractAlpineImage()` escreve o qcow2 direto em `vm/base.img`; valida qcow2 magic (`QFI\xfb`); idempotência via marker file `.alpine-image-size` (evita stale `base.img` de versões antigas) |
| `internal/embed/installboot.go` | `RunInstallBoot` e `CreateDisk` viram no-ops (a cloud image já é o disco instalado) |
| `internal/embed/cloudinit.go` | Removido `poweroff` (VM precisa ficar UP); adicionado `users:` com `lock_passwd: false`; **adicionado `write_files:` com `/etc/ssh/sshd_config.d/99-golovebox.conf` contendo `PermitRootLogin prohibit-password`** (a cloud image vem com `PermitRootLogin no`); `runcmd` faz `rc-service sshd restart` para o drop-in vigorar |
| `internal/sandbox/qemu.go` | Boot device explícito: `if=none,id=disk0` + `-device virtio-blk-pci,drive=disk0,bootindex=0` + `-boot order=c,menu=off`; anexa `cidata.iso` como segundo disco virtio (sem bootindex); valida magic do qcow2 antes do boot; redireciona stdout/stderr para `vm/qemu.log` e stdin para `os.DevNull`; QMP retry loop com `StartTimeout` parametrizável (default 15s) |
| `internal/setup/init.go` | Reordenado para 7 steps: **config interativa virou step 2** (config.toml gravado ANTES de qualquer trabalho pesado); removido `step5CreateDisk` e `step7InstallBoot`; smoke test polla SSH por até 5 min (cloud-init demora ~1m42s no first boot); detecta `config.toml` existente e oferece reuso (defaults entre `[colchetes]`) |
| `cmd/golovebox/main.go` | Novo comando `golovebox reset` (apaga `base.img`, marker, `cidata.iso`, logs) e `reset --hard` (apaga `.golovebox/` inteiro); ambos com confirmação interativa (`y/N` no soft, `yes` literal no hard); flag `-y/--yes` para CI; flag `--timeout` no `web` |

## Decisões de Design

### Config wizard primeiro (step 2)
A config interativa foi movida do step 8 para o step 2. Razão: se algum step posterior falhar, o usuário **não perde o input** das API keys e tokens. `config.toml` é gravado em disco antes do extract da imagem (~164MB) e antes do boot da VM.

### Idempotência por marker file, não por size
A check antiga `if fi.Size() == int64(len(data))` falhou silenciosamente quando havia um `base.img` de 8GB sparse qcow2 vazio de versões antigas — o `Size()` retornava algo diferente, mas a comparação não detectava que o **conteúdo** estava errado. O marker `.alpine-image-size` torna a idempotência explícita: ele só existe se a extração completou corretamente.

### Validação de qcow2 antes do boot
SeaBIOS retorna "could not read the boot disk" para qualquer disco inválido — impossível diagnosticar de dentro do QEMU. `sandbox.Start()` agora lê os 4 primeiros bytes do `base.img` e exige magic `QFI\xfb`, abortando com erro claro ("delete it and re-run init") antes mesmo de subir o QEMU.

### Boot device com bootindex explícito
A sintaxe antiga `-drive if=virtio,...` deixava o bootindex ambíguo, e o SeaBIOS às vezes tentava bootar pela rede (iPXE) ou pelo CIDATA antes do disco principal. A sintaxe moderna `if=none,id=disk0` + `-device virtio-blk-pci,drive=disk0,bootindex=0` força a ordem certa.

### sshd drop-in via cloud-init
A imagem cloud Alpine vem com `PermitRootLogin no`. Só instalar a authorized_key não basta — sshd recusa o root login mesmo com a key correta. Solução: usar `write_files:` do cloud-init para criar `/etc/ssh/sshd_config.d/99-golovebox.conf` com `PermitRootLogin prohibit-password` e `PubkeyAuthentication yes`, e fazer `rc-service sshd restart` no `runcmd`.

### Reset com confirmação
Comando destrutivo precisa de barreira. `reset` pede `[y/N]` (default não); `reset --hard` exige digitar `yes` literal. Flag `-y` pula a confirmação para automação. Listagem detalhada do que será apagado é mostrada antes do prompt.

### Reuso de config no init
`golovebox init` agora detecta `config.toml` existente e oferece reuso (default Yes). Se o usuário responder `n`, o wizard mostra os valores atuais entre `[colchetes]` como default de cada prompt — Enter mantém, valor novo substitui. Não é mais necessário lembrar a API key inteira para mudar só o modelo.

## Fluxo completo

```bash
# Setup inicial (uma vez)
mage clean
mage fetch          # baixa qemu (~80MB) + alpine cloud qcow2 (~164MB)
mage check          # valida assets
mage buildWindows   # → build/golovebox.exe (~450MB)

# No host destino
./golovebox.exe init    # config wizard + extract + boot + cloud-init (~3 min total)
./golovebox.exe web     # sobe VM + abre browser em localhost:8080

# Quando algo der errado com a VM
./golovebox.exe reset       # apaga base.img + cidata, init re-extrai
./golovebox.exe reset --hard -y  # apaga tudo, init refaz do zero
```

## Limitações Conhecidas

1. **Tamanho do binário**: subiu de ~300MB (com ISO) para ~450MB (com qcow2 cloud). Trade-off aceitável para eliminar a etapa de install (~10 min) no first boot.

2. **Versão Alpine hardcoded**: `magefile.go` aponta para 3.21.7-r0. Atualizar manualmente quando uma nova versão sair.

3. **TCG sem KVM no Windows**: o first boot da cloud image leva ~1m42s no Windows porque QEMU usa emulação software (TCG). Em Linux com KVM seria ~10s. Não há solução portable.

4. **Alpine cloud image só x86_64 BIOS**: para ARM ou UEFI teríamos que baixar a variante apropriada e ajustar `-bios`/`-machine`. Fora do escopo do V0.

5. **Cloud-init no second boot**: a cloud image marca cloud-init como "já rodou" via state em `/var/lib/cloud/`. Modificações no `cidata.iso` após o first boot **não são reaplicadas** — é preciso `golovebox reset` para forçar.

6. **Console serial via getty**: o `golovebox-vm login:` aparece no `vm/qemu.log` porque o getty no ttyS0 sempre roda. Não é erro, é só ruído visual no log.
