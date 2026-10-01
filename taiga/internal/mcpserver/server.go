package mcpserver

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"ntdsk.com/taiga/internal/taiga"
)

type Server struct {
	client *taiga.Client
}

var redactEnabled = true

func New(client *taiga.Client) *Server {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("TAIGA_REDACT")))
	if v == "false" || v == "0" || v == "no" || v == "off" {
		redactEnabled = false
	}
	return &Server{client: client}
}

func (s *Server) Register(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "taiga_list",
		Description: "List projects, cards (user stories), issues, tasks or recent activity. Set 'entity' (project|card|issue|task|activity). For project: filters member/page/page_size. For card/issue/task: require 'project_id' and use only the filters valid for that entity. For activity: project_id optional (0 = all), use my=true for your own, and card_id/issue_id/task_id to see one item's history. Set 'count_only' to get just the total count.",
	}, s.listItems)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "taiga_get",
		Description: "Get a project (by project_id or slug) or a card/issue/task (by id or ref+project_id). Set 'entity' (project|card|issue|task).",
	}, s.get)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "taiga_change_status",
		Description: "Change a card's, issue's or task's status. Set 'entity' plus 'status_id' (see taiga_get), and 'item_id' or 'ref'+'project_id'.",
	}, s.changeStatus)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "taiga_search",
		Description: "Search cards, issues and tasks by subject text (case-insensitive) or ref number. project_id scopes the search; limit caps results (default 10, max 30).",
	}, s.search)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "taiga_attachment",
		Description: "List or download attachments of a card, issue or task. Set 'entity' and 'action' (list|download). 'list' requires object_id & project_id; 'download' requires attachment_id.",
	}, s.attachment)
}

type ListActivityInput struct {
	ProjectID *int  `json:"project_id,omitempty" jsonschema:"Filter by project"`
	My        *bool `json:"my,omitempty" jsonschema:"Only your activities"`
	Limit     *int  `json:"limit,omitempty" jsonschema:"Max items (max 100, default 20)"`
	CardID    *int  `json:"card_id,omitempty" jsonschema:"Card ID; show only its activities"`
	IssueID   *int  `json:"issue_id,omitempty" jsonschema:"Issue ID; show only its activities"`
	TaskID    *int  `json:"task_id,omitempty" jsonschema:"Task ID; show only its activities"`
}

type activityItem struct {
	kind     string
	id       int
	ref      int
	subject  string
	status   string
	assigned string
	modified string
	action   string
}

func (s *Server) listActivity(ctx context.Context, input ListActivityInput) (*mcp.CallToolResult, any, error) {
	if input.CardID != nil || input.IssueID != nil || input.TaskID != nil {
		return s.listItemActivity(ctx, input)
	}
	limit := 20
	if input.Limit != nil && *input.Limit > 0 {
		limit = *input.Limit
		if limit > 100 {
			limit = 100
		}
	}
	var projectID int
	if input.ProjectID != nil {
		projectID = *input.ProjectID
	}
	var myID int
	if input.My != nil && *input.My {
		myID = s.loadCurrentUser(ctx)
	}
	query := url.Values{
		"page":      []string{"1"},
		"page_size": []string{strconv.Itoa(limit)},
	}
	if projectID > 0 {
		query.Set("project", strconv.Itoa(projectID))
	}
	if myID > 0 {
		query.Set("user", strconv.Itoa(myID))
	}
	raw, _, err := s.client.ListJSON(ctx, "/timeline", query, true)
	if err == nil {
		if items := activityItemsFromTimeline(raw); len(items) > 0 {
			sort.Slice(items, func(i, j int) bool { return items[i].modified > items[j].modified })
			if len(items) > limit {
				items = items[:limit]
			}
			return textResult(formatActivityTable(items))
		}
	}

	var items []activityItem
	items = append(items, s.fetchRecentItems(ctx, "/userstories", "card", projectID, myID, limit)...)
	items = append(items, s.fetchRecentItems(ctx, "/issues", "issue", projectID, myID, limit)...)
	sort.Slice(items, func(i, j int) bool { return items[i].modified > items[j].modified })
	if len(items) > limit {
		items = items[:limit]
	}
	s.enrichActivityActions(ctx, items)
	return textResult(formatActivityTable(items))
}

func (s *Server) listItemActivity(ctx context.Context, input ListActivityInput) (*mcp.CallToolResult, any, error) {
	var kind string
	var id int
	switch {
	case input.CardID != nil:
		kind, id = "card", *input.CardID
	case input.IssueID != nil:
		kind, id = "issue", *input.IssueID
	default:
		kind, id = "task", *input.TaskID
	}
	if id <= 0 {
		return nil, nil, fmt.Errorf("o id do %s deve ser maior que zero", kind)
	}

	historyKind := kind
	if historyKind == "card" {
		historyKind = "userstory"
	}

	var itemEndpoint string
	switch historyKind {
	case "userstory":
		itemEndpoint = "/userstories/" + strconv.Itoa(id)
	case "issue":
		itemEndpoint = "/issues/" + strconv.Itoa(id)
	default:
		itemEndpoint = "/tasks/" + strconv.Itoa(id)
	}
	itemData, _, err := s.client.GetJSON(ctx, itemEndpoint, url.Values{})
	if err != nil {
		return nil, nil, err
	}
	subject, ref, status := "", 0, ""
	if m, ok := itemData.(map[string]any); ok {
		subject = displayValue(m["subject"])
		ref = displayInt(m["ref"])
		status = translateStatus(resolveName(m, "status"))
	}

	raw, _, err := s.client.GetJSON(ctx, fmt.Sprintf("/history/%s/%d", historyKind, id), url.Values{})
	if err != nil {
		return nil, nil, err
	}
	entries, ok := raw.([]any)
	if !ok || len(entries) == 0 {
		return textResult("Nenhuma atividade encontrada para este item.")
	}

	views := make([]historyEntryView, 0, len(entries))
	for _, it := range entries {
		m, ok := it.(map[string]any)
		if !ok {
			continue
		}
		author := redactName(resolveCommentAuthor(m))
		if author == "" {
			author = "—"
		}
		date := displayValue(m["created_at"])
		action := ""
		if comment := strings.TrimSpace(displayValue(m["comment"])); comment != "" {
			action = "💬 " + comment
		} else if diff, ok := m["diff"].(map[string]any); ok {
			action = summarizeHistoryDiff(diff)
		} else {
			action = "📝 Modificado"
		}
		views = append(views, historyEntryView{author: author, date: date, action: action})
	}
	for i, j := 0, len(views)-1; i < j; i, j = i+1, j-1 {
		views[i], views[j] = views[j], views[i]
	}
	if input.Limit != nil && *input.Limit > 0 && len(views) > *input.Limit {
		views = views[:*input.Limit]
	}
	return textResult(formatHistoryTable(kind, ref, subject, status, views))
}

