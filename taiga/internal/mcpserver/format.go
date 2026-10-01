package mcpserver

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"ntdsk.com/taiga/internal/taiga"
)

func formatProjectsTable(data any, meta taiga.ResponseMeta) string {
	items, ok := data.([]any)
	if !ok {
		return singleValue(data)
	}
	var b strings.Builder
	b.WriteString("| ID | Nome | Slug | Criado em | Atividade (total) | Você é admin? |\n")
	b.WriteString("|----|------|------|-----------|-------------------|---------------|\n")
	for _, item := range items {
		proj, ok := item.(map[string]any)
		if !ok {
			continue
		}
		created := displayValue(proj["created_date"])
		if len(created) > 10 {
			created = created[:10]
		}
		admin := "não"
		if displayBool(proj["i_am_admin"]) {
			admin = "**sim**"
		}
		fmt.Fprintf(&b, "| %s | %s | `%s` | %s | %s | %s |\n",
			displayValue(proj["id"]),
			displayValue(proj["name"]),
			stringOrEmpty(proj["slug"]),
			created,
			formatInt(displayInt(proj["total_activity"])),
			admin,
		)
	}
	if text := formatPagination(meta); text != "" {
		b.WriteString("\n")
		b.WriteString(text)
		b.WriteString("\n")
	}
	return b.String()
}

func formatCardsTable(data any, meta taiga.ResponseMeta) string {
	items, ok := data.([]any)
	if !ok {
		return singleValue(data)
	}
	var b strings.Builder
	b.WriteString("| ID | Ref | Assunto | Status | Responsável | Fechado? | Bloqueado? | Modificado em |\n")
	b.WriteString("|----|-----|---------|--------|-------------|----------|-----------|--------------|\n")
	for _, item := range items {
		card, ok := item.(map[string]any)
		if !ok {
			continue
		}
		modified := displayValue(card["modified_date"])
		if len(modified) > 10 {
			modified = modified[:10]
		}
		closed := "não"
		if displayBool(card["is_closed"]) {
			closed = "**sim**"
		}
		blocked := "não"
		if displayBool(card["is_blocked"]) {
			blocked = "**sim**"
		}
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %s | %s | %s |\n",
			displayValue(card["id"]),
			displayValue(card["ref"]),
			redactText(displayValue(card["subject"])),
			translateStatus(resolveName(card, "status")),
			redactName(resolveName(card, "assigned_to")),
			closed,
			blocked,
			modified,
		)
	}
	if text := formatPagination(meta); text != "" {
		b.WriteString("\n")
		b.WriteString(text)
		b.WriteString("\n")
	}
	return b.String()
}

func formatCount(label string, count, projectID int, projectName string, filters []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "📊 **Total de %s: %s**\n\n", label, formatInt(count))
	if projectName != "" {
		fmt.Fprintf(&b, "- **Projeto:** %s (id %d)\n", projectName, projectID)
	} else {
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
	var b strings.Builder
	b.WriteString("| ID | Ref | Assunto | Status | Prioridade | Severidade | Fechado? | Modificado em |\n")
	b.WriteString("|----|-----|---------|--------|------------|------------|----------|--------------|\n")
	for _, item := range items {
		issue, ok := item.(map[string]any)
		if !ok {
			continue
		}
		modified := displayValue(issue["modified_date"])
		if len(modified) > 10 {
			modified = modified[:10]
		}
		closed := "não"
		if displayBool(issue["is_closed"]) {
			closed = "**sim**"
		}
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %s | %s | %s |\n",
			displayValue(issue["id"]),
			displayValue(issue["ref"]),
			redactText(displayValue(issue["subject"])),
			translateStatus(resolveName(issue, "status")),
			resolveFieldValue(issue, "priority", priorities),
			resolveFieldValue(issue, "severity", severities),
			closed,
			modified,
		)
	}
	if text := formatPagination(meta); text != "" {
		b.WriteString("\n")
		b.WriteString(text)
		b.WriteString("\n")
	}
	return b.String()
}

