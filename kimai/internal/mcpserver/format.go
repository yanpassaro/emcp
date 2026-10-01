package mcpserver

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"ntdsk.com/kimai/internal/kimai"
)

const (
	SECONDS_HOUR    = 3600
	SECONDS_MINUTE  = 60
	DASH            = "-"
	MAX_LINE_LENGTH = 80
	ELLIPSIS        = "..."
	CALENDAR_COLS   = 7
)

func ptWeekday() map[time.Weekday]string {
	return map[time.Weekday]string{
		time.Sunday:    "domingo",
		time.Monday:    "segunda-feira",
		time.Tuesday:   "terça-feira",
		time.Wednesday: "quarta-feira",
		time.Thursday:  "quinta-feira",
		time.Friday:    "sexta-feira",
		time.Saturday:  "sábado",
	}
}

func ptMonth() map[time.Month]string {
	return map[time.Month]string{
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
		ptWeekday()[t.Weekday()], t.Day(), ptMonth()[t.Month()], t.Year(),
		t.Format("15:04:05"))
}

func monthCells(t time.Time) []string {
	first := time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, t.Location())
	last := first.AddDate(0, 1, -1)

	cells := []string{}
	lead := int(first.Weekday())
	for i := 0; i < lead; i++ {
		cells = append(cells, "   ")
	}

	for d := 1; d <= last.Day(); d++ {
		if d == t.Day() {
			cells = append(cells, fmt.Sprintf("%2d*", d))
		}
		if d != t.Day() {
			cells = append(cells, fmt.Sprintf("%3d", d))
		}
	}

	for len(cells)%CALENDAR_COLS != 0 {
		cells = append(cells, "   ")
	}

	return cells
}

func formatMonthCalendar(t time.Time) string {
	cells := monthCells(t)

	b := strings.Builder{}
	fmt.Fprintf(&b, "      %s %d\n", capitalize(ptMonth()[t.Month()]), t.Year())
	b.WriteString("Dom Seg Ter Qua Qui Sex Sáb\n")

	for i := 0; i < len(cells); i += CALENDAR_COLS {
		b.WriteString(strings.Join(cells[i:i+CALENDAR_COLS], " "))
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
	m, ok := v.(map[string]any)
	if !ok {
		return nil
	}
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
		n, err := strconv.Atoi(strings.TrimSpace(t))
		if err == nil {
			return n
		}
		return 0
	default:
		return 0
	}
}

func displayString(v any) string {
	s := stringOrEmpty(v)
	if strings.TrimSpace(s) == "" {
		return DASH
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
		return DASH
	case string:
		if strings.TrimSpace(t) == "" {
			return DASH
		}
		return t
	case bool:
		return yesNo(t)
	case float64:
		return floatText(t)
	case int:
		return strconv.Itoa(t)
	case int64:
		return strconv.FormatInt(t, 10)
	default:
		return fmt.Sprintf("%v", t)
	}
}

func floatText(t float64) string {
	if t == float64(int64(t)) {
		return strconv.Itoa(int(t))
	}
	return strconv.FormatFloat(t, 'f', 2, 64)
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
	b := strings.Builder{}
	for i, r := range s {
		separate := i > 0
		if separate {
			separate = (len(s)-i)%3 == 0
		}
		if separate {
			b.WriteRune('.')
		}
		b.WriteRune(r)
	}

	if neg {
		return fmt.Sprintf("-%s", b.String())
	}
	return b.String()
}

func formatSeconds(secs int) string {
	if secs <= 0 {
		return "0m"
	}

	h := secs / SECONDS_HOUR
	m := (secs % SECONDS_HOUR) / SECONDS_MINUTE
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
		t, err := time.Parse(layout, value)
		if err == nil {
			return t, true
		}
	}

	return time.Time{}, false
}

func entrySeconds(te map[string]any, now time.Time) int {
	secs := displayInt(te["duration"])
	if secs > 0 {
		return secs
	}

	start, ok := parseKimaiTime(stringOrEmpty(te["begin"]))
	if !ok {
		return 0
	}

	endStr := stringOrEmpty(te["end"])
	if endStr != "" {
		end, ok := parseKimaiTime(endStr)
		if !ok {
			return 0
		}
		d := int(end.Sub(start).Seconds())
		if d > 0 {
			return d
		}
		return 0
	}

	d := int(now.Sub(start).Seconds())
	if d > 0 {
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
		return DASH
	}

	if i := strings.IndexAny(s, "\n\r"); i >= 0 {
		s = s[:i]
	}

	if len(s) > MAX_LINE_LENGTH {
		s = fmt.Sprintf("%s%s", s[:MAX_LINE_LENGTH-3], ELLIPSIS)
	}

	return s
}

