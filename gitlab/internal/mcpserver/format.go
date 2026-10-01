package mcpserver

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

func clean(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "|", "/")
	s = strings.ReplaceAll(s, "\t", " ")
	for strings.Contains(s, "  ") {
		s = strings.ReplaceAll(s, "  ", " ")
	}
	return strings.TrimSpace(s)
}

func toStr(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(t)
	case bool:
		if t {
			return "sim"
		}
		return "não"
	case float64:
		if t == float64(int64(t)) {
			return strconv.Itoa(int(t))
		}
		return strconv.FormatFloat(t, 'f', 2, 64)
	case int:
		return strconv.Itoa(t)
	case []any:
		parts := make([]string, 0, len(t))
		for _, x := range t {
			if s := toStr(x); s != "" {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, ", ")
	case map[string]any:
		for _, k := range []string{"name", "username", "title", "ref", "path", "path_with_namespace", "message", "description"} {
			if s, ok := t[k].(string); ok && s != "" {
				return strings.TrimSpace(s)
			}
		}
		return ""
	default:
		return fmt.Sprintf("%v", v)
	}
}

func toInt(v any) int {
	switch t := v.(type) {
	case float64:
		return int(t)
	case int:
		return t
	case string:
		if n, err := strconv.Atoi(strings.TrimSpace(t)); err == nil {
			return n
		}
		return 0
	default:
		return 0
	}
}

func userName(m map[string]any) string {
	for _, k := range []string{"username", "name", "email"} {
		if v, ok := m[k].(string); ok && v != "" {
			return v
		}
	}
	return ""
}

func nestedUser(m map[string]any, key string) string {
	if v, ok := m[key].(map[string]any); ok {
		return userName(v)
	}
	return ""
}

func usersList(m map[string]any, key string) string {
	if arr, ok := m[key].([]any); ok && len(arr) > 0 {
		parts := make([]string, 0, len(arr))
		for _, it := range arr {
			if um, ok := it.(map[string]any); ok {
				if n := userName(um); n != "" {
					parts = append(parts, n)
				}
			}
		}
		return strings.Join(parts, ", ")
	}
	if v, ok := m[strings.TrimSuffix(key, "ees")].(map[string]any); ok {
		return userName(v)
	}
	return ""
}

func firstOf(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			if s := toStr(v); s != "" {
				return s
			}
		}
	}
	return ""
}

func gitlabDate(value string) string {
	s := strings.TrimSpace(value)
	if s == "" {
		return ""
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05Z07:00", "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.Format("02/01/2006 15:04")
		}
	}
	return s
}

func accessLevelName(level int) string {
	switch level {
	case 10:
		return "Guest"
	case 20:
		return "Reporter"
	case 30:
		return "Developer"
	case 40:
		return "Maintainer"
	case 50:
		return "Owner"
	default:
		return strconv.Itoa(level)
	}
}

func shortSHA(sha string) string {
	sha = strings.TrimSpace(sha)
	if len(sha) > 8 {
		return sha[:8]
	}
	return sha
}

func formatDuration(v any) string {
	switch t := v.(type) {
	case float64:
		return humanDuration(t)
	case int:
		return humanDuration(float64(t))
	default:
		return ""
	}
}

func humanDuration(secs float64) string {
	if secs <= 0 {
		return ""
	}
	if secs < 60 {
		return fmt.Sprintf("%.0fs", secs)
	}
	if secs < 3600 {
		m := int(secs) / 60
		s := int(secs) % 60
		if s == 0 {
			return fmt.Sprintf("%dm", m)
		}
		return fmt.Sprintf("%dm%ds", m, s)
	}
	h := int(secs) / 3600
	m := (int(secs) % 3600) / 60
	if m == 0 {
		return fmt.Sprintf("%dh", h)
	}
	return fmt.Sprintf("%dh%dm", h, m)
}

func statusEmoji(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "success", "passed":
		return "✅ success"
	case "running":
		return "🔄 **running**"
	case "created":
		return "⏸ created"
	case "pending":
		return "⏳ pending"
	case "failed":
		return "❌ failed"
	case "canceled", "cancelled":
		return "⏹ canceled"
	case "skipped":
		return "⏭️ skipped"
	case "manual":
		return "🖐 manual"
	case "waiting_for_resource":
		return "⏳ waiting"
	case "preparing":
		return "⚙️ preparing"
	case "scheduled":
		return "🗓 scheduled"
	default:
		return status
	}
}