func formatTasksTable(data any, meta taiga.ResponseMeta) string {
	items, ok := data.([]any)
	if !ok {
		return singleValue(data)
	}
	var b strings.Builder
	b.WriteString("| ID | Ref | Assunto | Status | Responsável | Card vinculado | Fechado? | Modificado em |\n")
	b.WriteString("|----|-----|---------|--------|-------------|----------------|----------|--------------|\n")
	for _, item := range items {
		task, ok := item.(map[string]any)
		if !ok {
			continue
		}
		modified := displayValue(task["modified_date"])
		if len(modified) > 10 {
			modified = modified[:10]
		}
		closed := "não"
		if displayBool(task["is_closed"]) {
			closed = "**sim**"
		}
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %s | %s | %s |\n",
			displayValue(task["id"]),
			displayValue(task["ref"]),
			redactText(displayValue(task["subject"])),
			translateStatus(resolveName(task, "status")),
			redactName(resolveName(task, "assigned_to")),
			linkedStory(task),
			closed,
			modified,
		)
	}
	if text := formatPagination(meta); text != "" {
		b.WriteString("\n")
		b.WriteString(text)
		b.WriteString("\n")
	}
	return b.String()
}

func formatAttachmentsTable(data any) string {
	items, ok := data.([]any)
	if !ok {
		return singleValue(data)
	}
	var b strings.Builder
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
			displayValue(att["id"]),
			name,
			mime,
			formatFileSize(size),
		)
	}
	return b.String()
}

func fileTypeFromName(name string) string {
	ext := ""
	if i := strings.LastIndex(name, "."); i >= 0 && i < len(name)-1 {
		ext = strings.ToLower(name[i+1:])
	}
	switch ext {
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
		return "—"
	default:
		return strings.ToUpper(ext)
	}
}