func inlineList(items []any) string {
	if len(items) == 0 {
		return DASH
	}

	parts := make([]string, 0, len(items))
	for _, it := range items {
		parts = append(parts, displayValue(it))
	}
	return strings.Join(parts, " ")
}

func formatPagination(meta kimai.PaginationMeta) string {
	if meta.TotalCount == 0 {
		if meta.CurrentPage == 0 {
			return ""
		}
	}

	parts := []string{}
	if meta.TotalCount > 0 {
		parts = append(parts, fmt.Sprintf("total de %s registros", formatInt(meta.TotalCount)))
	}
	if meta.CurrentPage > 0 {
		p := fmt.Sprintf("página %s", formatInt(meta.CurrentPage))
		if meta.PageSize > 0 {
			p = fmt.Sprintf("%s (%s por página)", p, formatInt(meta.PageSize))
		}
		parts = append(parts, p)
	}
	if meta.TotalPages > 0 {
		parts = append(parts, fmt.Sprintf("%s páginas no total", formatInt(meta.TotalPages)))
	}

	if len(parts) == 0 {
		return ""
	}

	return fmt.Sprintf("📄 Paginação: %s.", strings.Join(parts, ", "))
}

func appendPagination(b *strings.Builder, meta kimai.PaginationMeta) {
	text := formatPagination(meta)
	if text == "" {
		return
	}
	fmt.Fprintf(b, "\n%s\n", text)
}

func timeEntryEnd(te map[string]any) string {
	e := stringOrEmpty(te["end"])
	if e == "" {
		return DASH
	}
	return e
}

func formatTimeEntriesTable(items []any, meta kimai.PaginationMeta) string {
	if len(items) == 0 {
		return "Nenhuma marcação de tempo encontrada."
	}

	b := strings.Builder{}
	b.WriteString("| ID | Início | Fim | Duração | Projeto | Atividade | Faturável | Tags | Descrição |\n")
	b.WriteString("|----|--------|-----|----------|---------|-----------|-----------|------|------------|\n")

	for _, item := range items {
		te := asMap(item)
		if te == nil {
			continue
		}

		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %s | %s | %s | %s |\n",
			displayValue(te["id"]),
			displayValue(te["begin"]),
			timeEntryEnd(te),
			formatDuration(te),
			displayValue(te["project"]),
			displayValue(te["activity"]),
			yesNo(displayBool(te["billable"])),
			inlineList(listField(te, "tags")),
			tableCell(firstLine(displayString(te["description"]))),
		)
	}

	appendPagination(&b, meta)
	return strings.TrimSpace(b.String())
}

func hasRate(te map[string]any) bool {
	r, ok := te["rate"].(float64)
	if !ok {
		return false
	}
	return r != 0
}

func formatTimeEntry(item any) string {
	te := asMap(item)
	if te == nil {
		return "Marcação de tempo não encontrada."
	}

	b := strings.Builder{}
	fmt.Fprintf(&b, "⏱️ **Marcação de tempo #%s**\n\n", displayValue(te["id"]))
	fmt.Fprintf(&b, "- **Projeto:** %s\n", displayValue(te["project"]))
	fmt.Fprintf(&b, "- **Atividade:** %s\n", displayValue(te["activity"]))
	fmt.Fprintf(&b, "- **Início:** %s\n", displayValue(te["begin"]))

	e := stringOrEmpty(te["end"])
	if e != "" {
		fmt.Fprintf(&b, "- **Fim:** %s\n", e)
	}

	secs := entrySeconds(te, time.Now())
	fmt.Fprintf(&b, "- **Duração:** %s (%d s)\n", formatSeconds(secs), secs)
	fmt.Fprintf(&b, "- **Faturável:** %s\n", yesNo(displayBool(te["billable"])))
	fmt.Fprintf(&b, "- **Exportado:** %s\n", yesNo(displayBool(te["exported"])))

	if hasRate(te) {
		fmt.Fprintf(&b, "- **Rate:** %s\n", displayValue(te["rate"]))
	}

	fmt.Fprintf(&b, "- **Tags:** %s\n", inlineList(listField(te, "tags")))

	d := stringOrEmpty(te["description"])
	if d != "" {
		fmt.Fprintf(&b, "\n**Descrição:**\n%s\n", d)
	}

	return strings.TrimSpace(b.String())
}

