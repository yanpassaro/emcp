package mcpserver

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"ntdsk.com/kimai/internal/kimai"
)

var ptWeekday = map[time.Weekday]string{
	time.Sunday:    "domingo",
	time.Monday:    "segunda-feira",
	time.Tuesday:   "terça-feira",
	time.Wednesday: "quarta-feira",
	time.Thursday:  "quinta-feira",
	time.Friday:    "sexta-feira",
	time.Saturday:  "sábado",
}

var ptMonth = map[time.Month]string{
	time.January:   "janeiro",
	time.February:  "fevereiro",
	time.March:     "março",
	time.April:     "abril",
	time.May:       "maio",
	time.June:      "junho",
	time.July:      "julho",
	time.August:    "agosto",
	time.September: "setembro",
	time.October:   "outubro",
	time.November:  "novembro",
	time.December:  "dezembro",
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	r[0] = []rune(strings.ToUpper(string(r[0])))[0]
	return string(r)
}

func formatNiceDateTime(t time.Time) string {
	return fmt.Sprintf("%s, %d de %s de %d · %s",
		ptWeekday[t.Weekday()], t.Day(), ptMonth[t.Month()], t.Year(),
		t.Format("15:04:05"))
}

func formatMonthCalendar(t time.Time) string {
	loc := t.Location()
	first := time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, loc)
	last := first.AddDate(0, 1, -1)
	lead := int(first.Weekday())

	var cells []string
	for i := 0; i < lead; i++ {
		cells = append(cells, "   ")
	}
	for d := 1; d <= last.Day(); d++ {
		if d == t.Day() {
			cells = append(cells, fmt.Sprintf("%2d*", d))
		} else {
			cells = append(cells, fmt.Sprintf("%3d", d))
		}
	}
	for len(cells)%7 != 0 {
		cells = append(cells, "   ")
	}

	var b strings.Builder
	fmt.Fprintf(&b, "      %s %d\n", capitalize(ptMonth[t.Month()]), t.Year())
	b.WriteString("Dom Seg Ter Qua Qui Sex Sáb\n")
	for i := 0; i < len(cells); i += 7 {
		b.WriteString(strings.Join(cells[i:i+7], " "))
		b.WriteByte('\n')
	}
	return strings.TrimSpace(b.String())
}

func textResult(text string) (*mcp.CallToolResult, any, error) {
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: text}},
	}, nil, nil
}

func asMap(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}

func listField(v map[string]any, key string) []any {
	if l, ok := v[key].([]any); ok {
		return l
	}
	return nil
}

func displayBool(v any) bool {
	b, ok := v.(bool)
	if !ok {
		return false
	}
	return b
}

func yesNo(b bool) string {
	if b {
		return "**sim**"
	}
	return "não"
}

func displayInt(v any) int {
	switch t := v.(type) {
	case float64:
		return int(t)
	case int:
		return t
	case int64:
		return int(t)
	case string:
		n, _ := strconv.Atoi(strings.TrimSpace(t))
		return n
	default:
		return 0
	}
}

func displayString(v any) string {
	s, ok := v.(string)
	if !ok || strings.TrimSpace(s) == "" {
		return "-"
	}
	return s
}

func stringOrEmpty(v any) string {
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return s
}

func displayValue(v any) string {
	switch t := v.(type) {
	case nil:
		return "-"
	case string:
		if strings.TrimSpace(t) == "" {
			return "-"
		}
		return t
	case bool:
		return yesNo(t)
	case float64:
		if t == float64(int(t)) {
			return strconv.Itoa(int(t))
		}
		return strconv.FormatFloat(t, 'f', 2, 64)
	case int:
		return strconv.Itoa(t)
	case int64:
		return strconv.FormatInt(t, 10)
	default:
		return fmt.Sprintf("%v", t)
	}
}

func formatInt(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	s := strconv.Itoa(n)
	var b strings.Builder
	for i, r := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteRune('.')
		}
		b.WriteRune(r)
	}
	if neg {
		return "-" + b.String()
	}
	return b.String()
}

