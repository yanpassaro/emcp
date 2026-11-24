# Servidor MCP Taiga

Servidor Model Context Protocol para [Taiga](https://www.taiga.io). Lê e
atualiza user stories (cards), issues, tasks, tags, comentários e anexos dos
projetos visíveis ao token configurado. A saída é Markdown organizado (com
redação de PII ativada por padrão) em vez de JSON cru.

## Tools

| Tool | Descrição |
|---|---|
| `taiga_list` | Lista projetos, cards, issues, tasks ou atividade recente (`entity=project|card|issue|task|activity`) com filtros; `count_only` retorna só o total. Em `activity`, use `project_id` (opcional), `my=true` e `card_id`/`issue_id`/`task_id` para histórico de um item |
| `taiga_get` | Detalhe de um projeto (`entity=project` + id/slug), de um card/issue/task (`entity=card|issue|task` + id/ref) ou do seu usuário (`entity=info`: id, username, nome, e-mail, papéis e os projetos em que você participa) |
| `taiga_change` | Altera status, responsáveis e/ou tags de um card, issue ou task, e/ou publica um comentário (`entity` + `item_id` ou `ref`+`project_id`; `status_id`, `assigned_to`/`assigned_users`, `add_tags`, `remove_tags`, `comment`) |
| `taiga_search` | Busca cards, issues e tasks por assunto/ref |
| `taiga_attachment` | Lista (`action=list`), baixa (`action=download`) ou envia (`action=upload`) anexos de um item; o upload exige `source_path` dentro de `TAIGA_UPLOAD_DIRS` |

## Política de escrita

Comentários e uploads passam por um filtro no servidor (não no prompt) antes de
qualquer chamada ao Taiga. Conteúdo bloqueado retorna erro e nada é enviado.

Comentários e descrições de anexo são recusados quando contêm:

- termos ofensivos: lista embutida ampla em pt-BR/en, comparada por palavra inteira e
  insensível a acento e maiúsculas (`otário` = `otario`); pode ser estendida por
  `TAIGA_BANNED_WORDS` (vírgula) ou por uma wordlist em `TAIGA_BANNED_WORDS_FILE`
  (um termo por linha ou separado por vírgula, linhas com `#` são ignoradas). Se o
  arquivo configurado não puder ser lido, comentários e uploads ficam bloqueados;
- credenciais em texto (`senha=…`, `token: …`, `api_key=…`), chave privada,
  tokens de GitLab/GitHub/Slack/AWS, URL de banco com usuário e senha;
- PII: e-mail, CPF, CNPJ, telefone;
- IP interno ou hostname interno (`*.internal`, `*.local`, `*.lan`, `*.corp`);
- mais de 4000 caracteres.

Uploads são recusados quando:

- o arquivo resolvido (symlinks incluídos) está fora de `TAIGA_UPLOAD_DIRS`;
- o caminho ou o nome é oculto (começa com `.`);
- a extensão ou o nome é de segredo/banco: `.env`, `.pem`, `.key`, `.pfx`,
  `.p12`, `.kdbx`, `.ovpn`, `.sql`, `.dump`, `.bak`, `.sqlite`, `.db`, `.ppk`,
  `id_rsa`, `credentials`, `secrets`, `kubeconfig`, `.npmrc`, `.netrc`, `.pgpass`;
- é um arquivo compactado (`.zip`, `.7z`, `.rar`, `.tar`, `.gz`, …), a menos que
  `TAIGA_UPLOAD_ALLOW_ARCHIVES` esteja ligado;
- excede `TAIGA_MAX_UPLOAD_MB` ou está vazio;
- os primeiros 64 KB contêm algum dos padrões de credencial/PII acima.

Upload fica **desativado** enquanto `TAIGA_UPLOAD_DIRS` não for configurado.

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
| `TAIGA_UPLOAD_DIRS` | – | Diretórios (separados por vírgula) de onde o upload é permitido |
| `TAIGA_MAX_UPLOAD_MB` | `10` | Limite de tamanho para enviar anexos |
| `TAIGA_UPLOAD_ALLOW_ARCHIVES` | desligado | `true`, `1`, `yes` ou `on` libera arquivos compactados |
| `TAIGA_BANNED_WORDS` | – | Termos extras (separados por vírgula) bloqueados em comentários |
| `TAIGA_BANNED_WORDS_FILE` | – | Arquivo com termos extras (um por linha ou separado por vírgula); se não puder ser lido, a escrita fica bloqueada |
| `TAIGA_REDACT` | habilitado | Use `false`, `0`, `no` ou `off` para desativar a redação de PII |

Para autenticar, é usado o primeiro disponível entre: `TAIGA_TOKEN`,
`TAIGA_REFRESH_TOKEN` ou `TAIGA_USERNAME` + `TAIGA_PASSWORD`.

Os logs ficam em `~/.local/share/mcp/taiga/logs` (ou seja,
`%USERPROFILE%\.local\share\mcp\taiga\logs` no Windows).