func (s *Server) fetchRecentItems(ctx context.Context, endpoint, kind string, projectID, assignedTo, limit int) []activityItem {
	return s.fetchRecentItemsWithFilters(ctx, endpoint, kind, projectID, limit, nil, nil, intPtrOrNil(assignedTo), "")
}

func intPtrOrNil(value int) *int {
	if value <= 0 {
		return nil
	}
	return &value
}

func (s *Server) fetchRecentItemsWithFilters(ctx context.Context, endpoint, kind string, projectID, limit int, swimlaneID, statusID, assignedTo *int, tags string) []activityItem {
	query := url.Values{
		"order_by":  []string{"-modified_date"},
		"page":      []string{"1"},
		"page_size": []string{strconv.Itoa(limit)},
	}
	if projectID > 0 {
		query.Set("project", strconv.Itoa(projectID))
	}
	if assignedTo != nil && *assignedTo > 0 {
		query.Set("assigned_to", strconv.Itoa(*assignedTo))
	}
	if statusID != nil && *statusID > 0 {
		query.Set("status", strconv.Itoa(*statusID))
	}
	if swimlaneID != nil && *swimlaneID > 0 {
		query.Set("swimlane", strconv.Itoa(*swimlaneID))
	}
	if strings.TrimSpace(tags) != "" {
		query.Set("tags", strings.TrimSpace(tags))
	}
	raw, _, err := s.client.ListJSON(ctx, endpoint, query, true)
	if err != nil {
		return nil
	}
	items, ok := raw.([]any)
	if !ok {
		return nil
	}
	out := make([]activityItem, 0, len(items))
	for _, it := range items {
		m, ok := it.(map[string]any)
		if !ok {
			continue
		}
		out = append(out, activityItem{
			kind:     kind,
			id:       displayInt(m["id"]),
			ref:      displayInt(m["ref"]),
			subject:  displayValue(m["subject"]),
			status:   resolveName(m, "status"),
			assigned: redactName(resolveName(m, "assigned_to")),
			modified: displayValue(m["modified_date"]),
		})
	}
	return out
}

func (s *Server) enrichActivityActions(ctx context.Context, items []activityItem) {
	var wg sync.WaitGroup
	for i := range items {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			items[index].action = s.loadLastAction(ctx, items[index].kind, items[index].id)
		}(i)
	}
	wg.Wait()
}

func activityItemsFromTimeline(raw any) []activityItem {
	items, ok := raw.([]any)
	if !ok {
		if envelope, ok := raw.(map[string]any); ok {
			for _, key := range []string{"events", "activities", "timeline", "results", "data"} {
				if candidate, ok := envelope[key].([]any); ok {
					items = candidate
					break
				}
			}
		}
	}
	out := make([]activityItem, 0, len(items))
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		obj := firstMap(m, "content", "object", "item")
		if obj == nil {
			obj = m
		}
		kind := strings.ToLower(firstStringValue(m, "type", "event_type", "content_type"))
		if strings.Contains(kind, "issue") {
			kind = "issue"
		} else {
			kind = "card"
		}
		ref := displayInt(firstValue(obj, "ref", "reference"))
		subject := displayValue(firstValue(obj, "subject", "name", "title"))
		if subject == "" {
			subject = displayValue(firstValue(m, "subject", "name", "title"))
		}
		modified := displayValue(firstValue(m, "created_at", "created_date", "created", "timestamp", "date", "modified_date"))
		if modified == "" {
			modified = displayValue(firstValue(obj, "modified_date", "created_date"))
		}
		actorValue := firstValue(m, "user", "author", "actor", "by")
		actor := redactName(displayValue(actorValue))
		if actor == "" {
			if actorMap, ok := actorValue.(map[string]any); ok {
				actor = redactName(displayValue(firstValue(actorMap, "full_name_display", "full_name", "username", "name")))
			}
		}
		if actor == "objeto" {
			actor = ""
		}
		action := redactText(displayValue(firstValue(m, "description", "event", "event_type", "verb", "action")))
		if action == "" {
			action = "📝 Modificado"
		}
		out = append(out, activityItem{kind: kind, id: displayInt(firstValue(obj, "id")), ref: ref, subject: subject, status: translateStatus(displayValue(firstValue(obj, "status"))), assigned: actor, modified: modified, action: action})
	}
	return out
}

func firstValue(m map[string]any, keys ...string) any {
	for _, key := range keys {
		if value, ok := m[key]; ok && value != nil {
			return value
		}
	}
	return nil
}

func firstStringValue(m map[string]any, keys ...string) string {
	return displayValue(firstValue(m, keys...))
}

func firstMap(m map[string]any, keys ...string) map[string]any {
	for _, key := range keys {
		if value, ok := m[key].(map[string]any); ok {
			return value
		}
	}
	return nil
}

func (s *Server) recentProjectActivity(ctx context.Context, projectID, limit int, swimlaneID, statusID, assignedTo *int, tags string) []activityItem {
	if projectID <= 0 {
		return nil
	}
	items := append(
		s.fetchRecentItemsWithFilters(ctx, "/userstories", "card", projectID, limit, swimlaneID, statusID, assignedTo, tags),
		s.fetchRecentItemsWithFilters(ctx, "/issues", "issue", projectID, limit, nil, statusID, assignedTo, tags)...,
	)
	sort.SliceStable(items, func(i, j int) bool {
		return parseActivityTime(items[i].modified).After(parseActivityTime(items[j].modified))
	})
	if len(items) > limit {
		items = items[:limit]
	}
	s.enrichActivityActions(ctx, items)
	return items
}

func parseActivityTime(value string) time.Time {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}
	}
	return parsed
}

func (s *Server) loadCurrentUser(ctx context.Context) int {
	raw, _, err := s.client.GetJSON(ctx, "/users/me", url.Values{})
	if err != nil {
		return 0
	}
	m, ok := raw.(map[string]any)
	if !ok {
		return 0
	}
	return displayInt(m["id"])
}


