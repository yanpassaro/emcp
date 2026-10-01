package mcpserver

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"ntdsk.com/taiga/internal/taiga"
)

const (
	SIM           = "sim"
	NAO           = "não"
	DASH          = "—"
	KILOBYTE      = 1024
	MEGABYTE      = 1024 * 1024
	DATE_ONLY_LEN = 10
	MAX_LINKS     = 50
	DATE_FORMAT   = "02/01/2006 às 15:04"
)

func yesNoFlag(v any) string {
	if displayBool(v) {
		return "**sim**"
	}
	return NAO
}

func shortDate(obj map[string]any, key string) string {
	value := displayValue(obj[key])
	if len(value) > DATE_ONLY_LEN {
		return value[:DATE_ONLY_LEN]
	}
	return value
}

func appendPagination(b *strings.Builder, meta taiga.ResponseMeta) {
	text := formatPagination(meta)
	if text == "" {
		return
	}
	b.WriteString("\n")
	b.WriteString(text)
	b.WriteString("\n")
}

func formatProjectsTable(data any, meta taiga.ResponseMeta) string {
	items, ok := data.([]any)
	if !ok {
		return singleValue(data)
	}

	b := strings.Builder{}
	b.WriteString("| ID | Nome | Slug | Criado em | Atividade (total) | Você é admin? |\n")
	b.WriteString("|----|------|------|-----------|-------------------|---------------|\n")

	for _, item := range items {
		proj, ok := item.(map[string]any)
		if !ok {
			continue
		}

		fmt.Fprintf(&b, "| %s | %s | `%s` | %s | %s | %s |\n",
			displayValue(proj["id"]),
			displayValue(proj["name"]),
			stringOrEmpty(proj["slug"]),
			shortDate(proj, "created_date"),
			formatInt(displayInt(proj["total_activity"])),
			yesNoFlag(proj["i_am_admin"]),
		)
	}

	appendPagination(&b, meta)
	return b.String()
}

func formatCardsTable(data any, meta taiga.ResponseMeta) string {
	items, ok := data.([]any)
	if !ok {
		return singleValue(data)
	}

	b := strings.Builder{}
	b.WriteString("| ID | Ref | Assunto | Status | Responsável | Fechado? | Bloqueado? | Modificado em |\n")
	b.WriteString("|----|-----|---------|--------|-------------|----------|-----------|--------------|\n")

	for _, item := range items {
		card, ok := item.(map[string]any)
		if !ok {
			continue
		}

		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %s | %s | %s |\n",
			displayValue(card["id"]),
			displayValue(card["ref"]),
			redactText(displayValue(card["subject"])),
			translateStatus(resolveName(card, "status")),
			redactName(resolveName(card, "assigned_to")),
			yesNoFlag(card["is_closed"]),
			yesNoFlag(card["is_blocked"]),
			shortDate(card, "modified_date"),
		)
	}

	appendPagination(&b, meta)
	return b.String()
}

func formatCount(label string, count, projectID int, projectName string, filters []string) string {
	b := strings.Builder{}
	fmt.Fprintf(&b, "📊 **Total de %s: %s**\n\n", label, formatInt(count))

	if projectName != "" {
		fmt.Fprintf(&b, "- **Projeto:** %s (id %d)\n", projectName, projectID)
	}
	if projectName == "" {
		fmt.Fprintf(&b, "- **Projeto:** id %d\n", projectID)
	}

	if len(filters) > 0 {
		b.WriteString("\nFiltros aplicados:\n")
		for _, f := range filters {
			fmt.Fprintf(&b, "- %s\n", f)
		}
	}

	return strings.TrimSpace(b.String())
}

func formatIssuesTable(data any, meta taiga.ResponseMeta, priorities, severities map[int]string) string {
	items, ok := data.([]any)
	if !ok {
		return singleValue(data)
	}

	b := strings.Builder{}
	b.WriteString("| ID | Ref | Assunto | Status | Prioridade | Severidade | Fechado? | Modificado em |\n")
	b.WriteString("|----|-----|---------|--------|------------|------------|----------|--------------|\n")

	for _, item := range items {
		issue, ok := item.(map[string]any)
		if !ok {
			continue
		}

		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %s | %s | %s |\n",
			displayValue(issue["id"]),
			displayValue(issue["ref"]),
			redactText(displayValue(issue["subject"])),
			translateStatus(resolveName(issue, "status")),
			resolveFieldValue(issue, "priority", priorities),
			resolveFieldValue(issue, "severity", severities),
			yesNoFlag(issue["is_closed"]),
			shortDate(issue, "modified_date"),
		)
	}

	appendPagination(&b, meta)
	return b.String()
}