func formatSeconds(secs int) string {
	if secs <= 0 {
		return "0m"
	}
	h := secs / 3600
	m := (secs % 3600) / 60
	if h > 0 {
		return fmt.Sprintf("%dh%02dm", h, m)
	}
	return fmt.Sprintf("%dm", m)
}

func parseKimaiTime(value string) (time.Time, bool) {
	layouts := []string{
		"2006-01-02T15:04:05-0700",
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05",
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, value); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

func entrySeconds(te map[string]any, now time.Time) int {
	if secs := displayInt(te["duration"]); secs > 0 {
		return secs
	}
	start, ok := parseKimaiTime(stringOrEmpty(te["begin"]))
	if !ok {
		return 0
	}
	if endStr := stringOrEmpty(te["end"]); endStr != "" {
		if end, ok := parseKimaiTime(endStr); ok {
			if d := int(end.Sub(start).Seconds()); d > 0 {
				return d
			}
		}
		return 0
	}
	if d := int(now.Sub(start).Seconds()); d > 0 {
		return d
	}
	return 0
}

func formatDuration(te map[string]any) string {
	return formatSeconds(entrySeconds(te, time.Now()))
}

func tableCell(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "|", "\\|")
	return strings.TrimSpace(s)
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "-"
	}
	if i := strings.IndexAny(s, "\n\r"); i >= 0 {
		s = s[:i]
	}
	if len(s) > 80 {
		s = s[:77] + "..."
	}
	return s
}

func inlineList(items []any) string {
	if len(items) == 0 {
		return "-"
	}
	parts := make([]string, 0, len(items))
	for _, it := range items {
		parts = append(parts, displayValue(it))
	}
	return strings.Join(parts, " ")
}

func formatPagination(meta kimai.PaginationMeta) string {
	if meta.TotalCount == 0 && meta.CurrentPage == 0 {
		return ""
	}
	var parts []string
	if meta.TotalCount > 0 {
		parts = append(parts, "total de "+formatInt(meta.TotalCount)+" registros")
	}
	if meta.CurrentPage > 0 {
		p := "página " + formatInt(meta.CurrentPage)
		if meta.PageSize > 0 {
			p += " (" + formatInt(meta.PageSize) + " por página)"
		}
		parts = append(parts, p)
	}
	if meta.TotalPages > 0 {
		parts = append(parts, formatInt(meta.TotalPages)+" páginas no total")
	}
	if len(parts) == 0 {
		return ""
	}
	return "📄 Paginação: " + strings.Join(parts, ", ") + "."
}

func formatTimeEntriesTable(items []any, meta kimai.PaginationMeta) string {
	if len(items) == 0 {
		return "Nenhuma marcação de tempo encontrada."
	}
	var b strings.Builder
	b.WriteString("| ID | Início | Fim | Duração | Projeto | Atividade | Faturável | Tags | Descrição |\n")
	b.WriteString("|----|--------|-----|----------|---------|-----------|-----------|------|------------|\n")
	for _, item := range items {
		te := asMap(item)
		if te == nil {
			continue
		}
		end := "-"
		if e, ok := te["end"].(string); ok && e != "" {
			end = e
		}
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %s | %s | %s | %s |\n",
			displayValue(te["id"]),
			displayValue(te["begin"]),
			end,
			formatDuration(te),
			displayValue(te["project"]),
			displayValue(te["activity"]),
			yesNo(displayBool(te["billable"])),
			inlineList(listField(te, "tags")),
			tableCell(firstLine(displayString(te["description"]))),
		)
	}
	if text := formatPagination(meta); text != "" {
		fmt.Fprintf(&b, "\n%s\n", text)
	}
	return strings.TrimSpace(b.String())
}

