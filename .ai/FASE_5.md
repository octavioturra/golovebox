# FASE 5 — Self-Contained Binary

## Objetivo

Tornar o golovebox completamente autossuficiente: QEMU binaries + Alpine ISO embutidos no binário Go via `//go:embed`. O comando `golovebox init` funciona offline, sem instalação manual de dependências.

## Arquivos Criados

| Arquivo | Descrição |
|---|---|
| `internal/embed/embed.go` | Declara `alpineAssets embed.FS` e `ReadAlpineISO()` |
| `internal/embed/embed_windows.go` | `//go:build windows` — embute `assets/qemu/windows-amd64/` |
| `internal/embed/embed_linux.go` | `//go:build linux` — embute `assets/qemu/linux-amd64/` |
| `internal/embed/embed_darwin.go` | `//go:build darwin` — embute `assets/qemu/darwin-arm64/` |
| `internal/embed/extract.go` | `ExtractQEMU`, `ExtractAlpineISO`, `QEMUExePath`, `QEMUImgPath` |
| `internal/embed/keygen.go` | `GenerateSSHKeypair` — RSA 4096, idempotente |
| `internal/embed/cloudinit.go` | `CreateCloudInitISO` via `kdomanski/iso9660` (pure Go) |
| `internal/embed/installboot.go` | `CreateDisk` + `RunInstallBoot` — QEMU com timeout 10 min |
| `internal/embed/assets/*/placeholder.txt` | Permite `go build` antes do `mage fetch` |
| `magefile.go` | Targets: Fetch, FetchAlpine, FetchWindows, Build, BuildWindows, Clean, Check |

## Arquivos Modificados

| Arquivo | Mudança |
|---|---|
| `internal/setup/init.go` | Reescrito: 9 steps usando `embedassets.*` — sem downloads em runtime |
| `internal/sandbox/qemu.go` | Deriva `QEMUExePath` do `embedassets` se `cfg.QEMUPath` está vazio |
| `.gitignore` | Exclui assets baixados, mantém `placeholder.txt` |
| `go.mod` | Adicionado `github.com/kdomanski/iso9660 v0.4.0` |

## Decisões de Design

### Placeholder Strategy
Go's `//go:embed` exclui arquivos iniciando com `.` ou `_`. Portanto, `.keep` não funciona. Usamos `placeholder.txt` (arquivo de texto simples) para garantir que o diretório seja incluído no embed FS mesmo antes do `mage fetch` rodar. O código detecta o placeholder e retorna `ErrAssetsNotFetched`.

### Nomenclatura do Package
O package é `embedassets` (não `embed`) para evitar conflito de nome com o stdlib `"embed"` que é importado dentro dos próprios arquivos do package.

### Estrutura de Diretórios do MSYS2 (Windows)
O pacote MSYS2 para QEMU usa o layout `mingw64/{bin,share,lib}/`. Após `--strip-components=1`:
- `bin/qemu-system-x86_64.exe` — executável principal
- `bin/qemu-img.exe` — ferramenta de imagem  
- `bin/*.dll` — dezenas de DLLs necessárias
- `share/qemu/` — firmware (bios-256k.bin, efi-virtio.rom, etc.)

Por isso `QEMUExePath` em Windows retorna `filepath.Join(qemuDir, "bin", "qemu-system-x86_64.exe")` e `RunInstallBoot` passa `-L filepath.Join(qemuDir, "share", "qemu")` quando esse diretório existe.

### Cloud-init NoCloud Datasource
O Alpine Virt ISO (3.19+) detecta automaticamente um disco rotulado `CIDATA` via cloud-init NoCloud datasource. Criamos um ISO9660 com:
- `meta-data`: `instance-id` e `local-hostname`
- `user-data`: `#cloud-config` com SSH key, pacotes e `poweroff` no runcmd

O `poweroff` no runcmd garante que o QEMU saia após a configuração. O argumento `-no-reboot` faz o QEMU terminar quando a VM desliga.

### Idempotência
Todas as funções são idempotentes:
- `ExtractQEMU` / `ExtractAlpineISO`: skip se tamanho coincide
- `GenerateSSHKeypair`: reusa par existente se ambos os arquivos existem
- `CreateDisk`: skip se `base.img > 100MB` (indica VM instalada)
- `RunInstallBoot`: skip se `base.img > 100MB` (mesma heurística)
- `step8Config`: skip se `config.toml` existe e `--repair` não foi passado

### Firmware QEMU
O QEMU compilado pelo MSYS2 tem o QEMU_DATADIR hardcoded para `/mingw64/share/qemu/`. Ao extrair o binário para `.golovebox/qemu/`, esse path deixa de existir. Solução: sempre passar `-L .golovebox/qemu/share/qemu` quando esse diretório existir. O código verifica via `os.Stat` antes de adicionar o argumento — em sistemas com QEMU do sistema (Linux dev), o diretório não existe e o argumento não é adicionado.

## Fluxo de Build

```bash
# Pré-requisito: instalar mage
go install github.com/magefile/mage@latest

# Baixar QEMU Windows + Alpine ISO
TARGET_OS=windows mage fetch

# Compilar binário Windows (~300MB com assets)
mage buildWindows

# Ou compilar para host atual
mage build
```

## Dependências Novas

| Pacote | Versão | Razão | CGO? |
|---|---|---|---|
| `github.com/kdomanski/iso9660` | v0.4.0 | Criar ISO9660 CIDATA para cloud-init | Pure Go ✓ |

## Limitações Conhecidas

1. **Windows QEMU Size**: O pacote MSYS2 inclui dezenas de DLLs (~100-200MB extraído). O binário final com todos os assets pode chegar a ~300MB. É aceitável para um "portable app" que elimina toda instalação.

2. **macOS QEMU**: `FetchQemu` para darwin não está automatizado. Requer cópia manual dos binários do Homebrew. O build para macOS é suportado mas não testado no pipeline.

3. **Linux QEMU**: Instruções manuais apenas — não há fonte universal para static builds. Em ambientes de dev Linux, usa-se o QEMU do sistema.

4. **Alpine ISO URL**: Hardcoded para Alpine 3.21.3. Atualizar `magefile.go` quando uma nova versão estiver disponível.

5. **MSYS2 Package Version**: Hardcoded para `qemu-9.2.2-1`. Verificar https://repo.msys2.org/mingw/mingw64/ para versões mais novas.

6. **tar com zstd**: `mage fetchWindows` requer GNU tar 1.31+ ou BSD tar no macOS 12+. Em sistemas mais antigos, instalar `zstd` separadamente e usar `tar -I zstd`.