func formatTasksTable(data any, meta taiga.ResponseMeta) string {
	items, ok := data.([]any)
	if !ok {
		return singleValue(data)
	}

	b := strings.Builder{}
	b.WriteString("| ID | Ref | Assunto | Status | Responsável | Card vinculado | Fechado? | Modificado em |\n")
	b.WriteString("|----|-----|---------|--------|-------------|----------------|----------|--------------|\n")

	for _, item := range items {
		task, ok := item.(map[string]any)
		if !ok {
			continue
		}

		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %s | %s | %s |\n",
			displayValue(task["id"]),
			displayValue(task["ref"]),
			redactText(displayValue(task["subject"])),
			translateStatus(resolveName(task, "status")),
			redactName(resolveName(task, "assigned_to")),
			linkedStory(task),
			yesNoFlag(task["is_closed"]),
			shortDate(task, "modified_date"),
		)
	}

	appendPagination(&b, meta)
	return b.String()
}

func formatAttachmentsTable(data any) string {
	items, ok := data.([]any)
	if !ok {
		return singleValue(data)
	}

	b := strings.Builder{}
	b.WriteString("| ID | Nome | Tipo | Tamanho |\n")
	b.WriteString("|----|------|------|---------|\n")

	for _, item := range items {
		att, ok := item.(map[string]any)
		if !ok {
			continue
		}

		name := displayValue(att["name"])
		mime := firstString(att, "content_type", "mime_type", "mimetype", "type")
		if mime == "" {
			mime = fileTypeFromName(name)
		}

		size := int64(displayInt(att["size"]))
		fmt.Fprintf(&b, "| %s | %s | %s | %s |\n",
			displayValue(att["id"]), name, mime, formatFileSize(size),
		)
	}

	return b.String()
}

func fileExt(name string) string {
	i := strings.LastIndex(name, ".")
	if i < 0 {
		return ""
	}
	if i >= len(name)-1 {
		return ""
	}
	return strings.ToLower(name[i+1:])
}

func fileTypeFromName(name string) string {
	switch fileExt(name) {
	case "png":
		return "image/png"
	case "jpg", "jpeg":
		return "image/jpeg"
	case "gif":
		return "image/gif"
	case "webp":
		return "image/webp"
	case "svg":
		return "image/svg+xml"
	case "mp4", "mov", "avi", "mkv", "webm":
		return "video/mp4"
	case "mp3", "wav", "ogg":
		return "audio/mpeg"
	case "pdf":
		return "application/pdf"
	case "doc", "docx":
		return "application/msword"
	case "xls", "xlsx", "csv":
		return "application/vnd.ms-excel"
	case "zip", "rar", "7z", "tar", "gz":
		return "application/zip"
	case "":
		return DASH
	default:
		return strings.ToUpper(fileExt(name))
	}
}

func activityKind(it activityItem) string {
	if it.kind == "issue" {
		return "Issue"
	}
	return "Card"
}

func formatActivityTable(items []activityItem) string {
	if len(items) == 0 {
		return "Nenhuma atividade encontrada."
	}

	b := strings.Builder{}
	for _, it := range items {
		action := it.action
		if action == "" {
			action = "📝 Atividade registrada"
		}

		modified := formatActivityDate(it.modified)
		fmt.Fprintf(&b, "### %s #%s — %s\n\n", activityKind(it), displayValue(it.ref), redactText(it.subject))
		fmt.Fprintf(&b, "- **O que aconteceu:** %s\n", action)

		if it.assigned != "" {
			fmt.Fprintf(&b, "- **Responsável:** %s\n", it.assigned)
		}
		if modified != "" {
			fmt.Fprintf(&b, "- **Quando:** %s\n", modified)
		}

		b.WriteString("\n")
	}

	return strings.TrimSpace(b.String())
}

type historyEntryView struct {
	author string
	date   string
	action string
}

func historyKindLabel(kind string) string {
	switch kind {
	case "issue":
		return "Issue"
	case "task":
		return "Task"
	default:
		return "Card"
	}
}