func formatTimeEntry(item any) string {
	te := asMap(item)
	if te == nil {
		return "Marcação de tempo não encontrada."
	}
	var b strings.Builder
	fmt.Fprintf(&b, "⏱️ **Marcação de tempo #%s**\n\n", displayValue(te["id"]))
	fmt.Fprintf(&b, "- **Projeto:** %s\n", displayValue(te["project"]))
	fmt.Fprintf(&b, "- **Atividade:** %s\n", displayValue(te["activity"]))
	fmt.Fprintf(&b, "- **Início:** %s\n", displayValue(te["begin"]))
	if e, ok := te["end"].(string); ok && e != "" {
		fmt.Fprintf(&b, "- **Fim:** %s\n", e)
	}
	secs := entrySeconds(te, time.Now())
	fmt.Fprintf(&b, "- **Duração:** %s (%d s)\n", formatSeconds(secs), secs)
	fmt.Fprintf(&b, "- **Faturável:** %s\n", yesNo(displayBool(te["billable"])))
	fmt.Fprintf(&b, "- **Exportado:** %s\n", yesNo(displayBool(te["exported"])))
	if r, ok := te["rate"].(float64); ok && r != 0 {
		fmt.Fprintf(&b, "- **Rate:** %s\n", displayValue(te["rate"]))
	}
	fmt.Fprintf(&b, "- **Tags:** %s\n", inlineList(listField(te, "tags")))
	if d, ok := te["description"].(string); ok && d != "" {
		fmt.Fprintf(&b, "\n**Descrição:**\n%s\n", d)
	}
	return strings.TrimSpace(b.String())
}

func displayCustomer(v any, names map[int]string) string {
	if m, ok := v.(map[string]any); ok {
		if n := stringOrEmpty(m["name"]); n != "" {
			return tableCell(n)
		}
		return displayValue(m["id"])
	}
	if name, ok := names[displayInt(v)]; ok {
		return tableCell(name)
	}
	return displayValue(v)
}

func formatProjectsTable(items []any, meta kimai.PaginationMeta, customerNames map[int]string) string {
	if len(items) == 0 {
		return "Nenhum projeto encontrado."
	}
	var b strings.Builder
	b.WriteString("| ID | Nome | Cliente | Visível | Faturável | Número |\n")
	b.WriteString("|----|------|---------|---------|-----------|--------|\n")
	for _, item := range items {
		p := asMap(item)
		if p == nil {
			continue
		}
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %s |\n",
			displayValue(p["id"]),
			tableCell(displayString(p["name"])),
			displayCustomer(p["customer"], customerNames),
			yesNo(displayBool(p["visible"])),
			yesNo(displayBool(p["billable"])),
			tableCell(displayString(p["number"])),
		)
	}
	if text := formatPagination(meta); text != "" {
		fmt.Fprintf(&b, "\n%s\n", text)
	}
	return strings.TrimSpace(b.String())
}

func formatActivitiesTable(items []any, meta kimai.PaginationMeta) string {
	if len(items) == 0 {
		return "Nenhuma atividade encontrada."
	}
	var b strings.Builder
	b.WriteString("| ID | Nome | Projeto | Visível | Faturável | Número |\n")
	b.WriteString("|----|------|---------|---------|-----------|--------|\n")
	for _, item := range items {
		a := asMap(item)
		if a == nil {
			continue
		}
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %s |\n",
			displayValue(a["id"]),
			tableCell(displayString(a["name"])),
			displayValue(a["project"]),
			yesNo(displayBool(a["visible"])),
			yesNo(displayBool(a["billable"])),
			tableCell(displayString(a["number"])),
		)
	}
	if text := formatPagination(meta); text != "" {
		fmt.Fprintf(&b, "\n%s\n", text)
	}
	return strings.TrimSpace(b.String())
}