type GetInput struct {
	Entity        string `json:"entity" jsonschema:"Object type: project, card, issue or task"`
	ID            *int   `json:"id,omitempty" jsonschema:"Item ID (cards/issues/tasks)"`
	Ref           *int   `json:"ref,omitempty" jsonschema:"Item ref number (cards/issues/tasks)"`
	ProjectID     *int   `json:"project_id,omitempty" jsonschema:"Project ID (to fetch a project by ID, or required when using ref)"`
	Slug          string `json:"slug,omitempty" jsonschema:"Project slug (projects only; alternative to project_id)"`
	SwimlaneID    *int   `json:"swimlane_id,omitempty" jsonschema:"Filter recent activity by swimlane (projects only)"`
	StatusID      *int   `json:"status_id,omitempty" jsonschema:"Filter recent activity by status (projects only)"`
	AssignedTo    *int   `json:"assigned_to,omitempty" jsonschema:"Filter recent activity by assignee (projects only)"`
	Tags          string `json:"tags,omitempty" jsonschema:"Filter recent activity by tags (projects only)"`
	ActivityLimit *int   `json:"activity_limit,omitempty" jsonschema:"Number of recent activities (1-20, default 5) (projects only)"`
}

type ListItemsInput struct {
	Entity       string `json:"entity" jsonschema:"Object type: project, card, issue, task or activity"`
	Member       *int   `json:"member,omitempty" jsonschema:"Member ID; omit to list all visible projects (projects only)"`
	ProjectID    int    `json:"project_id" jsonschema:"Project ID (required for card/issue/task; ignored for project; optional for activity = all)"`
	My           *bool  `json:"my,omitempty" jsonschema:"Only your own activity (activity only)"`
	Limit        *int   `json:"limit,omitempty" jsonschema:"Max items, default 20, max 100 (activity only)"`
	CardID       *int   `json:"card_id,omitempty" jsonschema:"Card ID; show only its activity (activity only)"`
	IssueID      *int   `json:"issue_id,omitempty" jsonschema:"Issue ID; show only its activity (activity only)"`
	TaskID       *int   `json:"task_id,omitempty" jsonschema:"Task ID; show only its activity (activity only)"`
	StatusID     *int   `json:"status_id,omitempty" jsonschema:"Filter by status ID"`
	SwimlaneID   *int   `json:"swimlane_id,omitempty" jsonschema:"Filter by swimlane ID (cards only)"`
	MilestoneID  *int   `json:"milestone_id,omitempty" jsonschema:"Filter by milestone ID (cards/tasks)"`
	UserStoryID  *int   `json:"user_story_id,omitempty" jsonschema:"Filter by parent card ID (tasks only)"`
	AssignedTo   *int   `json:"assigned_to,omitempty" jsonschema:"Filter by assignee ID"`
	SeverityID   *int   `json:"severity_id,omitempty" jsonschema:"Filter by severity ID (issues only)"`
	PriorityID   *int   `json:"priority_id,omitempty" jsonschema:"Filter by priority ID (issues only)"`
	TypeID       *int   `json:"type_id,omitempty" jsonschema:"Filter by type ID (issues only)"`
	Tags         string `json:"tags,omitempty" jsonschema:"Comma-separated tags"`
	Closed       *bool  `json:"closed,omitempty" jsonschema:"Filter by closed status"`
	Blocked      *bool  `json:"blocked,omitempty" jsonschema:"Only blocked (true) or unblocked (false) cards (cards only)"`
	FilterColumn string `json:"filter_column,omitempty" jsonschema:"Column to filter rows by (subject, status, assignee, closed, blocked, ref, id, modified)"`
	FilterValue  string `json:"filter_value,omitempty" jsonschema:"Value (substring, case-insensitive) combined with filter_column"`
	Page         *int   `json:"page,omitempty" jsonschema:"Page number (1-based; default 50 per page)"`
	PageSize     *int   `json:"page_size,omitempty" jsonschema:"Items per page (1-1000)"`
	CountOnly    *bool  `json:"count_only,omitempty" jsonschema:"Return only the total count of matching items"`
}

type ChangeStatusInput struct {
	Entity    string `json:"entity" jsonschema:"Object type: card, issue or task"`
	ItemID    *int   `json:"item_id,omitempty" jsonschema:"Item ID"`
	Ref       *int   `json:"ref,omitempty" jsonschema:"Item ref number"`
	ProjectID *int   `json:"project_id,omitempty" jsonschema:"Project ID (required when using ref)"`
	StatusID  int    `json:"status_id" jsonschema:"Target status ID (see taiga_get)"`
}

type AttachmentInput struct {
	Entity       string `json:"entity" jsonschema:"Object type: card, issue or task"`
	Action       string `json:"action" jsonschema:"Action: 'list' or 'download'"`
	ObjectID     int    `json:"object_id,omitempty" jsonschema:"Item ID (required for action=list)"`
	ProjectID    int    `json:"project_id,omitempty" jsonschema:"Project ID (required for action=list)"`
	AttachmentID int    `json:"attachment_id,omitempty" jsonschema:"Attachment ID (required for action=download)"`
	Filename     string `json:"filename,omitempty" jsonschema:"Output filename (defaults to Taiga's name)"`
	Path         string `json:"path,omitempty" jsonschema:"Output path (defaults to the configured directory)"`
}



func itemCountLabel(isCard, isIssue bool) string {
	if isCard {
		return "cards"
	}
	if isIssue {
		return "issues"
	}
	return "tasks"
}

func listEntityConfig(entity string) (endpoint string, columns map[string]string, isCard, isIssue, isProject bool, err error) {
	switch entity {
	case "project", "projects":
		return "/projects", nil, false, false, true, nil
	case "userstory", "userstories", "card", "cards":
		return "/userstories", cardColumns, true, false, false, nil
	case "issue", "issues":
		return "/issues", issueColumns, false, true, false, nil
	case "task", "tasks", "subtask":
		return "/tasks", taskColumns, false, false, false, nil
	default:
		return "", nil, false, false, false, fmt.Errorf("entidade inválida: %q (use 'project', 'card', 'issue' ou 'task')", entity)
	}
}