func formatHistoryTable(kind string, ref int, subject, status string, entries []historyEntryView) string {
	b := strings.Builder{}
	fmt.Fprintf(&b, "### %s #%s — %s\n\n", historyKindLabel(kind), displayValue(ref), redactText(subject))

	if status != "" {
		fmt.Fprintf(&b, "- **Status:** %s\n", status)
	}

	b.WriteString("\n")
	b.WriteString("| Data | Autor | Atividade |\n")
	b.WriteString("|------|-------|-----------|\n")

	for _, e := range entries {
		date := formatActivityDate(e.date)
		action := strings.TrimSpace(e.action)
		if action == "" {
			action = "📝 Modificado"
		}
		fmt.Fprintf(&b, "| %s | %s | %s |\n", date, e.author, action)
	}

	return strings.TrimSpace(b.String())
}

func formatActivityDate(value string) string {
	parsed := parseActivityTime(value)
	if parsed.IsZero() {
		return value
	}
	return parsed.Format(DATE_FORMAT)
}

func projectMetaLines(obj map[string]any) []string {
	meta := []string{}
	addMeta := func(label, key string) {
		if v := displayValue(obj[key]); v != "" {
			meta = append(meta, fmt.Sprintf("**%s:** %s", label, v))
		}
	}

	addMeta("Criado em", "created_date")
	addMeta("Modificado em", "modified_date")
	return meta
}

func projectFeatures(obj map[string]any) []string {
	features := []string{}
	if displayBool(obj["is_private"]) {
		features = append(features, "**Privado:** sim")
	}

	m := displayInt(obj["total_memberships"])
	if m > 0 {
		features = append(features, fmt.Sprintf("**Membros:** %s", formatInt(m)))
	}

	w := displayInt(obj["total_watchers"])
	if w > 0 {
		features = append(features, fmt.Sprintf("**Watchers:** %s", formatInt(w)))
	}

	pairs := [][2]string{
		{"is_kanban_activated", "Kanban"}, {"is_wiki_activated", "Wiki"},
		{"is_issues_activated", "Issues"}, {"is_contact_activated", "Contato"},
	}

	for _, pair := range pairs {
		if displayBool(obj[pair[0]]) {
			features = append(features, fmt.Sprintf("**%s:** sim", pair[1]))
		}
	}

	return features
}

func projectActivityLine(obj map[string]any) string {
	total := displayInt(obj["total_activity"])
	if total <= 0 {
		return ""
	}

	line := fmt.Sprintf("**Atividade total:** %s", formatInt(total))
	month := displayInt(obj["total_activity_last_month"])
	if month > 0 {
		line = fmt.Sprintf("%s (último mês: %s)", line, formatInt(month))
	}

	return line
}

func formatProject(data any, activities []activityItem) string {
	obj, ok := data.(map[string]any)
	if !ok {
		return singleValue(data)
	}

	b := strings.Builder{}
	fmt.Fprintf(&b, "# %s\n\n", redactText(displayValue(obj["name"])))

	desc := redactText(displayValue(obj["description"]))
	if desc != "" {
		fmt.Fprintf(&b, "**Descrição:** %s\n\n", desc)
	}

	slug := stringOrEmpty(obj["slug"])
	if slug != "" {
		fmt.Fprintf(&b, "**Slug:** `%s`\n\n", slug)
	}

	meta := projectMetaLines(obj)
	if len(meta) > 0 {
		b.WriteString(strings.Join(meta, " | "))
		b.WriteString("\n\n")
	}

	features := projectFeatures(obj)
	if len(features) > 0 {
		b.WriteString(strings.Join(features, " | "))
		b.WriteString("\n\n")
	}

	statuses := formatProjectOptions("Status", obj["us_statuses"])
	if statuses != "" {
		b.WriteString(statuses)
	}

	swimlanes := formatProjectOptions("Baias", obj["swimlanes"])
	if swimlanes != "" {
		b.WriteString(swimlanes)
	}

	tags := formatProjectTags(obj["tags_colors"])
	if tags != "" {
		b.WriteString(tags)
	}

	line := projectActivityLine(obj)
	if line != "" {
		b.WriteString(line)
		b.WriteByte('\n')
	}

	if len(activities) > 0 {
		b.WriteString("\n### Últimas atividades\n\n")
		b.WriteString(formatActivityTable(activities))
	}

	return b.String()
}