func displayCustomer(v any, names map[int]string) string {
	m, ok := v.(map[string]any)
	if ok {
		n := stringOrEmpty(m["name"])
		if n != "" {
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

	b := strings.Builder{}
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

	appendPagination(&b, meta)
	return strings.TrimSpace(b.String())
}

func formatActivitiesTable(items []any, meta kimai.PaginationMeta) string {
	if len(items) == 0 {
		return "Nenhuma atividade encontrada."
	}

	b := strings.Builder{}
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

	appendPagination(&b, meta)
	return strings.TrimSpace(b.String())
}

func formatCustomersTable(items []any, meta kimai.PaginationMeta) string {
	if len(items) == 0 {
		return "Nenhum cliente encontrado."
	}

	b := strings.Builder{}
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

	appendPagination(&b, meta)
	return strings.TrimSpace(b.String())
}

func formatUser(item any) string {
	u := asMap(item)
	if u == nil {
		return "Usuário não encontrado."
	}

	b := strings.Builder{}
	fmt.Fprintf(&b, "👤 **Usuário #%s: %s**\n\n", displayValue(u["id"]), tableCell(displayString(u["username"])))

	v := stringOrEmpty(u["alias"])
	if v != "" {
		fmt.Fprintf(&b, "- **Nome:** %s\n", tableCell(v))
	}

	v = stringOrEmpty(u["email"])
	if v != "" {
		fmt.Fprintf(&b, "- **Email:** %s\n", tableCell(v))
	}

	v = stringOrEmpty(u["title"])
	if v != "" {
		fmt.Fprintf(&b, "- **Título:** %s\n", tableCell(v))
	}

	fmt.Fprintf(&b, "- **Ativo:** %s\n", yesNo(displayBool(u["enabled"])))

	v = stringOrEmpty(u["timezone"])
	if v != "" {
		fmt.Fprintf(&b, "- **Timezone:** %s\n", v)
	}

	v = stringOrEmpty(u["language"])
	if v != "" {
		fmt.Fprintf(&b, "- **Idioma:** %s\n", v)
	}

	return strings.TrimSpace(b.String())
}

func formatTags(data any) string {
	items, ok := data.([]any)
	if !ok {
		return "Nenhuma tag encontrada."
	}

	if len(items) == 0 {
		return "Nenhuma tag encontrada."
	}

	b := strings.Builder{}
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

func blockStatus(now time.Time, blk WorkBlock) string {
	start, okStart := blockTime(now, blk.Start)
	if !okStart {
		return ""
	}

	end, okEnd := blockTime(now, blk.End)
	if !okEnd {
		return ""
	}

	switch {
	case now.Before(start):
		return "⏳ à frente"
	case now.After(end):
		return "✅ concluído"
	default:
		return "🔵 em andamento"
	}
}

func blockLabel(blk WorkBlock) string {
	if blk.Label == "" {
		return "—"
	}
	return blk.Label
}

func formatCurrentTimeView(now time.Time, s *Schedule) string {
	loc := s.location()
	nowInLoc := now.In(loc)

	b := strings.Builder{}
	fmt.Fprintf(&b, "🕒 **Agora:** %s (%s)\n", formatNiceDateTime(nowInLoc), loc.String())

	fmt.Fprintf(&b, "\n📅 **Seu horário hoje (%s):**\n", capitalize(ptWeekday()[nowInLoc.Weekday()]))

	blocks := s.BlocksFor(nowInLoc)
	if len(blocks) == 0 {
		b.WriteString("Nenhum bloco de horário fixo para hoje.\n")
	}

	for _, blk := range blocks {
		fmt.Fprintf(&b, "- %s–%s **%s** %s\n", blk.Start, blk.End, blockLabel(blk), blockStatus(nowInLoc, blk))
	}

	b.WriteString("\n🗓️ **Calendário (mês atual):**\n")
	b.WriteString(formatMonthCalendar(nowInLoc))
	b.WriteByte('\n')
	b.WriteString("\n")
	b.WriteString(formatScheduleSection(s))

	return strings.TrimSpace(b.String())
}