func (s *Server) listItems(ctx context.Context, _ *mcp.CallToolRequest, input ListItemsInput) (*mcp.CallToolResult, any, error) {
	entity := strings.ToLower(strings.TrimSpace(input.Entity))
	if entity == "activity" || entity == "activities" {
		return s.listActivity(ctx, ListActivityInput{
			ProjectID: intPtrOrNil(input.ProjectID),
			My:        input.My,
			Limit:     input.Limit,
			CardID:    input.CardID,
			IssueID:   input.IssueID,
			TaskID:    input.TaskID,
		})
	}
	endpoint, columns, isCard, isIssue, isProject, err := listEntityConfig(entity)
	if err != nil {
		return nil, nil, err
	}
	if isProject {
		query, paginated, err := paging(input.Page, input.PageSize)
		if err != nil {
			return nil, nil, err
		}
		if input.Member != nil {
			if err := positive("member", *input.Member); err != nil {
				return nil, nil, err
			}
			query.Set("member", strconv.Itoa(*input.Member))
		}
		if input.CountOnly != nil && *input.CountOnly {
			count, err := s.countItems(ctx, endpoint, query)
			if err != nil {
				return nil, nil, err
			}
			return textResult(fmt.Sprintf("📊 **Total de projetos: %s**\n", formatInt(count)))
		}
		data, meta, err := s.client.ListJSON(ctx, endpoint, query, paginated)
		if err != nil {
			return nil, nil, err
		}
		return textResult(formatProjectsTable(data, meta))
	}
	if err := positive("project_id", input.ProjectID); err != nil {
		return nil, nil, err
	}
	columnKey := normalizeColumn(input.FilterColumn, columns)
	fetchAll := columnKey != "" && strings.TrimSpace(input.FilterValue) != ""
	query, paginated, err := listQuery(input.Page, input.PageSize, fetchAll)
	if err != nil {
		return nil, nil, err
	}
	query.Set("project", strconv.Itoa(input.ProjectID))

	if input.StatusID != nil {
		if err := positive("status_id", *input.StatusID); err != nil {
			return nil, nil, err
		}
		query.Set("status", strconv.Itoa(*input.StatusID))
	}
	if input.AssignedTo != nil {
		if err := positive("assigned_to", *input.AssignedTo); err != nil {
			return nil, nil, err
		}
		query.Set("assigned_to", strconv.Itoa(*input.AssignedTo))
	}
	if strings.TrimSpace(input.Tags) != "" {
		query.Set("tags", strings.TrimSpace(input.Tags))
	}
	if input.Closed != nil {
		query.Set("status__is_closed", strconv.FormatBool(*input.Closed))
	}

	filters := []string{}
	if isCard {
		if input.SwimlaneID != nil {
			if err := positive("swimlane_id", *input.SwimlaneID); err != nil {
				return nil, nil, err
			}
			query.Set("swimlane", strconv.Itoa(*input.SwimlaneID))
		}
		if input.MilestoneID != nil {
			if err := positive("milestone_id", *input.MilestoneID); err != nil {
				return nil, nil, err
			}
			query.Set("milestone", strconv.Itoa(*input.MilestoneID))
		}
		if input.Blocked != nil {
			query.Set("is_blocked", strconv.FormatBool(*input.Blocked))
		}
		if input.StatusID != nil {
			filters = append(filters, fmt.Sprintf("status = %d", *input.StatusID))
		}
		if input.SwimlaneID != nil {
			filters = append(filters, fmt.Sprintf("baia = %d", *input.SwimlaneID))
		}
		if input.MilestoneID != nil {
			filters = append(filters, fmt.Sprintf("marco = %d", *input.MilestoneID))
		}
		if input.AssignedTo != nil {
			filters = append(filters, fmt.Sprintf("responsável = %d", *input.AssignedTo))
		}
		if strings.TrimSpace(input.Tags) != "" {
			filters = append(filters, fmt.Sprintf("tags = %s", strings.TrimSpace(input.Tags)))
		}
		if input.Closed != nil {
			filters = append(filters, fmt.Sprintf("fechado = %v", *input.Closed))
		}
		if input.Blocked != nil {
			filters = append(filters, fmt.Sprintf("bloqueado = %v", *input.Blocked))
		}
	} else if isIssue {
		for name, value := range map[string]*int{
			"severity": input.SeverityID, "priority": input.PriorityID, "type": input.TypeID,
		} {
			if value != nil {
				if err := positive(name, *value); err != nil {
					return nil, nil, err
				}
				query.Set(name, strconv.Itoa(*value))
			}
		}
	} else {
		if input.MilestoneID != nil {
			if err := positive("milestone_id", *input.MilestoneID); err != nil {
				return nil, nil, err
			}
			query.Set("milestone", strconv.Itoa(*input.MilestoneID))
		}
		if input.UserStoryID != nil {
			if err := positive("user_story_id", *input.UserStoryID); err != nil {
				return nil, nil, err
			}
			query.Set("user_story", strconv.Itoa(*input.UserStoryID))
		}
	}

	if input.CountOnly != nil && *input.CountOnly {
		count, err := s.countItems(ctx, endpoint, query)
		if err != nil {
			return nil, nil, err
		}
		projectName := ""
		if proj, _, perr := s.client.GetJSON(ctx, "/projects/"+strconv.Itoa(input.ProjectID), url.Values{}); perr == nil {
			if m, ok := proj.(map[string]any); ok {
				projectName = displayValue(m["name"])
			}
		}
		return textResult(formatCount(itemCountLabel(isCard, isIssue), count, input.ProjectID, projectName, filters))
	}

	data, meta, err := s.client.ListJSON(ctx, endpoint, query, paginated)
	if err != nil {
		return nil, nil, err
	}
	if fetchAll {
		filtered, total := filterRows(data, columnKey, input.FilterValue)
		slice, _ := filtered.([]any)
		if len(slice) > displayCap {
			slice = slice[:displayCap]
		}
		footer := fmt.Sprintf("🔎 %d correspondências para %q (exibindo até %d).", total, input.FilterValue, displayCap)
		switch {
		case isCard:
			return textResult(formatCardsTable(slice, taiga.ResponseMeta{}) + "\n" + footer)
		case isIssue:
			priorities, severities := s.loadIssueValues(ctx, input.ProjectID)
			return textResult(formatIssuesTable(slice, taiga.ResponseMeta{}, priorities, severities) + "\n" + footer)
		default:
			return textResult(formatTasksTable(slice, taiga.ResponseMeta{}) + "\n" + footer)
		}
	}
	switch {
	case isCard:
		return textResult(formatCardsTable(data, meta))
	case isIssue:
		priorities, severities := s.loadIssueValues(ctx, input.ProjectID)
		return textResult(formatIssuesTable(data, meta, priorities, severities))
	default:
		return textResult(formatTasksTable(data, meta))
	}
}

