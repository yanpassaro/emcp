# Servidores MCP

Servidores Model Context Protocol (MCP) escritos em Go para ferramentas
internas: gestão de projetos (Taiga), registro de horas (Kimai) e hospedagem de
código (GitLab).

| Servidor | Módulo | Binário | Finalidade |
|---|---|---|---|
| [taiga](./taiga/README.md) | `ntdsk.com/taiga` | `taiga-mcp` | Lê/altera user stories, issues, tasks, tags e anexos do Taiga |
| [kimai](./kimai/README.md) | `ntdsk.com/kimai` | `kimai-mcp` | Registra horas no Kimai, lista projetos/atividades, visão de horário |
| [gitlab](./gitlab/README.md) | `ntdsk.com/gitlab` | `gitlab-mcp` | GitLab somente leitura: busca, issues, MRs, pipelines, arquivos |

## Layout

Todos os servidores seguem a mesma estrutura:

```text
server/
├── cmd/<server>-mcp/main.go
├── internal/
│   ├── <api>/client.go        # cliente HTTP da API
│   └── mcpserver/             # tools MCP, handlers e formatação
│       ├── server.go
│       └── format.go
├── go.mod
└── README.md
```

## Requisitos

- Go 1.25+ (dependência do Go SDK usada por todos os servidores)
- [Task](https://taskfile.dev) (opcional — só para o build compartilhado)

## Build

Na pasta raiz:

```sh
task              # gera todos os servidores como .exe em dist/
task build:taiga  # gera um único servidor
task clean        # remove a pasta dist/
```

Os binários ficam em `dist/`:
`dist/taiga-mcp.exe`, `dist/kimai-mcp.exe`, `dist/gitlab-mcp.exe`.

O build usa `windows/amd64` com `CGO_ENABLED=0` por padrão. Para gerar para
outra plataforma, sobrescreva as variáveis, ex.: `task build GOOS=linux GOARCH=arm64`.

## Build no GitHub Actions

O workflow [`.github/workflows/release.yml`](.github/workflows/release.yml) roda
**sem necessidade de tag** e publica os binários **direto numa Release**:

- em todo **push para `main`/`master`** (e manualmente pela aba *Actions*,
  `workflow_dispatch`), gera os binários **Windows** (`*.exe`) dos 3 servidores:
  `taiga-mcp.exe`, `kimai-mcp.exe`, `gitlab-mcp.exe`;
- publica/atualiza a **Release rolante `latest`** (a tag `latest` é criada e
gerenciada pelo próprio workflow) com os binários já anexados — é só baixar em
*Releases → Latest build*;
- como bônus, se uma tag `v*` for criada (ex.: `git tag v1.0.0 && git push --tags`),
  publica também uma **Release versionada** com os binários anexados.

## Logs

Todos os servidores gravam os logs em
`~/.local/share/mcp/<servidor>/logs/<servidor>-<data-hora>.log`
(no Windows: `%USERPROFILE%\.local\share\mcp\<servidor>\logs`). O Taiga também
salva os anexos baixados em `~/.local/share/mcp/taiga/downloads` por padrão.

## Configuração

Cada servidor lê suas configurações de variáveis de ambiente — veja o README de
cada servidor para a lista completa. Exemplo para o Zed (`mcp.json` no projeto
ou na pasta pessoal, ou em um arquivo de configuração do `.zed`):

```json
{
  "mcp": {
    "taiga": {
      "command": "C:\\tools\\mcp\\dist\\taiga-mcp.exe",
      "env": { "TAIGA_TOKEN": "seu-token" }
    },
    "kimai": {
      "command": "C:\\tools\\mcp\\dist\\kimai-mcp.exe",
      "env": { "KIMAI_URL": "https://kimai.exemplo.com", "KIMAI_TOKEN": "seu-token" }
    },
    "gitlab": {
      "command": "C:\\tools\\mcp\\dist\\gitlab-mcp.exe",
      "env": { "GITLAB_URL": "https://gitlab.exemplo.com", "GITLAB_TOKEN": "seu-token" }
    }
  }
}
```