func formatProjectOptions(title string, value any) string {
	items, ok := value.([]any)
	if !ok {
		return ""
	}

	if len(items) == 0 {
		return ""
	}

	lines := make([]string, 0, len(items))
	for _, item := range items {
		option, ok := item.(map[string]any)
		if !ok {
			continue
		}

		id := displayValue(option["id"])
		name := redactText(displayValue(option["name"]))
		if id == "" {
			continue
		}
		if name == "" {
			continue
		}

		lines = append(lines, fmt.Sprintf("- `%s`: %s", id, name))
	}

	if len(lines) == 0 {
		return ""
	}

	b := strings.Builder{}
	fmt.Fprintf(&b, "**%s:**\n", title)
	b.WriteString(strings.Join(lines, "\n"))
	b.WriteString("\n\n")
	return b.String()
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func formatProjectTags(value any) string {
	colors, ok := value.(map[string]any)
	if !ok {
		return ""
	}

	if len(colors) == 0 {
		return ""
	}

	b := strings.Builder{}
	b.WriteString("🏷️ **Tags do projeto** (ortografia exata para uso nas tags):\n")
	for _, t := range sortedKeys(colors) {
		fmt.Fprintf(&b, "- `%s`\n", redactText(t))
	}
	b.WriteString("\n")
	return b.String()
}

func formatObject(data any) string {
	obj, ok := data.(map[string]any)
	if !ok {
		return singleValue(data)
	}

	b := strings.Builder{}
	for _, k := range sortedKeys(obj) {
		fmt.Fprintf(&b, "- **%s**: %s\n", k, redactText(displayValue(obj[k])))
	}
	return b.String()
}

type commentView struct {
	author string
	date   string
	text   string
}

func detailedBadges(obj map[string]any, priorities, severities map[int]string) []string {
	badges := []string{fmt.Sprintf("**Status:** %s", translateStatus(resolveName(obj, "status")))}

	assigned := redactName(resolveName(obj, "assigned_to"))
	if assigned != "" {
		badges = append(badges, fmt.Sprintf("**Responsável:** %s", assigned))
	}

	project := resolveName(obj, "project")
	if project != "" {
		badges = append(badges, fmt.Sprintf("**Projeto:** %s", project))
	}

	priority := resolveFieldValue(obj, "priority", priorities)
	if priority != "" {
		badges = append(badges, fmt.Sprintf("**Prioridade:** %s", priority))
	}

	severity := resolveFieldValue(obj, "severity", severities)
	if severity != "" {
		badges = append(badges, fmt.Sprintf("**Severidade:** %s", severity))
	}

	return badges
}

func detailedMeta(obj map[string]any) []string {
	meta := []string{}
	addMeta := func(label, key string) {
		if v := displayValue(obj[key]); v != "" {
			meta = append(meta, fmt.Sprintf("- **%s:** %s", label, v))
		}
	}

	addMeta("Criado em", "created_date")
	addMeta("Modificado em", "modified_date")
	addMeta("Fechado?", "is_closed")

	tags := inlineList(obj["tags"])
	if tags != "" {
		meta = append(meta, fmt.Sprintf("- **Tags:** %s", tags))
	}

	return meta
}

func detailedCounts(obj map[string]any) []string {
	counts := []string{}

	a := displayInt(obj["total_attachments"])
	if a > 0 {
		counts = append(counts, fmt.Sprintf("📎 %s anexos", formatInt(a)))
	}

	c := displayInt(obj["total_comments"])
	if c > 0 {
		counts = append(counts, fmt.Sprintf("💬 %s comentários", formatInt(c)))
	}

	w := displayInt(obj["total_watchers"])
	if w > 0 {
		counts = append(counts, fmt.Sprintf("👁 %s observadores", formatInt(w)))
	}

	return counts
}

func writeAttachments(b *strings.Builder, attachments []string) {
	if len(attachments) == 0 {
		return
	}

	b.WriteString("\n### Anexos\n\n")

	listed := attachments
	if len(listed) > MAX_LINKS {
		listed = listed[len(listed)-MAX_LINKS:]
	}

	b.WriteString("\n")
	for _, name := range listed {
		fmt.Fprintf(b, "%s\n", name)
	}
}

func commentHeader(c commentView) string {
	who := redactName(c.author)
	if who == "" {
		who = DASH
	}

	when := ""
	if c.date != "" {
		when = fmt.Sprintf(" · %s", c.date)
	}

	return fmt.Sprintf("%s%s", who, when)
}

func writeComments(b *strings.Builder, comments []commentView) {
	if len(comments) == 0 {
		return
	}

	fmt.Fprintf(b, "\n### Comentários (%d)\n\n", len(comments))
	for _, c := range comments {
		fmt.Fprintf(b, "**%s**\n\n%s\n\n", commentHeader(c), redactText(stripHTML(c.text)))
	}
}

func formatDetailedItem(data any, comments []commentView, attachments []string, subtasks []string, priorities, severities map[int]string) string {
	obj, ok := data.(map[string]any)
	if !ok {
		return singleValue(data)
	}

	b := strings.Builder{}
	fmt.Fprintf(&b, "# %s\n\n", redactText(displayValue(obj["subject"])))

	id := displayValue(obj["id"])
	ref := displayValue(obj["ref"])
	writeRefLine(&b, id, ref)

	b.WriteString(strings.Join(detailedBadges(obj, priorities, severities), "  |  "))
	b.WriteString("\n\n")

	meta := detailedMeta(obj)
	if len(meta) > 0 {
		b.WriteString(strings.Join(meta, "\n"))
		b.WriteString("\n\n")
	}

	desc := strings.TrimSpace(redactText(displayValue(obj["description"])))
	if desc != "" {
		b.WriteString("### Descrição\n\n")
		b.WriteString(desc)
		b.WriteString("\n\n")
	}

	counts := detailedCounts(obj)
	if len(counts) > 0 {
		b.WriteString(strings.Join(counts, "  "))
		b.WriteString("\n")
	}

	linked := formatLinkedCards(obj)
	if linked != "" {
		b.WriteString("\n### Cards vinculados\n\n")
		b.WriteString(linked)
		b.WriteString("\n")
	}

	subtaskText := strings.Join(subtasks, "\n")
	if subtaskText == "" {
		subtaskText = formatSubtasks(obj)
	}
	if subtaskText != "" {
		b.WriteString("\n### Subtarefas\n\n")
		b.WriteString(subtaskText)
		b.WriteString("\n")
	}

	writeAttachments(&b, attachments)
	writeComments(&b, comments)
	return b.String()
}

func writeRefLine(b *strings.Builder, id, ref string) {
	if id == "" {
		if ref == "" {
			return
		}
		fmt.Fprintf(b, "ref %s\n\n", ref)
		return
	}

	fmt.Fprintf(b, "#%s", id)
	if ref != "" {
		fmt.Fprintf(b, "  ·  ref %s", ref)
	}
	b.WriteString("\n\n")
}

func formatTaskItem(data any, comments []commentView, attachments []string) string {
	obj, ok := data.(map[string]any)
	if !ok {
		return singleValue(data)
	}

	b := strings.Builder{}
	parent := linkedStory(obj)
	if parent != DASH {
		b.WriteString("### Card vinculado\n\n")
		fmt.Fprintf(&b, "- %s\n\n", parent)
	}

	b.WriteString(formatDetailedItem(data, comments, attachments, nil, nil, nil))
	return b.String()
}

func linkedStory(obj map[string]any) string {
	extra, ok := obj["user_story_extra_info"].(map[string]any)
	if !ok {
		return DASH
	}

	if len(extra) == 0 {
		return DASH
	}

	ref := displayValue(extra["ref"])
	subject := redactText(displayValue(extra["subject"]))

	switch {
	case ref != "":
		if subject != "" {
			return fmt.Sprintf("%s — %s", ref, subject)
		}
		return ref
	case subject != "":
		return subject
	default:
		return DASH
	}
}

func itemSubject(m map[string]any) string {
	subject := redactText(displayValue(m["subject"]))
	if subject != "" {
		return subject
	}
	return redactText(displayValue(m["name"]))
}

func itemPrefix(id, ref string) string {
	prefix := ""
	if ref != "" {
		prefix = fmt.Sprintf("ref %s", ref)
	}

	if id == "" {
		return prefix
	}

	if prefix == "" {
		return fmt.Sprintf("id %s", id)
	}

	return fmt.Sprintf("%s · id %s", prefix, id)
}

func formatSubtasks(obj map[string]any) string {
	value, ok := obj["tasks"]
	if !ok {
		return ""
	}

	items, ok := value.([]any)
	if !ok {
		return ""
	}

	if len(items) == 0 {
		return ""
	}

	lines := make([]string, 0, len(items))
	for _, item := range items {
		task, ok := item.(map[string]any)
		if !ok {
			continue
		}

		subject := itemSubject(task)
		if subject == "" {
			continue
		}

		prefix := itemPrefix(displayValue(task["id"]), displayValue(task["ref"]))
		status := translateStatus(resolveName(task, "status"))
		assigned := redactName(resolveName(task, "assigned_to"))

		parts := []string{}
		if prefix != "" {
			parts = append(parts, prefix)
		}
		parts = append(parts, subject)
		if status != "" {
			parts = append(parts, status)
		}
		if assigned != "" {
			parts = append(parts, assigned)
		}

		lines = append(lines, fmt.Sprintf("- %s", strings.Join(parts, " — ")))
	}

	return strings.Join(lines, "\n")
}

func linkedLines(items []any) []string {
	lines := make([]string, 0, len(items))
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}

		subject := itemSubject(m)
		if subject == "" {
			continue
		}

		prefix := itemPrefix(displayValue(m["id"]), displayValue(m["ref"]))
		if prefix == "" {
			lines = append(lines, fmt.Sprintf("- %s", subject))
			continue
		}

		lines = append(lines, fmt.Sprintf("- %s — %s", prefix, subject))
	}

	return lines
}