func (s *Server) countItems(ctx context.Context, endpoint string, query url.Values) (int, error) {
	paged := url.Values{}
	for k, v := range query {
		paged[k] = v
	}
	paged.Set("page", "1")
	paged.Set("page_size", "1")
	if raw, meta, err := s.client.ListJSON(ctx, endpoint, paged, true); err == nil {
		if meta.Total != "" {
			if total, perr := strconv.Atoi(strings.TrimSpace(meta.Total)); perr == nil {
				return total, nil
			}
		}
		if items, ok := raw.([]any); ok {
			return len(items), nil
		}
	}
	if raw, _, err := s.client.ListJSON(ctx, endpoint, query, false); err == nil {
		if items, ok := raw.([]any); ok {
			return len(items), nil
		}
	}
	return 0, nil
}

func itemKinds(entity string) (commentKind, attachKind string, isCard, isIssue, isTask bool, err error) {
	switch entity {
	case "userstory", "userstories", "card", "cards":
		return "userstory", "userstories", true, false, false, nil
	case "issue", "issues":
		return "issue", "issues", false, true, false, nil
	case "task", "tasks", "subtask":
		return "task", "tasks", false, false, true, nil
	default:
		return "", "", false, false, false, fmt.Errorf("entidade inválida: %q (use 'card', 'issue' ou 'task')", entity)
	}
}

func (s *Server) get(ctx context.Context, _ *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, any, error) {
	entity := strings.ToLower(strings.TrimSpace(input.Entity))

	if entity == "project" || entity == "projects" {
		if (input.ProjectID == nil) == (strings.TrimSpace(input.Slug) == "") {
			return nil, nil, errors.New("informe exatamente um de project_id ou slug")
		}
		var endpoint string
		var query url.Values
		if input.ProjectID != nil {
			if err := positive("project_id", *input.ProjectID); err != nil {
				return nil, nil, err
			}
			endpoint = "/projects/" + strconv.Itoa(*input.ProjectID)
			query = url.Values{}
		} else {
			endpoint = "/projects/by_slug"
			query = url.Values{"slug": []string{strings.TrimSpace(input.Slug)}}
		}
		data, _, err := s.client.GetJSON(ctx, endpoint, query)
		if err != nil {
			return nil, nil, err
		}
		projectID := extractIntField(data, "id")
		if input.ProjectID != nil {
			projectID = *input.ProjectID
		}
		activityLimit := 5
		if input.ActivityLimit != nil && *input.ActivityLimit > 0 {
			activityLimit = *input.ActivityLimit
			if activityLimit > 20 {
				activityLimit = 20
			}
		}
		activities := s.recentProjectActivity(ctx, projectID, activityLimit, input.SwimlaneID, input.StatusID, input.AssignedTo, input.Tags)
		return textResult(formatProject(data, activities))
	}

	if (input.ID == nil) == (input.Ref == nil) {
		return nil, nil, errors.New("informe exatamente um de id ou ref")
	}
	base, _, _, err := itemBase(entity)
	if err != nil {
		return nil, nil, err
	}
	commentKind, attachKind, isCard, isIssue, isTask, err := itemKinds(entity)
	if err != nil {
		return nil, nil, err
	}
	var endpoint string
	query := url.Values{}
	if input.ID != nil {
		if err := positive("id", *input.ID); err != nil {
			return nil, nil, err
		}
		endpoint = base + "/" + strconv.Itoa(*input.ID)
	} else {
		if err := positive("ref", *input.Ref); err != nil {
			return nil, nil, err
		}
		if input.ProjectID == nil {
			return nil, nil, errors.New("project_id é obrigatório quando ref for usado")
		}
		if err := positive("project_id", *input.ProjectID); err != nil {
			return nil, nil, err
		}
		endpoint = base + "/by_ref"
		query.Set("ref", strconv.Itoa(*input.Ref))
		query.Set("project", strconv.Itoa(*input.ProjectID))
	}
	data, _, err := s.client.GetJSON(ctx, endpoint, query)
	if err != nil {
		return nil, nil, err
	}
	comments := s.loadComments(ctx, commentKind, extractID(data))
	attachments := s.loadAttachmentNames(ctx, attachKind, extractIntField(data, "project"), extractID(data))
	if isTask {
		return textResult(formatTaskItem(data, comments, attachments))
	}
	var subtasks []string
	if isCard {
		subtasks = s.loadSubtasks(ctx, extractIntField(data, "project"), extractID(data))
	}
	var priorities, severities map[int]string
	if isIssue {
		priorities, severities = s.loadIssueValues(ctx, extractIntField(data, "project"))
	}
	return textResult(formatDetailedItem(data, comments, attachments, subtasks, priorities, severities))
}

type resolvedItem struct {
	obj     map[string]any
	id      int
	version int
}

func (s *Server) fetchItem(ctx context.Context, base string, itemID, refValue, projectValue *int) (*resolvedItem, error) {
	if (itemID == nil) == (refValue == nil) {
		return nil, errors.New("informe exatamente um de id ou ref")
	}
	endpoint := ""
	query := url.Values{}
	if itemID != nil {
		if err := positive("id", *itemID); err != nil {
			return nil, err
		}
		endpoint = "/" + base + "/" + strconv.Itoa(*itemID)
	} else {
		if err := positive("ref", *refValue); err != nil {
			return nil, err
		}
		if projectValue == nil {
			return nil, errors.New("project_id é obrigatório quando ref for usado")
		}
		if err := positive("project_id", *projectValue); err != nil {
			return nil, err
		}
		endpoint = "/" + base + "/by_ref"
		query.Set("ref", strconv.Itoa(*refValue))
		query.Set("project", strconv.Itoa(*projectValue))
	}
	raw, _, err := s.client.GetJSON(ctx, endpoint, query)
	if err != nil {
		return nil, err
	}
	obj, _ := raw.(map[string]any)
	id := extractID(raw)
	if id <= 0 {
		return nil, errors.New("não foi possível identificar o item")
	}
	version := extractIntField(raw, "version")
	if version <= 0 {
		return nil, errors.New("não foi possível obter a versão do item (campo version)")
	}
	return &resolvedItem{obj: obj, id: id, version: version}, nil
}