func formatCustomersTable(items []any, meta kimai.PaginationMeta) string {
	if len(items) == 0 {
		return "Nenhum cliente encontrado."
	}
	var b strings.Builder
	b.WriteString("| ID | Nome | Empresa | Visível | Faturável | Email |\n")
	b.WriteString("|----|------|---------|---------|-----------|-------|\n")
	for _, item := range items {
		c := asMap(item)
		if c == nil {
			continue
		}
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %s |\n",
			displayValue(c["id"]),
			tableCell(displayString(c["name"])),
			tableCell(displayString(c["company"])),
			yesNo(displayBool(c["visible"])),
			yesNo(displayBool(c["billable"])),
			tableCell(displayString(c["email"])),
		)
	}
	if text := formatPagination(meta); text != "" {
		fmt.Fprintf(&b, "\n%s\n", text)
	}
	return strings.TrimSpace(b.String())
}

func formatUser(item any) string {
	u := asMap(item)
	if u == nil {
		return "Usuário não encontrado."
	}
	var b strings.Builder
	fmt.Fprintf(&b, "👤 **Usuário #%s: %s**\n\n", displayValue(u["id"]), tableCell(displayString(u["username"])))
	if v := stringOrEmpty(u["alias"]); v != "" {
		fmt.Fprintf(&b, "- **Nome:** %s\n", tableCell(v))
	}
	if v := stringOrEmpty(u["email"]); v != "" {
		fmt.Fprintf(&b, "- **Email:** %s\n", tableCell(v))
	}
	if v := stringOrEmpty(u["title"]); v != "" {
		fmt.Fprintf(&b, "- **Título:** %s\n", tableCell(v))
	}
	fmt.Fprintf(&b, "- **Ativo:** %s\n", yesNo(displayBool(u["enabled"])))
	if v := stringOrEmpty(u["timezone"]); v != "" {
		fmt.Fprintf(&b, "- **Timezone:** %s\n", v)
	}
	if v := stringOrEmpty(u["language"]); v != "" {
		fmt.Fprintf(&b, "- **Idioma:** %s\n", v)
	}
	return strings.TrimSpace(b.String())
}

func formatTags(data any) string {
	items, ok := data.([]any)
	if !ok || len(items) == 0 {
		return "Nenhuma tag encontrada."
	}
	var b strings.Builder
	fmt.Fprintf(&b, "**Tags** (%d):\n", len(items))
	for _, t := range items {
		switch v := t.(type) {
		case string:
			fmt.Fprintf(&b, "- %s\n", tableCell(v))
		case map[string]any:
			fmt.Fprintf(&b, "- #%s %s\n", displayValue(v["id"]), tableCell(displayString(v["name"])))
		default:
			fmt.Fprintf(&b, "- %s\n", displayValue(t))
		}
	}
	return strings.TrimSpace(b.String())
}

func formatCurrentTimeView(now time.Time, s *Schedule) string {
	loc := s.location()
	nowInLoc := now.In(loc)

	var b strings.Builder
	fmt.Fprintf(&b, "🕒 **Agora:** %s (%s)\n", formatNiceDateTime(nowInLoc), loc.String())

	fmt.Fprintf(&b, "\n📅 **Seu horário hoje (%s):**\n", capitalize(ptWeekday[nowInLoc.Weekday()]))
	blocks := s.BlocksFor(nowInLoc)
	if len(blocks) == 0 {
		b.WriteString("Nenhum bloco de horário fixo para hoje.\n")
	} else {
		for _, blk := range blocks {
			startT, ok1 := blockTime(nowInLoc, blk.Start)
			endT, ok2 := blockTime(nowInLoc, blk.End)
			status := ""
			if ok1 && ok2 {
				switch {
				case nowInLoc.Before(startT):
					status = "⏳ à frente"
				case nowInLoc.After(endT):
					status = "✅ concluído"
				default:
					status = "🔵 em andamento"
				}
			}
			name := blk.Label
			if name == "" {
				name = "—"
			}
			fmt.Fprintf(&b, "- %s–%s **%s** %s\n", blk.Start, blk.End, name, status)
		}
	}

	b.WriteString("\n🗓️ **Calendário (mês atual):**\n")
	b.WriteString(formatMonthCalendar(nowInLoc))
	b.WriteByte('\n')

	b.WriteString("\n")
	b.WriteString(formatScheduleSection(s))
	return strings.TrimSpace(b.String())
}
