# winget Submission

Estes arquivos devem ser submetidos ao repositório `microsoft/winget-pkgs` após o primeiro release.

## Passos

1. Faça o primeiro release (`make release TAG=v0.1.0`) e aguarde o workflow terminar
2. Pegue o SHA256 do arquivo `.msi` gerado:
   ```sh
   curl -fsSL https://github.com/co2-lab/polvo/releases/download/v0.1.0/Polvo_0.1.0_x64_en-US.msi | sha256sum
   ```
3. Substitua `REPLACE_WITH_SHA256_OF_MSI_FILE` em `co2-lab.Polvo.installer.yaml` pelo hash em **MAIÚSCULAS**
4. Copie a pasta `manifests/` para o fork `co2-lab/winget-pkgs`
5. Abra PR para `microsoft/winget-pkgs` com o título:
   ```
   New package: co2-lab.Polvo version 0.1.0
   ```

A partir do segundo release o `winget-releaser` no workflow de release faz isso automaticamente.