func formatLinkedCards(obj map[string]any) string {
	for _, key := range []string{"subcards", "sub_cards", "children", "related_userstories", "related_cards"} {
		items, ok := obj[key].([]any)
		if !ok {
			continue
		}
		if len(items) == 0 {
			continue
		}

		lines := linkedLines(items)
		if len(lines) == 0 {
			continue
		}

		return strings.Join(lines, "\n")
	}

	return ""
}

func resolveName(obj map[string]any, base string) string {
	extra, ok := obj[fmt.Sprintf("%s_extra_info", base)]
	if !ok {
		return displayValue(obj[base])
	}

	switch typed := extra.(type) {
	case string:
		if strings.TrimSpace(typed) != "" {
			return strings.TrimSpace(typed)
		}
	case map[string]any:
		for _, key := range []string{"full_name_display", "username", "name", "subject"} {
			v := stringOrEmpty(typed[key])
			if v != "" {
				return v
			}
		}
	}

	return displayValue(obj[base])
}

func translateStatus(status string) string {
	translations := map[string]string{
		"new":            "Novo",
		"ready for test": "Pronto para teste",
		"in progress":    "Em andamento",
		"ready":          "Pronto",
		"test":           "Em teste",
		"closed":         "Fechado",
		"done":           "Concluído",
		"archived":       "Arquivado",
		"rejected":       "Rejeitado",
		"blocked":        "Bloqueado",
	}

	translated, ok := translations[strings.ToLower(strings.TrimSpace(status))]
	if ok {
		return translated
	}

	return status
}