func formatActivityTable(items []activityItem) string {
	if len(items) == 0 {
		return "Nenhuma atividade encontrada."
	}
	var b strings.Builder
	for _, it := range items {
		kind := "Card"
		if it.kind == "issue" {
			kind = "Issue"
		}
		action := it.action
		if action == "" {
			action = "📝 Atividade registrada"
		}
		modified := formatActivityDate(it.modified)
		fmt.Fprintf(&b, "### %s #%s — %s\n\n", kind, displayValue(it.ref), redactText(it.subject))
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

func formatHistoryTable(kind string, ref int, subject, status string, entries []historyEntryView) string {
	kindLabel := "Card"
	switch kind {
	case "issue":
		kindLabel = "Issue"
	case "task":
		kindLabel = "Task"
	}
	var b strings.Builder
	title := fmt.Sprintf("%s #%s — %s", kindLabel, displayValue(ref), redactText(subject))
	fmt.Fprintf(&b, "### %s\n\n", title)
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
	return parsed.Format("02/01/2006 às 15:04")
}

func formatProject(data any, activities []activityItem) string {
	obj, ok := data.(map[string]any)
	if !ok {
		return singleValue(data)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", redactText(displayValue(obj["name"])))
	if desc := redactText(displayValue(obj["description"])); desc != "" {
		fmt.Fprintf(&b, "**Descrição:** %s\n\n", desc)
	}
	if slug := stringOrEmpty(obj["slug"]); slug != "" {
		fmt.Fprintf(&b, "**Slug:** `%s`\n\n", slug)
	}
	meta := []string{}
	addMeta := func(label, key string) {
		if v := displayValue(obj[key]); v != "" {
			meta = append(meta, fmt.Sprintf("**%s:** %s", label, v))
		}
	}
	addMeta("Criado em", "created_date")
	addMeta("Modificado em", "modified_date")
	if len(meta) > 0 {
		b.WriteString(strings.Join(meta, " | "))
		b.WriteString("\n\n")
	}
	features := []string{}
	if displayBool(obj["is_private"]) {
		features = append(features, "**Privado:** sim")
	}
	if m := displayInt(obj["total_memberships"]); m > 0 {
		features = append(features, fmt.Sprintf("**Membros:** %s", formatInt(m)))
	}
	if w := displayInt(obj["total_watchers"]); w > 0 {
		features = append(features, fmt.Sprintf("**Watchers:** %s", formatInt(w)))
	}
	for _, pair := range [][2]string{
		{"is_kanban_activated", "Kanban"}, {"is_wiki_activated", "Wiki"},
		{"is_issues_activated", "Issues"}, {"is_contact_activated", "Contato"},
	} {
		if displayBool(obj[pair[0]]) {
			features = append(features, "**"+pair[1]+":** sim")
		}
	}
	if len(features) > 0 {
		b.WriteString(strings.Join(features, " | "))
		b.WriteString("\n\n")
	}
	if statuses := formatProjectOptions("Status", obj["us_statuses"]); statuses != "" {
		b.WriteString(statuses)
	}
	if swimlanes := formatProjectOptions("Baias", obj["swimlanes"]); swimlanes != "" {
		b.WriteString(swimlanes)
	}
	if tags := formatProjectTags(obj["tags_colors"]); tags != "" {
		b.WriteString(tags)
	}
	if total := displayInt(obj["total_activity"]); total > 0 {
		line := fmt.Sprintf("**Atividade total:** %s", formatInt(total))
		if month := displayInt(obj["total_activity_last_month"]); month > 0 {
			line += fmt.Sprintf(" (último mês: %s)", formatInt(month))
		}
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
	if !ok || len(items) == 0 {
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
		if id == "" || name == "" {
			continue
		}
		lines = append(lines, fmt.Sprintf("- `%s`: %s", id, name))
	}
	if len(lines) == 0 {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "**%s:**\n", title)
	b.WriteString(strings.Join(lines, "\n"))
	b.WriteString("\n\n")
	return b.String()
}

func formatProjectTags(value any) string {
	colors, ok := value.(map[string]any)
	if !ok || len(colors) == 0 {
		return ""
	}
	keys := make([]string, 0, len(colors))
	for t := range colors {
		keys = append(keys, t)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString("🏷️ **Tags do projeto** (ortografia exata para uso nas tags):\n")
	for _, t := range keys {
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
	keys := make([]string, 0, len(obj))
	for k := range obj {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		fmt.Fprintf(&b, "- **%s**: %s\n", k, redactText(displayValue(obj[k])))
	}
	return b.String()
}

type commentView struct {
	author string
	date   string
	text   string
}

func formatDetailedItem(data any, comments []commentView, attachments []string, subtasks []string, priorities, severities map[int]string) string {
	obj, ok := data.(map[string]any)
	if !ok {
		return singleValue(data)
	}
	var b strings.Builder
	subject := redactText(displayValue(obj["subject"]))
	b.WriteString("# ")
	b.WriteString(subject)
	b.WriteString("\n\n")

	id := displayValue(obj["id"])
	ref := displayValue(obj["ref"])
	if id != "" || ref != "" {
		if id != "" {
			fmt.Fprintf(&b, "#%s", id)
		}
		if ref != "" {
			if id != "" {
				b.WriteString("  ·  ")
			}
			fmt.Fprintf(&b, "ref %s", ref)
		}
		b.WriteString("\n\n")
	}

	badges := []string{"**Status:** " + translateStatus(resolveName(obj, "status"))}
	if assigned := redactName(resolveName(obj, "assigned_to")); assigned != "" {
		badges = append(badges, "**Responsável:** "+assigned)
	}
	if project := resolveName(obj, "project"); project != "" {
		badges = append(badges, "**Projeto:** "+project)
	}
	if priority := resolveFieldValue(obj, "priority", priorities); priority != "" {
		badges = append(badges, "**Prioridade:** "+priority)
	}
	if severity := resolveFieldValue(obj, "severity", severities); severity != "" {
		badges = append(badges, "**Severidade:** "+severity)
	}
	b.WriteString(strings.Join(badges, "  |  "))
	b.WriteString("\n\n")

	meta := []string{}
	addMeta := func(label, key string) {
		if v := displayValue(obj[key]); v != "" {
			meta = append(meta, fmt.Sprintf("- **%s:** %s", label, v))
		}
	}
	addMeta("Criado em", "created_date")
	addMeta("Modificado em", "modified_date")
	addMeta("Fechado?", "is_closed")
	if tags := inlineList(obj["tags"]); tags != "" {
		meta = append(meta, "- **Tags:** "+tags)
	}
	if len(meta) > 0 {
		b.WriteString(strings.Join(meta, "\n"))
		b.WriteString("\n\n")
	}

	if desc := strings.TrimSpace(redactText(displayValue(obj["description"]))); desc != "" {
		b.WriteString("### Descrição\n\n")
		b.WriteString(desc)
		b.WriteString("\n\n")
	}

	counts := []string{}
	if a := displayInt(obj["total_attachments"]); a > 0 {
		counts = append(counts, fmt.Sprintf("📎 %s anexos", formatInt(a)))
	}
	if c := displayInt(obj["total_comments"]); c > 0 {
		counts = append(counts, fmt.Sprintf("💬 %s comentários", formatInt(c)))
	}
	if w := displayInt(obj["total_watchers"]); w > 0 {
		counts = append(counts, fmt.Sprintf("👁 %s observadores", formatInt(w)))
	}
	if len(counts) > 0 {
		b.WriteString(strings.Join(counts, "  "))
		b.WriteString("\n")
	}
	if linked := formatLinkedCards(obj); linked != "" {
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
	if len(attachments) > 0 {
		b.WriteString("\n### Anexos\n\n")
		listed := attachments
		if len(listed) > 50 {
			listed = listed[len(listed)-50:]
		}
		b.WriteString("\n")
		for _, name := range listed {
			fmt.Fprintf(&b, "%s\n", name)
		}
	}
	if len(comments) > 0 {
		fmt.Fprintf(&b, "\n### Comentários (%d)\n\n", len(comments))
		for _, c := range comments {
			who := redactName(c.author)
			if who == "" {
				who = "—"
			}
			when := ""
			if c.date != "" {
				when = " · " + c.date
			}
			fmt.Fprintf(&b, "**%s**%s\n\n%s\n\n", who, when, redactText(stripHTML(c.text)))
		}
	}
	return b.String()
}

func formatTaskItem(data any, comments []commentView, attachments []string) string {
	obj, ok := data.(map[string]any)
	if !ok {
		return singleValue(data)
	}
	var b strings.Builder
	if parent := linkedStory(obj); parent != "—" {
		b.WriteString("### Card vinculado\n\n")
		fmt.Fprintf(&b, "- %s\n\n", parent)
	}
	b.WriteString(formatDetailedItem(data, comments, attachments, nil, nil, nil))
	return b.String()
}

func linkedStory(obj map[string]any) string {
	extra, ok := obj["user_story_extra_info"].(map[string]any)
	if !ok || len(extra) == 0 {
		return "—"
	}
	ref := displayValue(extra["ref"])
	subject := redactText(displayValue(extra["subject"]))
	switch {
	case ref != "" && subject != "":
		return ref + " — " + subject
	case ref != "":
		return ref
	case subject != "":
		return subject
	}
	return "—"
}

func formatSubtasks(obj map[string]any) string {
	value, ok := obj["tasks"]
	if !ok {
		return ""
	}
	items, ok := value.([]any)
	if !ok || len(items) == 0 {
		return ""
	}
	lines := make([]string, 0, len(items))
	for _, item := range items {
		task, ok := item.(map[string]any)
		if !ok {
			continue
		}
		id := displayValue(task["id"])
		ref := displayValue(task["ref"])
		subject := redactText(displayValue(task["subject"]))
		if subject == "" {
			subject = redactText(displayValue(task["name"]))
		}
		if subject == "" {
			continue
		}
		prefix := ""
		if ref != "" {
			prefix = "ref " + ref
		}
		if id != "" {
			if prefix != "" {
				prefix += " · "
			}
			prefix += "id " + id
		}
		status := translateStatus(resolveName(task, "status"))
		assigned := redactName(resolveName(task, "assigned_to"))
		line := "- "
		if prefix != "" {
			line += prefix + " — "
		}
		line += subject
		if status != "" {
			line += " — " + status
		}
		if assigned != "" {
			line += " — " + assigned
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func formatLinkedCards(obj map[string]any) string {
	for _, key := range []string{"subcards", "sub_cards", "children", "related_userstories", "related_cards"} {
		items, ok := obj[key].([]any)
		if !ok || len(items) == 0 {
			continue
		}
		lines := make([]string, 0, len(items))
		for _, item := range items {
			m, ok := item.(map[string]any)
			if !ok {
				continue
			}
			id := displayValue(m["id"])
			ref := displayValue(m["ref"])
			subject := redactText(displayValue(m["subject"]))
			if subject == "" {
				subject = redactText(displayValue(m["name"]))
			}
			if subject == "" {
				continue
			}
			prefix := ""
			if ref != "" {
				prefix = "ref " + ref
			}
			if id != "" {
				if prefix != "" {
					prefix += " · "
				}
				prefix += "id " + id
			}
			if prefix != "" {
				prefix += " — "
			}
			lines = append(lines, "- "+prefix+subject)
		}
		if len(lines) > 0 {
			return strings.Join(lines, "\n")
		}
	}
	return ""
}

func resolveName(obj map[string]any, base string) string {
	extra, ok := obj[base+"_extra_info"]
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
			if v, ok := typed[key].(string); ok && strings.TrimSpace(v) != "" {
				return strings.TrimSpace(v)
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

		"closed":   "Fechado",
		"done":     "Concluído",
		"archived": "Arquivado",
		"rejected": "Rejeitado",
		"blocked":  "Bloqueado",
	}
	if translated, ok := translations[strings.ToLower(strings.TrimSpace(status))]; ok {
		return translated
	}
	return status
}

func stripHTML(s string) string {
	re := regexp.MustCompile(`(?i)<br\s*/?>|</p>|</li>`)
	s = re.ReplaceAllString(s, "\n")
	re = regexp.MustCompile(`(?i)<[^>]+>`)
	s = re.ReplaceAllString(s, "")
	re = regexp.MustCompile(`!\[[^\]]*\]\([^)]*\)`)
	s = re.ReplaceAllString(s, "[IMAGEM]")
	return strings.TrimSpace(s)
}

func redactText(s string) string {
	if !redactEnabled {
		return s
	}
	re := regexp.MustCompile(`(?i)[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}`)
	s = re.ReplaceAllString(s, "[email]")
	re = regexp.MustCompile(`\b\d{3}\.?\d{3}\.?\d{3}-?\d{2}\b`)
	s = re.ReplaceAllString(s, "[cpf]")
	re = regexp.MustCompile(`\(?\d{2}\)?[\s-]\d{4,5}[\s-]?\d{4}`)
	s = re.ReplaceAllString(s, "[telefone]")
	re = regexp.MustCompile(`https?://[^\s)>\]]+`)
	s = re.ReplaceAllString(s, "[url]")
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
	return parts[0] + " " + strings.ToUpper(string(r[0]))
}

func resolveFieldValue(obj map[string]any, base string, lookup map[int]string) string {
	name := resolveName(obj, base)
	if lookup != nil {
		if id := displayInt(obj[base]); id > 0 {
			if mapped, ok := lookup[id]; ok && mapped != "" {
				return mapped
			}
		}
	}
	return name
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
		if typed {
			return "sim"
		}
		return "não"
	case float64:
		if typed == float64(int64(typed)) {
			return strconv.Itoa(int(typed))
		}
		return strconv.FormatFloat(typed, 'f', 2, 64)
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

func displayBool(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case float64:
		return typed != 0
	case string:
		return typed == "true" || typed == "True" || typed == "1"
	default:
		return false
	}
}

func displayInt(value any) int {
	switch typed := value.(type) {
	case float64:
		return int(typed)
	case int:
		return typed
	case string:
		if n, err := strconv.Atoi(strings.TrimSpace(typed)); err == nil {
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
	return "```json\n" + fmt.Sprintf("%v", value) + "\n```"
}

func formatInt(n int) string {
	neg := n < 0
	if neg {
		n = -n
	}
	s := strconv.Itoa(n)
	var out strings.Builder
	for i, r := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			out.WriteByte('.')
		}
		out.WriteRune(r)
	}
	result := out.String()
	if neg {
		return "-" + result
	}
	return result
}

func formatDownload(result taiga.DownloadResult) string {
	var b strings.Builder
	b.WriteString("Anexo salvo com sucesso ✅\n\n")
	fmt.Fprintf(&b, "- **Caminho**: `%s`\n", result.Path)
	fmt.Fprintf(&b, "- **Tamanho**: %s\n", formatFileSize(result.Bytes))
	if result.ContentType != "" {
		fmt.Fprintf(&b, "- **Tipo**: %s\n", result.ContentType)
	}
	return b.String()
}

func formatFileSize(bytes int64) string {
	if bytes >= 1024*1024 {
		return fmt.Sprintf("%.2f MB", float64(bytes)/(1024*1024))
	}
	if bytes >= 1024 {
		return fmt.Sprintf("%.1f KB", float64(bytes)/1024)
	}
	return fmt.Sprintf("%d bytes", bytes)
}

func formatPagination(meta taiga.ResponseMeta) string {
	parts := []string{}
	if meta.Total != "" {
		parts = append(parts, "total de "+meta.Total+" registros")
	}
	if meta.CurrentPage != "" {
		parts = append(parts, "página "+meta.CurrentPage)
		if meta.PaginatedBy != "" {
			parts[len(parts)-1] += " (" + meta.PaginatedBy + " por página)"
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return "📄 Paginação: " + strings.Join(parts, ", ") + "."
}