func jobDuration(v any, status string) string {
	secs := 0.0
	switch t := v.(type) {
	case float64:
		secs = t
	case int:
		secs = float64(t)
	}
	if secs <= 0 {
		if strings.EqualFold(strings.TrimSpace(status), "running") {
			return "…"
		}
		return "—"
	}
	label := humanDuration(secs)
	if strings.EqualFold(strings.TrimSpace(status), "running") {
		return "~" + label
	}
	return label
}

func markdownTable(headers []string, rows [][]string) string {
	if len(rows) == 0 {
		return "Nenhum item encontrado."
	}
	var b strings.Builder
	b.WriteString("| ")
	b.WriteString(strings.Join(headers, " | "))
	b.WriteString(" |\n")
	seps := make([]string, len(headers))
	for i := range seps {
		seps[i] = "---"
	}
	b.WriteString("| ")
	b.WriteString(strings.Join(seps, " | "))
	b.WriteString(" |\n")
	for _, row := range rows {
		cells := make([]string, len(headers))
		for i := range headers {
			if i < len(row) {
				cells[i] = clean(row[i])
			}
		}
		b.WriteString("| ")
		b.WriteString(strings.Join(cells, " | "))
		b.WriteString(" |\n")
	}
	return b.String()
}

func mapItems(items []any) []map[string]any {
	out := make([]map[string]any, 0, len(items))
	for _, it := range items {
		if m, ok := it.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

func formatProjectsTable(items []any) string {
	entries := mapItems(items)
	if len(entries) == 0 {
		return "Nenhum projeto encontrado."
	}
	var b strings.Builder
	for _, m := range entries {
		path := clean(toStr(m["path_with_namespace"]))
		url := toStr(m["web_url"])
		if url != "" {
			fmt.Fprintf(&b, "### [%s](%s)\n", path, url)
		} else {
			fmt.Fprintf(&b, "### %s\n", path)
		}
		pieces := []string{}
		if s := toStr(m["star_count"]); s != "" && s != "0" {
			pieces = append(pieces, "⭐ "+s)
		}
		if f := toStr(m["forks_count"]); f != "" && f != "0" {
			pieces = append(pieces, "🍴 "+f)
		}
		if v := toStr(m["visibility"]); v != "" {
			pieces = append(pieces, "🔒 "+v)
		}
		if d := gitlabDate(toStr(m["last_activity_at"])); d != "" {
			pieces = append(pieces, "🕓 "+d)
		}
		b.WriteString(metaLine(pieces...))
		if db := toStr(m["default_branch"]); db != "" {
			fmt.Fprintf(&b, "🔖 `%s`\n", db)
		}
		if d := clean(toStr(m["description"])); d != "" {
			fmt.Fprintf(&b, "%s\n", truncate(d, 400))
		}
		b.WriteString("\n")
	}
	return strings.TrimSpace(b.String())
}

func formatUsersTable(items []any) string {
	entries := mapItems(items)
	if len(entries) == 0 {
		return "Nenhum usuário encontrado."
	}
	var b strings.Builder
	for _, m := range entries {
		username := clean(toStr(m["username"]))
		url := toStr(m["web_url"])
		if url != "" {
			fmt.Fprintf(&b, "### [@%s](%s)\n", username, url)
		} else {
			fmt.Fprintf(&b, "### @%s\n", username)
		}
		pieces := []string{}
		if n := toStr(m["name"]); n != "" && n != username {
			pieces = append(pieces, "👤 "+n)
		}
		if s := toStr(m["state"]); s != "" {
			pieces = append(pieces, "📌 "+s)
		}
		b.WriteString(metaLine(pieces...))
		b.WriteString("\n")
	}
	return strings.TrimSpace(b.String())
}

func formatIssuesTable(items []any) string {
	entries := mapItems(items)
	if len(entries) == 0 {
		return "Nenhuma issue encontrada."
	}
	var b strings.Builder
	for _, m := range entries {
		num := toStr(m["iid"])
		title := clean(toStr(m["title"]))
		url := toStr(m["web_url"])
		if url != "" {
			fmt.Fprintf(&b, "### [#%s](%s) %s\n", num, url, title)
		} else {
			fmt.Fprintf(&b, "### #%s %s\n", num, title)
		}
		state := clean(toStr(m["state"]))
		stateIcon := "🟢"
		if state == "closed" {
			stateIcon = "🔴"
		}
		pieces := []string{stateIcon + " " + state}
		if a := nestedUser(m, "author"); a != "" {
			pieces = append(pieces, "👤 "+a)
		}
		if lb := toStr(m["labels"]); lb != "" {
			pieces = append(pieces, "🏷️ "+lb)
		}
		if u := gitlabDate(toStr(m["updated_at"])); u != "" {
			pieces = append(pieces, "🕓 "+u)
		}
		b.WriteString(metaLine(pieces...))
		if d := clean(toStr(m["description"])); d != "" {
			fmt.Fprintf(&b, "> %s\n", truncate(d, 300))
		}
		b.WriteString("\n")
	}
	return strings.TrimSpace(b.String())
}

func formatMergeRequestsTable(items []any) string {
	entries := mapItems(items)
	if len(entries) == 0 {
		return "Nenhum merge request encontrado."
	}
	var b strings.Builder
	for _, m := range entries {
		num := toStr(m["iid"])
		title := clean(toStr(m["title"]))
		url := toStr(m["web_url"])
		if url != "" {
			fmt.Fprintf(&b, "### [!%s](%s) %s\n", num, url, title)
		} else {
			fmt.Fprintf(&b, "### !%s %s\n", num, title)
		}
		state := clean(toStr(m["state"]))
		stateIcon := "🟢"
		switch state {
		case "merged":
			stateIcon = "🟣"
		case "closed":
			stateIcon = "🔴"
		}
		pieces := []string{stateIcon + " " + state}
		if a := nestedUser(m, "author"); a != "" {
			pieces = append(pieces, "👤 "+a)
		}
		if sb := toStr(m["source_branch"]); sb != "" {
			pieces = append(pieces, "🔀 `"+sb+"` → `"+toStr(m["target_branch"])+"`")
		}
		if u := gitlabDate(toStr(m["updated_at"])); u != "" {
			pieces = append(pieces, "🕓 "+u)
		}
		b.WriteString(metaLine(pieces...))
		if d := clean(toStr(m["description"])); d != "" {
			fmt.Fprintf(&b, "> %s\n", truncate(d, 300))
		}
		b.WriteString("\n")
	}
	return strings.TrimSpace(b.String())
}

func formatPipelinesTable(items []any) string {
	headers := []string{"ID", "Status", "Branch", "SHA", "Origem", "Autor", "Criado em"}
	rows := [][]string{}
	for _, m := range mapItems(items) {
		rows = append(rows, []string{
			link(toStr(m["id"]), toStr(m["web_url"])),
			statusEmoji(clean(toStr(m["status"]))),
			"`" + clean(toStr(m["ref"])) + "`",
			"`" + shortSHA(toStr(m["sha"])) + "`",
			clean(toStr(m["source"])),
			nestedUser(m, "user"),
			gitlabDate(toStr(m["created_at"])),
		})
	}
	return markdownTable(headers, rows)
}

func formatBranchesTable(items []any) string {
	headers := []string{"Branch", "Padrão", "Protegida", "Commit", "Mensagem", "Data"}
	rows := [][]string{}
	for _, m := range mapItems(items) {
		var commitDate, commitMsg, commitSHA string
		if cm, ok := m["commit"].(map[string]any); ok {
			commitSHA = shortSHA(toStr(cm["id"]))
			commitMsg = clean(toStr(cm["title"]))
			commitDate = gitlabDate(toStr(cm["committed_date"]))
			if commitDate == "" {
				commitDate = gitlabDate(toStr(cm["authored_date"]))
			}
		}
		rows = append(rows, []string{
			"`" + clean(toStr(m["name"])) + "`",
			boolMark(toStr(m["default"]) == "sim"),
			boolMark(toStr(m["protected"]) == "sim"),
			"`" + commitSHA + "`",
			truncate(commitMsg, 60),
			commitDate,
		})
	}
	return markdownTable(headers, rows)
}

func formatCommitsTable(items []any) string {
	entries := mapItems(items)
	if len(entries) == 0 {
		return "Nenhum commit encontrado."
	}
	var b strings.Builder
	for _, m := range entries {
		sha := shortSHA(toStr(m["id"]))
		if s := toStr(m["short_id"]); s != "" {
			sha = s
		}
		title := clean(toStr(m["title"]))
		url := toStr(m["web_url"])
		if url != "" {
			fmt.Fprintf(&b, "### [`%s`](%s) %s\n", sha, url, title)
		} else {
			fmt.Fprintf(&b, "### `%s` %s\n", sha, title)
		}
		pieces := []string{}
		if a := toStr(m["author_name"]); a != "" {
			pieces = append(pieces, "👤 "+a)
		}
		if d := gitlabDate(toStr(m["authored_date"])); d != "" {
			pieces = append(pieces, "🕓 "+d)
		}
		b.WriteString(metaLine(pieces...))
		if msg := clean(toStr(m["message"])); msg != "" {
			fmt.Fprintf(&b, "> %s\n", truncate(msg, 300))
		}
		b.WriteString("\n")
	}
	return strings.TrimSpace(b.String())
}

func formatMembersTable(items []any) string {
	headers := []string{"Usuário", "Nome", "Estado", "Nível"}
	rows := [][]string{}
	for _, m := range mapItems(items) {
		rows = append(rows, []string{
			"`" + clean(toStr(m["username"])) + "`",
			clean(toStr(m["name"])),
			clean(toStr(m["state"])),
			accessLevelName(toInt(m["access_level"])),
		})
	}
	return markdownTable(headers, rows)
}

func formatTagsTable(items []any) string {
	headers := []string{"Tag", "Commit", "Mensagem", "Protegida"}
	rows := [][]string{}
	for _, m := range mapItems(items) {
		var sha, msg string
		if cm, ok := m["commit"].(map[string]any); ok {
			sha = shortSHA(toStr(cm["short_id"]))
			if sha == "" {
				sha = shortSHA(toStr(cm["id"]))
			}
			msg = clean(toStr(cm["title"]))
		}
		rows = append(rows, []string{
			"`" + clean(toStr(m["name"])) + "`",
			"`" + sha + "`",
			truncate(msg, 60),
			boolMark(toStr(m["protected"]) == "sim"),
		})
	}
	return markdownTable(headers, rows)
}

func formatJobsTable(items []any) string {
	headers := []string{"Job", "Stage", "Status", "Duração"}
	rows := [][]string{}
	for _, m := range mapItems(items) {
		status := clean(toStr(m["status"]))
		rows = append(rows, []string{
			"`" + clean(toStr(m["name"])) + "`",
			clean(toStr(m["stage"])),
			statusEmoji(status),
			jobDuration(m["duration"], status),
		})
	}
	return markdownTable(headers, rows)
}

func formatGeneric(items []any) string {
	headers := []string{"ID", "Resumo"}
	rows := [][]string{}
	for _, m := range mapItems(items) {
		rows = append(rows, []string{
			toStr(m["id"]),
			clean(firstOf(m, "title", "name", "message", "path")),
		})
	}
	return markdownTable(headers, rows)
}

func formatSearchTable(scope string, items []any) string {
	switch strings.ToLower(strings.TrimSpace(scope)) {
	case "projects":
		return formatProjectsTable(items)
	case "issues":
		return formatIssuesTable(items)
	case "merge_requests":
		return formatMergeRequestsTable(items)
	case "users":
		return formatUsersTable(items)
	case "commits":
		return formatCommitsTable(items)
	default:
		return formatGeneric(items)
	}
}

type fileTreeNode struct {
	name     string
	isDir    bool
	children map[string]*fileTreeNode
}

func newFileTreeRoot() *fileTreeNode {
	return &fileTreeNode{name: "", children: map[string]*fileTreeNode{}}
}

func (n *fileTreeNode) child(name string) *fileTreeNode {
	if c, ok := n.children[name]; ok {
		return c
	}
	c := &fileTreeNode{name: name, children: map[string]*fileTreeNode{}}
	n.children[name] = c
	return c
}

func countNodes(n *fileTreeNode) int {
	c := 0
	for _, ch := range n.children {
		c += 1 + countNodes(ch)
	}
	return c
}

func formatTreeTable(items []any) string {
	entries := mapItems(items)
	if len(entries) == 0 {
		return "Nenhum item encontrado."
	}
	root := newFileTreeRoot()
	dirs, files := 0, 0
	for _, m := range entries {
		path := toStr(m["path"])
		typ := toStr(m["type"])
		if path == "" {
			continue
		}
		parts := strings.Split(path, "/")
		cur := root
		for i, p := range parts {
			cur = cur.child(p)
			if i < len(parts)-1 || typ == "tree" {
				cur.isDir = true
			}
		}
		if typ == "tree" {
			dirs++
		} else {
			files++
		}
	}
	const maxTreeLines = 400
	var lines []string
	counter := 0
	var walk func(n *fileTreeNode, prefix string)
	walk = func(n *fileTreeNode, prefix string) {
		if counter >= maxTreeLines {
			return
		}
		names := make([]string, 0, len(n.children))
		for k := range n.children {
			names = append(names, k)
		}
		sort.Slice(names, func(i, j int) bool {
			ci, cj := n.children[names[i]], n.children[names[j]]
			if ci.isDir != cj.isDir {
				return ci.isDir
			}
			return names[i] < names[j]
		})
		for i, name := range names {
			if counter >= maxTreeLines {
				return
			}
			last := i == len(names)-1
			connector := "├── "
			if last {
				connector = "└── "
			}
			label := name
			if n.children[name].isDir {
				label += "/"
			}
			lines = append(lines, prefix+connector+label)
			counter++
			childPrefix := prefix
			if last {
				childPrefix += "    "
			} else {
				childPrefix += "│   "
			}
			walk(n.children[name], childPrefix)
		}
	}
	walk(root, "")

	var b strings.Builder
	fmt.Fprintf(&b, "📊 %d entradas — %d diretórios, %d arquivos\n\n```\n", len(entries), dirs, files)
	b.WriteString(strings.Join(lines, "\n"))
	if countNodes(root) > counter {
		fmt.Fprintf(&b, "\n… lista truncada em %d linhas. Use path/ref mais específico ou recursive=true para ver mais.\n", maxTreeLines)
	}
	b.WriteString("\n```")
	return strings.TrimSpace(b.String())
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	r := []rune(s)
	if len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}

func orMissing(v any) string {
	s := toStr(v)
	if s == "" {
		return "—"
	}
	return s
}

func metaLine(pieces ...string) string {
	parts := make([]string, 0, len(pieces))
	for _, p := range pieces {
		if strings.TrimSpace(p) != "" {
			parts = append(parts, strings.TrimSpace(p))
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, " · ") + "\n"
}

func langFromPath(path string) string {
	idx := strings.LastIndex(path, ".")
	if idx >= 0 {
		path = path[idx+1:]
	}
	switch strings.ToLower(path) {
	case "rs":
		return "rust"
	case "go":
		return "go"
	case "py", "pyi":
		return "python"
	case "js", "mjs", "cjs":
		return "javascript"
	case "ts":
		return "typescript"
	case "tsx", "jsx":
		return "tsx"
	case "json":
		return "json"
	case "yml", "yaml":
		return "yaml"
	case "toml":
		return "toml"
	case "md", "markdown":
		return "markdown"
	case "sh", "bash", "zsh":
		return "bash"
	case "c", "h":
		return "c"
	case "cpp", "cc", "cxx", "hpp":
		return "cpp"
	case "java":
		return "java"
	case "rb":
		return "ruby"
	case "php":
		return "php"
	case "cs":
		return "csharp"
	case "swift":
		return "swift"
	case "kt", "kts":
		return "kotlin"
	case "sql":
		return "sql"
	case "html", "htm":
		return "html"
	case "css", "scss", "sass":
		return "css"
	case "xml":
		return "xml"
	case "proto":
		return "protobuf"
	default:
		return ""
	}
}

func formatDetailItem(entity string, raw any) string {
	m, ok := raw.(map[string]any)
	if !ok {
		return "Item não encontrado."
	}
	switch strings.ToLower(strings.TrimSpace(entity)) {
	case "project", "projects":
		return formatProjectDetail(m)
	case "issue", "issues":
		return formatIssueDetail(m)
	case "merge_request", "merge_requests", "mr":
		return formatMRDetail(m)
	case "commit", "commits":
		return formatCommitDetail(m)
	case "branch", "branches":
		return formatBranchDetail(m)
	case "tag", "tags":
		return formatTagDetail(m)
	case "pipeline", "pipelines":
		return formatPipelineDetail(m)
	case "job", "jobs":
		return formatJobDetail(m)
	case "member", "members":
		return formatMemberDetail(m)
	case "user", "users":
		return formatUserDetail(m)
	default:
		return formatGenericDetail(m)
	}
}

func formatProjectDetail(m map[string]any) string {
	var b strings.Builder
	path := clean(toStr(m["path_with_namespace"]))
	url := toStr(m["web_url"])
	if url != "" {
		fmt.Fprintf(&b, "### [%s](%s)\n", path, url)
	} else {
		fmt.Fprintf(&b, "### %s\n", path)
	}
	pieces := []string{}
	if s := toStr(m["star_count"]); s != "" && s != "0" {
		pieces = append(pieces, "⭐ "+s)
	}
	if f := toStr(m["forks_count"]); f != "" && f != "0" {
		pieces = append(pieces, "🍴 "+f)
	}
	if v := toStr(m["visibility"]); v != "" {
		pieces = append(pieces, "🔒 "+v)
	}
	if a := gitlabDate(toStr(m["last_activity_at"])); a != "" {
		pieces = append(pieces, "🕓 "+a)
	}
	b.WriteString(metaLine(pieces...))
	fmt.Fprintf(&b, "- **Branch padrão:** %s\n", orMissing(m["default_branch"]))
	if o := toStr(m["open_issues_count"]); o != "" && o != "0" {
		fmt.Fprintf(&b, "- **Issues abertas:** %s\n", o)
	}
	if topics, ok := m["topics"].([]any); ok && len(topics) > 0 {
		parts := make([]string, 0, len(topics))
		for _, tp := range topics {
			parts = append(parts, toStr(tp))
		}
		fmt.Fprintf(&b, "- **Topics:** %s\n", strings.Join(parts, ", "))
	}
	fmt.Fprintf(&b, "- **Criado:** %s\n", orMissing(gitlabDate(toStr(m["created_at"]))))
	if d := toStr(m["description"]); d != "" {
		fmt.Fprintf(&b, "\n%s\n", d)
	}
	return strings.TrimSpace(b.String())
}

func formatIssueDetail(m map[string]any) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Issue #%s — %s\n", toStr(m["iid"]), clean(toStr(m["title"])))
	if url := toStr(m["web_url"]); url != "" {
		fmt.Fprintf(&b, "[abrir](%s)\n\n", url)
	}
	fmt.Fprintf(&b, "- **Estado:** %s\n", toStr(m["state"]))
	fmt.Fprintf(&b, "- **Autor:** %s\n", orMissing(nestedUser(m, "author")))
	if a := usersList(m, "assignees"); a != "" {
		fmt.Fprintf(&b, "- **Responsáveis:** %s\n", a)
	}
	if lb := toStr(m["labels"]); lb != "" {
		fmt.Fprintf(&b, "- **Labels:** %s\n", lb)
	}
	fmt.Fprintf(&b, "- **Criada:** %s\n", orMissing(gitlabDate(toStr(m["created_at"]))))
	if u := toStr(m["updated_at"]); u != "" {
		fmt.Fprintf(&b, "- **Atualizada:** %s\n", gitlabDate(u))
	}
	if c := toStr(m["user_notes_count"]); c != "" && c != "0" {
		fmt.Fprintf(&b, "- **Comentários:** %s\n", c)
	}
	if body := toStr(m["description"]); body != "" {
		fmt.Fprintf(&b, "\n---\n%s\n", body)
	}
	return strings.TrimSpace(b.String())
}

func formatMRDetail(m map[string]any) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## MR !%s — %s\n", toStr(m["iid"]), clean(toStr(m["title"])))
	if url := toStr(m["web_url"]); url != "" {
		fmt.Fprintf(&b, "[abrir](%s)\n\n", url)
	}
	state := toStr(m["state"])
	if toStr(m["merged_at"]) != "" {
		state = "merged"
	}
	fmt.Fprintf(&b, "- **Estado:** %s\n", state)
	fmt.Fprintf(&b, "- **Autor:** %s\n", orMissing(nestedUser(m, "author")))
	fmt.Fprintf(&b, "- **De:** `%s` → **Para:** `%s`\n", orMissing(m["source_branch"]), orMissing(m["target_branch"]))
	if a := usersList(m, "assignees"); a != "" {
		fmt.Fprintf(&b, "- **Responsáveis:** %s\n", a)
	}
	if lb := toStr(m["labels"]); lb != "" {
		fmt.Fprintf(&b, "- **Labels:** %s\n", lb)
	}
	if u := toStr(m["updated_at"]); u != "" {
		fmt.Fprintf(&b, "- **Atualizada:** %s\n", gitlabDate(u))
	}
	if body := toStr(m["description"]); body != "" {
		fmt.Fprintf(&b, "\n---\n%s\n", body)
	}
	return strings.TrimSpace(b.String())
}

func formatCommitDetail(m map[string]any) string {
	var b strings.Builder
	sha := toStr(m["id"])
	short := toStr(m["short_id"])
	title := clean(toStr(m["title"]))
	url := toStr(m["web_url"])
	label := sha
	if short != "" {
		label = short
	}
	if url != "" {
		fmt.Fprintf(&b, "### [`%s`](%s) %s\n", label, url, title)
	} else {
		fmt.Fprintf(&b, "### `%s` %s\n", label, title)
	}
	pieces := []string{}
	if a := toStr(m["author_name"]); a != "" {
		pieces = append(pieces, "👤 "+a)
	}
	if d := gitlabDate(toStr(m["committed_date"])); d != "" {
		pieces = append(pieces, "🕓 "+d)
	}
	b.WriteString(metaLine(pieces...))
	if msg := toStr(m["message"]); msg != "" {
		fmt.Fprintf(&b, "```\n%s\n```\n", msg)
	}
	return strings.TrimSpace(b.String())
}

func formatBranchDetail(m map[string]any) string {
	var b strings.Builder
	name := clean(toStr(m["name"]))
	if url := toStr(m["web_url"]); url != "" {
		fmt.Fprintf(&b, "### [%s](%s)\n", name, url)
	} else {
		fmt.Fprintf(&b, "### %s\n", name)
	}
	pieces := []string{}
	if toStr(m["default"]) == "sim" {
		pieces = append(pieces, "🏷️ padrão")
	}
	if toStr(m["protected"]) == "sim" {
		pieces = append(pieces, "🔒 protegida")
	}
	b.WriteString(metaLine(pieces...))
	if cm, ok := m["commit"].(map[string]any); ok {
		fmt.Fprintf(&b, "- **Commit:** `%s` — %s\n", orMissing(cm["id"]), clean(toStr(cm["title"])))
		if d := gitlabDate(toStr(cm["committed_date"])); d != "" {
			fmt.Fprintf(&b, "- **Data:** %s\n", d)
		}
	}
	return strings.TrimSpace(b.String())
}

func formatTagDetail(m map[string]any) string {
	var b strings.Builder
	name := clean(toStr(m["name"]))
	if url := toStr(m["web_url"]); url != "" {
		fmt.Fprintf(&b, "### [%s](%s)\n", name, url)
	} else {
		fmt.Fprintf(&b, "### %s\n", name)
	}
	if toStr(m["protected"]) == "sim" {
		fmt.Fprintf(&b, "- **Protegida:** sim\n")
	}
	if cm, ok := m["commit"].(map[string]any); ok {
		fmt.Fprintf(&b, "- **Commit:** `%s` — %s\n", orMissing(cm["id"]), clean(toStr(cm["title"])))
	}
	if msg := toStr(m["message"]); msg != "" {
		fmt.Fprintf(&b, "\n```\n%s\n```\n", msg)
	}
	return strings.TrimSpace(b.String())
}

func formatPipelineDetail(m map[string]any) string {
	var b strings.Builder
	id := toStr(m["id"])
	if url := toStr(m["web_url"]); url != "" {
		fmt.Fprintf(&b, "### Pipeline #%s\n[abrir](%s)\n\n", id, url)
	} else {
		fmt.Fprintf(&b, "### Pipeline #%s\n\n", id)
	}
	pieces := []string{statusEmoji(clean(toStr(m["status"])))}
	if ref := toStr(m["ref"]); ref != "" {
		pieces = append(pieces, "🔖 "+ref)
	}
	if sha := toStr(m["sha"]); sha != "" {
		pieces = append(pieces, "`"+shortSHA(sha)+"`")
	}
	if u := nestedUser(m, "user"); u != "" {
		pieces = append(pieces, "👤 "+u)
	}
	if d := gitlabDate(toStr(m["created_at"])); d != "" {
		pieces = append(pieces, "🕓 "+d)
	}
	b.WriteString(metaLine(pieces...))
	return strings.TrimSpace(b.String())
}

func formatJobDetail(m map[string]any) string {
	var b strings.Builder
	id := toStr(m["id"])
	name := clean(toStr(m["name"]))
	if url := toStr(m["web_url"]); url != "" {
		fmt.Fprintf(&b, "### Job #%s — %s\n[abrir](%s)\n\n", id, name, url)
	} else {
		fmt.Fprintf(&b, "### Job #%s — %s\n\n", id, name)
	}
	pieces := []string{statusEmoji(clean(toStr(m["status"])))}
	if stage := toStr(m["stage"]); stage != "" {
		pieces = append(pieces, "🎯 "+stage)
	}
	if ref := toStr(m["ref"]); ref != "" {
		pieces = append(pieces, "🔖 "+ref)
	}
	if u := nestedUser(m, "user"); u != "" {
		pieces = append(pieces, "👤 "+u)
	}
	if dur := formatDuration(m["duration"]); dur != "" {
		pieces = append(pieces, "⏱ "+dur)
	}
	if d := gitlabDate(toStr(m["created_at"])); d != "" {
		pieces = append(pieces, "🕓 "+d)
	}
	b.WriteString(metaLine(pieces...))
	return strings.TrimSpace(b.String())
}

func formatMemberDetail(m map[string]any) string {
	var b strings.Builder
	fmt.Fprintf(&b, "### %s\n", orMissing(m["username"]))
	pieces := []string{"🏷️ " + accessLevelName(toInt(m["access_level"]))}
	if s := toStr(m["state"]); s != "" {
		pieces = append(pieces, "📌 "+s)
	}
	b.WriteString(metaLine(pieces...))
	if name := toStr(m["name"]); name != "" {
		fmt.Fprintf(&b, "- **Nome:** %s\n", name)
	}
	if e := toStr(m["expires_at"]); e != "" {
		fmt.Fprintf(&b, "- **Expira:** %s\n", e)
	}
	return strings.TrimSpace(b.String())
}

func formatUserDetail(m map[string]any) string {
	var b strings.Builder
	fmt.Fprintf(&b, "### %s\n\n", orMissing(m["username"]))
	if name := toStr(m["name"]); name != "" {
		fmt.Fprintf(&b, "- **Nome:** %s\n", name)
	}
	fmt.Fprintf(&b, "- **Estado:** %s\n", orMissing(m["state"]))
	if em := toStr(m["email"]); em != "" {
		fmt.Fprintf(&b, "- **Email:** %s\n", em)
	}
	return strings.TrimSpace(b.String())
}

func formatGenericDetail(m map[string]any) string {
	var b strings.Builder
	title := firstOf(m, "title", "name", "username", "path", "ref")
	if title == "" {
		title = "Item"
	}
	fmt.Fprintf(&b, "### %s\n\n", clean(title))
	for _, f := range []string{"id", "iid", "state", "status", "ref", "sha", "default_branch", "visibility", "path_with_namespace", "web_url", "stage"} {
		if v := toStr(m[f]); v != "" {
			fmt.Fprintf(&b, "- **%s:** %s\n", labelOf(f), clean(v))
		}
	}
	if desc := toStr(m["description"]); desc != "" {
		fmt.Fprintf(&b, "\n%s\n", desc)
	}
	return strings.TrimSpace(b.String())
}

var itemLabels = map[string]string{
	"id": "ID", "iid": "IID", "state": "Estado", "status": "Status", "ref": "Ref",
	"sha": "SHA", "default_branch": "Branch padrão", "visibility": "Visibilidade",
	"path_with_namespace": "Path", "web_url": "URL", "stage": "Stage",
}

func labelOf(f string) string {
	if l, ok := itemLabels[f]; ok {
		return l
	}
	return f
}

func boolMark(cond bool) string {
	if cond {
		return "✅"
	}
	return "—"
}

func link(text, url string) string {
	url = strings.TrimSpace(url)
	if url == "" {
		return text
	}
	return "[" + text + "](" + url + ")"
}
