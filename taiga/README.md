# Servidor MCP Taiga

Servidor Model Context Protocol para [Taiga](https://www.taiga.io). Lê e
atualiza user stories (cards), issues, tasks, tags e anexos dos projetos
visíveis ao token configurado. A saída é Markdown organizado (com redação de
PII ativada por padrão) em vez de JSON cru.

## Tools

| Tool | Descrição |
|---|---|
| `taiga_list` | Lista projetos, cards, issues, tasks ou atividade recente (`entity=project|card|issue|task|activity`) com filtros; `count_only` retorna só o total. Em `activity`, use `project_id` (opcional), `my=true` e `card_id`/`issue_id`/`task_id` para histórico de um item |
| `taiga_get` | Detalhe de um projeto (`entity=project` + id/slug) ou de um card/issue/task (`entity=card|issue|task` + id/ref) |
| `taiga_change_status` | Altera o status de um card, issue ou task (`entity` + `status_id`) |
| `taiga_search` | Busca cards, issues e tasks por assunto/ref |
| `taiga_attachment` | Lista (`action=list`) ou baixa (`action=download`) anexos de um item |

## Build

```sh
cd ..
task build:taiga      # -> ../dist/taiga-mcp.exe
# ou direto:
go build -o bin/taiga-mcp.exe ./cmd/taiga-mcp
```

## Variáveis de ambiente

| Variável | Padrão | Descrição |
|---|---|---|
| `TAIGA_URL` | `https://api.taiga.io` | URL base da API do Taiga |
| `TAIGA_TOKEN` | – | Token de autenticação estático (Bearer) |
| `TAIGA_REFRESH_TOKEN` | – | Refresh token (renovado na inicialização) |
| `TAIGA_USERNAME` / `TAIGA_PASSWORD` | – | Login com usuário/senha (fallback) |
| `TAIGA_AUTH_SCHEME` | `Bearer` | Esquema de autenticação usado com o token |
| `TAIGA_HTTP_TIMEOUT_SECONDS` | `60` | Timeout do cliente HTTP |
| `TAIGA_MAX_DOWNLOAD_MB` | `100` | Limite de tamanho para baixar anexos |
| `TAIGA_DOWNLOAD_DIR` | `~/.local/share/mcp/taiga/downloads` | Onde os anexos são salvos |
| `TAIGA_REDACT` | habilitado | Use `false`, `0`, `no` ou `off` para desativar a redação de PII |

Para autenticar, é usado o primeiro disponível entre: `TAIGA_TOKEN`,
`TAIGA_REFRESH_TOKEN` ou `TAIGA_USERNAME` + `TAIGA_PASSWORD`.

Os logs ficam em `~/.local/share/mcp/taiga/logs` (ou seja,
`%USERPROFILE%\.local\share\mcp\taiga\logs` no Windows).
