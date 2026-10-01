package mcpserver

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	MAX_TREE_LINES   = 400
	ELLIPSIS         = "…"
	MISSING          = "—"
	SIM              = "sim"
	NAO              = "não"
	TREE_BRANCH      = "├── "
	TREE_LAST        = "└── "
	TREE_INDENT      = "    "
	TREE_CONTINUE    = "│   "
	TRUNCATE_ISSUE   = 300
	TRUNCATE_COMMITS = 300
	TRUNCATE_DESC    = 400
	TRUNCATE_MESSAGE = 60
	SECONDS_MINUTE   = 60
	SECONDS_HOUR     = 3600
)

func labelKeys() []string {
	return []string{"id", "iid", "state", "status", "ref", "sha", "default_branch", "visibility", "path_with_namespace", "web_url", "stage"}
}

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
		return boolText(t)
	case float64:
		return floatText(t)
	case int:
		return strconv.Itoa(t)
	case []any:
		return joinAny(t)
	case map[string]any:
		return mapText(t)
	default:
		return fmt.Sprintf("%v", v)
	}
}

func boolText(t bool) string {
	if t {
		return SIM
	}
	return NAO
}

func floatText(t float64) string {
	if t == float64(int64(t)) {
		return strconv.Itoa(int(t))
	}
	return strconv.FormatFloat(t, 'f', 2, 64)
}