func (s *Server) applyStatusChange(ctx context.Context, kind, base string, itemID, refValue, projectValue *int, statusID int, idLabel string) (*mcp.CallToolResult, any, error) {
	if err := positive("status_id", statusID); err != nil {
		return nil, nil, err
	}
	item, err := s.fetchItem(ctx, base, itemID, refValue, projectValue)
	if err != nil {
		return nil, nil, err
	}
	beforeObj := item.obj
	oldStatus := translateStatus(resolveName(beforeObj, "status"))
	ref := displayInt(beforeObj["ref"])
	if ref == 0 {
		ref = item.id
	}
	subject := redactText(displayValue(beforeObj["subject"]))

	after, _, err := s.client.PatchJSON(ctx, "/"+base+"/"+strconv.Itoa(item.id), nil, map[string]any{"status": statusID, "version": item.version})
	if err != nil {
		return nil, nil, err
	}
	afterObj, _ := after.(map[string]any)
	newStatus := translateStatus(resolveName(afterObj, "status"))

	var b strings.Builder
	fmt.Fprintf(&b, "✅ %s #%d: **%s**\n\n", kind, ref, subject)
	fmt.Fprintf(&b, "- **Status:** %s → **%s**\n", oldStatus, newStatus)
	fmt.Fprintf(&b, "- **%s:** %d\n", idLabel, item.id)
	return textResult(b.String())
}

func (s *Server) changeStatus(ctx context.Context, _ *mcp.CallToolRequest, input ChangeStatusInput) (*mcp.CallToolResult, any, error) {
	base, kind, idLabel, err := itemBase(input.Entity)
	if err != nil {
		return nil, nil, err
	}
	return s.applyStatusChange(ctx, kind, base, input.ItemID, input.Ref, input.ProjectID, input.StatusID, idLabel)
}

func itemBase(entity string) (base, kind, idLabel string, err error) {
	switch strings.ToLower(strings.TrimSpace(entity)) {
	case "userstory", "userstories", "card":
		return "/userstories", "Card", "card_id", nil
	case "issue", "issues":
		return "/issues", "Issue", "issue_id", nil
	case "task", "tasks", "subtask":
		return "/tasks", "Task", "task_id", nil
	default:
		return "", "", "", fmt.Errorf("entidade inválida: %q (use 'card', 'issue' ou 'task')", entity)
	}
}

type SearchInput struct {
	ProjectID *int   `json:"project_id,omitempty" jsonschema:"Project ID; omit to search all visible projects"`
	Query     string `json:"query" jsonschema:"Subject text (case-insensitive) or ref number"`
	Limit     *int   `json:"limit,omitempty" jsonschema:"Max results (default 10, max 30)"`
}

type searchRow struct {
	kind     string
	id       int
	ref      int
	subject  string
	status   string
	assigned string
	project  string
}

func (s *Server) search(ctx context.Context, _ *mcp.CallToolRequest, input SearchInput) (*mcp.CallToolResult, any, error) {
	q := strings.ToLower(strings.TrimSpace(input.Query))
	if q == "" {
		return nil, nil, errors.New("'query' é obrigatório")
	}
	limit := 10
	if input.Limit != nil && *input.Limit > 0 {
		limit = *input.Limit
		if limit > 30 {
			limit = 30
		}
	}
	query := url.Values{}
	if input.ProjectID != nil {
		if err := positive("project_id", *input.ProjectID); err != nil {
			return nil, nil, err
		}
		query.Set("project", strconv.Itoa(*input.ProjectID))
	}

	rows := make([]searchRow, 0, limit)
	for _, bucket := range []struct{ kind, path string }{
		{"Card", "/userstories"},
		{"Issue", "/issues"},
		{"Task", "/tasks"},
	} {
		if len(rows) >= limit {
			break
		}
		data, _, err := s.client.ListJSON(ctx, bucket.path, query, false)
		if err != nil {
			continue
		}
		items, ok := data.([]any)
		if !ok {
			continue
		}
		for _, item := range items {
			if len(rows) >= limit {
				break
			}
			m, ok := item.(map[string]any)
			if !ok {
				continue
			}
			subject := strings.ToLower(displayValue(m["subject"]))
			refNo := displayValue(m["ref"])
			if !strings.Contains(subject, q) && !strings.EqualFold(refNo, q) {
				continue
			}
			rows = append(rows, searchRow{
				kind:     bucket.kind,
				id:       displayInt(m["id"]),
				ref:      displayInt(m["ref"]),
				subject:  redactText(displayValue(m["subject"])),
				status:   translateStatus(resolveName(m, "status")),
				assigned: redactName(resolveName(m, "assigned_to")),
				project:  redactText(resolveName(m, "project")),
			})
		}
	}

	rawQuery := strings.TrimSpace(input.Query)
	if len(rows) == 0 {
		return textResult(fmt.Sprintf("🔎 Nenhum resultado para %q.", rawQuery))
	}
	var b strings.Builder
	fmt.Fprintf(&b, "🔎 **%d resultado(s)** para %q\n\n", len(rows), rawQuery)
	b.WriteString("| Tipo | id | Ref | Assunto | Status | Responsável | Projeto |\n")
	b.WriteString("|------|----|-----|--------|--------|-------------|---------|\n")
	for _, r := range rows {
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %s | %s |\n", r.kind, strconv.Itoa(r.id), strconv.Itoa(r.ref), r.subject, r.status, r.assigned, r.project)
	}
	if len(rows) == limit {
		fmt.Fprintf(&b, "\n_(primeiros %d resultados; use taiga_get com o id para abrir)_\n", limit)
	}
	return textResult(b.String())
}





func (s *Server) attachment(ctx context.Context, _ *mcp.CallToolRequest, input AttachmentInput) (*mcp.CallToolResult, any, error) {
	endpoint, err := attachmentEndpoint(input.Entity)
	if err != nil {
		return nil, nil, err
	}
	switch strings.ToLower(strings.TrimSpace(input.Action)) {
	case "list":
		if err := positive("project_id", input.ProjectID); err != nil {
			return nil, nil, err
		}
		if err := positive("object_id", input.ObjectID); err != nil {
			return nil, nil, err
		}
		query := url.Values{
			"project":   []string{strconv.Itoa(input.ProjectID)},
			"object_id": []string{strconv.Itoa(input.ObjectID)},
		}
		data, _, err := s.client.ListJSON(ctx, endpoint+"/attachments", query, false)
		if err != nil {
			return nil, nil, err
		}
		return textResult(formatAttachmentsTable(data))
	case "download":
		if err := positive("attachment_id", input.AttachmentID); err != nil {
			return nil, nil, err
		}
		data, _, err := s.client.GetJSON(ctx, endpoint+"/attachments/"+strconv.Itoa(input.AttachmentID), url.Values{})
		if err != nil {
			return nil, nil, err
		}
		attachment, ok := data.(map[string]any)
		if !ok {
			return nil, nil, errors.New("resposta do Taiga para o anexo não é um objeto JSON")
		}
		rawURL := firstString(attachment, "url", "attached_file", "file_url")
		if rawURL == "" {
			return nil, nil, errors.New("o anexo não contém uma URL de download")
		}
		filename := strings.TrimSpace(input.Filename)
		if filename == "" {
			filename = firstString(attachment, "name", "filename", "file_name", "attached_file")
		}
		result, err := s.client.Download(ctx, rawURL, filename, input.Path)
		if err != nil {
			return nil, nil, err
		}
		return textResult(formatDownload(result))
	default:
		return nil, nil, errors.New("action deve ser 'list' ou 'download'")
	}
}

