# Servidor MCP GitLab

Servidor Model Context Protocol para [GitLab](https://gitlab.com). Acesso
somente leitura à API REST do GitLab: busca, projetos, usuários, issues, merge
requests, pipelines, branches, commits, membros, tags, jobs de CI/CD e conteúdo
de arquivos de repositório. A saída é composta por tabelas Markdown organizadas.

## Tools

| Tool | Descrição |
|---|---|
| `gitlab_search` | Busca em vários escopos: projects, issues, merge_requests, milestones, users, commits, notes, wiki_blobs, blobs |
| `gitlab_list` | Lista recursos (`entity=projects|users|issues|merge_requests|pipelines|branches|commits|members|tags|jobs`) com filtros |
| `gitlab_get_item` | Item específico por `id` + `entity` (project|user|issue|merge_request|pipeline|branch|commit|member|tag|job) |
| `gitlab_get_file` | Conteúdo de um arquivo do repositório (arquivos acima de 200KB são truncados) |
| `gitlab_get_tree` | Árvore de arquivos do repositório (`path`/`ref`/`recursive`) |

## Build

```sh
cd ..
task build:gitlab      # -> ../dist/gitlab-mcp.exe
# ou direto:
go build -o bin/gitlab-mcp.exe ./cmd/gitlab-mcp
```

## Variáveis de ambiente

| Variável | Padrão | Descrição |
|---|---|---|
| `GITLAB_TOKEN` | – | Token de acesso pessoal (obrigatório) |
| `GITLAB_URL` | `https://gitlab.com` | URL base do GitLab |
| `GITLAB_HTTP_TIMEOUT_SECONDS` | `60` | Timeout do cliente HTTP |

Os logs ficam em `~/.local/share/mcp/gitlab/logs` (ou seja,
`%USERPROFILE%\.local\share\mcp\gitlab\logs` no Windows).