func joinAny(items []any) string {
	parts := make([]string, 0, len(items))
	for _, x := range items {
		if s := toStr(x); s != "" {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, ", ")
}

func mapText(t map[string]any) string {
	for _, k := range []string{"name", "username", "title", "ref", "path", "path_with_namespace", "message", "description"} {
		s, ok := t[k].(string)
		if !ok {
			continue
		}
		if s != "" {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

func toInt(v any) int {
	switch t := v.(type) {
	case float64:
		return int(t)
	case int:
		return t
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

func userName(m map[string]any) string {
	for _, k := range []string{"username", "name", "email"} {
		v, ok := m[k].(string)
		if !ok {
			continue
		}
		if v != "" {
			return v
		}
	}
	return ""
}

func nestedUser(m map[string]any, key string) string {
	v, ok := m[key].(map[string]any)
	if !ok {
		return ""
	}
	return userName(v)
}

func usersList(m map[string]any, key string) string {
	arr, ok := m[key].([]any)
	if ok {
		if len(arr) > 0 {
			parts := make([]string, 0, len(arr))
			for _, it := range arr {
				um, ok := it.(map[string]any)
				if !ok {
					continue
				}
				if n := userName(um); n != "" {
					parts = append(parts, n)
				}
			}
			return strings.Join(parts, ", ")
		}
	}

	v, ok := m[strings.TrimSuffix(key, "ees")].(map[string]any)
	if !ok {
		return ""
	}
	return userName(v)
}

func firstOf(m map[string]any, keys ...string) string {
	for _, k := range keys {
		v, ok := m[k]
		if !ok {
			continue
		}
		if s := toStr(v); s != "" {
			return s
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
		t, err := time.Parse(layout, s)
		if err == nil {
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

	if secs < SECONDS_MINUTE {
		return fmt.Sprintf("%.0fs", secs)
	}

	if secs < SECONDS_HOUR {
		m := int(secs) / SECONDS_MINUTE
		s := int(secs) % SECONDS_MINUTE
		if s == 0 {
			return fmt.Sprintf("%dm", m)
		}
		return fmt.Sprintf("%dm%ds", m, s)
	}

	h := int(secs) / SECONDS_HOUR
	m := (int(secs) % SECONDS_HOUR) / SECONDS_MINUTE
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

func isRunning(status string) bool {
	return strings.EqualFold(strings.TrimSpace(status), "running")
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
		if isRunning(status) {
			return ELLIPSIS
		}
		return MISSING
	}

	label := humanDuration(secs)
	if isRunning(status) {
		return fmt.Sprintf("~%s", label)
	}
	return label
}

func markdownTable(headers []string, rows [][]string) string {
	if len(rows) == 0 {
		return "Nenhum item encontrado."
	}

	b := strings.Builder{}
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

func countLabel(m map[string]any, key string) string {
	s := toStr(m[key])
	if s == "" {
		return ""
	}
	if s == "0" {
		return ""
	}
	return s
}

func isNotZero(m map[string]any, key string) bool {
	return countLabel(m, key) != ""
}

func formatProjectsTable(items []any) string {
	entries := mapItems(items)
	if len(entries) == 0 {
		return "Nenhum projeto encontrado."
	}

	b := strings.Builder{}
	for _, m := range entries {
		path := clean(toStr(m["path_with_namespace"]))
		fmt.Fprintf(&b, "### %s\n", link(path, toStr(m["web_url"])))

		pieces := []string{}
		if isNotZero(m, "star_count") {
			pieces = append(pieces, fmt.Sprintf("⭐ %s", countLabel(m, "star_count")))
		}
		if isNotZero(m, "forks_count") {
			pieces = append(pieces, fmt.Sprintf("🍴 %s", countLabel(m, "forks_count")))
		}
		if v := toStr(m["visibility"]); v != "" {
			pieces = append(pieces, fmt.Sprintf("🔒 %s", v))
		}
		if d := gitlabDate(toStr(m["last_activity_at"])); d != "" {
			pieces = append(pieces, fmt.Sprintf("🕓 %s", d))
		}

		b.WriteString(metaLine(pieces...))

		if db := toStr(m["default_branch"]); db != "" {
			fmt.Fprintf(&b, "🔖 `%s`\n", db)
		}
		if d := clean(toStr(m["description"])); d != "" {
			fmt.Fprintf(&b, "%s\n", truncate(d, TRUNCATE_DESC))
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

	b := strings.Builder{}
	for _, m := range entries {
		username := clean(toStr(m["username"]))
		fmt.Fprintf(&b, "### %s\n", link(fmt.Sprintf("@%s", username), toStr(m["web_url"])))

		pieces := []string{}
		n := toStr(m["name"])
		if n != "" {
			if n != username {
				pieces = append(pieces, fmt.Sprintf("👤 %s", n))
			}
		}
		if s := toStr(m["state"]); s != "" {
			pieces = append(pieces, fmt.Sprintf("📌 %s", s))
		}

		b.WriteString(metaLine(pieces...))
		b.WriteString("\n")
	}

	return strings.TrimSpace(b.String())
}

func stateIcon(state string) string {
	switch state {
	case "merged":
		return "🟣"
	case "closed":
		return "🔴"
	default:
		return "🟢"
	}
}

func formatIssuesTable(items []any) string {
	entries := mapItems(items)
	if len(entries) == 0 {
		return "Nenhuma issue encontrada."
	}

	b := strings.Builder{}
	for _, m := range entries {
		num := toStr(m["iid"])
		title := clean(toStr(m["title"]))
		fmt.Fprintf(&b, "### %s %s\n", link(fmt.Sprintf("#%s", num), toStr(m["web_url"])), title)

		state := clean(toStr(m["state"]))
		pieces := []string{fmt.Sprintf("%s %s", stateIcon(state), state)}
		if a := nestedUser(m, "author"); a != "" {
			pieces = append(pieces, fmt.Sprintf("👤 %s", a))
		}
		if lb := toStr(m["labels"]); lb != "" {
			pieces = append(pieces, fmt.Sprintf("🏷️ %s", lb))
		}
		if u := gitlabDate(toStr(m["updated_at"])); u != "" {
			pieces = append(pieces, fmt.Sprintf("🕓 %s", u))
		}

		b.WriteString(metaLine(pieces...))

		if d := clean(toStr(m["description"])); d != "" {
			fmt.Fprintf(&b, "> %s\n", truncate(d, TRUNCATE_ISSUE))
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

	b := strings.Builder{}
	for _, m := range entries {
		num := toStr(m["iid"])
		title := clean(toStr(m["title"]))
		fmt.Fprintf(&b, "### %s %s\n", link(fmt.Sprintf("!%s", num), toStr(m["web_url"])), title)

		state := clean(toStr(m["state"]))
		pieces := []string{fmt.Sprintf("%s %s", stateIcon(state), state)}
		if a := nestedUser(m, "author"); a != "" {
			pieces = append(pieces, fmt.Sprintf("👤 %s", a))
		}
		if sb := toStr(m["source_branch"]); sb != "" {
			pieces = append(pieces, fmt.Sprintf("🔀 `%s` → `%s`", sb, toStr(m["target_branch"])))
		}
		if u := gitlabDate(toStr(m["updated_at"])); u != "" {
			pieces = append(pieces, fmt.Sprintf("🕓 %s", u))
		}

		b.WriteString(metaLine(pieces...))

		if d := clean(toStr(m["description"])); d != "" {
			fmt.Fprintf(&b, "> %s\n", truncate(d, TRUNCATE_ISSUE))
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
			fmt.Sprintf("`%s`", clean(toStr(m["ref"]))),
			fmt.Sprintf("`%s`", shortSHA(toStr(m["sha"]))),
			clean(toStr(m["source"])),
			nestedUser(m, "user"),
			gitlabDate(toStr(m["created_at"])),
		})
	}
	return markdownTable(headers, rows)
}

func commitInfo(m map[string]any) (string, string, string) {
	cm, ok := m["commit"].(map[string]any)
	if !ok {
		return "", "", ""
	}

	commitDate := gitlabDate(toStr(cm["committed_date"]))
	if commitDate == "" {
		commitDate = gitlabDate(toStr(cm["authored_date"]))
	}

	return shortSHA(toStr(cm["id"])), clean(toStr(cm["title"])), commitDate
}

func formatBranchesTable(items []any) string {
	headers := []string{"Branch", "Padrão", "Protegida", "Commit", "Mensagem", "Data"}
	rows := [][]string{}
	for _, m := range mapItems(items) {
		commitSHA, commitMsg, commitDate := commitInfo(m)
		rows = append(rows, []string{
			fmt.Sprintf("`%s`", clean(toStr(m["name"]))),
			boolMark(toStr(m["default"]) == SIM),
			boolMark(toStr(m["protected"]) == SIM),
			fmt.Sprintf("`%s`", commitSHA),
			truncate(commitMsg, TRUNCATE_MESSAGE),
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

	b := strings.Builder{}
	for _, m := range entries {
		sha := shortSHA(toStr(m["id"]))
		if s := toStr(m["short_id"]); s != "" {
			sha = s
		}

		title := clean(toStr(m["title"]))
		fmt.Fprintf(&b, "### %s %s\n", codeLink(sha, toStr(m["web_url"])), title)

		pieces := []string{}
		if a := toStr(m["author_name"]); a != "" {
			pieces = append(pieces, fmt.Sprintf("👤 %s", a))
		}
		if d := gitlabDate(toStr(m["authored_date"])); d != "" {
			pieces = append(pieces, fmt.Sprintf("🕓 %s", d))
		}

		b.WriteString(metaLine(pieces...))

		if msg := clean(toStr(m["message"])); msg != "" {
			fmt.Fprintf(&b, "> %s\n", truncate(msg, TRUNCATE_COMMITS))
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
			fmt.Sprintf("`%s`", clean(toStr(m["username"]))),
			clean(toStr(m["name"])),
			clean(toStr(m["state"])),
			accessLevelName(toInt(m["access_level"])),
		})
	}
	return markdownTable(headers, rows)
}

func tagCommitInfo(m map[string]any) (string, string) {
	cm, ok := m["commit"].(map[string]any)
	if !ok {
		return "", ""
	}

	sha := shortSHA(toStr(cm["short_id"]))
	if sha == "" {
		sha = shortSHA(toStr(cm["id"]))
	}

	return sha, clean(toStr(cm["title"]))
}

func formatTagsTable(items []any) string {
	headers := []string{"Tag", "Commit", "Mensagem", "Protegida"}
	rows := [][]string{}
	for _, m := range mapItems(items) {
		sha, msg := tagCommitInfo(m)
		rows = append(rows, []string{
			fmt.Sprintf("`%s`", clean(toStr(m["name"]))),
			fmt.Sprintf("`%s`", sha),
			truncate(msg, TRUNCATE_MESSAGE),
			boolMark(toStr(m["protected"]) == SIM),
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
			fmt.Sprintf("`%s`", clean(toStr(m["name"]))),
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

func (n *fileTreeNode) less(a, b string) bool {
	ci, cj := n.children[a], n.children[b]
	if ci.isDir != cj.isDir {
		return ci.isDir
	}
	return a < b
}

func countNodes(n *fileTreeNode) int {
	c := 0
	for _, ch := range n.children {
		c += 1 + countNodes(ch)
	}
	return c
}

type treeWalk struct {
	lines   []string
	counter int
}

func (w *treeWalk) add(n *fileTreeNode, prefix string) {
	if w.counter >= MAX_TREE_LINES {
		return
	}

	names := make([]string, 0, len(n.children))
	for k := range n.children {
		names = append(names, k)
	}
	sort.Slice(names, func(i, j int) bool { return n.less(names[i], names[j]) })

	for i, name := range names {
		if w.counter >= MAX_TREE_LINES {
			return
		}

		last := i == len(names)-1
		connector := TREE_BRANCH
		if last {
			connector = TREE_LAST
		}

		label := name
		if n.children[name].isDir {
			label = fmt.Sprintf("%s/", label)
		}

		w.lines = append(w.lines, fmt.Sprintf("%s%s%s", prefix, connector, label))
		w.counter++

		indent := TREE_CONTINUE
		if last {
			indent = TREE_INDENT
		}

		w.add(n.children[name], fmt.Sprintf("%s%s", prefix, indent))
	}
}

func buildFileTree(entries []map[string]any) (*fileTreeNode, int, int) {
	root := newFileTreeRoot()
	dirs := 0
	files := 0

	for _, m := range entries {
		path := toStr(m["path"])
		if path == "" {
			continue
		}

		typ := toStr(m["type"])
		parts := strings.Split(path, "/")
		cur := root
		for i, p := range parts {
			cur = cur.child(p)
			if i < len(parts)-1 {
				cur.isDir = true
			}
			if typ == "tree" {
				cur.isDir = true
			}
		}

		if typ == "tree" {
			dirs++
		}
		if typ != "tree" {
			files++
		}
	}

	return root, dirs, files
}

func formatTreeTable(items []any) string {
	entries := mapItems(items)
	if len(entries) == 0 {
		return "Nenhum item encontrado."
	}

	root, dirs, files := buildFileTree(entries)
	w := &treeWalk{lines: []string{}}
	w.add(root, "")

	b := strings.Builder{}
	fmt.Fprintf(&b, "📊 %d entradas — %d diretórios, %d arquivos\n\n```\n", len(entries), dirs, files)
	b.WriteString(strings.Join(w.lines, "\n"))

	if countNodes(root) > w.counter {
		fmt.Fprintf(&b, "\n%s lista truncada em %d linhas. Use path/ref mais específico ou recursive=true para ver mais.\n", ELLIPSIS, MAX_TREE_LINES)
	}

	b.WriteString("\n```")
	return strings.TrimSpace(b.String())
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	r := []rune(s)
	if len(r) > n {
		return fmt.Sprintf("%s%s", string(r[:n]), ELLIPSIS)
	}
	return s
}

func orMissing(v any) string {
	s := toStr(v)
	if s == "" {
		return MISSING
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

	return fmt.Sprintf("%s\n", strings.Join(parts, " · "))
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
	b := strings.Builder{}
	path := clean(toStr(m["path_with_namespace"]))
	fmt.Fprintf(&b, "### %s\n", link(path, toStr(m["web_url"])))

	pieces := []string{}
	if isNotZero(m, "star_count") {
		pieces = append(pieces, fmt.Sprintf("⭐ %s", countLabel(m, "star_count")))
	}
	if isNotZero(m, "forks_count") {
		pieces = append(pieces, fmt.Sprintf("🍴 %s", countLabel(m, "forks_count")))
	}
	if v := toStr(m["visibility"]); v != "" {
		pieces = append(pieces, fmt.Sprintf("🔒 %s", v))
	}
	if a := gitlabDate(toStr(m["last_activity_at"])); a != "" {
		pieces = append(pieces, fmt.Sprintf("🕓 %s", a))
	}

	b.WriteString(metaLine(pieces...))
	fmt.Fprintf(&b, "- **Branch padrão:** %s\n", orMissing(m["default_branch"]))

	if isNotZero(m, "open_issues_count") {
		fmt.Fprintf(&b, "- **Issues abertas:** %s\n", countLabel(m, "open_issues_count"))
	}

	topics, ok := m["topics"].([]any)
	if ok {
		if len(topics) > 0 {
			parts := make([]string, 0, len(topics))
			for _, tp := range topics {
				parts = append(parts, toStr(tp))
			}
			fmt.Fprintf(&b, "- **Topics:** %s\n", strings.Join(parts, ", "))
		}
	}

	fmt.Fprintf(&b, "- **Criado:** %s\n", orMissing(gitlabDate(toStr(m["created_at"]))))

	if d := toStr(m["description"]); d != "" {
		fmt.Fprintf(&b, "\n%s\n", d)
	}

	return strings.TrimSpace(b.String())
}

func formatIssueDetail(m map[string]any) string {
	b := strings.Builder{}
	fmt.Fprintf(&b, "## Issue #%s — %s\n", toStr(m["iid"]), clean(toStr(m["title"])))

	url := toStr(m["web_url"])
	if url != "" {
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
	if isNotZero(m, "user_notes_count") {
		fmt.Fprintf(&b, "- **Comentários:** %s\n", countLabel(m, "user_notes_count"))
	}
	if body := toStr(m["description"]); body != "" {
		fmt.Fprintf(&b, "\n---\n%s\n", body)
	}

	return strings.TrimSpace(b.String())
}

func formatMRDetail(m map[string]any) string {
	b := strings.Builder{}
	fmt.Fprintf(&b, "## MR !%s — %s\n", toStr(m["iid"]), clean(toStr(m["title"])))

	url := toStr(m["web_url"])
	if url != "" {
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
	b := strings.Builder{}
	title := clean(toStr(m["title"]))
	label := toStr(m["id"])

	short := toStr(m["short_id"])
	if short != "" {
		label = short
	}

	fmt.Fprintf(&b, "### %s %s\n", codeLink(label, toStr(m["web_url"])), title)

	pieces := []string{}
	if a := toStr(m["author_name"]); a != "" {
		pieces = append(pieces, fmt.Sprintf("👤 %s", a))
	}
	if d := gitlabDate(toStr(m["committed_date"])); d != "" {
		pieces = append(pieces, fmt.Sprintf("🕓 %s", d))
	}

	b.WriteString(metaLine(pieces...))

	if msg := toStr(m["message"]); msg != "" {
		fmt.Fprintf(&b, "```\n%s\n```\n", msg)
	}

	return strings.TrimSpace(b.String())
}

func formatBranchDetail(m map[string]any) string {
	b := strings.Builder{}
	name := clean(toStr(m["name"]))
	fmt.Fprintf(&b, "### %s\n", link(name, toStr(m["web_url"])))

	pieces := []string{}
	if toStr(m["default"]) == SIM {
		pieces = append(pieces, "🏷️ padrão")
	}
	if toStr(m["protected"]) == SIM {
		pieces = append(pieces, "🔒 protegida")
	}

	b.WriteString(metaLine(pieces...))

	cm, ok := m["commit"].(map[string]any)
	if ok {
		fmt.Fprintf(&b, "- **Commit:** `%s` — %s\n", orMissing(cm["id"]), clean(toStr(cm["title"])))
		if d := gitlabDate(toStr(cm["committed_date"])); d != "" {
			fmt.Fprintf(&b, "- **Data:** %s\n", d)
		}
	}

	return strings.TrimSpace(b.String())
}

func formatTagDetail(m map[string]any) string {
	b := strings.Builder{}
	name := clean(toStr(m["name"]))
	fmt.Fprintf(&b, "### %s\n", link(name, toStr(m["web_url"])))

	if toStr(m["protected"]) == SIM {
		fmt.Fprintf(&b, "- **Protegida:** %s\n", SIM)
	}

	cm, ok := m["commit"].(map[string]any)
	if ok {
		fmt.Fprintf(&b, "- **Commit:** `%s` — %s\n", orMissing(cm["id"]), clean(toStr(cm["title"])))
	}
	if msg := toStr(m["message"]); msg != "" {
		fmt.Fprintf(&b, "\n```\n%s\n```\n", msg)
	}

	return strings.TrimSpace(b.String())
}

func formatPipelineDetail(m map[string]any) string {
	b := strings.Builder{}
	id := toStr(m["id"])
	fmt.Fprintf(&b, "### Pipeline #%s\n", id)

	url := toStr(m["web_url"])
	if url != "" {
		fmt.Fprintf(&b, "[abrir](%s)\n", url)
	}

	b.WriteString("\n")

	pieces := []string{statusEmoji(clean(toStr(m["status"])))}
	if ref := toStr(m["ref"]); ref != "" {
		pieces = append(pieces, fmt.Sprintf("🔖 %s", ref))
	}
	if sha := toStr(m["sha"]); sha != "" {
		pieces = append(pieces, fmt.Sprintf("`%s`", shortSHA(sha)))
	}
	if u := nestedUser(m, "user"); u != "" {
		pieces = append(pieces, fmt.Sprintf("👤 %s", u))
	}
	if d := gitlabDate(toStr(m["created_at"])); d != "" {
		pieces = append(pieces, fmt.Sprintf("🕓 %s", d))
	}

	b.WriteString(metaLine(pieces...))
	return strings.TrimSpace(b.String())
}

func formatJobDetail(m map[string]any) string {
	b := strings.Builder{}
	id := toStr(m["id"])
	name := clean(toStr(m["name"]))
	fmt.Fprintf(&b, "### Job #%s — %s\n", id, name)

	url := toStr(m["web_url"])
	if url != "" {
		fmt.Fprintf(&b, "[abrir](%s)\n", url)
	}

	b.WriteString("\n")

	pieces := []string{statusEmoji(clean(toStr(m["status"])))}
	if stage := toStr(m["stage"]); stage != "" {
		pieces = append(pieces, fmt.Sprintf("🎯 %s", stage))
	}
	if ref := toStr(m["ref"]); ref != "" {
		pieces = append(pieces, fmt.Sprintf("🔖 %s", ref))
	}
	if u := nestedUser(m, "user"); u != "" {
		pieces = append(pieces, fmt.Sprintf("👤 %s", u))
	}
	if dur := formatDuration(m["duration"]); dur != "" {
		pieces = append(pieces, fmt.Sprintf("⏱ %s", dur))
	}
	if d := gitlabDate(toStr(m["created_at"])); d != "" {
		pieces = append(pieces, fmt.Sprintf("🕓 %s", d))
	}

	b.WriteString(metaLine(pieces...))
	return strings.TrimSpace(b.String())
}

func formatMemberDetail(m map[string]any) string {
	b := strings.Builder{}
	fmt.Fprintf(&b, "### %s\n", orMissing(m["username"]))

	pieces := []string{fmt.Sprintf("🏷️ %s", accessLevelName(toInt(m["access_level"])))}
	if s := toStr(m["state"]); s != "" {
		pieces = append(pieces, fmt.Sprintf("📌 %s", s))
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
	b := strings.Builder{}
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
	b := strings.Builder{}
	title := firstOf(m, "title", "name", "username", "path", "ref")
	if title == "" {
		title = "Item"
	}

	fmt.Fprintf(&b, "### %s\n\n", clean(title))

	for _, f := range labelKeys() {
		if v := toStr(m[f]); v != "" {
			fmt.Fprintf(&b, "- **%s:** %s\n", labelOf(f), clean(v))
		}
	}

	if desc := toStr(m["description"]); desc != "" {
		fmt.Fprintf(&b, "\n%s\n", desc)
	}

	return strings.TrimSpace(b.String())
}

func labelOf(f string) string {
	switch f {
	case "id":
		return "ID"
	case "iid":
		return "IID"
	case "state":
		return "Estado"
	case "status":
		return "Status"
	case "ref":
		return "Ref"
	case "sha":
		return "SHA"
	case "default_branch":
		return "Branch padrão"
	case "visibility":
		return "Visibilidade"
	case "path_with_namespace":
		return "Path"
	case "web_url":
		return "URL"
	case "stage":
		return "Stage"
	default:
		return f
	}
}

func boolMark(cond bool) string {
	if cond {
		return "✅"
	}
	return MISSING
}

func link(text, url string) string {
	url = strings.TrimSpace(url)
	if url == "" {
		return text
	}
	return fmt.Sprintf("[%s](%s)", text, url)
}

func codeLink(text, url string) string {
	return link(fmt.Sprintf("`%s`", text), url)
}