func attachmentEndpoint(entity string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(entity)) {
	case "userstory", "user_story", "card", "cards":
		return "/userstories", nil
	case "issue", "issues":
		return "/issues", nil
	case "task", "tasks", "subtask", "subtasks":
		return "/tasks", nil
	default:
		return "", errors.New("entity deve ser userstory, issue ou task")
	}
}

func firstString(object map[string]any, keys ...string) string {
	for _, key := range keys {
		value, ok := object[key]
		if !ok {
			continue
		}
		switch typed := value.(type) {
		case string:
			if strings.TrimSpace(typed) != "" {
				return strings.TrimSpace(typed)
			}
		case map[string]any:
			for _, nestedKey := range []string{"url", "name", "filename"} {
				if nested, ok := typed[nestedKey].(string); ok && strings.TrimSpace(nested) != "" {
					return strings.TrimSpace(nested)
				}
			}
		}
	}
	return ""
}

func paging(page, pageSize *int) (url.Values, bool, error) {
	query := url.Values{}
	if page != nil {
		if err := positive("page", *page); err != nil {
			return nil, false, err
		}
		query.Set("page", strconv.Itoa(*page))
	}
	if pageSize != nil {
		if *pageSize < 1 || *pageSize > 1000 {
			return nil, false, errors.New("page_size deve estar entre 1 e 1000")
		}
		query.Set("page_size", strconv.Itoa(*pageSize))
	}
	return query, page != nil || pageSize != nil, nil
}

func positive(name string, value int) error {
	if value < 1 {
		return fmt.Errorf("%s deve ser maior que zero", name)
	}
	return nil
}

func textResult(text string) (*mcp.CallToolResult, any, error) {
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: text}},
	}, nil, nil
}

const defaultListLimit = 50

const displayCap = 50

func listQuery(page, pageSize *int, fetchAll bool) (url.Values, bool, error) {
	if fetchAll {
		return paging(nil, nil)
	}
	effectivePageSize := pageSize
	if page == nil && pageSize == nil {
		d := defaultListLimit
		effectivePageSize = &d
	}
	return paging(page, effectivePageSize)
}

func normalizeColumn(name string, allowed map[string]string) string {
	key, ok := allowed[strings.ToLower(strings.TrimSpace(name))]
	if !ok {
		return ""
	}
	return key
}

func filterRows(data any, columnKey, value string) (any, int) {
	items, ok := data.([]any)
	if !ok {
		return data, 0
	}
	needle := strings.ToLower(strings.TrimSpace(value))
	out := make([]any, 0, len(items))
	for _, it := range items {
		row, ok := it.(map[string]any)
		if !ok {
			continue
		}
		if strings.Contains(strings.ToLower(displayValue(row[columnKey])), needle) {
			out = append(out, it)
		}
	}
	return out, len(out)
}

var cardColumns = map[string]string{
	"id":            "id",
	"ref":           "ref",
	"assunto":       "subject",
	"subject":       "subject",
	"status":        "status",
	"bloqueado":     "is_blocked",
	"is_blocked":    "is_blocked",
	"responsável":   "assigned_to",
	"responsavel":   "assigned_to",
	"assigned_to":   "assigned_to",
	"fechado":       "is_closed",
	"is_closed":     "is_closed",
	"modificado":    "modified_date",
	"modificado em": "modified_date",
	"modified_date": "modified_date",
}

var issueColumns = map[string]string{
	"id":            "id",
	"ref":           "ref",
	"assunto":       "subject",
	"subject":       "subject",
	"status":        "status",
	"responsável":   "assigned_to",
	"responsavel":   "assigned_to",
	"assigned_to":   "assigned_to",
	"prioridade":    "priority",
	"priority":      "priority",
	"severidade":    "severity",
	"severity":      "severity",
	"fechado":       "is_closed",
	"is_closed":     "is_closed",
	"modificado":    "modified_date",
	"modificado em": "modified_date",
	"modified_date": "modified_date",
}

var taskColumns = map[string]string{
	"id":            "id",
	"ref":           "ref",
	"assunto":       "subject",
	"subject":       "subject",
	"status":        "status",
	"responsável":   "assigned_to",
	"responsavel":   "assigned_to",
	"assigned_to":   "assigned_to",
	"fechado":       "is_closed",
	"is_closed":     "is_closed",
	"modificado":    "modified_date",
	"modificado em": "modified_date",
	"modified_date": "modified_date",
}

func extractID(data any) int {
	m, ok := data.(map[string]any)
	if !ok {
		return 0
	}
	if f, ok := m["id"].(float64); ok {
		return int(f)
	}
	return 0
}

func (s *Server) loadComments(ctx context.Context, kind string, id int) []commentView {
	if id == 0 {
		return nil
	}
	endpoint := fmt.Sprintf("/history/%s/%d", kind, id)
	raw, _, err := s.client.GetJSON(ctx, endpoint, url.Values{})
	if err != nil {
		return nil
	}
	items, ok := raw.([]any)
	if !ok {
		return nil
	}
	out := make([]commentView, 0, len(items))
	for _, it := range items {
		m, ok := it.(map[string]any)
		if !ok {
			continue
		}
		text := strings.TrimSpace(displayValue(m["comment"]))
		if text == "" {
			continue
		}
		out = append(out, commentView{
			author: resolveCommentAuthor(m),
			date:   displayValue(m["created_at"]),
			text:   text,
		})
	}
	return out
}

func resolveCommentAuthor(m map[string]any) string {
	if extra, ok := m["user_extra_info"].(map[string]any); ok {
		for _, key := range []string{"full_name_display", "username", "name"} {
			if v, ok := extra[key].(string); ok && strings.TrimSpace(v) != "" {
				return strings.TrimSpace(v)
			}
		}
	}
	return displayValue(m["user"])
}

