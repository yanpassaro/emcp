# Servidor MCP Taiga

Servidor Model Context Protocol para [Taiga](https://www.taiga.io). Lê e
atualiza user stories (cards), issues, tasks, tags, comentários e anexos dos
projetos visíveis ao token configurado — as escritas só são liberadas com
`TAIGA_ALLOW_ACT`; sem ela o servidor é somente leitura. A saída é Markdown
organizado (com redação de PII ativada por padrão) em vez de JSON cru.

## Tools

| Tool | Descrição |
|---|---|
| `taiga_list` | Lista projetos, cards, issues, tasks ou atividade recente (`entity=project|card|issue|task|activity`) com filtros; `count_only` retorna só o total. Em `activity`, use `project_id` (opcional), `my=true` e `card_id`/`issue_id`/`task_id` para histórico de um item |
| `taiga_get` | Detalhe de um projeto (`entity=project` + id/slug), de um card/issue/task (`entity=card|issue|task` + id/ref) ou do seu usuário (`entity=info`: id, username, nome, e-mail, papéis e os projetos em que você participa) |
| `taiga_change` | Altera status, responsáveis e/ou tags de um card, issue ou task, e/ou publica um comentário (`entity` + `item_id` ou `ref`+`project_id`; `status_id`, `assigned_to`/`assigned_users`, `add_tags`, `remove_tags`, `comment`). Só é registrada quando `TAIGA_ALLOW_ACT` está ligado |
| `taiga_search` | Busca cards, issues e tasks por assunto/ref |
| `taiga_attachment` | Lista (`action=list`), baixa (`action=download`) ou envia (`action=upload`) anexos de um item; o upload exige `source_path` e aceita arquivos legíveis, exceto nomes/extensões de segredo, compactados e imagens de disco, e retorna a `URL` e o `Markdown` pronto (use `from_comment=true` para embutir no comentário) — indisponível sem `TAIGA_ALLOW_ACT` |

## Controle de ações

Sem `TAIGA_ALLOW_ACT`, o servidor sobe somente leitura:

- a tool `taiga_change` **não é registrada** — o modelo nem vê que ela existe;
- `taiga_attachment action=upload` é recusado, e a descrição da tool passa a
  anunciar apenas `list` e `download`;
- `taiga_list`, `taiga_get`, `taiga_search` e o download de anexos continuam
  normais.

Com `TAIGA_ALLOW_ACT` ligado (`true`, `1`, `yes` ou `on`), alterações,
comentários e upload ficam liberados.

A checagem também roda dentro dos handlers, então mesmo uma chamada direta
(fora da lista de tools) é recusada com
`conceda TAIGA_ALLOW_ACT para liberar escritas`.

## Comentário com imagem

O Taiga faz isso em dois passos. A `url` que a API devolve já vem pronta para o
comentário: ela termina em `#_taiga-refresh=<tipo>:<id>`, o marcador que o front
(`wysiwyg.service` → `refreshAttachmentURLFromMarkdown`) troca pela URL assinada
atual na hora de renderizar. É por isso que não se deve mexer nessa URL.

**1. subir a imagem como anexo de comentário:**

```json
{
  "entity": "card", "action": "upload",
  "object_id": 8542, "project_id": 174,
  "source_path": "task/523/evidence/evidence-1.png",
  "from_comment": true
}
```

A resposta traz a `URL` e o `Markdown para o comentário`, por exemplo:

```
![evidence-1.png](/media/.../evidence-1.png?token=...#_taiga-refresh=userstory:9)
```

**2. publicar o comentário com esse markdown:**

```json
{
  "entity": "card", "item_id": 8542,
  "comment": "Segue a evidência do bug. ![evidence-1.png](/media/.../evidence-1.png?token=...#_taiga-refresh=userstory:9)"
}
```

O markdown de referência de anexo (`![nome](url#_taiga-refresh=…)`) é ignorado
pelo filtro de conteúdo: sem isso, uma URL de mídia em host interno barraria o
próprio comentário que a referencia. O resto do texto é filtrado normalmente.

## Política de escrita

Comentários e uploads passam por um filtro no servidor (não no prompt) antes de
qualquer chamada ao Taiga. Conteúdo bloqueado retorna erro e nada é enviado.

Comentários e descrições de anexo são recusados quando contêm:

- termos ofensivos: lista embutida ampla em pt-BR/en, comparada por palavra
  inteira. A comparação ignora acento e caixa (`otário` = `otario`), leetspeak
  comum (`0`→`o`, `3`→`e`, `4`→`a`, `@`→`a`) e letras repetidas
  (`meeerda` = `merda`). Pode ser estendida por `TAIGA_BANNED_WORDS` (uma palavra
  por termo, separadas por vírgula);
- credenciais em texto (`senha=…`, `token: …`, `api_key=…`), chave privada,
  tokens de GitLab/GitHub/Slack/AWS, URL de banco com usuário e senha;
- PII: e-mail, CPF, CNPJ, telefone;
- IP interno ou hostname interno (`*.internal`, `*.local`, `*.lan`, `*.corp`);
- mais de 4000 caracteres.

Uploads aceitam qualquer arquivo legível, exceto tipos opacos que o filtro não
consegue inspecionar. São recusados quando:

- o nome é de segredo/configuração: `.env*`, `id_rsa`, `id_dsa`, `id_ecdsa`,
  `id_ed25519`, `credentials`, `secrets`, `secret`, `kubeconfig`, `.npmrc`,
  `.netrc`, `.pgpass`;
- a extensão é de segredo, credencial ou banco: chaves e certificados (`.pem`,
  `.key`, `.crt`, `.cert`, `.cer`, `.der`, `.csr`, `.p7b`, `.p7c`, `.p8`,
  `.pfx`, `.p12`, `.jks`, `.keystore`, `.bks`, `.ppk`), chaves PGP (`.gpg`,
  `.pgp`, `.asc`), cofres (`.kdb`, `.kdbx`, `.psafe3`, `.1pux`, `.1pif`),
  configuração com segredo (`.env`, `.envrc`, `.npmrc`, `.netrc`, `.pgpass`,
  `.ovpn`, `.kubeconfig`), estado de IaC (`.tfstate`, `.tfvars`), dumps de banco
  (`.dump`, `.dmp`, `.bak`, `.sqlite`, `.sqlite3`, `.db`, `.db3`, `.mdb`,
  `.accdb`, `.dbf`, `.mdf`, `.ldf`, `.frm`, `.ibd`, `.myd`, `.myi`, `.bson`,
  `.rdb`, `.aof`, `.ldb`, `.sst`) e capturas de tráfego (`.har`, `.pcap`,
  `.pcapng`, `.ldif`);
- é compactado, imagem de disco ou pacote: `.zip`, `.zipx`, `.z01`, `.rar`,
  `.r00`, `.7z`, `.tar`, `.gz`, `.tgz`, `.bz2`, `.tbz`, `.tbz2`, `.xz`,
  `.txz`, `.lz`, `.lz4`, `.lzma`, `.zst`, `.zstd`, `.br`, `.z`, `.ace`,
  `.arj`, `.lzh`, `.lha`, `.cab`, `.sit`, `.sitx`, `.cpio`, `.iso`, `.img`,
  `.dmg`, `.vhd`, `.vhdx`, `.vmdk`, `.ova`, `.jar`, `.war`, `.ear`, `.apk`,
  `.aab`, `.ipa`, `.deb`, `.rpm`, `.msi`, `.msp`, `.pkg`, `.appimage`,
  `.snap` — não há como abrir e conferir o conteúdo, então vale como fail-closed;
- excede `TAIGA_MAX_UPLOAD_MB` ou está vazio;
- os primeiros 64 KB contêm algum dos padrões de credencial/PII acima.

Executáveis e documentos comuns (`.exe`, `.pdf`, `.png`, `.xlsx`, `.docx`, …)
continuam liberados: barrar todo binário opaco tiraria o uso real do dia a dia. O
alvo são os formatos de "dump" — compactados, imagens de disco e pacotes — que
costumam carregar um volume grande de conteúdo sem passar pelo filtro.

Não há restrição de diretório: qualquer caminho acessível ao processo é aceito,
mas o arquivo é resolvido com `EvalSymlinks` antes da checagem de nome e
conteúdo — um symlink chamado `nota.txt` apontando para `id_rsa` é recusado.

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
| `TAIGA_MAX_UPLOAD_MB` | `10` | Limite de tamanho para enviar anexos |
| `TAIGA_BANNED_WORDS` | – | Palavras extras (uma por termo, separadas por vírgula) bloqueadas em comentários |
| `TAIGA_REDACT` | habilitado | Use `false`, `0`, `no` ou `off` para desativar a redação de PII |
| `TAIGA_ALLOW_ACT` | desligado | `true`, `1`, `yes` ou `on` libera escritas (alterações, comentários e upload); sem ela o servidor é somente leitura |

Para autenticar, é usado o primeiro disponível entre: `TAIGA_TOKEN`,
`TAIGA_REFRESH_TOKEN` ou `TAIGA_USERNAME` + `TAIGA_PASSWORD`.

Os logs ficam em `~/.local/share/mcp/taiga/logs` (ou seja,
`%USERPROFILE%\.local\share\mcp\taiga\logs` no Windows).
