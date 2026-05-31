# FASE 6 — QEMU Extraction Fix

## Problema com a Fase 5

A Fase 5 usava pacotes MSYS2 (`.pkg.tar.zst`) para obter o QEMU Windows. Esses pacotes:
1. Exigem GNU tar com `--zstd` (não disponível em sistemas mais antigos)
2. Têm layout `mingw64/{bin,share,lib}/` — exige `--strip-components` e lógica complexa
3. **Não incluem as DLLs transitivas** — o pacote QEMU depende de dezenas de outros pacotes
   MSYS2 (libglib, libgcc, libstdc++, etc.) que precisariam ser baixados separadamente
4. O `bin/` como subdiretório criou inconsistência: `QEMUExePath` no Windows retornava
   `qemu/bin/qemu-system-x86_64.exe`, mas no Linux/macOS usava `qemu/qemu-system-x86_64`

## Solução

Usar o **installer oficial de Stefan Weil** (`qemu.weilnetz.de/w64/`):
- Inclui TUDO: qemu-system-x86_64.exe, qemu-img.exe, todas as DLLs, firmware
- Formato NSIS — extraível pelo 7-zip sem executar o installer (sem privilégios de admin)
- O 7-zip standalone (`7zr.exe` no Windows, `7za` no Linux/macOS) é baixado automaticamente
- SHA512 verificada via arquivo `.sha512` hospedado no mesmo servidor

## Arquivos Modificados

| Arquivo | Mudança |
|---|---|
| `magefile.go` | Reescrito: `FetchWindows` usa weilnetz.de + 7-zip; novos targets `FetchLinux`, `FetchDarwin`; `Check` valida tamanhos e contagem de DLLs |
| `internal/embed/extract.go` | Revertido: `QEMUExePath`/`QEMUImgPath` voltam à estrutura flat (sem `bin/`) |
| `internal/setup/init.go` | `step8Config`: QEMUPath flat (`qemu/qemu-system-x86_64.exe`, não `qemu/bin/...`) |
| `.gitignore` | Adicionados `build/tools/`, `build/tmp/`; removido `*.exe` global (conflitava com `7zr.exe` em tools/) |
| `README.md` | Seção Build atualizada: flow correto, tabela de targets, diagrama de assets |
| `.ai/FASE_6.md` | Este arquivo |

## Decisões de Design

### Layout flat no winQEMUDir
O installer weilnetz.de extrai para estrutura flat:
```
qemu-system-x86_64.exe  (root)
qemu-img.exe            (root)
*.dll                   (root, ~100 DLLs)
share/qemu/             (firmware)
```
Isso simplifica `QEMUExePath` — mesma lógica para todos os arquivos executáveis.

### 7-zip como build tool, não dependência
O 7-zip standalone (`7zr.exe` / `7za`) é:
- **Build-time only**: baixado por `mage fetch`, colocado em `build/tools/` (gitignored)
- **Zero impacto no binário final**: o `magefile.go` tem `//go:build mage` — não compila no binário
- **Auto-downloaded**: sem requisito de instalação manual

### parseLatestQEMUURL — descoberta dinâmica
Em vez de hardcodar versão, o magefile scrapa `https://qemu.weilnetz.de/w64/` e encontra o installer mais recente via regex `qemu-w64-setup-(\d{8})\.exe`. Ordenação por data (descending) garante o mais novo.

### SHA512 — não-fatal
Se o download do `.sha512` falhar (network issue, etc.), o mage continua com aviso. O HTTPS já fornece garantia mínima de integridade.

### FetchLinux — copy do sistema
QEMU Linux não tem builds estáticos confiáveis e universais. A abordagem pragmática: copiar os binários do sistema (`which qemu-system-x86_64`). O binário Linux resultante não é portátil entre distribuições, mas atende ao caso de desenvolvimento.

### FetchDarwin — brew --prefix
Em vez de fazer download do Homebrew bottle (que usa ghcr.io com autenticação complexa), o magefile usa `brew --prefix qemu` para encontrar os binários locais. Requer `brew install qemu` previamente.

## Limitações Conhecidas

1. **NSIS extraction**: 7-zip extrai instaladores NSIS, mas a estrutura exata depende de como o installer foi construído. Se weilnetz.de mudar o formato, `filterAndCopyQEMU` pode precisar de ajuste.

2. **DLL count check**: O `mage check` verifica `dlls < 50`. Versões futuras do QEMU podem ter mais ou menos DLLs — ajustar o threshold conforme necessário.

3. **macOS cross-compile**: Não é possível cross-compilar para macOS a partir de Linux. O `FetchDarwin` só funciona quando executado em macOS.

4. **7-zip URL**: O URL do 7-zip usa a versão `7z2409`. Quando 7-zip lançar nova versão, atualizar as constantes `sz7zLinURL` e `sz7zMacURL`.

5. **qemu.weilnetz.de**: Se o servidor ficar indisponível, `mage fetchWindows` falhará. Alternativa: espelhar o installer ou usar GitHub Actions para caching.

## Fluxo completo de build

```bash
# No Linux, para gerar golovebox.exe Windows:
go install github.com/magefile/mage@latest
mage fetchWindows   # 7za baixado, installer ~200MB, extrai ~100 DLLs
mage fetchAlpine    # Alpine ISO ~60MB
mage check          # verifica tudo
mage buildWindows   # → build/golovebox.exe (~300MB com assets)
```
