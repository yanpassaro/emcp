# Servidor MCP Kimai

Servidor Model Context Protocol para o registro de horas do
[Kimai](https://www.kimai.org). Inicia, interrompe e atualiza lançamentos de
tempo, lista projetos, atividades, clientes e tags, e mostra uma visão amigável
da hora atual e do horário fixo do usuário (padrões em pt_BR).

## Tools

| Tool | Descrição |
|---|---|
| `kimai_start_time_entry` | Inicia um lançamento (omita `end` para deixá-lo em andamento) |
| `kimai_get_time_entries` | Lista lançamentos com filtros/paginação; com `id` pega um lançamento específico; com `running=true` pega o que está em andamento |
| `kimai_time_entry` | Altera um lançamento: `action=stop` encerra, `action=update` edita campos, `action=delete` exclui |
| `kimai_list` | Lista projetos, clientes, atividades ou tags (`entity=projects|customers|activities|tags`) com filtros |
| `kimai_get_user_info` | Usuário atual + hora atual, horário de hoje com bloco ativo, horário fixo semanal e calendário |

## Build

```sh
cd ..
task build:kimai      # -> ../dist/kimai-mcp.exe
# ou direto:
go build -o bin/kimai-mcp.exe ./cmd/kimai-mcp
```

## Variáveis de ambiente

| Variável | Padrão | Descrição |
|---|---|---|
| `KIMAI_URL` | – | URL base do Kimai (obrigatório, ex.: `https://kimai.exemplo.com`) |
| `KIMAI_TOKEN` | – | Token da API (obrigatório) |
| `KIMAI_SCHEDULE` | segunda–sexta 09:00–18:00 | JSON que descreve o horário fixo |

### KIMAI_SCHEDULE

```json
{
  "timezone": "America/Sao_Paulo",
  "days": {
    "Monday": [
      { "start": "09:00", "end": "09:30", "label": "Daily" },
      { "start": "09:30", "end": "12:00", "label": "Manhã" },
      { "start": "13:00", "end": "18:00", "label": "Tarde" }
    ],
    "Default": [
      { "start": "09:00", "end": "18:00", "label": "Horário comercial" }
    ]
  }
}
```

As chaves são nomes dos dias da semana (`Sunday`–`Saturday`); um `Default`
opcional vale para qualquer dia não listado. O `label` é opcional. Um valor
vazio ou inválido cai no horário padrão (segunda–sexta, Daily 09:00 + blocos de
manhã/tarde, `America/Sao_Paulo`).

Os logs ficam em `~/.local/share/mcp/kimai/logs` (ou seja,
`%USERPROFILE%\.local\share\mcp\kimai\logs` no Windows).