func stripHTML(s string) string {
	s = regexp.MustCompile(`(?i)<br\s*/?>|</p>|</li>`).ReplaceAllString(s, "\n")
	s = regexp.MustCompile(`(?i)<[^>]+>`).ReplaceAllString(s, "")
	s = regexp.MustCompile(`!\[[^\]]*\]\([^)]*\)`).ReplaceAllString(s, "[IMAGEM]")
	return strings.TrimSpace(s)
}

func redactText(s string) string {
	if !redactEnabled {
		return s
	}

	s = regexp.MustCompile(`(?i)[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}`).ReplaceAllString(s, "[email]")
	s = regexp.MustCompile(`\b\d{3}\.?\d{3}\.?\d{3}-?\d{2}\b`).ReplaceAllString(s, "[cpf]")
	s = regexp.MustCompile(`\(?\d{2}\)?[\s-]\d{4,5}[\s-]?\d{4}`).ReplaceAllString(s, "[telefone]")
	s = regexp.MustCompile(`https?://[^\s)>\]]+`).ReplaceAllString(s, "[url]")
	return s
}

func redactName(s string) string {
	if !redactEnabled {
		return s
	}

	s = strings.TrimSpace(s)
	if s == "" {
		return s
	}

	parts := strings.Fields(s)
	if len(parts) <= 1 {
		return s
	}

	r := []rune(parts[1])
	return fmt.Sprintf("%s %s", parts[0], strings.ToUpper(string(r[0])))
}