func (s *Server) loadLastAction(ctx context.Context, kind string, id int) string {
	if id == 0 {
		return ""
	}
	historyKind := kind
	if historyKind == "card" {
		historyKind = "userstory"
	}
	endpoint := fmt.Sprintf("/history/%s/%d", historyKind, id)
	raw, _, err := s.client.GetJSON(ctx, endpoint, url.Values{})
	if err != nil {
		return ""
	}
	items, ok := raw.([]any)
	if !ok || len(items) == 0 {
		return ""
	}
	latest, ok := items[len(items)-1].(map[string]any)
	if !ok {
		return ""
	}
	userName := redactName(resolveCommentAuthor(latest))
	if userName == "" {
		userName = "—"
	}
	if comment := strings.TrimSpace(displayValue(latest["comment"])); comment != "" {
		return fmt.Sprintf("💬 %s comentou", userName)
	}
	if diff, ok := latest["diff"].(map[string]any); ok {
		return summarizeHistoryDiff(diff)
	}
	return "📝 Modificado"
}

func summarizeHistoryDiff(diff map[string]any) string {
	fields := make([]string, 0, len(diff))
	descriptionChanged := false
	for field, value := range diff {
		label := map[string]string{
			"status": "o status", "assigned_to": "o responsável",
			"assigned_users": "os responsáveis", "swimlane": "a baia",
			"tags": "as tags", "description": "a descrição",
			"description_html": "a descrição", "subject": "o assunto",
			"attachments": "um anexo", "due_date": "a data de vencimento",
			"milestone": "o marco", "points": "os pontos",
		}[field]
		if label == "" {
			continue
		}
		if field == "attachments" {
			fields = append(fields, "adicionou um anexo")
			continue
		}
		if field == "description" || field == "description_html" {
			descriptionChanged = true
			continue
		}
		if field == "subject" {
			fields = append(fields, "alterou o assunto")
			continue
		}
		newValue := historyNewValue(value)
		if field == "status" && newValue != "" {
			newValue = translateStatus(newValue)
		}
		if newValue != "" {
			fields = append(fields, fmt.Sprintf("alterou %s para %s", label, redactText(newValue)))
		} else {
			fields = append(fields, "alterou "+label)
		}
	}
	if descriptionChanged {
		fields = append(fields, "alterou a descrição")
	}
	if len(fields) == 0 {
		return "📝 Atualizou o card"
	}
	return "✏️ " + strings.Join(fields, "; ")
}

func historyNewValue(value any) string {
	switch typed := value.(type) {
	case map[string]any:
		for _, key := range []string{"to", "new", "new_value", "value"} {
			if candidate := displayValue(typed[key]); candidate != "" {
				return candidate
			}
		}
	case []any:
		if len(typed) > 1 {
			return displayValue(typed[len(typed)-1])
		}
	default:
		return displayValue(value)
	}
	return ""
}

func (s *Server) loadIssueValues(ctx context.Context, projectID int) (map[int]string, map[int]string) {
	priorities := map[int]string{}
	severities := map[int]string{}
	if projectID == 0 {
		return priorities, severities
	}
	pID := strconv.Itoa(projectID)
	raw, _, err := s.client.GetJSON(ctx, "/projects/"+pID+"/issue-priorities", url.Values{})
	if err == nil {
		if items, ok := raw.([]any); ok {
			for _, it := range items {
				if m, ok := it.(map[string]any); ok {
					if id := displayInt(m["id"]); id > 0 {
						priorities[id] = strings.TrimSpace(displayValue(m["name"]))
					}
				}
			}
		}
	}
	raw, _, err = s.client.GetJSON(ctx, "/projects/"+pID+"/issue-severities", url.Values{})
	if err == nil {
		if items, ok := raw.([]any); ok {
			for _, it := range items {
				if m, ok := it.(map[string]any); ok {
					if id := displayInt(m["id"]); id > 0 {
						severities[id] = strings.TrimSpace(displayValue(m["name"]))
					}
				}
			}
		}
	}
	return priorities, severities
}

func extractIntField(data any, key string) int {
	m, ok := data.(map[string]any)
	if !ok {
		return 0
	}
	if f, ok := m[key].(float64); ok {
		return int(f)
	}
	return 0
}

func (s *Server) loadSubtasks(ctx context.Context, project, object int) []string {
	if project <= 0 || object <= 0 {
		return nil
	}
	query := url.Values{
		"project":    []string{strconv.Itoa(project)},
		"user_story": []string{strconv.Itoa(object)},
	}
	raw, _, err := s.client.ListJSON(ctx, "/tasks", query, false)
	if err != nil {
		return nil
	}
	items, ok := raw.([]any)
	if !ok {
		return nil
	}
	return formatSubtaskItems(items)
}

func formatSubtaskItems(items []any) []string {
	lines := make([]string, 0, len(items))
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		ref := displayValue(m["ref"])
		id := displayValue(m["id"])
		subject := redactText(displayValue(m["subject"]))
		if subject == "" {
			continue
		}
		prefix := ""
		if ref != "" {
			prefix = "#" + ref
		}
		if id != "" {
			if prefix != "" {
				prefix += " · "
			}
			prefix += "id " + id
		}
		status := translateStatus(resolveName(m, "status"))
		assigned := redactName(resolveName(m, "assigned_to"))
		line := "- " + prefix + " — " + subject
		if status != "" {
			line += " — " + status
		}
		if assigned != "" {
			line += " — " + assigned
		}
		lines = append(lines, line)
	}
	return lines
}

func (s *Server) loadAttachmentNames(ctx context.Context, kind string, project, object int) []string {
	if project == 0 || object == 0 {
		return nil
	}
	query := url.Values{
		"project":   []string{strconv.Itoa(project)},
		"object_id": []string{strconv.Itoa(object)},
	}
	raw, _, err := s.client.ListJSON(ctx, kind+"/attachments", query, false)
	if err != nil {
		return nil
	}
	items, ok := raw.([]any)
	if !ok {
		return nil
	}
	names := make([]string, 0, len(items))
	for _, it := range items {
		m, ok := it.(map[string]any)
		if !ok {
			continue
		}
		if name := strings.TrimSpace(displayValue(m["name"])); name != "" {
			id := displayValue(m["id"])
			desc := strings.TrimSpace(displayValue(m["description"]))
			prefix := ""
			if id != "" {
				prefix = "[" + id + "] "
			}
			if desc != "" {
				names = append(names, prefix+name+" — "+desc)
			} else {
				names = append(names, prefix+name)
			}
		}
	}
	return names
}