func resolveFieldValue(obj map[string]any, base string, lookup map[int]string) string {
	name := resolveName(obj, base)
	if lookup == nil {
		return name
	}

	id := displayInt(obj[base])
	if id <= 0 {
		return name
	}

	mapped, ok := lookup[id]
	if !ok {
		return name
	}
	if mapped == "" {
		return name
	}

	return mapped
}

func inlineList(value any) string {
	items, ok := value.([]any)
	if !ok {
		return displayValue(value)
	}

	parts := make([]string, 0, len(items))
	for _, it := range items {
		switch typed := it.(type) {
		case string:
			parts = append(parts, typed)
		case []any:
			if len(typed) > 0 {
				parts = append(parts, displayValue(typed[0]))
			}
		default:
			parts = append(parts, displayValue(typed))
		}
	}

	if len(parts) == 0 {
		return displayValue(value)
	}
	return strings.Join(parts, ", ")
}

func displayValue(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(typed)
	case bool:
		return boolText(typed)
	case float64:
		return floatText(typed)
	case int:
		return strconv.Itoa(typed)
	case map[string]any:
		if name, ok := typed["name"].(string); ok {
			return strings.TrimSpace(name)
		}
		return "objeto"
	case []any:
		return fmt.Sprintf("[%d itens]", len(typed))
	default:
		return fmt.Sprintf("%v", typed)
	}
}

func boolText(v bool) string {
	if v {
		return SIM
	}
	return NAO
}

func floatText(v float64) string {
	if v == float64(int64(v)) {
		return strconv.Itoa(int(v))
	}
	return strconv.FormatFloat(v, 'f', 2, 64)
}

func displayBool(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case float64:
		return typed != 0
	case string:
		return truthyText(typed)
	default:
		return false
	}
}

func truthyText(v string) bool {
	if v == "true" {
		return true
	}
	if v == "True" {
		return true
	}
	return v == "1"
}

func displayInt(value any) int {
	switch typed := value.(type) {
	case float64:
		return int(typed)
	case int:
		return typed
	case string:
		n, err := strconv.Atoi(strings.TrimSpace(typed))
		if err == nil {
			return n
		}
		return 0
	default:
		return 0
	}
}

func stringOrEmpty(value any) string {
	if s, ok := value.(string); ok {
		return strings.TrimSpace(s)
	}
	return ""
}

func singleValue(value any) string {
	return fmt.Sprintf("```json\n%v\n```", value)
}

func formatInt(n int) string {
	neg := n < 0
	if neg {
		n = -n
	}

	s := strconv.Itoa(n)
	out := strings.Builder{}
	for i, r := range s {
		separate := i > 0
		if separate {
			separate = (len(s)-i)%3 == 0
		}
		if separate {
			out.WriteByte('.')
		}
		out.WriteRune(r)
	}

	result := out.String()
	if neg {
		return fmt.Sprintf("-%s", result)
	}
	return result
}

func formatDownload(result taiga.DownloadResult) string {
	b := strings.Builder{}
	b.WriteString("Anexo salvo com sucesso ✅\n\n")
	fmt.Fprintf(&b, "- **Caminho**: `%s`\n", result.Path)
	fmt.Fprintf(&b, "- **Tamanho**: %s\n", formatFileSize(result.Bytes))

	if result.ContentType != "" {
		fmt.Fprintf(&b, "- **Tipo**: %s\n", result.ContentType)
	}

	return b.String()
}

func formatFileSize(bytes int64) string {
	if bytes >= MEGABYTE {
		return fmt.Sprintf("%.2f MB", float64(bytes)/MEGABYTE)
	}
	if bytes >= KILOBYTE {
		return fmt.Sprintf("%.1f KB", float64(bytes)/KILOBYTE)
	}
	return fmt.Sprintf("%d bytes", bytes)
}

func formatPagination(meta taiga.ResponseMeta) string {
	parts := []string{}
	if meta.Total != "" {
		parts = append(parts, fmt.Sprintf("total de %s registros", meta.Total))
	}

	if meta.CurrentPage != "" {
		page := fmt.Sprintf("página %s", meta.CurrentPage)
		if meta.PaginatedBy != "" {
			page = fmt.Sprintf("%s (%s por página)", page, meta.PaginatedBy)
		}
		parts = append(parts, page)
	}

	if len(parts) == 0 {
		return ""
	}

	return fmt.Sprintf("📄 Paginação: %s.", strings.Join(parts, ", "))
}
